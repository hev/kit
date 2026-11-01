package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type Meta struct {
	Ephemeral       bool     `json:"ephemeral,omitempty"`
	ResolvedRepo    string   `json:"resolved_repo,omitempty"`
	BranchCreated   bool     `json:"branch_created,omitempty"`
	WorktreeCreated bool     `json:"worktree_created,omitempty"`
	LaunchEnv       []string `json:"launch_env,omitempty"`
	LaunchResolved  bool     `json:"launch_resolved,omitempty"`
	HarnessWrapper  []string `json:"harness_wrapper"`
	SameHost        bool     `json:"same_host,omitempty"`
	ParentHostName  string   `json:"parent_host_name,omitempty"`
	Parent          string   `json:"parent,omitempty"`
	ParentHost      Host     `json:"parent_host,omitempty"`
	Revision        string   `json:"revision,omitempty"`
	Order
	Branch string    `json:"branch"`
	Start  time.Time `json:"start"`

	ConfigPath string `json:"config_path,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}
type Result struct {
	LaunchUnconfirmed bool      `json:"launch_unconfirmed,omitempty"`
	AdmissionFailed   bool      `json:"admission_failed,omitempty"`
	SameHost          bool      `json:"same_host,omitempty"`
	Verdict           *Verdict  `json:"verdict,omitempty"`
	Validation        string    `json:"validation,omitempty"`
	NeedsReview       bool      `json:"needs_review,omitempty"`
	ExitCode          int       `json:"exit_code"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	Branch            string    `json:"branch"`
	Head              string    `json:"head_sha"`
	PR                string    `json:"pr_url,omitempty"`
	Session           string    `json:"session_id,omitempty"`
	Timeout           bool      `json:"timeout"`
	Signal            string    `json:"signal,omitempty"`
	Stopped           bool      `json:"stopped,omitempty"`
	Error             string    `json:"error,omitempty"`
	NotifyError       string    `json:"notify_error,omitempty"`
}

func State(r *Result, live bool) string {
	if live {
		return "running"
	}
	if r != nil && r.LaunchUnconfirmed {
		return "launch-unconfirmed"
	}
	if r == nil || r.End.IsZero() {
		return "died"
	}
	if r.NeedsReview {
		return "needs-review"
	}
	if r.Timeout {
		return "timeout"
	}
	if r.ExitCode == 0 {
		return "done"
	}
	return "failed"
}
func WriteJSON(p string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(p+".tmp", append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func readJSON(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func command(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("%s: %w: %s", args[0], e, b)
	}
	return strings.TrimSpace(string(b)), nil
}

const Prompt = "Read ../order.md (the full order). Work in this git worktree. Check ../inbox.md between steps and treat its entries as instructions from the dispatcher. Keep ../status.md current with short progress notes and blockers. Commit, push the branch, open a draft PR, and write ../report.md. Meet every Acceptance item. A test that fails and then passes on rerun is a flaky test, not a pass. Report it by name in ../report.md and fix it if it is in scope; never rerun until green. Explicit instructions in the order override these defaults, including instructions not to push or open a PR."

func Notify(ctx context.Context, args []string, m Meta, r Result) error {
	if len(args) == 0 {
		return nil
	}
	summary := fmt.Sprintf("exit=%d", r.ExitCode)
	if r.PR != "" {
		summary += " " + r.PR
	}
	if r.Validation != "" {
		summary += "; validation=" + r.Validation
	}
	if r.Verdict != nil {
		summary += "; verdict=" + r.Verdict.Verdict
	}
	repl := strings.NewReplacer("{name}", m.Name, "{state}", State(&r, false), "{summary}", summary, "{reply}", m.Reply)
	a := make([]string, len(args))
	for i, s := range args {
		a[i] = repl.Replace(s)
	}
	out, e := exec.CommandContext(ctx, a[0], a[1:]...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("%w: %s", e, out)
	}
	return nil
}
func Supervise(name string) (superviseErr error) {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, terminationSignals()...)
	defer signal.Stop(signals)
	if !ValidName(name) {
		return fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if e := readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
		return e
	}
	r := Result{SameHost: m.SameHost, ExitCode: 1, Start: time.Now().UTC(), Branch: m.Branch}
	if m.Parent != "" {
		defer func() {
			update := ValidationRecord{SameHost: m.SameHost, Revision: m.Revision, Name: m.Name, Host: m.Host, State: "fail", Verdict: r.Verdict, Evidence: filepath.Join(dir, "evidence"), Error: r.Error}
			if superviseErr != nil {
				update.Error = superviseErr.Error()
				update.Verdict = nil
			}
			if update.Verdict != nil && update.Error == "" {
				update.State = update.Verdict.Verdict
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if _, err := RPC(ctx, m.ParentHost, Request{Action: "validation-result", Name: m.Parent, ValidationRecord: update}); err != nil {
				superviseErr = fmt.Errorf("parent update: %w", err)
			}
		}()
	}
	if e := writePID(dir); e != nil {
		return e
	}
	defer os.Remove(filepath.Join(dir, "supervisor.json"))
	defer os.Remove(filepath.Join(dir, "harness.json"))
	resultPath := filepath.Join(dir, "result.json")
	wt := filepath.Join(dir, "wt")
	args := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox"}
	if m.Harness == "claude" {
		args = []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}
	}
	if m.Model != "" {
		args = append(args, "--model", m.Model)
	}
	prompt := Prompt
	if m.Parent != "" {
		prompt = ValidatorPrompt
	}
	args = append(args, prompt)
	log, e := os.OpenFile(filepath.Join(dir, "log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	child := harnessCommand(harnessWrapper(m), m.Harness, args)
	child.Dir = wt
	child.Stdout = log
	child.Stderr = log
	env, err := launchEnvironment(dir)
	if err != nil {
		return err
	}
	child.Env = append(os.Environ(), env...)
	if m.Namespace != "" {
		child.Env = append(child.Env, "LAYER_NAMESPACE="+m.Namespace)
	}
	setupChild(child)
	_ = WriteJSON(resultPath, r)
	kill, startErr := startChild(child)
	if startErr == nil {
		if err := recordHarness(dir, child); err != nil {
			kill()
			_ = child.Wait()
			startErr = err
		}
	}
	if e = startErr; e != nil {
		r.Error = e.Error()
	} else {
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(time.Duration(m.Timeout) * time.Minute)
		defer deadline.Stop()
		finished := false
		for !finished {
			select {
			case e = <-done:
				finished = true
			case <-ticker.C:
				if r.Session == "" {
					if id := sessionFromLog(filepath.Join(dir, "log")); id != "" {
						r.Session = id
						_ = WriteJSON(resultPath, r)
					}
				}
				if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
					r.Stopped = true
					kill()
					e = <-done
					finished = true
				}
			case sig := <-signals:
				r.Stopped = true
				r.Signal = sig.String()
				kill()
				e = <-done
				finished = true
			case <-deadline.C:
				r.Timeout = true
				kill()
				e = <-done
				finished = true
			}
		}
		// Also remove descendants left behind by an exited harness.
		kill()
		r.ExitCode = 0
		if e != nil {
			r.ExitCode = 1
			if ee, ok := e.(*exec.ExitError); ok {
				r.ExitCode = ee.ExitCode()
				if r.ExitCode < 0 {
					r.ExitCode = 137
				}
			}
		}
		if r.Stopped {
			r.ExitCode = 143
		}
	}
	if r.Session == "" {
		r.Session = sessionFromLog(filepath.Join(dir, "log"))
	}
	r.End = time.Now().UTC()
	if m.Parent == "" {
		r.NeedsReview = TwiceFailed(validationRecords(dir))
	}
	r.Head, _ = command(wt, "git", "rev-parse", "HEAD")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pr := exec.CommandContext(ctx, "gh", "pr", "view", m.Branch, "--json", "url", "--jq", ".url")
	pr.Dir = wt
	if b, err := pr.Output(); err == nil {
		r.PR = strings.TrimSpace(string(b))
	}
	cancel()
	if m.Parent != "" {
		b, err := os.ReadFile(filepath.Join(dir, "verdict.json"))
		if err == nil {
			r.Verdict, err = ParseVerdict(b)
		}
		if err == nil && (r.ExitCode != 0 || r.Timeout || r.Stopped) {
			err = fmt.Errorf("validator run did not complete successfully")
		}
		if err == nil {
			err = VerifyVerdict(dir, r.Verdict)
		}
		if err == nil {
			currentHead, headErr := command(wt, "git", "rev-parse", "HEAD")
			_, diffErr := command(wt, "git", "diff", "--exit-code", "HEAD", "--")
			if headErr != nil || diffErr != nil || currentHead != m.Revision {
				err = fmt.Errorf("validator changed implementation code or HEAD")
			}
		}
		if err == nil && r.Verdict == nil {
			err = fmt.Errorf("missing verdict")
		}
		if err != nil {
			r.ExitCode = 1
			r.Error = "invalid validator verdict: " + err.Error()
			r.Verdict = nil
		}
	}
	if e = WriteJSON(resultPath, r); e != nil {
		return e
	}
	cfg, configErr := Load(m.ConfigPath)
	if configErr != nil && m.Parent == "" {
		r.NotifyError = "target notification config: " + configErr.Error()
		return WriteJSON(resultPath, r)
	}
	if configErr != nil {
		r.Error = "validator config: " + configErr.Error()
		r.Verdict = nil
	}
	if m.Parent == "" && ShouldValidate(m, r, cfg) {
		validationCtx, validationCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		_, err := validateFrom(validationCtx, cfg, m.Name, "", false, m.Host)
		validationCancel()
		unlock, lockErr := lockRoot()
		if lockErr != nil {
			return lockErr
		}
		var latest Result
		if readJSON(resultPath, &latest) == nil {
			r = latest
		}
		if err != nil {
			if r.Validation == "" {
				r.Validation = "pending"
			}
			r.Error = "validation: " + err.Error()
		}
		e = WriteJSON(resultPath, r)
		unlock()
		if e != nil {
			return e
		}
	}
	if m.Parent != "" {
		if e = WriteJSON(resultPath, r); e != nil {
			return e
		}
		return nil
	}
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer notifyCancel()
	if err := Notify(notifyCtx, cfg.Notify, m, r); err != nil {
		return setNotifyError(dir, err)
	}
	return nil
}
func sessionFromLog(path string) string {
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(b), "\n") {
		var v map[string]any
		if json.Unmarshal([]byte(line), &v) != nil {
			continue
		}
		if s, ok := v["thread_id"].(string); ok {
			return s
		}
		if s, ok := v["session_id"].(string); ok {
			return s
		}
	}
	return ""
}

func setNotifyError(dir string, notifyErr error) error {
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	var r Result
	if err = readJSON(filepath.Join(dir, "result.json"), &r); err != nil {
		return err
	}
	r.NotifyError = notifyErr.Error()
	return WriteJSON(filepath.Join(dir, "result.json"), r)
}
func notifyValidation(dir string, m Meta) {
	cfg, err := Load(m.ConfigPath)
	if err == nil {
		var r Result
		err = readJSON(filepath.Join(dir, "result.json"), &r)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err = Notify(ctx, cfg.Notify, m, r)
		}
	}
	if err != nil {
		_ = setNotifyError(dir, err)
	}
}

// NotifyValidation is a separate process so an RPC can reply and exit promptly.
func NotifyValidation(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return err
	}
	notifyValidation(dir, m)
	return nil
}
func launchValidationNotification(dir string, m Meta) {
	exe, err := os.Executable()
	if err == nil {
		c := exec.Command(exe, "dispatch", "notify-validation", m.Name)
		setupChild(c)
		err = c.Start()
		if err == nil {
			go c.Wait()
		}
	}
	if err != nil {
		_ = setNotifyError(dir, err)
	}
}

// harnessCommand preserves argument boundaries for the platform exec implementation.
func harnessCommand(wrapper []string, harness string, args []string) *exec.Cmd {
	argv := append([]string{}, wrapper...)
	if len(wrapper) > 0 {
		argv = append(argv, "--")
	}
	argv = append(argv, harness)
	argv = append(argv, args...)
	c := exec.Command(argv[0], argv[1:]...)
	configureHarnessCommandLine(c)
	return c
}

func harnessWrapper(m Meta) []string { return m.HarnessWrapper }
