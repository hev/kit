package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/store"
	tracepkg "github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/search"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls [date]",
	Short: "List captured traces",
	Long: `List sessions indexed in your Layer namespace.

By default lists sessions started in the last 24h. Use --since to widen
or narrow the window (e.g. --since 1h, --since 5d, --since 30m), or -A
for the last 5 days. Pass a positional YYYY-MM-DD to list a specific day.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLs,
}

var traceCmd = &cobra.Command{
	Use:   "trace <id>",
	Short: "Inspect a trace by session ID (prefix match)",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrace,
}

var (
	lsSince        string
	lsAll          bool
	lsDays         int
	lsLong         bool
	traceJSON      bool
	traceNamespace string
)

func init() {
	lsCmd.Flags().StringVar(&lsSince, "since", "1d", "Show sessions started within this window (e.g. 1h, 30m, 1d, 5d)")
	lsCmd.Flags().BoolVarP(&lsAll, "all", "A", false, "Shortcut for --since 5d")
	lsCmd.Flags().IntVar(&lsDays, "days", 0, "Deprecated alias: set window to N days")
	_ = lsCmd.Flags().MarkHidden("days")
	lsCmd.Flags().BoolVar(&lsLong, "long", false, "Show full session IDs")
	lsCmd.Flags().StringVar(&traceNamespace, "namespace", "", "Layer namespace (default $LAYER_NAMESPACE or hev-traces)")
	traceCmd.Flags().StringVar(&traceNamespace, "namespace", "", "Layer namespace (default $LAYER_NAMESPACE or hev-traces)")
	traceCmd.Flags().BoolVar(&traceJSON, "json", false, "Dump normalized turns as JSON")
}

func runLs(cmd *cobra.Command, args []string) error {
	cl, err := client(traceNamespace)
	if err != nil {
		return err
	}
	now := time.Now()
	var filter any
	if len(args) == 1 {
		day, err := time.ParseInLocation("2006-01-02", args[0], now.Location())
		if err != nil {
			return fmt.Errorf("invalid date %q: %w", args[0], err)
		}
		filter = []any{"And", []any{
			[]any{"start", "Gte", day.UnixMilli()},
			[]any{"start", "Lt", day.AddDate(0, 0, 1).UnixMilli()},
		}}
	} else {
		spec := lsSince
		if lsAll {
			spec = "5d"
		}
		if lsDays > 0 {
			spec = fmt.Sprintf("%dd", lsDays)
		}
		since, err := parseSinceDuration(spec)
		if err != nil {
			return err
		}
		filter = []any{"start", "Gte", now.Add(-since).UnixMilli()}
	}
	all, err := cl.ListSessionRows(1000, filter)
	if err != nil {
		return err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start > all[j].Start })

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "STARTED\tID\tHARNESS\tMODEL\tHOST\tTOKENS\tPROMPTS\tTITLE")
	hosts := map[string]bool{}
	for _, sess := range all {
		if sess.Host != "" {
			hosts[sess.Host] = true
		}
		id := sess.SessionID
		if !lsLong && len(id) > 8 {
			id = id[:8]
		}
		started := formatStarted(time.UnixMilli(sess.Start), now)
		tokens := formatTokenPair(sess.InputTokens+sess.CacheReadTokens+sess.CacheCreationTokens, sess.OutputTokens)
		label := sess.Summary
		if label == "" {
			label = sess.FirstPrompt
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			started, id, formatHarness(sess.Harness), formatModel(sess.Model), sess.Host, tokens, sess.PromptCount, label)
	}
	w.Flush()
	hintSharedKey(os.Stderr, cl, hosts)
	return nil
}

// parseSinceDuration reads a lookback the way every kit query does
// (search.ParseSince: Go durations, Nd, Nw); empty is one day.
func parseSinceDuration(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 24 * time.Hour, nil
	}
	return search.ParseSince(s)
}

// datesInWindow returns the set of YYYY-MM-DD partitions that overlap [now-since, now].
func datesInWindow(now time.Time, since time.Duration) []string {
	cutoff := now.Add(-since)
	startDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, cutoff.Location())
	endDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var dates []string
	for d := endDay; !d.Before(startDay); d = d.AddDate(0, 0, -1) {
		dates = append(dates, d.Format("2006-01-02"))
	}
	return dates
}

func formatStarted(ts, now time.Time) string {
	if ts.IsZero() {
		return ""
	}
	ts = ts.Local()
	nowLocal := now.Local()
	today := nowLocal.Format("2006-01-02")
	yesterday := nowLocal.AddDate(0, 0, -1).Format("2006-01-02")
	tsDate := ts.Format("2006-01-02")
	switch tsDate {
	case today:
		return ts.Format("15:04")
	case yesterday:
		return "Yest " + ts.Format("15:04")
	default:
		return ts.Format("Jan 02 15:04")
	}
}

func formatTokens(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

func formatTokenPair(in, out int64) string {
	if in == 0 && out == 0 {
		return ""
	}
	return formatTokens(in) + "→" + formatTokens(out)
}

func runTrace(cmd *cobra.Command, args []string) error {
	cl, err := client(traceNamespace)
	if err != nil {
		return err
	}
	turns, err := sessionTurns(cl, args[0])
	if err != nil {
		return err
	}
	return showNamespaceTurns(turns, os.Stdout)
}

// sessionTurns fetches one session's chunks from the namespace by id prefix
// and reassembles them into turns.
func sessionTurns(cl *layer.Client, prefix string) ([]tracepkg.Turn, error) {
	rows, err := cl.SessionRows(prefix)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no trace matching prefix %q", prefix)
	}
	sessions := map[string]bool{}
	chunks := make([]tracepkg.Chunk, 0, len(rows))
	for _, r := range rows {
		sessions[r.SessionID] = true
		chunks = append(chunks, tracepkg.Chunk{ID: r.ID, Text: r.Text, SessionID: r.SessionID,
			TurnUUID: r.TurnUUID, ParentUUID: r.ParentUUID, Seq: r.Seq, Block: r.Block, Part: r.Part,
			TS: r.TS, Role: r.Role, BlockType: r.BlockType, Tier: r.Tier, ToolName: r.ToolName,
			Workdir: r.Workdir, Branch: r.Branch, Harness: r.Harness, SourcePath: r.SourcePath,
			IsSidechain: r.IsSidechain})
	}
	if len(sessions) > 1 {
		return nil, fmt.Errorf("trace prefix %q is ambiguous (%d sessions); use more of the id", prefix, len(sessions))
	}
	return tracepkg.Reassemble(chunks), nil
}

func showNamespaceTurns(turns []tracepkg.Turn, w io.Writer) error {
	if traceJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(turns)
	}
	renderNamespaceTurns(turns, w, defaultRenderOpts())
	return nil
}

func renderNamespaceTurns(turns []tracepkg.Turn, w io.Writer, opts renderOpts) {
	first := turns[0]
	when := first.TS
	if ts, err := time.Parse(time.RFC3339, first.TS); err == nil {
		when = ts.Local().Format("Jan 02 15:04")
	}
	fmt.Fprintf(w, "%s  ·  %s  ·  %s  ·  %d turns  ·  %s\n\n", first.SessionID, formatHarness(first.Harness), when, len(turns), first.Workdir)
	for _, turn := range turns {
		for _, block := range turn.Blocks {
			switch block.Type {
			case "text":
				if turn.Role == "user" {
					printUser(block.Text, w)
				} else {
					printAssistantText(block.Text, w)
				}
			case "thinking":
				fmt.Fprintf(w, "%s %s\n\n", bulletStyle.Render("•"), dimStyle.Render(fmt.Sprintf("thinking (%d chars)", len([]rune(block.Text)))))
			case "tool_use", "tool_result":
				if opts.showTools {
					fmt.Fprintf(w, "%s %s\n\n", bulletStyle.Render("•"), block.Text)
				}
			}
		}
	}
}

func formatHarness(harness string) string {
	switch harness {
	case "claude_code":
		return "claude"
	case "codex_cli":
		return "codex"
	default:
		if harness == "" {
			return ""
		}
		return harness
	}
}

// formatModel renders a raw model id like "claude-opus-4-7" into a short
// human-readable label ("Opus 4.7"). Falls back to the raw string if the
// shape isn't recognized.
func formatModel(model string) string {
	if model == "" {
		return ""
	}
	lower := strings.ToLower(model)
	// claude-<family>-<major>-<minor>[-...]
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		needle := "claude-" + family + "-"
		if i := strings.Index(lower, needle); i >= 0 {
			rest := model[i+len(needle):]
			parts := strings.SplitN(rest, "-", 3)
			version := parts[0]
			if len(parts) > 1 {
				version += "." + parts[1]
			}
			label := strings.ToUpper(family[:1]) + family[1:]
			if version != "" {
				label += " " + version
			}
			return label
		}
	}
	if strings.HasPrefix(lower, "gpt-") {
		return "GPT-" + model[len("gpt-"):]
	}
	return model
}

// ---- rendering ----

// lipgloss styles. Lipgloss automatically downgrades to plain text when stdout
// isn't a TTY or NO_COLOR is set, so no manual gating is needed.
var (
	metaStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	metaIDStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	bulletStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	userBorder     = lipgloss.NewStyle().BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(lipgloss.Color("39")).PaddingLeft(1)
	toolLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	toolNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	toolArgStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	cornerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	okStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	failStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// renderBlock and friends produce structured items so we can attach tool_result
// events to their originating tool_use, then walk them in render order.
type renderBlock struct {
	kind string // "user", "text", "thinking", "tool", "ask", "fallback"
	text string

	toolName   string
	toolInput  json.RawMessage
	toolResult *toolResultMeta

	thinkingChars int

	ask askBlock
}

// askBlock holds the decoded AskUserQuestion payload so we can render the
// question(s) and options as a structured interactive moment rather than a
// blob of JSON.
type askBlock struct {
	questions []askQuestion
}

type askQuestion struct {
	question    string
	header      string
	multiSelect bool
	options     []askOption
}

type askOption struct {
	label       string
	description string
}

type toolResultMeta struct {
	success    string
	durationMs string
	sizeBytes  string
	err        string
}

// renderOpts controls optional rendering features (e.g. hiding tool blocks
// in the TUI). The non-interactive `hev trace` command uses the defaults.
type renderOpts struct {
	showTools bool
}

func defaultRenderOpts() renderOpts { return renderOpts{showTools: true} }

func showOTLPSession(ctx context.Context, s *store.Store, sess store.OTLPSession, events []store.OTLPEvent, w io.Writer) error {
	return showOTLPSessionOpts(ctx, s, sess, events, w, defaultRenderOpts())
}

func showOTLPSessionOpts(ctx context.Context, s *store.Store, sess store.OTLPSession, events []store.OTLPEvent, w io.Writer, opts renderOpts) error {
	if traceJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}

	printMetaHeader(sess, w)

	blocks := buildBlocks(ctx, s, sess, events)
	for _, b := range blocks {
		if !opts.showTools && b.kind == "tool" {
			continue
		}
		renderOne(b, w)
	}
	return nil
}

func showCodexSession(sess store.OTLPSession, events []store.CodexRolloutEvent, w io.Writer) error {
	return showCodexSessionOpts(sess, events, w, defaultRenderOpts())
}

func showCodexSessionOpts(sess store.OTLPSession, events []store.CodexRolloutEvent, w io.Writer, opts renderOpts) error {
	if traceJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}

	printMetaHeader(sess, w)

	blocks := buildCodexBlocks(events)
	for _, b := range blocks {
		if !opts.showTools && b.kind == "tool" {
			continue
		}
		renderOne(b, w)
	}
	return nil
}

func printMetaHeader(sess store.OTLPSession, w io.Writer) {
	id := sess.ID
	if len(id) > 8 {
		id = id[:8]
	}
	parts := []string{metaIDStyle.Render(id)}
	if sess.Harness != "" {
		parts = append(parts, formatHarness(sess.Harness))
	}
	if sess.Service != "" {
		parts = append(parts, sess.Service)
	}
	if !sess.FirstTS.IsZero() {
		parts = append(parts, sess.FirstTS.Local().Format("Jan 02 15:04"))
	}
	totalIn := sess.InputTokens + sess.CacheReadTokens + sess.CacheCreationTokens
	if totalIn+sess.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s → %s tokens", formatTokens(totalIn), formatTokens(sess.OutputTokens)))
	}
	if sess.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", sess.CostUSD))
	}
	if sess.EventCount > 0 {
		parts = append(parts, fmt.Sprintf("%d events", sess.EventCount))
	}
	fmt.Fprintln(w, metaStyle.Render(strings.Join(parts, "  ·  ")))
	fmt.Fprintln(w)
}

func buildBlocks(ctx context.Context, s *store.Store, sess store.OTLPSession, events []store.OTLPEvent) []renderBlock {
	var blocks []renderBlock
	var pendingTools []int // indices into blocks of tool calls awaiting their result

	for _, evt := range events {
		switch evt.Name {
		case "user_prompt":
			text, _ := evt.Attrs["prompt"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			blocks = append(blocks, renderBlock{kind: "user", text: text})

		case "api_response_body":
			ref, _ := evt.Attrs["body_ref"].(string)
			if ref == "" {
				continue
			}
			envBytes, err := s.GetRawAPIBody(ctx, sess.Date, ref)
			if err != nil {
				blocks = append(blocks, renderBlock{kind: "fallback", text: fmt.Sprintf("[response body not yet uploaded: %s]", filepathBase(ref))})
				continue
			}
			var env daemon.TraceEnvelope
			if err := json.Unmarshal(envBytes, &env); err != nil {
				continue
			}
			var resp struct {
				Content []struct {
					Type     string          `json:"type"`
					Text     string          `json:"text"`
					Thinking string          `json:"thinking"`
					Name     string          `json:"name"`
					Input    json.RawMessage `json:"input"`
				} `json:"content"`
			}
			if err := json.Unmarshal([]byte(env.PayloadRaw), &resp); err != nil {
				continue
			}
			for _, c := range resp.Content {
				switch c.Type {
				case "text":
					if strings.TrimSpace(c.Text) == "" {
						continue
					}
					blocks = append(blocks, renderBlock{kind: "text", text: c.Text})
				case "tool_use":
					if c.Name == "AskUserQuestion" {
						blocks = append(blocks, renderBlock{kind: "ask", toolName: c.Name, ask: parseAskInput(c.Input)})
					} else {
						blocks = append(blocks, renderBlock{kind: "tool", toolName: c.Name, toolInput: c.Input})
					}
					pendingTools = append(pendingTools, len(blocks)-1)
				case "thinking":
					// Extended-thinking responses emit a short encrypted signature
					// in every assistant turn; skip those so the transcript isn't
					// peppered with "thinking (10 chars)" noise.
					n := len(c.Thinking)
					if n == 0 {
						n = len(c.Text)
					}
					if n > 80 {
						blocks = append(blocks, renderBlock{kind: "thinking", thinkingChars: n})
					}
				}
			}

		case "tool_result":
			res := &toolResultMeta{
				success:    asString(evt.Attrs["success"]),
				durationMs: asString(evt.Attrs["duration_ms"]),
				sizeBytes:  asString(evt.Attrs["tool_result_size_bytes"]),
				err:        asString(evt.Attrs["error"]),
			}
			if len(pendingTools) > 0 {
				blocks[pendingTools[0]].toolResult = res
				pendingTools = pendingTools[1:]
			}
		}
	}
	return blocks
}

func buildCodexBlocks(events []store.CodexRolloutEvent) []renderBlock {
	var blocks []renderBlock
	pendingTools := map[string]int{}

	for _, evt := range events {
		switch {
		case evt.Type == "event_msg" && evt.PayloadType == "user_message":
			text := mapValueString(evt.Payload, "message")
			if strings.TrimSpace(text) == "" {
				continue
			}
			blocks = append(blocks, renderBlock{kind: "user", text: text})

		case evt.Type == "response_item" && evt.PayloadType == "message" && evt.Role == "assistant":
			for _, text := range codexContentTexts(evt.Payload, "output_text") {
				if strings.TrimSpace(text) == "" {
					continue
				}
				blocks = append(blocks, renderBlock{kind: "text", text: text})
			}

		case evt.Type == "response_item" && evt.PayloadType == "function_call":
			name := mapValueString(evt.Payload, "name")
			args := []byte(mapValueString(evt.Payload, "arguments"))
			if !json.Valid(args) {
				args = nil
			}
			blocks = append(blocks, renderBlock{kind: "tool", toolName: name, toolInput: args})
			if callID := mapValueString(evt.Payload, "call_id"); callID != "" {
				pendingTools[callID] = len(blocks) - 1
			}

		case evt.Type == "response_item" && evt.PayloadType == "function_call_output":
			callID := mapValueString(evt.Payload, "call_id")
			idx, ok := pendingTools[callID]
			if !ok {
				continue
			}
			output := mapValueString(evt.Payload, "output")
			blocks[idx].toolResult = &toolResultMeta{
				success:   "true",
				sizeBytes: strconv.Itoa(len(output)),
			}
			delete(pendingTools, callID)
		}
	}
	return blocks
}

func renderOne(b renderBlock, w io.Writer) {
	switch b.kind {
	case "user":
		printUser(b.text, w)
	case "text":
		printAssistantText(b.text, w)
	case "thinking":
		fmt.Fprintf(w, "%s %s\n\n",
			bulletStyle.Render("•"),
			dimStyle.Render(fmt.Sprintf("thinking (%d chars)", b.thinkingChars)))
	case "tool":
		printToolBlock(b, w)
	case "ask":
		printAskBlock(b, w)
	case "fallback":
		fmt.Fprintf(w, "%s %s\n\n", bulletStyle.Render("•"), dimStyle.Render(b.text))
	}
}

var (
	askBulletStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	askQStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	askLabelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	askDescStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)

func parseAskInput(raw json.RawMessage) askBlock {
	var payload struct {
		Questions []struct {
			Question    string `json:"question"`
			Header      string `json:"header"`
			MultiSelect bool   `json:"multiSelect"`
			Options     []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return askBlock{}
	}
	out := askBlock{}
	for _, q := range payload.Questions {
		aq := askQuestion{
			question:    q.Question,
			header:      q.Header,
			multiSelect: q.MultiSelect,
		}
		for _, o := range q.Options {
			aq.options = append(aq.options, askOption{label: o.Label, description: o.Description})
		}
		out.questions = append(out.questions, aq)
	}
	return out
}

func printAskBlock(b renderBlock, w io.Writer) {
	bullet := askBulletStyle.Render("?")
	label := toolLabelStyle.Render("Asked")
	tag := toolNameStyle.Render(b.toolName)
	fmt.Fprintf(w, "%s %s %s\n", bullet, label, tag)
	for i, q := range b.ask.questions {
		header := q.header
		if header == "" {
			header = fmt.Sprintf("Q%d", i+1)
		}
		select_ := "single-select"
		if q.multiSelect {
			select_ = "multi-select"
		}
		meta := dimStyle.Render(fmt.Sprintf("[%s · %s]", header, select_))
		fmt.Fprintf(w, "  %s %s\n", askQStyle.Render(q.question), meta)
		for _, o := range q.options {
			line := "    " + cornerStyle.Render("·") + " " + askLabelStyle.Render(o.label)
			if o.description != "" {
				line += "  " + askDescStyle.Render(o.description)
			}
			fmt.Fprintln(w, line)
		}
	}
	if b.toolResult != nil {
		fmt.Fprintf(w, "  %s %s\n", cornerStyle.Render("└"), renderToolResult(*b.toolResult))
	}
	fmt.Fprintln(w)
}

func printUser(text string, w io.Writer) {
	text = strings.TrimRight(text, "\n")
	// Wrap at the same width as assistant markdown; the TUI viewport clips
	// long lines rather than wrapping them.
	fmt.Fprintln(w, userBorder.Render(ansi.Wrap(text, wrapWidth()-userBorder.GetHorizontalFrameSize(), "")))
	fmt.Fprintln(w)
}

func printAssistantText(text string, w io.Writer) {
	md := renderMarkdown(text)
	md = strings.Trim(md, "\n")
	if md == "" {
		return
	}
	lines := strings.Split(md, "\n")

	// Glamour pads every line with a left margin; find and strip it so we can
	// apply our own bullet/indent without compounding the indentation.
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := 0
		for n < len(line) && line[n] == ' ' {
			n++
		}
		if minIndent == -1 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	bullet := bulletStyle.Render("•") + " "
	indent := "  "
	first := true
	for _, line := range lines {
		if len(line) >= minIndent {
			line = line[minIndent:]
		}
		if first && strings.TrimSpace(line) != "" {
			fmt.Fprintln(w, bullet+line)
			first = false
		} else {
			fmt.Fprintln(w, indent+line)
		}
	}
	fmt.Fprintln(w)
}

func printToolBlock(b renderBlock, w io.Writer) {
	args := formatToolInput(b.toolName, b.toolInput)
	if budget := wrapWidth() - 24; budget > 40 && len(args) > budget {
		args = args[:budget-1] + "…"
	}
	header := fmt.Sprintf("%s %s %s",
		bulletStyle.Render("•"),
		toolLabelStyle.Render("Ran"),
		toolNameStyle.Render(b.toolName),
	)
	if args != "" {
		header += " " + toolArgStyle.Render(args)
	}
	fmt.Fprintln(w, header)
	if b.toolResult != nil {
		fmt.Fprintf(w, "  %s %s\n", cornerStyle.Render("└"), renderToolResult(*b.toolResult))
	}
	fmt.Fprintln(w)
}

func renderToolResult(r toolResultMeta) string {
	parts := []string{}
	if r.success == "" || r.success == "true" {
		parts = append(parts, okStyle.Render("ok"))
	} else {
		parts = append(parts, failStyle.Render("fail"))
	}
	if r.durationMs != "" {
		parts = append(parts, dimStyle.Render(r.durationMs+"ms"))
	}
	if r.sizeBytes != "" {
		parts = append(parts, dimStyle.Render(humanBytes(r.sizeBytes)))
	}
	if r.err != "" {
		parts = append(parts, failStyle.Render(truncateForLog(r.err, 80)))
	}
	return strings.Join(parts, "  ")
}

// formatToolInput renders a tool_use's input arguments in a tool-specific
// short form (the command for Bash, the path for Read/Write/Edit, etc.) so
// the transcript reads like a script rather than a wall of JSON.
func formatToolInput(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return compactJSON(input)
	}
	switch name {
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			return strings.ReplaceAll(cmd, "\n", " ")
		}
	case "exec_command":
		if cmd, ok := m["cmd"].(string); ok {
			return strings.ReplaceAll(cmd, "\n", " ")
		}
	case "write_stdin":
		if session, ok := m["session_id"]; ok {
			return "session " + fmt.Sprint(session)
		}
	case "Read", "Write", "Edit", "NotebookEdit":
		if p, ok := m["file_path"].(string); ok {
			return p
		}
	case "view_image":
		if p, ok := m["path"].(string); ok {
			return p
		}
	case "Glob":
		if p, ok := m["pattern"].(string); ok {
			if dir, ok := m["path"].(string); ok && dir != "" {
				return dir + ":" + p
			}
			return p
		}
	case "Grep":
		if p, ok := m["pattern"].(string); ok {
			return p
		}
	case "WebFetch":
		if u, ok := m["url"].(string); ok {
			return u
		}
	case "WebSearch":
		if q, ok := m["query"].(string); ok {
			return q
		}
	case "Task", "Agent":
		if d, ok := m["description"].(string); ok {
			return d
		}
	}
	return compactJSON(input)
}

var mdRenderer *glamour.TermRenderer

func renderMarkdown(text string) string {
	if mdRenderer == nil {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(wrapWidth()),
		)
		if err != nil {
			return text
		}
		mdRenderer = r
	}
	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return out
}

func wrapWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 20 {
			return n
		}
	}
	return 100
}

func codexContentTexts(payload map[string]any, wantedType string) []string {
	content, ok := payload["content"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range content {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if wantedType != "" && mapValueString(m, "type") != wantedType {
			continue
		}
		if text := mapValueString(m, "text"); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func mapValueString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func humanBytes(s string) string {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s + "B"
	}
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	}
}

func filepathBase(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func compactJSON(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return string(b)
	}
	return buf.String()
}

func truncateForLog(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
