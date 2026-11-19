package feedsend

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/redact"
)

// File is an attachment for a message's thread. When it cannot be uploaded
// (too large, no files:write) Fallback is posted instead, if set.
type File struct {
	Name  string
	Title string
	Data  []byte
	// Size is the file's size when Data was not read (over the cap).
	Size int64
	// Link is where the file can be seen without the upload.
	Link string
}

// Message is what one record posts.
type Message struct {
	Key string
	// Parent starts the thread when it does not exist yet. Empty means the
	// message itself starts it.
	Parent string
	Text   string
	Files  []File
	// Summarized says the model wrote the text; false is the fallback.
	Summarized bool
}

// Composer turns spool records into messages.
type Composer struct {
	Model Summarizer
	// MaxUpload caps each attachment (bytes).
	MaxUpload int64
	Tag       string
	// HTTP fetches board images.
	HTTP *http.Client
	// Warn records why a summary fell back.
	Warn func(string)
	Home string
}

var (
	scrubOnce sync.Once
	scrubber  *redact.Scrubber
)

// scrub redacts credentials from text bound for the model or Slack. If the
// scrubber cannot be built, nothing is sent.
func scrub(s string) string {
	scrubOnce.Do(func() {
		var salt [32]byte
		if _, err := rand.Read(salt[:]); err == nil {
			scrubber, _ = redact.New(salt[:])
		}
	})
	if scrubber == nil {
		return ""
	}
	out, _ := scrubber.Text(s)
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// tail keeps the last n characters: the end of a turn says what it did.
func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
}

// head keeps the first n characters.
func head(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// mrkdwn escapes Slack's control characters in free text.
func mrkdwn(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

var sentenceEnd = regexp.MustCompile(`[.!?](\s|$)`)

// firstSentence is a compact stand-in for a summary.
func firstSentence(s string, n int) string {
	s = oneLine(s)
	if loc := sentenceEnd.FindStringIndex(s); loc != nil && loc[0] < n {
		s = s[:loc[0]+1]
	}
	return clip(s, n)
}

func (c Composer) warn(msg string) {
	if c.Warn != nil {
		c.Warn(msg)
	}
}

// summarize asks the model, and returns "" when it fails or there is none.
func (c Composer) summarize(ctx context.Context, prompt string) string {
	if c.Model == nil {
		return ""
	}
	s, err := c.Model.Summarize(ctx, prompt)
	if err != nil {
		c.warn("summary fell back: " + err.Error())
		return ""
	}
	// The model's answer goes through the same redaction as everything else.
	return clipWords(oneLine(scrub(s)), 320)
}

// clipWords bounds s at a word boundary.
func clipWords(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n-1])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

func (c Composer) home() string {
	if c.Home != "" {
		return c.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

func (c Composer) tilde(p string) string {
	if h := c.home(); h != "" && (p == h || strings.HasPrefix(p, h+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, h)
	}
	return p
}

// Parent is the first line of a session, run or orchestrator thread: who,
// which harness, which host and where.
func (c Composer) Parent(rec feed.Record) string {
	th := rec.Thread
	var b strings.Builder
	if c.Tag != "" {
		b.WriteString(c.Tag + " ")
	}
	switch th.Kind {
	case feed.ThreadRun:
		fmt.Fprintf(&b, ":thread: *%s* · dispatch run", mrkdwn(th.Name))
	case feed.ThreadOrchestrator:
		fmt.Fprintf(&b, ":thread: *%s* · orchestrator", mrkdwn(th.Name))
	case feed.ThreadPeers:
		fmt.Fprintf(&b, ":thread: *Peer messages* on %s", th.Host)
		return b.String()
	default:
		fmt.Fprintf(&b, ":thread: *%s*", mrkdwn(th.Name))
	}
	if rec.Client {
		b.WriteString(" · client session")
	}
	who := th.Harness
	if who == "" {
		who = "kit"
	}
	if th.Kind == feed.ThreadOrchestrator || th.Harness == "" {
		fmt.Fprintf(&b, " on %s", th.Host)
	} else {
		fmt.Fprintf(&b, " · %s on %s", who, th.Host)
	}
	if th.Model != "" && !rec.Client {
		b.WriteString(" · " + th.Model)
	}
	if th.Cwd != "" && !rec.Client {
		b.WriteString(" · `" + strings.ReplaceAll(c.tilde(th.Cwd), "`", "'") + "`")
	}
	return b.String()
}

// Compose builds the message for one record. It never fails: anything
// missing becomes a shorter post, and a model error the compact fallback.
func (c Composer) Compose(ctx context.Context, rec feed.Record) (Message, bool) {
	m := Message{Key: rec.Thread.Key}
	switch rec.Kind {
	case feed.KindNeedsYou, feed.KindReady, feed.KindDone:
		m.Parent = c.Parent(rec)
		m.Text = c.notice(rec)
	case feed.KindBoardPost:
		return c.board(ctx, rec)
	case feed.KindEvidence:
		return c.evidence(rec)
	case feed.KindPeerMessage:
		m.Parent = c.Parent(rec)
		m.Text = c.peer(rec)
	default:
		if rec.Hook == nil {
			return m, false
		}
		m.Parent = c.Parent(rec)
		if rec.Client {
			m.Text = clientLine(rec)
			return m, true
		}
		m.Text, m.Summarized = c.hook(ctx, rec)
	}
	return m, m.Text != ""
}

// clientLine is all a client session posts: that it is active or ended a
// turn. No prompt, reply, path or command leaves the host.
func clientLine(rec feed.Record) string {
	switch rec.Kind {
	case feed.KindStop:
		return ":lock: Client session: turn ended."
	case feed.KindSessionStart:
		return ":lock: Client session active."
	}
	return ":lock: Client session active."
}

func (c Composer) hook(ctx context.Context, rec feed.Record) (string, bool) {
	h := rec.Hook
	switch rec.Kind {
	case feed.KindSessionStart:
		return c.sessionStart(ctx, rec)
	case feed.KindStop:
		return c.stop(ctx, rec)
	case feed.KindPromptSubmit:
		if h.Prompt == "" {
			// A recorded event (events.jsonl) keeps only the prompt's size.
			if h.PromptBytes > 0 {
				return fmt.Sprintf(":speech_balloon: New prompt (%d bytes, text not recorded).", h.PromptBytes), false
			}
			return ":speech_balloon: New prompt.", false
		}
		return ":speech_balloon: Asked: “" + mrkdwn(clip(oneLine(scrub(h.Prompt)), 200)) + "”", false
	case feed.KindPreCompact:
		return ":compression: Compacting context" + paren(h.Source) + ".", false
	case feed.KindPreTool, feed.KindPostTool:
		if h.Tool == nil {
			return "", false
		}
		line := ":hammer_and_wrench: " + mrkdwn(h.Tool.Name)
		if h.Tool.Command != "" {
			line += " `" + strings.ReplaceAll(clip(oneLine(scrub(h.Tool.Command)), 160), "`", "'") + "`"
		}
		return line, false
	}
	return "", false
}

func paren(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

// orderBody is a run's order without its front matter.
func orderBody(runDir string) string {
	b, err := os.ReadFile(filepath.Join(runDir, "order.md"))
	if err != nil {
		return ""
	}
	s := string(b)
	if strings.HasPrefix(s, "---\n") {
		if i := strings.Index(s[4:], "\n---"); i >= 0 {
			s = s[4+i+4:]
		}
	}
	return strings.TrimSpace(s)
}

// orderTitle is an order's first heading or line.
func orderTitle(order string) string {
	for _, l := range strings.Split(order, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(l, "# ")); l != "" {
			return l
		}
	}
	return ""
}

func (c Composer) sessionStart(ctx context.Context, rec feed.Record) (string, bool) {
	h, th := rec.Hook, rec.Thread
	switch h.Source {
	case "resume":
		return ":arrows_counterclockwise: Session resumed.", false
	case "compact":
		return ":compression: Context compacted; carrying on.", false
	case "clear":
		return ":broom: Context cleared; starting fresh.", false
	}
	if th.Kind == feed.ThreadRun {
		order := orderBody(rec.RunDir)
		if order != "" {
			safe := scrub(head(order, 6000))
			prompt := fmt.Sprintf("A coding agent just started dispatch run %q. Its work order begins:\n\n<order>\n%s\n</order>\n\nWrite one sentence under 160 characters in the form \"Started %s: <what it is going to do>.\"", th.Name, safe, th.Name)
			if s := c.summarize(ctx, prompt); s != "" {
				return ":rocket: " + mrkdwn(s), true
			}
			return ":rocket: Started " + mrkdwn(th.Name) + ": " + mrkdwn(clip(oneLine(scrub(orderTitle(order))), 200)) + ".", false
		}
		return ":rocket: Started " + mrkdwn(th.Name) + ".", false
	}
	// An interactive session has nothing to summarize before its first
	// prompt; its first turn is summarized when it stops.
	where := ""
	if th.Cwd != "" {
		where = " in `" + strings.ReplaceAll(c.tilde(th.Cwd), "`", "'") + "`"
	}
	return ":rocket: Session started" + where + ".", false
}

func (c Composer) stop(ctx context.Context, rec feed.Record) (string, bool) {
	h, th := rec.Hook, rec.Thread
	var t Turn
	if h.TranscriptPath != "" {
		t, _ = LastTurn(h.TranscriptPath, rec.TranscriptSize)
	}
	prompt, reply := scrub(tail(t.Prompt, 4000)), scrub(tail(t.Reply, 8000))
	if th.Kind == feed.ThreadOrchestrator {
		return c.round(ctx, rec, reply)
	}
	if strings.TrimSpace(prompt+reply) == "" {
		return ":white_check_mark: Finished a turn.", false
	}
	who := "an interactive coding session"
	if th.Kind == feed.ThreadRun {
		who = fmt.Sprintf("dispatch run %q", th.Name)
	}
	q := fmt.Sprintf("A coding agent (%s) just finished a turn.\n\nThe prompt it was working on:\n<prompt>\n%s\n</prompt>\n\nWhat the agent wrote this turn (its final reply last):\n<reply>\n%s\n</reply>\n\nIn under 200 characters, say what it did or found, and anything it is waiting on.", who, prompt, reply)
	if s := c.summarize(ctx, q); s != "" {
		return ":speech_balloon: " + mrkdwn(s), true
	}
	if reply != "" {
		return ":white_check_mark: Finished a turn: " + mrkdwn(firstSentence(lastParagraph(reply), 220)), false
	}
	return ":white_check_mark: Finished a turn on: “" + mrkdwn(clip(oneLine(prompt), 160)) + "”", false
}

func lastParagraph(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "\n\n")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := strings.TrimSpace(parts[i]); p != "" && p != "…" {
			return p
		}
	}
	return s
}

// round is an orchestrator round's one line, from its reply.md and the
// agent's last words.
func (c Composer) round(ctx context.Context, rec feed.Record, reply string) (string, bool) {
	label := "Round"
	if rec.Round > 0 {
		label = "Round " + strconv.Itoa(rec.Round)
	}
	var roundReply string
	if rec.Round > 0 {
		b, _ := os.ReadFile(filepath.Join(rec.RunDir, "rounds", fmt.Sprintf("%03d", rec.Round), "reply.md"))
		roundReply = scrub(head(string(b), 4000))
	}
	if strings.TrimSpace(roundReply+reply) != "" {
		q := fmt.Sprintf("An orchestrator (a supervising agent that checks on other agents' runs) finished a round.\n\nIts round reply:\n<round>\n%s\n</round>\n\nIts last words:\n<reply>\n%s\n</reply>\n\nWrite ONE short sentence, under 140 characters, on what this round decided or did. Do not repeat the round number.", roundReply, tail(reply, 3000))
		if s := c.summarize(ctx, q); s != "" {
			return "*" + label + ":* " + mrkdwn(s), true
		}
	}
	line := ""
	for _, l := range strings.Split(roundReply, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	if line == "" && reply != "" {
		line = firstSentence(lastParagraph(reply), 200)
	}
	if line == "" {
		return "*" + label + "* finished.", false
	}
	return "*" + label + ":* " + mrkdwn(clip(oneLine(line), 200)), false
}

// notice renders an attention notice: readable, with a decision's question
// verbatim.
func (c Composer) notice(rec feed.Record) string {
	var n struct {
		Event, Source, Name, Host, State, Text string
		Client                                 bool
		Links                                  struct{ PR, Run, Board string }
	}
	_ = json.Unmarshal(rec.Payload, &n)
	if rec.Client || n.Client {
		state := n.State
		if state == "" {
			state = rec.Kind
		}
		return fmt.Sprintf(":lock: %s: %s", kindLabel(rec.Kind), state)
	}
	var line string
	switch rec.Kind {
	case feed.KindNeedsYou:
		if key, q, ok := decision(n.Text); ok {
			q = fullQuestion(rec, key, q)
			line = fmt.Sprintf(":rotating_light: *Needs you* — decision `%s`: %s", key, mrkdwn(q))
		} else {
			line = ":rotating_light: *Needs you* — " + mrkdwn(orDefault(n.Text, n.State))
		}
	case feed.KindReady:
		line = ":tada: *Ready* — " + mrkdwn(orDefault(n.Text, "ready for review"))
	default:
		state := ""
		if n.State != "" && n.State != "done" {
			state = " (" + n.State + ")"
		}
		line = ":checkered_flag: *Done*" + state
		if t := strings.TrimSpace(n.Text); t != "" && t != n.State {
			line += " — " + mrkdwn(t)
		}
	}
	if n.Links.PR != "" {
		line += "\nPR: " + n.Links.PR
	}
	if n.Links.Board != "" {
		line += "\nBoard: " + n.Links.Board
	}
	if n.Links.Run != "" {
		line += "\n`" + strings.ReplaceAll(n.Links.Run, "`", "'") + "`"
	}
	return line
}

// fullQuestion recovers a decision question the notice cut at its length
// limit, from the run's status.md or the orchestrator's last round reply on
// this host.
func fullQuestion(rec feed.Record, key, q string) string {
	if !strings.HasSuffix(q, "…") {
		return q
	}
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "dispatch")
	var files []string
	if rec.Thread.Kind == feed.ThreadOrchestrator {
		rounds, _ := filepath.Glob(filepath.Join(root, "orch-"+rec.Thread.Name, "rounds", "*", "reply.md"))
		if len(rounds) > 0 {
			files = append(files, rounds[len(rounds)-1])
		}
	} else {
		dir := filepath.Join(root, rec.Thread.Name)
		var ds []struct{ Key, Question string }
		if b, err := os.ReadFile(filepath.Join(dir, "decisions.json")); err == nil && json.Unmarshal(b, &ds) == nil {
			for i := len(ds) - 1; i >= 0; i-- {
				if ds[i].Key == key && strings.HasPrefix(oneLine(ds[i].Question), oneLine(strings.TrimSuffix(q, "…"))) {
					return clip(oneLine(scrub(ds[i].Question)), 1500)
				}
			}
		}
		files = append(files, filepath.Join(dir, "status.md"))
	}
	prefix := strings.TrimSuffix(q, "…")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			k, full, ok := decision(lines[i])
			if ok && k == key && strings.HasPrefix(oneLine(full), oneLine(prefix)) {
				return clip(oneLine(scrub(full)), 1500)
			}
		}
	}
	return q
}

func kindLabel(k string) string {
	switch k {
	case feed.KindNeedsYou:
		return "Needs you"
	case feed.KindReady:
		return "Ready"
	}
	return "Done"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

var decisionLine = regexp.MustCompile(`needs-decision\s+([A-Za-z0-9_.-]+):\s*(.+)$`)

// decision splits "needs-decision KEY: QUESTION" (with or without a
// leading tag) into its key and question.
func decision(text string) (string, string, bool) {
	m := decisionLine.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

func (c Composer) peer(rec feed.Record) string {
	var p struct {
		From, To, Kind string
		Chat           bool
		Text           *string
		TextBytes      int `json:"text_bytes"`
	}
	_ = json.Unmarshal(rec.Payload, &p)
	line := fmt.Sprintf(":incoming_envelope: %s → %s", mrkdwn(p.From), mrkdwn(p.To))
	if p.Text != nil && !rec.Client {
		line += ": " + mrkdwn(clip(oneLine(scrub(*p.Text)), 200))
	} else {
		line += fmt.Sprintf(" (%d bytes)", p.TextBytes)
	}
	return line
}

// board renders a board thread or reply: its own thread per board thread,
// title, a short summary and the link, with the post's images attached.
func (c Composer) board(ctx context.Context, rec feed.Record) (Message, bool) {
	var b struct {
		Board, Kind, Title, Author, URL string
		ID                              string `json:"id"`
		ThreadID                        int64  `json:"thread_id"`
		Body                            *string
	}
	if json.Unmarshal(rec.Payload, &b) != nil {
		return Message{}, false
	}
	m := Message{Key: rec.Thread.Key}
	threadID := b.ID
	if b.Kind == "reply" && b.ThreadID > 0 {
		threadID = strconv.FormatInt(b.ThreadID, 10)
	}
	tag := ""
	if c.Tag != "" {
		tag = c.Tag + " "
	}
	if rec.Client {
		m.Parent = fmt.Sprintf("%s:clipboard: *%s #%s* (client host)", tag, mrkdwn(b.Board), threadID)
		m.Text = ":lock: " + map[bool]string{true: "New reply.", false: "New thread."}[b.Kind == "reply"]
		return m, true
	}
	summary := ""
	if b.Body != nil && strings.TrimSpace(*b.Body) != "" {
		body := scrub(head(*b.Body, 6000))
		q := fmt.Sprintf("Someone posted on the %q board, in a thread titled %q.\n\n<post>\n%s\n</post>\n\nWrite one sentence summarizing what the post says or asks.", b.Board, b.Title, body)
		if summary = c.summarize(ctx, q); summary != "" {
			m.Summarized = true
		} else {
			summary = firstSentence(body, 220)
		}
	}
	title := mrkdwn(clip(oneLine(scrub(b.Title)), 200))
	parent := fmt.Sprintf("%s:clipboard: *%s #%s:* %s", tag, mrkdwn(b.Board), threadID, title)
	if b.Kind == "reply" {
		m.Parent = parent + "\n" + boardThreadURL(b.URL)
		m.Text = ":speech_balloon: *" + mrkdwn(orDefault(b.Author, "someone")) + "* replied"
		if summary != "" {
			m.Text += ": " + mrkdwn(summary)
		}
		m.Text += "\n" + b.URL
	} else {
		m.Text = parent + "\n_by " + mrkdwn(orDefault(b.Author, "someone")) + "_"
		if summary != "" {
			m.Text += " — " + mrkdwn(summary)
		}
		m.Text += "\n" + b.URL
	}
	m.Files = c.boardImages(ctx, b.URL, threadID, b.Kind)
	return m, true
}

func boardThreadURL(u string) string {
	if i := strings.Index(u, "#"); i >= 0 {
		return u[:i]
	}
	return u
}

var postNumber = regexp.MustCompile(`#p(\d+)$`)

// boardImages fetches the images of the post an event is about from the
// board that served it.
func (c Composer) boardImages(ctx context.Context, entryURL, threadID, kind string) []File {
	base := entryURL
	i := strings.Index(base, "/t/")
	if i < 0 || threadID == "" {
		return nil
	}
	base = base[:i]
	n := 1
	if kind == "reply" {
		m := postNumber.FindStringSubmatch(entryURL)
		if m == nil {
			return nil
		}
		n, _ = strconv.Atoi(m[1])
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	var out struct {
		Posts []struct {
			N      int `json:"n"`
			Images []struct {
				URL  string `json:"url"`
				Name string `json:"name"`
				Size int64  `json:"size"`
			} `json:"images"`
		} `json:"posts"`
	}
	if getJSON(ctx, hc, base+"/api/threads/"+threadID+"?peek=1", &out) != nil {
		return nil
	}
	var files []File
	for _, p := range out.Posts {
		if p.N != n {
			continue
		}
		for j, im := range p.Images {
			u := im.URL
			if strings.HasPrefix(u, "/") {
				u = base + u
			}
			name := im.Name
			if name == "" {
				name = fmt.Sprintf("image-%d", j+1)
			}
			f := File{Name: name, Title: name, Size: im.Size, Link: entryURL}
			if c.MaxUpload > 0 && im.Size > c.MaxUpload {
				files = append(files, f)
				continue
			}
			data, err := getBytes(ctx, hc, u, c.MaxUpload)
			if err != nil {
				c.warn("board image " + name + ": " + err.Error())
				continue
			}
			f.Data, f.Size = data, int64(len(data))
			files = append(files, f)
		}
	}
	return files
}

func getJSON(ctx context.Context, hc *http.Client, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", u, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

// getBytes downloads u, refusing more than max bytes (0 for no cap).
func getBytes(ctx context.Context, hc *http.Client, u string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	limit := int64(1 << 30)
	if max > 0 {
		limit = max + 1
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, err
	}
	if max > 0 && int64(len(b)) > max {
		return nil, fmt.Errorf("over the %d MB upload cap", max>>20)
	}
	return b, nil
}

// Evidence is the part of an acceptance-evidence result.json the feed
// posts (hev/shell hosts/mini/acceptance-recorder/record.cjs).
type Evidence struct {
	Title          string `json:"title"`
	Revision       string `json:"revision"`
	CandidateURL   string `json:"candidate_url"`
	Expected       string `json:"expected"`
	Actual         string `json:"actual"`
	Verdict        string `json:"verdict"`
	Studio         string `json:"studio"`
	RecordingError string `json:"recordingError"`
	UploadError    string `json:"uploadError"`
}

// evidence renders a recording's verdict, expected and actual result and
// Studio link, with its still and video attached.
func (c Composer) evidence(rec feed.Record) (Message, bool) {
	b, err := os.ReadFile(filepath.Join(rec.EvidenceDir, "result.json"))
	if err != nil {
		c.warn("evidence " + rec.EvidenceDir + ": " + err.Error())
		return Message{}, false
	}
	var e Evidence
	if json.Unmarshal(b, &e) != nil {
		return Message{}, false
	}
	m := Message{Key: rec.Thread.Key}
	icon := map[string]string{"pass": ":white_check_mark:", "fail": ":x:", "blocked": ":warning:"}[e.Verdict]
	if icon == "" {
		icon = ":movie_camera:"
	}
	tag := ""
	if c.Tag != "" && rec.Thread.Kind == feed.ThreadEvidence {
		tag = c.Tag + " "
	}
	var t strings.Builder
	fmt.Fprintf(&t, "%s%s *Acceptance evidence: %s* — %s", tag, icon, strings.ToUpper(orDefault(e.Verdict, "unknown")), mrkdwn(clip(oneLine(scrub(e.Title)), 160)))
	fmt.Fprintf(&t, "\n*Expected:* %s", mrkdwn(clip(oneLine(scrub(e.Expected)), 400)))
	fmt.Fprintf(&t, "\n*Actual:* %s", mrkdwn(clip(oneLine(scrub(e.Actual)), 400)))
	if e.Revision != "" {
		fmt.Fprintf(&t, "\n*Revision:* `%s`", strings.ReplaceAll(clip(oneLine(e.Revision), 80), "`", "'"))
	}
	link := e.Studio
	switch {
	case e.Studio != "":
		fmt.Fprintf(&t, "\n*Studio:* %s", e.Studio)
	case e.UploadError != "":
		fmt.Fprintf(&t, "\n_Not saved to Studio: %s_", mrkdwn(clip(oneLine(scrub(e.UploadError)), 160)))
	default:
		t.WriteString("\n_Not saved to Studio._")
	}
	if e.RecordingError != "" {
		fmt.Fprintf(&t, "\n_Recording problem: %s_", mrkdwn(clip(oneLine(scrub(e.RecordingError)), 160)))
	}
	m.Text = t.String()
	if rec.Thread.Kind != feed.ThreadEvidence {
		m.Parent = c.Parent(rec)
	}
	still := "final.png"
	if _, err := os.Stat(filepath.Join(rec.EvidenceDir, still)); err != nil {
		still = "poster.jpg"
	}
	for _, name := range []string{still, "recording.mp4"} {
		if f, ok := c.localFile(filepath.Join(rec.EvidenceDir, name), link); ok {
			f.Title = clip(oneLine(e.Title), 80) + " · " + name
			m.Files = append(m.Files, f)
		}
	}
	return m, true
}

// localFile reads a file for upload, or records its size when over the cap.
func (c Composer) localFile(path, link string) (File, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return File{}, false
	}
	f := File{Name: filepath.Base(path), Size: info.Size(), Link: link}
	if c.MaxUpload > 0 && info.Size() > c.MaxUpload {
		return f, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, false
	}
	f.Data = data
	return f, true
}
