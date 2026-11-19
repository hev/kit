//go:build !windows

package dispatch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

// degradeHarness is a host whose fake claude and codex exit on the
// captured plan-limit record for every model in $LIMIT_MODELS ("codex" for
// Codex's default model) and otherwise run $DEGRADE_ACTION and succeed. The
// on_degrade hook appends its stdin to hook.log; the peers sender appends its
// argv to peers.log.
type degradeHarness struct {
	bin, cfg            string
	launches, hook, out string
	attention           *[]attention.Notice
}

func newDegradeHarness(t *testing.T) degradeHarness {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	h := degradeHarness{bin: bin, cfg: filepath.Join(bin, "dispatch.toml"), launches: filepath.Join(bin, "launches.log"), hook: filepath.Join(bin, "hook.log"), out: filepath.Join(bin, "peers.log")}
	t.Setenv("LAUNCH_LOG", h.launches)
	t.Setenv("HOOK_LOG", h.hook)
	t.Setenv("PEERS_LOG", h.out)
	// A fresh copy of the incident's records with a reset two days out, so
	// the test does not age past the captured reset.
	reset := strconv.FormatInt(time.Now().Add(48*time.Hour).Unix(), 10)
	claudeLimit := strings.ReplaceAll(string(degradeFixture(t, "claude-stream-usage-limit.jsonl")), "1795266000", reset)
	codexLimit := string(degradeFixture(t, "codex-exec-usage-limit.jsonl"))
	for name, body := range map[string]string{"claude-limit.jsonl": claudeLimit, "codex-limit.jsonl": codexLimit} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	harness := func(name string) string {
		return `#!/bin/sh
model=""
prev=""
for a; do
  if [ "$prev" = "--model" ]; then model="$a"; fi
  prev="$a"
done
label="${model:-` + name + `}"
echo "` + name + ` $label" >> "$LAUNCH_LOG"
case " $LIMIT_MODELS " in
*" $label "*) cat "` + filepath.Join(bin, name+"-limit.jsonl") + `"; exit 1 ;;
esac
printf '{"type":"system","subtype":"init","session_id":"ok-%s"}\n' "$label"
eval "$DEGRADE_ACTION"
exit 0
`
	}
	scripts := map[string]string{
		"claude":     harness("claude"),
		"codex":      harness("codex"),
		"on-degrade": "#!/bin/sh\ncat >> \"$HOOK_LOG\"\n",
		"fake-hev":   "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PEERS_LOG\"\n",
		"gh":         "#!/bin/sh\nexit 1\n",
		"git":        "#!/bin/sh\necho sha\n",
		"tmux":       "#!/bin/sh\nexit 0\n",
	}
	for file, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, file), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(h.cfg, []byte("[degrade]\non_degrade = [\""+filepath.Join(bin, "on-degrade")+"\"]\nhook_timeout = \"5s\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", h.cfg)
	oldExe, oldEmit := notificationExecutable, emitAttention
	var mu sync.Mutex
	notices := []attention.Notice{}
	h.attention = &notices
	notificationExecutable = func() (string, error) { return filepath.Join(bin, "fake-hev"), nil }
	emitAttention = func(n attention.Notice, _ string) { mu.Lock(); notices = append(notices, n); mu.Unlock() }
	t.Cleanup(func() { notificationExecutable, emitAttention = oldExe, oldEmit })
	return h
}

func (h degradeHarness) lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func (h degradeHarness) events(t *testing.T) []DegradeEvent {
	t.Helper()
	var out []DegradeEvent
	// The hook runs in the background; wait for every expected payload.
	for _, line := range h.lines(t, h.hook) {
		var ev DegradeEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("hook payload %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func degradeRun(t *testing.T, h degradeHarness, m Meta, order string) string {
	t.Helper()
	dir := filepath.Join(Root(), m.Name)
	if err := os.MkdirAll(filepath.Join(dir, "wt"), 0700); err != nil {
		t.Fatal(err)
	}
	m.Start, m.ConfigPath, m.LaunchResolved, m.Branch = time.Now().UTC(), h.cfg, true, "d/"+m.Name
	if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "order.md"), []byte(order), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

const degradeOrder = "---\nname: NAME\nrepo: app\n---\nDo it.\n\n## Acceptance\n\n- It works\n"

// A worker whose first model is out of plan relaunches on the next model in
// the same worktree, records the event, fires the hook and tells the reply
// session; ls shows the model running now.
func TestDegradeWorkerRelaunchesOnNextModel(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-fable-5-2")
	t.Setenv("DEGRADE_ACTION", `printf 'did it\ndone\n' > ../status.md; pwd > ../ran-in`)
	dir := degradeRun(t, h, Meta{Order: Order{Name: "deg-worker", Host: "oven", Repo: "app", Harness: "claude", Model: "claude-fable-5-2", Reply: "macbook/shell", Timeout: 1}}, strings.Replace(degradeOrder, "NAME", "deg-worker", 1))
	if err := superviseRound("deg-worker"); err != nil {
		t.Fatal(err)
	}
	if got := h.lines(t, h.launches); strings.Join(got, "|") != "claude claude-fable-5-2|claude claude-opus-5-5" {
		t.Fatalf("launches %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "ran-in")); strings.TrimSpace(string(b)) != filepath.Join(dir, "wt") {
		t.Fatalf("relaunch ran in %q", b)
	}
	r := readResult(t, dir)
	if State(&r, false) != "done" || r.Exhausted || len(r.Models) != 2 || len(r.Degrades) != 1 {
		t.Fatalf("result %+v", r)
	}
	first, second := r.Models[0], r.Models[1]
	if first.Model != "claude-fable-5-2" || first.Reason != "order" || first.Limit == nil || first.Limit.Kind != "usage_limit_reached" || first.Limit.LimitType != "seven_day_overage_included" || first.ExitCode != 1 {
		t.Fatalf("first model %+v", first)
	}
	if second.Model != "claude-opus-5-5" || second.Reason != "degrade: limit" || second.Limit != nil || second.ExitCode != 0 {
		t.Fatalf("second model %+v", second)
	}
	if r.Session != "ok-claude-opus-5-5" {
		t.Fatalf("session %q", r.Session)
	}
	waitFor(t, "hook", func() bool { return len(h.events(t)) == 1 })
	ev := h.events(t)[0]
	if ev.Event != "degrade" || ev.Run != "deg-worker" || ev.Host != "oven" || ev.Step != "worker" || ev.From.Model != "claude-fable-5-2" || ev.To == nil || ev.To.Model != "claude-opus-5-5" || ev.LimitType != "seven_day_overage_included" || ev.ResetsAt == nil || ev.Time.IsZero() || ev.Urgent {
		t.Fatalf("hook payload %+v", ev)
	}
	peers := h.lines(t, h.out)
	if len(peers) != 1 || !strings.Contains(peers[0], "peers send --from") || !strings.Contains(peers[0], "/d-deg-worker") || !strings.Contains(peers[0], "--inbox-only -- macbook/shell dispatch deg-worker on oven (worker) degraded claude/claude-fable-5-2 → claude/claude-opus-5-5 (seven_day_overage_included") {
		t.Fatalf("peers notice %q", peers)
	}
	if n := *h.attention; len(n) == 0 || n[0].Event != attention.KindDone || !strings.Contains(n[0].Text, "degraded") {
		t.Fatalf("attention %+v", n)
	}
	// The run log carries the event for the trace.
	if b, _ := os.ReadFile(filepath.Join(dir, "log")); !strings.Contains(string(b), `"type":"dispatch.degrade"`) {
		t.Fatal("log has no degrade record")
	}
	list, err := Handle(Request{Action: "list"})
	if err != nil || len(list.Rows) != 1 || list.Rows[0].ModelRunning != "opus-5-5↓" {
		t.Fatalf("ls row %+v %v", list.Rows, err)
	}
	spans := TraceSpanRows([]RunRecord{{Name: "deg-worker", Meta: Meta{Order: Order{Name: "deg-worker"}}, History: History{Attempts: []Attempt{{Number: 1}}}, Results: []Result{r}}}, "oven", nil)
	if len(spans) != 1 || !strings.Contains(spans[0].Models, `"model":"claude-fable-5-2","reason":"order","exit_code":1,"limit":"usage_limit_reached"`) || !strings.Contains(spans[0].Models, `"model":"claude-opus-5-5","reason":"degrade: limit"`) {
		t.Fatalf("trace models %+v", spans)
	}

	// The next launch on this host knows Fable is out and skips it.
	if err := os.Remove(h.launches); err != nil {
		t.Fatal(err)
	}
	degradeRun(t, h, Meta{Order: Order{Name: "deg-next", Repo: "app", Harness: "claude", Model: "claude-fable-5-2", Reply: "macbook/shell", Timeout: 1}}, strings.Replace(degradeOrder, "NAME", "deg-next", 1))
	if err := superviseRound("deg-next"); err != nil {
		t.Fatal(err)
	}
	if got := h.lines(t, h.launches); strings.Join(got, "|") != "claude claude-opus-5-5" {
		t.Fatalf("known-exhausted launches %q", got)
	}
	next := readResult(t, filepath.Join(Root(), "deg-next"))
	if len(next.Models) != 2 || !next.Models[0].Skipped || next.Degrades[0].Reason != "observed" || State(&next, false) != "done" {
		t.Fatalf("skip result %+v", next)
	}
}

// When every model is out the run is blocked, not failed, with the reset,
// and the exhausted event is urgent.
func TestDegradeWorkerExhaustedChain(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-opus-5-5 claude-sonnet-5-5 codex")
	dir := degradeRun(t, h, Meta{Order: Order{Name: "deg-out", Repo: "app", Harness: "claude", Model: "claude-opus-5-5", Reply: "macbook/shell", Timeout: 1}}, strings.Replace(degradeOrder, "NAME", "deg-out", 1))
	if err := superviseRound("deg-out"); err != nil {
		t.Fatal(err)
	}
	if got := h.lines(t, h.launches); strings.Join(got, "|") != "claude claude-opus-5-5|claude claude-sonnet-5-5|codex codex" {
		t.Fatalf("launches %q", got)
	}
	r := readResult(t, dir)
	if State(&r, false) != "blocked" || !r.Exhausted || r.Outcome != OutcomeBlocked || !strings.HasPrefix(r.Reason, "all models exhausted (resets ") || len(r.Models) != 3 || len(r.Degrades) != 3 {
		t.Fatalf("result %+v", r)
	}
	waitFor(t, "hook", func() bool { return len(h.events(t)) == 3 })
	last := h.events(t)[2]
	if last.Event != "exhausted" || !last.Urgent || last.To != nil || last.From.Harness != "codex" || last.ResetsAt == nil {
		t.Fatalf("exhausted payload %+v", last)
	}
	peers := h.lines(t, h.out)
	if len(peers) != 3 || !strings.Contains(peers[2], "[urgent] dispatch deg-out") || !strings.Contains(peers[2], "all models exhausted") {
		t.Fatalf("peers %q", peers)
	}
	found := false
	for _, n := range *h.attention {
		if n.Event == attention.KindNeedsYou && strings.Contains(n.Text, "all models exhausted") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no needs-you notice: %+v", *h.attention)
	}
}

// degrade: off pins the order's model: a limit is the run's end.
func TestDegradeOffPinsModel(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-fable-5-2")
	dir := degradeRun(t, h, Meta{Order: Order{Name: "deg-pin", Repo: "app", Harness: "claude", Model: "claude-fable-5-2", Degrade: DegradeOff, Timeout: 1}}, strings.Replace(degradeOrder, "NAME", "deg-pin", 1))
	if err := superviseRound("deg-pin"); err != nil {
		t.Fatal(err)
	}
	if got := h.lines(t, h.launches); len(got) != 1 {
		t.Fatalf("launches %q", got)
	}
	if r := readResult(t, dir); State(&r, false) != "blocked" || !r.Exhausted {
		t.Fatalf("result %+v", r)
	}
}

// A validator degrades too, past any model its implementer ran.
func TestDegradeValidatorStaysIndependent(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-fable-5-2")
	t.Setenv("DEGRADE_ACTION", `mkdir -p ../evidence; echo ok > ../evidence/check.log; printf '{"verdict":"pass","items":[{"item":"It works","result":"pass","evidence":["../evidence/check.log"],"note":"checked"}]}' > ../verdict.json`)
	parent := filepath.Join(Root(), "deg-app")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	_ = WriteJSON(filepath.Join(parent, "meta.json"), Meta{Order: Order{Name: "deg-app"}})
	_ = WriteJSON(filepath.Join(parent, "result.json"), Result{Head: "sha", End: time.Now()})
	child := degradeRun(t, h, Meta{Parent: "deg-app", ParentHost: Host{Local: true}, Revision: "sha", ReviewOnly: true, Implementer: []ModelStep{{Harness: "claude", Model: "claude-opus-5-5"}}, Order: Order{Name: "deg-app-v1", Repo: "app", Harness: "claude", Model: "claude-fable-5-2", Reply: "macbook/shell", Timeout: 1}}, strings.Replace(degradeOrder, "NAME", "deg-app", 1))
	if err := Supervise("deg-app-v1"); err != nil {
		t.Fatal(err)
	}
	// Opus is the implementer's: the validator goes from Fable to Sonnet.
	if got := h.lines(t, h.launches); strings.Join(got, "|") != "claude claude-fable-5-2|claude claude-sonnet-5-5" {
		t.Fatalf("launches %q", got)
	}
	records := validationRecords(parent)
	if len(records) != 1 || records[0].State != "pass" {
		t.Fatalf("validation %+v", records)
	}
	r := readResult(t, child)
	if len(r.Models) != 2 || r.Models[1].Model != "claude-sonnet-5-5" {
		t.Fatalf("validator models %+v", r.Models)
	}
	waitFor(t, "hook", func() bool { return len(h.events(t)) == 1 })
	if ev := h.events(t)[0]; ev.Step != "validate" || ev.To.Model != "claude-sonnet-5-5" {
		t.Fatalf("validator event %+v", ev)
	}
	if peers := h.lines(t, h.out); len(peers) != 1 || !strings.Contains(peers[0], "(validate) degraded claude/claude-fable-5-2 → claude/claude-sonnet-5-5") {
		t.Fatalf("validator notice %q", peers)
	}
}

func degradeOrchestrator(t *testing.T, h degradeHarness) (*[]OrchestratorNotice, OrchestratorSpec) {
	t.Helper()
	// orchHome resets HOME; keep the harness's.
	t.Setenv("PEERS_SELF", "test-host")
	t.Setenv("HEV_ORCHESTRATOR", "")
	var mu sync.Mutex
	sent := []OrchestratorNotice{}
	old := orchestratorNoticeSender
	orchestratorNoticeSender = func(_ context.Context, n OrchestratorNotice) error {
		mu.Lock()
		sent = append(sent, n)
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() { orchestratorNoticeSender = old })
	text := strings.Replace(orchestratorExample, "mode: loop", "mode: once", 1)
	text = strings.Replace(text, "harness: claude", "harness: claude\nmodel: claude-fable-5-2", 1)
	s, err := ApplyOrchestrator(text, false)
	if err != nil {
		t.Fatal(err)
	}
	return &sent, s
}

func orchestratorDriverFor(sent *[]OrchestratorNotice) OrchestratorDriver {
	d := DefaultOrchestratorDriver()
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }
	d.Tick = nil
	return d
}

// An orchestrator round (the incident: a follow-through round on Fable)
// degrades inside the round; the round succeeds and no failure is counted.
func TestDegradeOrchestratorRound(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-fable-5-2")
	t.Setenv("DEGRADE_ACTION", `printf 'Goal status: waiting.\n' > checkpoint.md`)
	sent, s := degradeOrchestrator(t, h)
	if err := SuperviseOrchestrator(context.Background(), s.Name, orchestratorDriverFor(sent)); err != nil {
		t.Fatal(err)
	}
	if got := h.lines(t, h.launches); strings.Join(got, "|") != "claude claude-fable-5-2|claude claude-opus-5-5" {
		t.Fatalf("launches %q", got)
	}
	st, err := orchestratorState(s.Name)
	if err != nil || st.ConsecutiveFailures != 0 || st.Rounds != 1 || st.Error != "" || st.State == "failed" {
		t.Fatalf("state %+v %v", st, err)
	}
	var models orchestratorModels
	if err := readJSON(filepath.Join(OrchestratorDir(s.Name), "rounds", "001", "models.json"), &models); err != nil || len(models.Models) != 2 || models.Running != "opus-5-5↓" || len(models.Degrades) != 1 {
		t.Fatalf("round models %+v %v", models, err)
	}
	if mark := orchestratorModelMark(s.Name); mark != "opus-5-5↓" {
		t.Fatalf("ls mark %q", mark)
	}
	waitFor(t, "hook", func() bool { return len(h.events(t)) == 1 })
	if ev := h.events(t)[0]; ev.Step != "orchestrator" || ev.Run != "o-release-train" || ev.From.Model != "claude-fable-5-2" || ev.To.Model != "claude-opus-5-5" || ev.Reply != "owner" {
		t.Fatalf("orchestrator event %+v", ev)
	}
	found := false
	for _, n := range *sent {
		if n.To == "owner" && strings.Contains(n.Message, "degraded claude/claude-fable-5-2 → claude/claude-opus-5-5") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no reply notice: %+v", *sent)
	}
}

// A round whose whole chain is out ends blocked with an urgent notice, is
// not counted as a failure, and sleeps until the reset.
func TestDegradeOrchestratorExhausted(t *testing.T) {
	h := newDegradeHarness(t)
	t.Setenv("LIMIT_MODELS", "claude-fable-5-2 claude-opus-5-5 claude-sonnet-5-5 codex")
	sent, s := degradeOrchestrator(t, h)
	if err := SuperviseOrchestrator(context.Background(), s.Name, orchestratorDriverFor(sent)); err != nil {
		b, _ := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "rounds", "001", "trace"))
		t.Fatalf("%v\nlaunches %q\ntrace %s", err, h.lines(t, h.launches), b)
	}
	if got := h.lines(t, h.launches); len(got) != 4 {
		t.Fatalf("launches %q", got)
	}
	st, err := orchestratorState(s.Name)
	if err != nil || st.State != "blocked" || st.ConsecutiveFailures != 0 || !strings.Contains(st.Error, "all models exhausted (resets ") || st.Next.Before(time.Now().Add(47*time.Hour)) {
		t.Fatalf("state %+v %v", st, err)
	}
	urgent := false
	for _, n := range *sent {
		if strings.HasPrefix(n.Message, "[urgent]") || strings.Contains(n.Message, "[urgent]") {
			urgent = true
		}
	}
	if !urgent {
		t.Fatalf("no urgent notice: %+v", *sent)
	}
	waitFor(t, "hook", func() bool { return len(h.events(t)) == 4 })
	if ev := h.events(t)[3]; ev.Event != "exhausted" || !ev.Urgent {
		t.Fatalf("exhausted payload %+v", ev)
	}
}
