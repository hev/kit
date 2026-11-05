package dispatch

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"gopkg.in/yaml.v3"
)

type SourceRule struct {
	Status   string `yaml:"status" json:"status"`
	Action   string `yaml:"action" json:"action"`
	Template string `yaml:"template" json:"template"`
	Mode     string `yaml:"mode" json:"mode"`
	// Name "event" uses the event ID as the run name; empty derives one.
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Batch    bool   `yaml:"batch,omitempty" json:"batch,omitempty"`
	Cooldown string `yaml:"cooldown,omitempty" json:"cooldown,omitempty"`
}
type OrchestratorSource struct {
	Plugin string   `yaml:"plugin,omitempty" json:"plugin,omitempty"`
	Team   string   `yaml:"team,omitempty" json:"team,omitempty"`
	Args   []string `yaml:"args,omitempty" json:"args,omitempty"`
	// Env names environment variables only the plugin exec may see.
	Env   []string     `yaml:"env,omitempty" json:"env,omitempty"`
	Rules []SourceRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

func (s *OrchestratorSource) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value == "none" {
			*s = OrchestratorSource{}
			return nil
		}
		return fmt.Errorf("source must be none or a mapping")
	}
	type plain OrchestratorSource
	b, e := yaml.Marshal(n)
	if e != nil {
		return e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	return d.Decode((*plain)(s))
}
func (s OrchestratorSource) Validate() error {
	if s.Plugin == "" {
		if len(s.Rules) > 0 || s.Team != "" || len(s.Args) > 0 || len(s.Env) > 0 {
			return fmt.Errorf("source plugin required")
		}
		return nil
	}
	if !ValidName(s.Plugin) {
		return fmt.Errorf("invalid source plugin")
	}
	if e := validatePluginEnv("source.env", s.Env); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, r := range s.Rules {
		if r.Status == "" || !ValidName(r.Action) || r.Template == "" || (r.Mode != "live" && r.Mode != "dry-run") {
			return fmt.Errorf("source rule requires status, action, template and live|dry-run mode")
		}
		if seen[r.Status] {
			return fmt.Errorf("duplicate source status %s", r.Status)
		}
		seen[r.Status] = true
		if r.Name != "" && r.Name != "event" {
			return fmt.Errorf("source rule name must be event or omitted")
		}
		if r.Batch && r.Name == "event" {
			return fmt.Errorf("batch source rules cannot use event names")
		}
		if r.Cooldown != "" {
			if !r.Batch {
				return fmt.Errorf("cooldown requires batch")
			}
			if _, e := OrchestratorDuration(r.Cooldown); e != nil {
				return fmt.Errorf("cooldown: %w", e)
			}
		}
	}
	for _, a := range append([]string{s.Team}, s.Args...) {
		if strings.ContainsAny(a, "\r\n\x00") {
			return fmt.Errorf("source team and args cannot contain CR, LF or NUL")
		}
	}
	for _, a := range s.Args {
		if a == "" || a == "--" {
			return fmt.Errorf("source args cannot be empty or --")
		}
	}
	return nil
}

type SourceEvent struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	URL    string         `json:"url"`
	Fields map[string]any `json:"fields"`
}
type SourceBatch struct {
	Events   []SourceEvent `json:"events"`
	Cursor   string        `json:"cursor"`
	Ack      bool          `json:"ack"`
	AckBatch bool          `json:"ack_batch,omitempty"`
}

var (
	// ErrSourceDeferred means a round left work for later without failing:
	// state stays waiting, no round directory is created and the cursor is kept.
	ErrSourceDeferred = errors.New("source round deferred")
	// ErrSourceUnsafeValue names an interpolated value containing CR, LF or NUL.
	ErrSourceUnsafeValue = errors.New("source template value contains CR, LF or NUL")
	// ErrSourceFrontmatter means rendering changed the template's own frontmatter keys.
	ErrSourceFrontmatter = errors.New("rendered frontmatter keys differ from the template")
	ErrSourceScope       = errors.New("source template must use RunName and remain in manages scope")
	ErrSourceEventName   = errors.New("source event ID is not an allowed run name")
)

// Tests replace the clock and the uncertainty grace period.
var (
	sourceNow          = time.Now
	sourceAttemptGrace = 15 * time.Minute
)

const (
	sourceDefaultCooldown = 6 * time.Hour
	// A rejected receipt retries after 1, 2, 4 ... polls, at most 64.
	sourceRetryMaxPolls = 64
	sourceSignalLimit   = 50
)

func fetchSource(ctx context.Context, s OrchestratorSpec) (SourceBatch, error) {
	var batch SourceBatch
	b, e := sourceCommand(ctx, s, sourceInvocation{Args: sourceArgs(s, "--hev-describe")})
	if e != nil {
		return batch, e
	}
	var desc struct {
		Source   bool `json:"source"`
		Ack      bool `json:"ack"`
		AckBatch bool `json:"ack_batch"`
	}
	if e = json.Unmarshal(b, &desc); e != nil {
		return batch, e
	}
	if !desc.Source {
		return batch, fmt.Errorf("plugin does not advertise source support")
	}
	batch.Ack, batch.AckBatch = desc.Ack, desc.Ack && desc.AckBatch
	var cursor struct {
		Cursor string `json:"cursor"`
	}
	path := filepath.Join(OrchestratorDir(s.Name), "source-cursor.json")
	if e = readJSON(path, &cursor); e != nil && !os.IsNotExist(e) {
		return batch, e
	}
	var rest []string
	if s.Source.Team != "" {
		rest = append(rest, "--team", s.Source.Team)
	}
	// The opaque cursor never enters argv, where batch files would expand it.
	b, e = sourceCommand(ctx, s, sourceInvocation{Args: sourceArgs(s, "events", rest...), Env: []string{"HEV_SOURCE_CURSOR=" + cursor.Cursor}})
	if e != nil {
		return batch, e
	}
	scan := bufio.NewScanner(bytes.NewReader(b))
	scan.Buffer(make([]byte, 4096), 4*1024*1024)
	terminal := false
	ids := map[string]bool{}
	for scan.Scan() {
		if terminal {
			return batch, fmt.Errorf("source data after cursor")
		}
		var raw map[string]json.RawMessage
		if e = json.Unmarshal(scan.Bytes(), &raw); e != nil {
			return batch, e
		}
		if _, ok := raw["cursor"]; ok {
			if len(raw) != 1 {
				return batch, fmt.Errorf("cursor must be a separate final record")
			}
			if e = json.Unmarshal(raw["cursor"], &batch.Cursor); e != nil {
				return batch, e
			}
			terminal = true
			continue
		}
		if _, ok := raw["url"]; !ok {
			return batch, fmt.Errorf("source event missing url")
		}
		var event SourceEvent
		if e = json.Unmarshal(scan.Bytes(), &event); e != nil {
			return batch, e
		}
		if event.ID == "" || event.Kind == "" || event.Fields == nil || ids[event.ID] {
			return batch, fmt.Errorf("invalid or duplicate source event")
		}
		ids[event.ID] = true
		batch.Events = append(batch.Events, event)
	}
	if e = scan.Err(); e != nil {
		return batch, e
	}
	if !terminal {
		return batch, fmt.Errorf("source missing final cursor")
	}
	return batch, nil
}

// sourcePausePath resolves a relative pause_file beside the applied spec copy
// (the state directory's spec.md). CLI apply stores it absolute, resolved
// beside the input file, so both paths name the same file.
func sourcePausePath(s OrchestratorSpec) string {
	if s.PauseFile == "" || filepath.IsAbs(s.PauseFile) {
		return s.PauseFile
	}
	return filepath.Join(OrchestratorDir(s.Name), s.PauseFile)
}
func sourcePaused(s OrchestratorSpec) bool {
	p := sourcePausePath(s)
	return p != "" && exists(p)
}

var sourceDispatch = RPC

// IntakeSignal is an edge-triggered intake health event, recorded once per
// receipt and kind: attempt-stale, rejected or needs-reconcile. ID is stable
// across rounds and restarts: intake:KIND:RECEIPT.
type IntakeSignal struct {
	Kind    string    `json:"kind"`
	Receipt string    `json:"receipt"`
	Event   string    `json:"event"`
	Run     string    `json:"run,omitempty"`
	Error   string    `json:"error,omitempty"`
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
}

// IntakeSignalHook delivers each signal from the receipt outbox. A nil error
// durably acknowledges the signal; an error, or a crash before the
// acknowledgement is written, redelivers it with the same ID on a later
// round, so the hook must deduplicate by ID. Owner notices attach here; this
// phase only records and surfaces signals.
var IntakeSignalHook = func(OrchestratorSpec, IntakeSignal) error { return nil }

// sourceReceipt is the durable record for one event under one rule and mode.
// It stores the event so retries and reconciliation never depend on replay.
type sourceReceipt struct {
	Mode     string       `json:"mode"`
	Event    string       `json:"event"`
	Run      string       `json:"run,omitempty"`
	Host     string       `json:"host,omitempty"`
	Rule     string       `json:"rule,omitempty"`
	Action   string       `json:"action,omitempty"`
	RuleMode string       `json:"rule_mode,omitempty"`
	RuleKey  string       `json:"rule_key,omitempty"`
	Members  []string     `json:"members,omitempty"`
	Source   *SourceEvent `json:"source_event,omitempty"`
	Order    string       `json:"order,omitempty"`
	// Class is the rendered order's class; the target holds its slot while
	// the receipt is attempting (classReservation).
	Class    string `json:"class,omitempty"`
	Error    string `json:"error,omitempty"`
	Attempts int    `json:"attempts,omitempty"`
	// RetryPoll is the source poll number at which a rejected receipt is due.
	RetryPoll   int64     `json:"retry_poll,omitempty"`
	AttemptedAt time.Time `json:"attempted_at,omitempty"`
	Updated     time.Time `json:"updated"`
	// Signals is the outbox: each signal is written in the same receipt write
	// as the transition that raised it. Delivered lists acknowledged IDs.
	Signals   []IntakeSignal `json:"signals,omitempty"`
	Delivered []string       `json:"delivered,omitempty"`
}

func uncertainReceipt(mode string) bool { return mode == "attempting" || mode == "needs-reconcile" }

func sourceHash(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}
func sourceKey(s OrchestratorSpec, id string, rule SourceRule) string {
	return sourceHash(s.Source.Plugin, id, rule.Action, rule.Mode)
}
func sourceRuleKey(s OrchestratorSpec, rule SourceRule) string {
	return sourceHash(s.Source.Plugin, rule.Status, rule.Action, rule.Mode)
}
func sourceMarkers(s OrchestratorSpec) string {
	return filepath.Join(OrchestratorDir(s.Name), "source-markers")
}

// sourceRunName is deterministic: a hash per event, a hash of the sorted
// member IDs for batches, or the event ID itself for name: event rules.
func sourceRunName(s OrchestratorSpec, rule SourceRule, events []SourceEvent) (string, error) {
	if rule.Batch {
		ids := []string{s.Source.Plugin, rule.Action, rule.Mode}
		for _, e := range events {
			ids = append(ids, e.ID)
		}
		slices.Sort(ids[3:])
		return "src-" + s.Name + "-" + sourceHash(ids...)[:16], nil
	}
	id := events[0].ID
	if rule.Name == "event" {
		re, e := regexp.Compile("^(?:" + s.NamePattern + ")$")
		if e != nil || s.NamePattern == "" || !ValidName(id) || !re.MatchString(id) {
			return "", fmt.Errorf("%w: %q", ErrSourceEventName, id)
		}
		return id, nil
	}
	return "src-" + s.Name + "-" + sourceKey(s, id, rule)[:16], nil
}

const sourceGuardFunc = "hevSourceValue"

func sourceGuard(v any) (string, error) {
	out := fmt.Sprint(v)
	if strings.ContainsAny(out, "\r\n\x00") {
		return "", fmt.Errorf("%w: %q", ErrSourceUnsafeValue, out)
	}
	return out, nil
}

// guardTemplate pipes every printed action through sourceGuard. Conditions
// and declarations are not printed, so they are left unchanged.
func guardTemplate(t *template.Template) {
	var walk func(*parse.Tree, parse.Node)
	walk = func(tree *parse.Tree, n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			if n == nil {
				return
			}
			for _, c := range n.Nodes {
				walk(tree, c)
			}
		case *parse.ActionNode:
			if len(n.Pipe.Decl) == 0 {
				guard := &parse.CommandNode{NodeType: parse.NodeCommand, Pos: n.Pos, Args: []parse.Node{parse.NewIdentifier(sourceGuardFunc).SetTree(tree).SetPos(n.Pos)}}
				n.Pipe.Cmds = append(n.Pipe.Cmds, guard)
			}
		case *parse.IfNode:
			walk(tree, n.List)
			walk(tree, n.ElseList)
		case *parse.RangeNode:
			walk(tree, n.List)
			walk(tree, n.ElseList)
		case *parse.WithNode:
			walk(tree, n.List)
			walk(tree, n.ElseList)
		}
	}
	for _, tt := range t.Templates() {
		if tt.Tree != nil {
			walk(tt.Tree, tt.Tree.Root)
		}
	}
}

// frontmatterKeys lists the flat order keys in the leading frontmatter, in
// sorted order. Lines without a colon (template actions) are not keys.
func frontmatterKeys(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	keys := []string{}
	if len(lines) == 0 || lines[0] != "---" {
		return keys
	}
	for _, line := range lines[1:] {
		if line == "---" {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, _, ok := strings.Cut(line, ":"); ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

func renderSourceOrder(s OrchestratorSpec, rule SourceRule, events []SourceEvent, name string) (string, Order, error) {
	var o Order
	path := rule.Template
	if !filepath.IsAbs(path) {
		path = filepath.Join(OrchestratorDir(s.Name), path)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return "", o, e
	}
	tpl, e := template.New("order").Option("missingkey=error").Funcs(template.FuncMap{sourceGuardFunc: sourceGuard}).Parse(string(raw))
	if e != nil {
		return "", o, e
	}
	guardTemplate(tpl)
	data := struct {
		Event                        SourceEvent
		Events                       []SourceEvent
		Action, RunName, RequestedBy string
	}{Events: events, Action: rule.Action, RunName: name, RequestedBy: LocalSender()}
	if !rule.Batch {
		data.Event = events[0]
	}
	var rendered bytes.Buffer
	if e = tpl.Execute(&rendered, data); e != nil {
		return "", o, e
	}
	if want, got := frontmatterKeys(string(raw)), frontmatterKeys(rendered.String()); !slices.Equal(want, got) {
		return "", o, fmt.Errorf("%w: template %v, rendered %v", ErrSourceFrontmatter, want, got)
	}
	if o, e = Parse(rendered.String()); e != nil {
		return "", o, e
	}
	if o.Name != name || !s.Includes(o) {
		return "", o, ErrSourceScope
	}
	return rendered.String(), o, nil
}

// sourceUnit is one dispatch: a single event, or every matching event for a
// batch rule.
type sourceUnit struct {
	Rule   SourceRule
	Events []SourceEvent
}

func sourceUnits(s OrchestratorSpec, events []SourceEvent) []sourceUnit {
	var out []sourceUnit
	for _, rule := range s.Source.Rules {
		var matching []SourceEvent
		for _, event := range events {
			if event.Fields["status"] == rule.Status {
				matching = append(matching, event)
			}
		}
		if rule.Batch {
			if len(matching) > 0 {
				slices.SortFunc(matching, func(a, b SourceEvent) int { return strings.Compare(a.ID, b.ID) })
				out = append(out, sourceUnit{rule, matching})
			}
			continue
		}
		for _, event := range matching {
			out = append(out, sourceUnit{rule, []SourceEvent{event}})
		}
	}
	return out
}

// SourcePreview is one rendered unit from RenderSourcePreview.
type SourcePreview struct {
	Status string   `json:"status"`
	Mode   string   `json:"mode"`
	Events []string `json:"events"`
	Run    string   `json:"run,omitempty"`
	Order  string   `json:"order,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// RenderSourcePreview uses the live render path and writes no receipt,
// artifact, budget or state.
func RenderSourcePreview(name string, events []SourceEvent) ([]SourcePreview, error) {
	s, e := LoadOrchestrator(name)
	if e != nil {
		return nil, e
	}
	if s.Source.Plugin == "" {
		return nil, fmt.Errorf("orchestrator %s has no source", name)
	}
	out := []SourcePreview{}
	for _, u := range sourceUnits(s, events) {
		p := SourcePreview{Status: u.Rule.Status, Mode: u.Rule.Mode}
		for _, event := range u.Events {
			p.Events = append(p.Events, event.ID)
		}
		run, err := sourceRunName(s, u.Rule, u.Events)
		if err == nil {
			p.Run = run
			p.Order, _, err = renderSourceOrder(s, u.Rule, u.Events, run)
		}
		if err != nil {
			p.Error = err.Error()
		}
		out = append(out, p)
	}
	return out, nil
}

func loadSourceReceipts(s OrchestratorSpec) (map[string]sourceReceipt, error) {
	out := map[string]sourceReceipt{}
	entries, e := os.ReadDir(sourceMarkers(s))
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	for _, f := range entries {
		key, ok := strings.CutSuffix(f.Name(), ".json")
		if !ok {
			continue
		}
		var r sourceReceipt
		if e = readJSON(filepath.Join(sourceMarkers(s), f.Name()), &r); e != nil {
			return nil, e
		}
		out[key] = r
	}
	return out, nil
}

// updateSourceReceipt applies fn to the current receipt under the markers
// lock, so the runner and reconcile never lose each other's transitions.
func updateSourceReceipt(s OrchestratorSpec, key string, fn func(r *sourceReceipt, existed bool) bool) (sourceReceipt, bool, error) {
	var r sourceReceipt
	dir := sourceMarkers(s)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return r, false, e
	}
	unlock, e := lockDirectory(dir)
	if e != nil {
		return r, false, e
	}
	defer unlock()
	path := filepath.Join(dir, key+".json")
	e = readJSON(path, &r)
	if e != nil && !os.IsNotExist(e) {
		return r, false, e
	}
	if !fn(&r, e == nil) {
		return r, false, nil
	}
	r.Updated = sourceNow().UTC()
	return r, true, WriteJSON(path, r)
}

func signalID(kind, key string) string { return "intake:" + kind + ":" + key }

func hasSignal(r sourceReceipt, kind string) bool {
	return slices.ContainsFunc(r.Signals, func(s IntakeSignal) bool { return s.Kind == kind })
}

// addSignal queues kind in the receipt's outbox unless that edge was already
// raised. The caller writes the receipt, so the transition and its signal are
// one atomic write.
func addSignal(r *sourceReceipt, key, kind string) {
	if !hasSignal(*r, kind) {
		r.Signals = append(r.Signals, IntakeSignal{Kind: kind, Receipt: key, Event: r.Event, Run: r.Run, Error: r.Error, At: sourceNow().UTC(), ID: signalID(kind, key)})
	}
}

func sourceSignals(receipts map[string]sourceReceipt) []IntakeSignal {
	out := []IntakeSignal{}
	for _, r := range receipts {
		out = append(out, r.Signals...)
	}
	slices.SortFunc(out, func(a, b IntakeSignal) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if n := len(out); n > sourceSignalLimit {
		out = out[n-sourceSignalLimit:]
	}
	return out
}

// OrchestratorSignals rebuilds the intake signals from the receipt outbox,
// so inventory never depends on state.json having been saved.
func OrchestratorSignals(name string) ([]IntakeSignal, error) {
	receipts, e := loadSourceReceipts(OrchestratorSpec{Name: name})
	if e != nil {
		return nil, e
	}
	return sourceSignals(receipts), nil
}

// deliverSourceSignals rebuilds st.Signals from the outbox and passes every
// unacknowledged signal to IntakeSignalHook. It runs at the start of every
// source round and after the rules, so a signal raised before a crash is
// delivered on the next round. Hook errors leave the signal queued.
func deliverSourceSignals(s OrchestratorSpec, st *OrchestratorState) error {
	receipts, e := loadSourceReceipts(s)
	if e != nil {
		return e
	}
	if st != nil {
		st.Signals = sourceSignals(receipts)
	}
	for _, key := range slices.Sorted(maps.Keys(receipts)) {
		for _, sig := range receipts[key].Signals {
			if slices.Contains(receipts[key].Delivered, sig.ID) || IntakeSignalHook(s, sig) != nil {
				continue
			}
			if _, _, e = updateSourceReceipt(s, key, func(c *sourceReceipt, existed bool) bool {
				if !existed || slices.Contains(c.Delivered, sig.ID) {
					return false
				}
				c.Delivered = append(c.Delivered, sig.ID)
				return true
			}); e != nil {
				return e
			}
		}
	}
	return nil
}

func sourceRetryPolls(attempts int) int64 {
	d := int64(1)
	for i := 1; i < attempts && d < sourceRetryMaxPolls; i++ {
		d *= 2
	}
	return min(d, sourceRetryMaxPolls)
}

func sourcePollPath(s OrchestratorSpec) string {
	return filepath.Join(OrchestratorDir(s.Name), "source-polls.json")
}

// sourcePoll is the number of completed source polls; rejected receipts
// back off in polls rather than wall-clock time.
func sourcePoll(s OrchestratorSpec) int64 {
	var p struct {
		Polls int64 `json:"polls"`
	}
	_ = readJSON(sourcePollPath(s), &p)
	return p.Polls
}
func advanceSourcePoll(s OrchestratorSpec) error {
	return WriteJSON(sourcePollPath(s), map[string]int64{"polls": sourcePoll(s) + 1})
}

// sourceUncertain reports whether a dispatch outcome is unknown. Only an
// unconfirmed launch or a transport failure is uncertain; a target that
// answered with an error rejected the request definitively.
func sourceUncertain(e error) bool {
	var t transportError
	return errors.As(e, &t) || errors.Is(e, errLaunchUnconfirmed) || strings.Contains(e.Error(), errLaunchUnconfirmed.Error()) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded)
}
func sourceRunExists(e error) bool {
	return errors.Is(e, errRunExists) || strings.Contains(e.Error(), errRunExists.Error())
}

type sourceAck struct {
	ID   string `json:"id"`
	Run  string `json:"run"`
	Mode string `json:"mode"`
}
type sourceOutcome struct {
	Deferred string
	Acks     []sourceAck
}
type sourceBatchState struct {
	Run  string    `json:"run"`
	Host string    `json:"host,omitempty"`
	Mode string    `json:"mode"`
	At   time.Time `json:"at"`
}

type sourceRunner struct {
	ctx      context.Context
	s        OrchestratorSpec
	st       *OrchestratorState
	cfg      Config
	ev       []OrchestratorEvidence
	roundDir func() (string, error)
	now      time.Time
	poll     int64
	active   int
	budget   struct {
		Day  string `json:"day"`
		Runs int    `json:"runs"`
	}
	budgetPath string
	receipts   map[string]sourceReceipt
	seen       map[string]bool
	deferred   []string
	halted     error
}

// runSourceRules is the deterministic agent: off path. Receipts are written
// before any live dispatch, so a crash or uncertain transport never repeats a
// launch. Per-event failures write rejected receipts and the batch continues.
func runSourceRules(ctx context.Context, s OrchestratorSpec, st *OrchestratorState, roundDir func() (string, error), batch SourceBatch, ev []OrchestratorEvidence) (out sourceOutcome, err error) {
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		return out, e
	}
	r := &sourceRunner{ctx: ctx, s: s, st: st, cfg: cfg, ev: ev, roundDir: roundDir, now: sourceNow().UTC(), poll: sourcePoll(s), seen: map[string]bool{}}
	for _, v := range ev {
		if v.Row.State == "running" {
			r.active++
		}
	}
	r.budgetPath = filepath.Join(OrchestratorDir(s.Name), "source-budget.json")
	if e = readJSON(r.budgetPath, &r.budget); e != nil && !os.IsNotExist(e) {
		return out, e
	}
	if day := r.now.Format("2006-01-02"); r.budget.Day != day {
		r.budget.Day, r.budget.Runs = day, 0
	}
	if r.receipts, e = loadSourceReceipts(s); e != nil {
		return out, e
	}
	defer func() {
		if de := deliverSourceSignals(s, st); de != nil && err == nil {
			err = de
		}
	}()
	inBatch := map[string]bool{}
	for _, event := range batch.Events {
		inBatch[event.ID] = true
	}
	keys := slices.Sorted(maps.Keys(r.receipts))
	for _, rule := range s.Source.Rules {
		var matching []SourceEvent
		for _, event := range batch.Events {
			if event.Fields["status"] == rule.Status {
				matching = append(matching, event)
			}
		}
		// Receipts carry their event, so retries and attempts outlive the
		// cursor. A replayed event uses its current status instead.
		for _, key := range keys {
			rc := r.receipts[key]
			if rc.Source != nil && rc.Rule == rule.Status && rc.Action == rule.Action && rc.RuleMode == rule.Mode && !inBatch[rc.Event] {
				matching = append(matching, *rc.Source)
			}
		}
		if rule.Batch {
			e = r.batchRule(rule, matching)
		} else {
			for _, event := range matching {
				if e = r.single(rule, event); e != nil {
					break
				}
			}
		}
		if e != nil {
			return out, e
		}
	}
	for _, event := range batch.Events {
		for _, rule := range s.Source.Rules {
			if event.Fields["status"] != rule.Status {
				continue
			}
			if rc, ok := r.receipts[sourceKey(s, event.ID, rule)]; ok {
				out.Acks = append(out.Acks, sourceAck{ID: event.ID, Run: rc.Run, Mode: rc.Mode})
			}
		}
	}
	out.Deferred = strings.Join(slices.Compact(r.deferred), "; ")
	if r.halted != nil {
		return out, r.halted
	}
	return out, nil
}

func (r *sourceRunner) deferUnit(reason string) { r.deferred = append(r.deferred, reason) }

func (r *sourceRunner) update(key string, signal string, fn func(*sourceReceipt, bool) bool) error {
	rc, wrote, e := updateSourceReceipt(r.s, key, func(rc *sourceReceipt, existed bool) bool {
		if !fn(rc, existed) {
			return false
		}
		if signal != "" {
			addSignal(rc, key, signal)
		}
		return true
	})
	if e != nil {
		return e
	}
	if wrote {
		r.receipts[key] = rc
	}
	return nil
}

func (r *sourceRunner) single(rule SourceRule, event SourceEvent) error {
	key := sourceKey(r.s, event.ID, rule)
	if r.seen[key] {
		return nil
	}
	r.seen[key] = true
	rc, ok := r.receipts[key]
	if ok {
		switch rc.Mode {
		case "dry-run", "live":
			return nil
		case "attempting", "needs-reconcile":
			return r.resolve(key, rc)
		case "rejected":
			if r.poll < rc.RetryPoll {
				return nil
			}
		}
	}
	return r.dispatch(rule, []SourceEvent{event}, []string{key}, rc.Attempts)
}

func (r *sourceRunner) batchRule(rule SourceRule, matching []SourceEvent) error {
	var members []SourceEvent
	attempts, pending := 0, false
	for _, event := range matching {
		key := sourceKey(r.s, event.ID, rule)
		if r.seen[key] {
			continue
		}
		r.seen[key] = true
		if rc, ok := r.receipts[key]; ok {
			switch rc.Mode {
			case "dry-run", "live":
				continue
			case "attempting", "needs-reconcile":
				if e := r.resolve(key, rc); e != nil {
					return e
				}
				pending = pending || uncertainReceipt(r.receipts[key].Mode)
				continue
			case "rejected":
				if r.poll < rc.RetryPoll {
					continue
				}
				attempts = max(attempts, rc.Attempts)
			}
		}
		members = append(members, event)
	}
	if len(members) == 0 {
		return nil
	}
	var last sourceBatchState
	statePath := filepath.Join(OrchestratorDir(r.s.Name), "source-batches", sourceRuleKey(r.s, rule)+".json")
	if e := readJSON(statePath, &last); e != nil && !os.IsNotExist(e) {
		return e
	}
	// Members are new to this rule, so any active batch means a changed set.
	if pending || (last.Run != "" && r.runActive(last.Run)) {
		r.deferUnit("active or attempting batch for " + rule.Status + " blocks a changed set")
		return nil
	}
	cooldown := sourceDefaultCooldown
	if rule.Cooldown != "" {
		cooldown, _ = OrchestratorDuration(rule.Cooldown)
	}
	if !last.At.IsZero() && r.now.Before(last.At.Add(cooldown)) {
		r.deferUnit("batch cooldown for " + rule.Status + " until " + last.At.Add(cooldown).Format(time.RFC3339))
		return nil
	}
	slices.SortFunc(members, func(a, b SourceEvent) int { return strings.Compare(a.ID, b.ID) })
	keys := []string{}
	for _, m := range members {
		keys = append(keys, sourceKey(r.s, m.ID, rule))
	}
	return r.dispatch(rule, members, keys, attempts)
}

func (r *sourceRunner) runActive(run string) bool {
	for _, v := range r.ev {
		if v.Row.Name == run && (v.Row.State == "running" || v.Row.State == "launch-unconfirmed") {
			return true
		}
	}
	return false
}

// sourceRunListed checks the target's inventory for a run name.
func sourceRunListed(ctx context.Context, cfg Config, host, run string) (bool, error) {
	h, ok := cfg.Hosts[host]
	if !ok {
		return false, fmt.Errorf("unknown source target %s", host)
	}
	resp, e := sourceDispatch(ctx, h, Request{Action: "list"})
	if e != nil {
		return false, e
	}
	for _, row := range resp.Rows {
		if row.Name == run {
			return true, nil
		}
	}
	return false, nil
}

// resolve promotes an uncertain receipt when its deterministic run exists.
// Past the grace period an absent run needs operator reconciliation.
func (r *sourceRunner) resolve(key string, rc sourceReceipt) error {
	listed, err := sourceRunListed(r.ctx, r.cfg, rc.Host, rc.Run)
	if err == nil && listed {
		if e := r.update(key, "", func(c *sourceReceipt, _ bool) bool {
			if !uncertainReceipt(c.Mode) {
				return false
			}
			c.Mode, c.Error = "live", ""
			return true
		}); e != nil {
			return e
		}
		return r.recordBatch(rc, "live")
	}
	if r.now.Sub(rc.AttemptedAt) < sourceAttemptGrace {
		return nil
	}
	if err != nil {
		return r.update(key, "attempt-stale", func(c *sourceReceipt, _ bool) bool {
			c.Error = "run lookup: " + err.Error()
			return uncertainReceipt(c.Mode) && !hasSignal(*c, "attempt-stale")
		})
	}
	return r.update(key, "needs-reconcile", func(c *sourceReceipt, _ bool) bool {
		if c.Mode != "attempting" {
			return false
		}
		c.Mode = "needs-reconcile"
		c.Error = "run " + c.Run + " not found after grace; reconcile with --admitted or --not-admitted"
		return true
	})
}

func (r *sourceRunner) recordBatch(rc sourceReceipt, mode string) error {
	if len(rc.Members) == 0 || rc.RuleKey == "" {
		return nil
	}
	return writeSourceBatch(r.s, rc.RuleKey, sourceBatchState{Run: rc.Run, Host: rc.Host, Mode: mode, At: rc.AttemptedAt})
}

func writeSourceBatch(s OrchestratorSpec, ruleKey string, b sourceBatchState) error {
	dir := filepath.Join(OrchestratorDir(s.Name), "source-batches")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	return WriteJSON(filepath.Join(dir, ruleKey+".json"), b)
}

func (r *sourceRunner) base(rule SourceRule, events []SourceEvent, i int, rc *sourceReceipt) {
	event := events[i]
	rc.Event, rc.Rule, rc.Action, rc.RuleMode, rc.Source = event.ID, rule.Status, rule.Action, rule.Mode, &event
	if rule.Batch {
		rc.RuleKey = sourceRuleKey(r.s, rule)
		rc.Members = nil
		for _, m := range events {
			rc.Members = append(rc.Members, m.ID)
		}
	}
}

func (r *sourceRunner) reject(rule SourceRule, events []SourceEvent, keys []string, run, host string, attempts int, cause error) error {
	for i, key := range keys {
		if e := r.update(key, "rejected", func(c *sourceReceipt, _ bool) bool {
			r.base(rule, events, i, c)
			c.Mode, c.Run, c.Host, c.Error = "rejected", run, host, cause.Error()
			c.Attempts = attempts + 1
			c.RetryPoll = r.poll + sourceRetryPolls(c.Attempts)
			return true
		}); e != nil {
			return e
		}
	}
	return nil
}

func (r *sourceRunner) admit(rule SourceRule, events []SourceEvent, keys []string, run, host string) error {
	for i, key := range keys {
		if e := r.update(key, "", func(c *sourceReceipt, _ bool) bool {
			r.base(rule, events, i, c)
			c.Mode, c.Run, c.Host, c.Error = "live", run, host, ""
			if c.AttemptedAt.IsZero() {
				c.AttemptedAt = r.now
			}
			return true
		}); e != nil {
			return e
		}
	}
	r.active++
	if rule.Batch {
		return writeSourceBatch(r.s, sourceRuleKey(r.s, rule), sourceBatchState{Run: run, Host: host, Mode: "live", At: r.now})
	}
	return nil
}

func (r *sourceRunner) dispatch(rule SourceRule, events []SourceEvent, keys []string, attempts int) error {
	s := r.s
	name, e := sourceRunName(s, rule, events)
	if e != nil {
		return r.reject(rule, events, keys, "", "", attempts, e)
	}
	rendered, order, e := renderSourceOrder(s, rule, events, name)
	if e != nil {
		return r.reject(rule, events, keys, name, "", attempts, e)
	}
	if rule.Mode == "live" {
		// Deferral happens before any artifact, so a fully deferred round
		// leaves no round directory.
		switch {
		case r.halted != nil:
			r.deferUnit("live dispatch halted after an uncertain outcome")
			return nil
		case sourcePaused(s):
			r.deferUnit("pause_file present")
			return nil
		case (s.Budget.ConcurrentRuns > 0 && r.active >= s.Budget.ConcurrentRuns) || (s.Budget.RunsPerDay > 0 && r.budget.Runs >= s.Budget.RunsPerDay):
			r.deferUnit(sourceCapDeferral)
			return nil
		}
	}
	dir, e := r.roundDir()
	if e != nil {
		return e
	}
	// Target admission for agent: off compares the request with this artifact.
	if e = os.WriteFile(filepath.Join(dir, name+".md"), []byte(rendered), 0600); e != nil {
		return e
	}
	if rule.Mode == "dry-run" {
		for i, key := range keys {
			if e = r.update(key, "", func(c *sourceReceipt, _ bool) bool {
				r.base(rule, events, i, c)
				c.Mode, c.Run, c.Order, c.Error = "dry-run", name, rendered, ""
				return true
			}); e != nil {
				return e
			}
		}
		if rule.Batch {
			return writeSourceBatch(s, sourceRuleKey(s, rule), sourceBatchState{Run: name, Mode: "dry-run", At: r.now})
		}
		return nil
	}
	host := order.Host
	if host == "" {
		host = r.cfg.DefaultHost
	}
	h, ok := r.cfg.Hosts[host]
	if !ok {
		return r.reject(rule, events, keys, name, host, attempts, fmt.Errorf("unknown source target %s", host))
	}
	q := Request{Action: "start", Name: name, Order: rendered, Orchestrator: s.Name, Meta: Meta{Order: order, Namespace: h.Namespace, HarnessWrapper: h.HarnessWrapper, ConfigPath: h.ConfigPath}, Cap: h.Cap, Workspace: h.Workspace}
	check := q
	check.AdmissionOnly = true
	// The probe also admits and reserves a classed order's slot on the
	// target, so the attempting receipt below is backed there in one step.
	check.ReserveClass = true
	resp, e := sourceDispatch(r.ctx, h, check)
	if e != nil {
		switch {
		case sourceRunExists(e):
			return r.admit(rule, events, keys, name, host)
		case sourceUncertain(e):
			// No receipt is needed to retry: a reservation the probe may have
			// made is re-made under the same run name, or lapses with the grace.
			r.halted = fmt.Errorf("source admission probe for %s: %w", name, e)
			r.deferUnit("target unreachable")
			return nil
		}
		return r.reject(rule, events, keys, name, host, attempts, e)
	}
	if order.Class != "" && !resp.ClassReserved {
		return r.reject(rule, events, keys, name, host, attempts, fmt.Errorf("target %s cannot reserve class %s; upgrade hev there", host, order.Class))
	}
	// The receipt precedes the budget charge and the launch.
	for i, key := range keys {
		if e = r.update(key, "", func(c *sourceReceipt, _ bool) bool {
			r.base(rule, events, i, c)
			c.Mode, c.Run, c.Host, c.Error, c.Attempts, c.AttemptedAt = "attempting", name, host, "", attempts+1, r.now
			c.Class = order.Class
			return true
		}); e != nil {
			return e
		}
	}
	r.budget.Runs++
	if e = WriteJSON(r.budgetPath, r.budget); e != nil {
		return e
	}
	if _, e = sourceDispatch(r.ctx, h, q); e != nil {
		switch {
		case sourceRunExists(e):
			return r.admit(rule, events, keys, name, host)
		case sourceUncertain(e):
			r.halted = fmt.Errorf("source dispatch %s uncertain; it is promoted if the run appears, otherwise reconcile: %w", name, e)
			return nil
		}
		return r.reject(rule, events, keys, name, host, attempts, e)
	}
	return r.admit(rule, events, keys, name, host)
}

// sourcePendingWork names receipts that need a round even when evidence is
// unchanged: unresolved attempts and rejected receipts due for retry.
func sourcePendingWork(s OrchestratorSpec) []string {
	if s.Source.Plugin == "" || s.Agent != "off" {
		return nil
	}
	receipts, _ := loadSourceReceipts(s)
	poll := sourcePoll(s)
	out := []string{}
	for key, r := range receipts {
		if r.Mode == "attempting" || (r.Mode == "rejected" && poll >= r.RetryPoll) {
			out = append(out, key+":"+r.Mode)
		}
	}
	slices.Sort(out)
	return out
}

func ackSource(ctx context.Context, s OrchestratorSpec, batch SourceBatch, acks []sourceAck) error {
	if !batch.Ack || len(acks) == 0 {
		return nil
	}
	if batch.AckBatch {
		var lines bytes.Buffer
		for _, a := range acks {
			b, _ := json.Marshal(a)
			lines.Write(append(b, '\n'))
		}
		_, e := sourceCommand(ctx, s, sourceInvocation{Args: sourceArgs(s, "ack", "--batch"), Stdin: lines.Bytes()})
		return e
	}
	for _, a := range acks {
		if _, e := sourceCommand(ctx, s, sourceInvocation{Args: sourceArgs(s, "ack", "--run", a.Run, "--mode", a.Mode, "--", a.ID)}); e != nil {
			return e
		}
	}
	return nil
}

// ReconcileSourceReceipt resolves attempting or needs-reconcile receipts for
// an event after the operator inspected the target. Admitted receipts become
// live; not-admitted receipts become rejected and retry on the next round.
// Batch members sharing the run are reconciled together.
func ReconcileSourceReceipt(name, event string, admitted bool) (int, error) {
	s, e := LoadOrchestrator(name)
	if e != nil {
		return 0, e
	}
	receipts, e := loadSourceReceipts(s)
	if e != nil {
		return 0, e
	}
	runs := map[string]bool{}
	for _, r := range receipts {
		if r.Event == event && uncertainReceipt(r.Mode) {
			runs[r.Run] = true
		}
	}
	if len(runs) == 0 {
		return 0, fmt.Errorf("no attempting or needs-reconcile receipt for event %s", event)
	}
	n := 0
	for _, key := range slices.Sorted(maps.Keys(receipts)) {
		if !runs[receipts[key].Run] || !uncertainReceipt(receipts[key].Mode) {
			continue
		}
		rc, wrote, e := updateSourceReceipt(s, key, func(c *sourceReceipt, _ bool) bool {
			if !uncertainReceipt(c.Mode) {
				return false
			}
			if admitted {
				c.Mode, c.Error = "live", ""
			} else {
				c.Mode, c.Error, c.RetryPoll = "rejected", "reconciled: not admitted", 0
			}
			return true
		})
		if e != nil {
			return n, e
		}
		if wrote {
			n++
			if admitted && len(rc.Members) > 0 && rc.RuleKey != "" {
				if e = writeSourceBatch(s, rc.RuleKey, sourceBatchState{Run: rc.Run, Host: rc.Host, Mode: "live", At: rc.AttemptedAt}); e != nil {
					return n, e
				}
			}
		}
	}
	return n, os.WriteFile(filepath.Join(OrchestratorDir(name), "wake"), []byte("reconcile"), 0600)
}

// A model round may choose whether to act, but cannot promote a dry-run rule,
// invent an event, or change a rule's rendered order. Dispatch admission owns
// the live caps and the deterministic name makes duplicate starts reject.
func admitSourceOrder(s OrchestratorSpec, q Request) error {
	if q.Action != "list" && q.Action != "show" && q.Action != "logs" && sourcePaused(s) {
		return fmt.Errorf("source pause_file is present")
	}
	if q.Action != "start" || q.Meta.Parent != "" || s.Source.Plugin == "" || len(s.Source.Rules) == 0 {
		return nil
	}
	st, e := orchestratorState(s.Name)
	if e != nil {
		return e
	}
	round := filepath.Join(OrchestratorDir(s.Name), "rounds", fmt.Sprintf("%03d", st.Rounds))
	if s.Agent == "off" {
		// No model runs; only the runner's own rendered artifact for this
		// round is admissible, including retries and batch subsets.
		if !ValidName(q.Name) {
			return fmt.Errorf("invalid run name")
		}
		b, e := os.ReadFile(filepath.Join(round, q.Name+".md"))
		if e == nil && strings.TrimSpace(string(b)) == strings.TrimSpace(q.Order) {
			return nil
		}
		return fmt.Errorf("source order does not match a rendered rule in the current round")
	}
	var packet OrchestratorPacket
	if e = readJSON(filepath.Join(round, "packet.json"), &packet); e != nil {
		return e
	}
	for _, u := range sourceUnits(s, packet.Source.Events) {
		if u.Rule.Mode != "live" {
			continue
		}
		name, e := sourceRunName(s, u.Rule, u.Events)
		if e != nil || q.Name != name {
			continue
		}
		rendered, _, e := renderSourceOrder(s, u.Rule, u.Events, name)
		if e != nil {
			return e
		}
		if strings.TrimSpace(q.Order) == strings.TrimSpace(rendered) {
			return nil
		}
	}
	return fmt.Errorf("source order does not match a live rule in the current packet")
}
