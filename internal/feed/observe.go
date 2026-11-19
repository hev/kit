package feed

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/hostlabel"
	"github.com/hev/kit/internal/secrets"
)

// Observer turns kit events into spool records. Its zero value uses this
// host's config, state directory and sender.
type Observer struct {
	Config *Config
	Dir    string
	// DispatchRoot is where dispatch runs and orchestrators live
	// (~/dispatch).
	DispatchRoot string
	Host         string
	// Kick wakes the sender; nil uses Kick.
	Kick func(dir string)
	// Replay reads recorded events: a stop's transcript is cut at the
	// event's time rather than its current size.
	Replay bool
}

func (o Observer) config() (Config, bool) {
	if o.Config != nil {
		return *o.Config, o.Config.Enabled
	}
	if off() {
		return Config{}, false
	}
	c, err := Load(secrets.ConfigPath())
	if err != nil {
		return c, false
	}
	return c, c.Enabled
}

func (o Observer) dir() string {
	if o.Dir != "" {
		return o.Dir
	}
	return Dir()
}

func (o Observer) host() string {
	if o.Host != "" {
		return o.Host
	}
	return hostlabel.Self(os.Hostname)
}

func (o Observer) dispatchRoot() string {
	if o.DispatchRoot != "" {
		return o.DispatchRoot
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "dispatch")
}

func (o Observer) add(rec Record) {
	dir := o.dir()
	if Add(dir, rec) != nil {
		return
	}
	if o.Kick != nil {
		o.Kick(dir)
		return
	}
	Kick(dir)
}

// recorder is the acceptance-evidence recorder command:
// node .../record.cjs FLOW.cjs NEW_OUTPUT_DIR [STUDIO_ORIGIN].
var recorder = regexp.MustCompile(`(?:^|[\s/])record\.cjs\s+(\S+)\s+(\S+)`)

// EvidenceDir finds the output directory of an acceptance-evidence
// recording in a shell command, relative to cwd. ok is false when the
// command does not run the recorder.
func EvidenceDir(command, cwd string) (string, bool) {
	m := recorder.FindStringSubmatch(command)
	if m == nil {
		return "", false
	}
	out := strings.Trim(m[2], `'"`)
	if strings.HasPrefix(out, "-") || out == "" {
		return "", false
	}
	out = expand(out)
	if !filepath.IsAbs(out) && cwd != "" {
		out = filepath.Join(cwd, out)
	}
	return filepath.Clean(out), true
}

// ObserveHook spools one normalized harness hook event (hev.hook.event/v1
// JSON, its target resolved). It returns at once: no network, no model.
func (o Observer) ObserveHook(raw []byte) {
	cfg, on := o.config()
	if !on {
		return
	}
	var h Hook
	if json.Unmarshal(raw, &h) != nil || h.Event == "" {
		return
	}
	th, runDir := o.sessionThread(h)
	client := cfg.Client || cfg.IsClientPath(h.Cwd) || o.clientRun(cfg, runDir)
	if h.Event == KindPostTool && h.Tool != nil && h.Tool.Command != "" && cfg.Wants(KindEvidence) && !client {
		if dir, ok := EvidenceDir(h.Tool.Command, h.Cwd); ok {
			o.add(Record{Kind: KindEvidence, Thread: evidenceThread(th, dir), EvidenceDir: dir, RunDir: runDir})
			return
		}
	}
	if !cfg.Wants(h.Event) {
		return
	}
	if h.Event == KindStop && h.StopHookActive {
		return
	}
	if th.Kind == ThreadOrchestrator && h.Event != KindStop {
		// An orchestrator is one thread with one line per round; the round
		// is summarized when its turn stops.
		return
	}
	rec := Record{Kind: h.Event, Thread: th, RunDir: runDir}
	if client {
		rec.Client = true
		th.Cwd = ""
		rec.Thread = th
		rec.Hook = &Hook{Event: h.Event, Harness: h.Harness, SessionID: h.SessionID, Source: h.Source, PromptBytes: len(h.Prompt), Target: h.Target}
		rec.RunDir = ""
		o.add(rec)
		return
	}
	if h.Tool != nil && h.Event != KindPreTool && h.Event != KindPostTool {
		h.Tool = nil
	}
	rec.Hook = &h
	if h.Event == KindStop && h.TranscriptPath != "" {
		if o.Replay {
			rec.TranscriptSize = TranscriptOffset(h.TranscriptPath, h.Time)
		} else if info, err := os.Stat(h.TranscriptPath); err == nil {
			rec.TranscriptSize = info.Size()
		}
	}
	if th.Kind == ThreadOrchestrator {
		rec.Round = roundOf(h.Cwd)
		if rec.Round == 0 {
			rec.Round = latestRound(runDir)
		}
	}
	o.add(rec)
}

// sessionThread is the thread a harness event belongs in: its dispatch run
// or orchestrator when the hook resolved one, else its own session.
func (o Observer) sessionThread(h Hook) (Thread, string) {
	host := o.host()
	th := Thread{Harness: h.Harness, Host: host, Cwd: h.Cwd, Model: h.Model}
	kind, name, _ := strings.Cut(h.Target, ":")
	switch {
	case kind == "dispatch" && name != "":
		th.Kind, th.Name, th.Key = ThreadRun, name, RunKey(host, name)
		return th, filepath.Join(o.dispatchRoot(), name)
	case kind == "orch" && name != "":
		th.Kind, th.Name, th.Key = ThreadOrchestrator, name, OrchestratorKey(host, name)
		// Rounds run in rounds/NNN; the thread outlives each of them.
		th.Cwd = ""
		return th, filepath.Join(o.dispatchRoot(), "orch-"+name)
	}
	sid := h.SessionID
	if sid == "" {
		sid = "no-session"
	}
	th.Kind, th.Key = ThreadSession, "session:"+hostlabel.Canonical(host)+"/"+sid
	th.Name = name
	if th.Name == "" {
		th.Name = sid
		if len(th.Name) > 8 {
			th.Name = th.Name[:8]
		}
	}
	return th, ""
}

// RunKey and OrchestratorKey match attention's notice thread keys.
func RunKey(host, name string) string { return "run:" + hostlabel.Canonical(host) + "/" + name }

// OrchestratorKey is an orchestrator's thread key.
func OrchestratorKey(host, name string) string {
	return "orchestrator:" + hostlabel.Canonical(host) + "/" + name
}

// NormalizeKey canonicalizes the host in a notice's thread key.
func NormalizeKey(key string) string {
	kind, rest, ok := strings.Cut(key, ":")
	if !ok {
		return key
	}
	host, name, ok := strings.Cut(rest, "/")
	if !ok {
		return key
	}
	return kind + ":" + hostlabel.Canonical(host) + "/" + name
}

func evidenceThread(session Thread, dir string) Thread {
	if session.Kind == ThreadRun || session.Kind == ThreadOrchestrator {
		return session
	}
	return Thread{Kind: ThreadEvidence, Key: "evidence:" + session.Host + ":" + dir, Name: filepath.Base(dir), Host: session.Host, Harness: session.Harness}
}

// clientRun reports a dispatch run whose repository is client work.
func (o Observer) clientRun(cfg Config, runDir string) bool {
	if runDir == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(runDir, "meta.json"))
	if err != nil {
		return false
	}
	var m struct {
		ResolvedRepo string `json:"resolved_repo"`
		Client       bool   `json:"client"`
	}
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	return m.Client || cfg.IsClientPath(m.ResolvedRepo)
}

// roundOf is the round a session runs in: orchestrator rounds run in
// orch-NAME/rounds/NNN.
func roundOf(cwd string) int {
	if filepath.Base(filepath.Dir(cwd)) != "rounds" {
		return 0
	}
	n, _ := strconv.Atoi(filepath.Base(cwd))
	return n
}

// TranscriptOffset is the end of the last transcript line stamped at or
// before t (0 when none is).
func TranscriptOffset(path string, t time.Time) int64 {
	f, err := os.Open(path)
	if err != nil || t.IsZero() {
		return 0
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	var off, end int64
	for {
		line, err := r.ReadBytes('\n')
		off += int64(len(line))
		var l struct {
			Timestamp time.Time `json:"timestamp"`
		}
		if json.Unmarshal(line, &l) == nil && !l.Timestamp.IsZero() {
			if l.Timestamp.After(t) {
				return end
			}
			end = off
		}
		if err != nil {
			return end
		}
	}
}

// latestRound is the highest numbered rounds/NNN directory.
func latestRound(dir string) int {
	entries, err := os.ReadDir(filepath.Join(dir, "rounds"))
	if err != nil {
		return 0
	}
	var ns []int
	for _, e := range entries {
		if n, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			ns = append(ns, n)
		}
	}
	if len(ns) == 0 {
		return 0
	}
	sort.Ints(ns)
	return ns[len(ns)-1]
}

// ObserveLifecycle spools a kit event: an attention notice (needs-you,
// ready, done), a peer message or a board post, as its hook handlers get it.
func (o Observer) ObserveLifecycle(event string, payload []byte) {
	cfg, on := o.config()
	if !on || !cfg.Wants(event) {
		return
	}
	host := o.host()
	rec := Record{Kind: event, Payload: append(json.RawMessage(nil), payload...)}
	switch event {
	case KindNeedsYou, KindReady, KindDone:
		var n struct {
			Source, Name, Host, Thread string
			Client                     bool
		}
		if json.Unmarshal(payload, &n) != nil || n.Name == "" {
			return
		}
		kind := ThreadRun
		if strings.HasPrefix(n.Thread, "orchestrator:") {
			kind = ThreadOrchestrator
		}
		h := n.Host
		if h == "" {
			h = host
		}
		key := NormalizeKey(n.Thread)
		if key == "" {
			key = RunKey(h, n.Name)
		}
		rec.Thread = Thread{Kind: kind, Key: key, Name: n.Name, Host: h}
		rec.Client = n.Client || cfg.Client
	case KindBoardPost:
		var b struct {
			Board, Kind, URL, Cwd string
			ID                    string `json:"id"`
			ThreadID              int64  `json:"thread_id"`
			Body                  *string
		}
		if json.Unmarshal(payload, &b) != nil || !cfg.WantsBoard(b.Board) {
			return
		}
		id := b.ID
		if b.Kind == "reply" && b.ThreadID > 0 {
			id = strconv.FormatInt(b.ThreadID, 10)
		}
		rec.Thread = Thread{Kind: ThreadBoard, Key: "board:" + boardBase(b.URL) + "/" + id, Name: b.Board + " #" + id, Host: host}
		rec.Client = cfg.Client || cfg.IsClientPath(b.Cwd)
	case KindPeerMessage:
		var p struct{ From, To, Cwd string }
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		rec.Thread = Thread{Kind: ThreadPeers, Key: "peers:" + hostlabel.Canonical(host), Name: "peer messages", Host: host}
		rec.Client = cfg.Client || cfg.IsClientPath(p.Cwd)
	default:
		return
	}
	if rec.Client {
		rec.Payload = stripContent(rec.Payload)
	}
	o.add(rec)
}

// stripContent drops free text from a payload bound for a client record.
func stripContent(raw json.RawMessage) json.RawMessage {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	for _, k := range []string{"text", "body", "title", "links", "url", "tags"} {
		delete(m, k)
	}
	b, _ := json.Marshal(m)
	return b
}

// boardBase is the board origin in a thread URL (BASE/t/ID#pN).
func boardBase(u string) string {
	if i := strings.Index(u, "/t/"); i >= 0 {
		return u[:i]
	}
	return u
}

// ObserveEvidence spools an acceptance-evidence recording directory. run,
// when set, is the dispatch run it belongs to (its thread).
func (o Observer) ObserveEvidence(dir, run string) bool {
	cfg, on := o.config()
	if !on || !cfg.Wants(KindEvidence) {
		return false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	if cfg.Client || cfg.IsClientPath(abs) {
		return false
	}
	host := o.host()
	th := Thread{Kind: ThreadEvidence, Key: "evidence:" + host + ":" + abs, Name: filepath.Base(abs), Host: host}
	runDir := ""
	if run != "" {
		if name, ok := strings.CutPrefix(run, "orch-"); ok {
			th = Thread{Kind: ThreadOrchestrator, Key: OrchestratorKey(host, name), Name: name, Host: host}
		} else {
			th = Thread{Kind: ThreadRun, Key: RunKey(host, run), Name: run, Host: host}
			runDir = filepath.Join(o.dispatchRoot(), run)
		}
		if o.clientRun(cfg, runDir) {
			return false
		}
	}
	o.add(Record{Kind: KindEvidence, Thread: th, EvidenceDir: abs, RunDir: runDir})
	return true
}

// ObserveHook, ObserveLifecycle and ObserveEvidence on this host's
// defaults.
func ObserveHook(raw []byte)                        { Observer{}.ObserveHook(raw) }
func ObserveLifecycle(event string, payload []byte) { Observer{}.ObserveLifecycle(event, payload) }
func ObserveEvidence(dir, run string) bool          { return Observer{}.ObserveEvidence(dir, run) }
