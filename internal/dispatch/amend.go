package dispatch

import (
	"context"
	"fmt"
	"github.com/hev/kit/internal/weave"
	"os"
	"path/filepath"
	"time"
)

type Revision struct {
	Number int       `json:"number"`
	Time   time.Time `json:"time"`
	Order  string    `json:"order"`
	Diff   string    `json:"diff,omitempty"`
}
type Attempt struct {
	Number   int     `json:"number"`
	Revision int     `json:"revision"`
	Meta     Meta    `json:"meta"`
	Result   *Result `json:"result,omitempty"`
}
type History struct {
	Revisions []Revision `json:"revisions"`
	Attempts  []Attempt  `json:"attempts"`
}

func history(dir string, m Meta) History {
	var h History
	_ = readJSON(filepath.Join(dir, "history.json"), &h)
	if len(h.Revisions) == 0 {
		b, _ := os.ReadFile(filepath.Join(dir, "order.md"))
		h.Revisions = append(h.Revisions, Revision{Number: 1, Time: m.Start, Order: string(b)})
	}
	if len(h.Attempts) == 0 {
		h.Attempts = append(h.Attempts, Attempt{Number: 1, Revision: 1, Meta: m})
	}
	var result Result
	if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
		h.Attempts[len(h.Attempts)-1].Result = &result
	}
	return h
}

// orderDiff uses Git's line diff, matching the repository's review format.
func orderDiff(before, after string) string {
	if before == after {
		return ""
	}
	dir, err := os.MkdirTemp("", "order-diff-")
	if err != nil {
		return fmt.Sprintf("- %s\n+ %s", before, after)
	}
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, "previous"), []byte(before), 0600)
	_ = os.WriteFile(filepath.Join(dir, "revised"), []byte(after), 0600)
	c := helperCommand(context.Background(), "git", "diff", "--no-index", "--no-ext-diff", "--no-color", "--", "previous", "revised")
	c.Dir = dir
	out, _ := c.Output()
	return string(out)
}

func atomicOrder(dir, order string) error {
	f, err := os.CreateTemp(dir, ".order-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(order); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "order.md"))
}

// restartOrder names the edits a live attempt cannot absorb. A class change
// restarts so the new class is admitted like any launch, and the old one is
// released only once its worker has stopped. A worker receives its context
// only at launch, so a changed recipe restarts too.
func restartOrder(a, b Order) bool {
	return a.Host != b.Host || a.Harness != b.Harness || a.Model != b.Model || a.Base != b.Base || a.Class != b.Class || a.Context != b.Context
}

// carriedRecipeChanged reports an amendment that adds, removes or changes a
// recipe carried in the order.
func carriedRecipeChanged(m Meta, o Order) bool {
	want := ""
	if raw, err := decodeContextRecipe(o.ContextRecipe); o.ContextRecipe != "" && err == nil {
		want = weave.Hash(raw)
	}
	return want != m.ContextRecipeSHA256
}

var amendLaunch = launch
var amendStop = stopAttempt

func appendInbox(dir, note string) error {
	f, err := os.OpenFile(filepath.Join(dir, "inbox.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\n## " + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + note + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// stopAttempt waits for the supervisor to finish writing its result and releasing
// its process records. Windows uses the supervisor's Job Object for descendants.
func stopAttempt(dir, name string) error {
	if err := os.WriteFile(filepath.Join(dir, "stop"), []byte("amend"), 0600); err != nil {
		return err
	}
	if !supervisorLive(dir) {
		if err := stopHarness(dir); err != nil {
			return err
		}
	}
	until := time.Now().Add(15 * time.Second)
	for {
		live, err := liveNames()
		if err != nil {
			return err
		}
		if !supervisorLive(dir) && !live[name] {
			return nil
		}
		if time.Now().After(until) {
			return fmt.Errorf("attempt still stopping; inspect show before retrying")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func archiveAttempt(dir string) error {
	stamp := fmt.Sprint(time.Now().UnixNano())
	for _, f := range []string{"result.json", "report.md", "status.md", "log"} {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			continue
		}
		if err := os.Rename(p, p+"."+stamp); err != nil {
			return err
		}
	}
	return nil
}

func checkAmendAdmission(q Request, m Meta, live map[string]bool, existing bool) (Config, error) {
	policy, err := targetPolicy()
	if err != nil {
		return policy, err
	}
	cap := policy.Cap
	if q.Cap > 0 && q.Cap < cap {
		cap = q.Cap
	}
	n := len(live)
	if existing && live[q.Name] {
		n--
	}
	if err = CheckCap(n, cap); err != nil {
		return policy, err
	}
	if err = admitClass(policy.Classes, m.Class, q.Name, live); err != nil {
		return policy, err
	}
	cfg, err := loadTarget(m.ConfigPath, m.Host)
	if err != nil && (!os.IsNotExist(err) || m.ConfigPath != "") {
		return cfg, err
	}
	repo := filepath.Join(Root(), q.Name, "wt")
	if !existing {
		repo = resolveRepo(m.Repo, cfg.Workspace)
	}
	return cfg, admitTargetRepo(repo, m.Repo, m.Host)
}

func amend(q Request) (Response, error) {
	var r Response
	o, err := Parse(q.Order)
	if err != nil {
		return r, err
	}
	if o.Name != q.Name {
		return r, fmt.Errorf("amend cannot change name")
	}
	dir := filepath.Join(Root(), q.Name)
	unlock, err := lockRoot()
	if err != nil {
		return r, err
	}
	defer unlock()
	var m Meta
	if err = readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return r, err
	}
	if m.Parent != "" {
		return r, fmt.Errorf("amend implementation orders, not validators")
	}
	if o.Repo != m.Repo {
		return r, fmt.Errorf("amend cannot change repo")
	}
	live, err := liveNames()
	if err != nil {
		return r, err
	}
	if err = addReservations(live); err != nil {
		return r, err
	}
	current, err := os.ReadFile(filepath.Join(dir, "order.md"))
	if err != nil {
		return r, err
	}
	if string(current) == q.Order {
		r.Message = "order unchanged"
		return r, nil
	}
	next := m
	next.Order = o
	restarting := !live[q.Name] || restartOrder(m.Order, o) || carriedRecipeChanged(m, o)
	h := history(dir, m)
	before := h.Revisions[len(h.Revisions)-1].Order
	rev := Revision{Number: len(h.Revisions) + 1, Time: time.Now().UTC(), Order: q.Order, Diff: orderDiff(before, q.Order)}
	askSpan := recordAsk(&next, "amend", amendAskText(rev.Number, rev.Diff), q.Caller)
	if o.Host != m.Host {
		beginAttempt(&next, askSpan)
		return migrateAmend(q, m, next, live)
	}
	var cfg Config
	var contextErr error
	if restarting {
		cfg, err = checkAmendAdmission(q, next, live, true)
		if err != nil {
			return r, err
		}
		if _, err = os.Stat(filepath.Join(dir, "wt")); err != nil {
			return r, fmt.Errorf("worktree was cleaned")
		}
	}
	if restarting && (live[q.Name] || processRecordLive(filepath.Join(dir, "harness.json"))) {
		if err = amendStop(dir, q.Name); err != nil {
			return r, err
		}
		h = history(dir, m)
	}
	h.Revisions = append(h.Revisions, rev)
	if restarting {
		if err = archiveAttempt(dir); err != nil {
			return r, err
		}
		if m.Every == "" {
			next.Start = time.Now().UTC()
		}
		resolveLaunch(&next, cfg)
		// The amended order is the sender's: pin its carried recipe, if any.
		unpinRecipe(dir, &next)
		beginAttempt(&next, askSpan)
		contextErr = prepareContext(dir, &next, q.Order)
		h.Attempts = append(h.Attempts, Attempt{Number: len(h.Attempts) + 1, Revision: rev.Number, Meta: next})
		if err = resetValidation(dir); err != nil {
			return r, err
		}
	}
	if err = atomicOrder(dir, q.Order); err != nil {
		return r, err
	}
	if err = WriteJSON(filepath.Join(dir, "meta.json"), next); err != nil {
		return r, err
	}
	if err = WriteJSON(filepath.Join(dir, "history.json"), h); err != nil {
		return r, err
	}
	if !restarting {
		err = appendInbox(dir, fmt.Sprintf("Order amended to revision %d. Read order.md.\n\n%s", rev.Number, rev.Diff))
	} else if contextErr != nil {
		// The amendment is recorded; its attempt fails before launch.
		failAttempt(dir, next, contextErr)
		return r, contextErr
	} else {
		_ = os.Remove(filepath.Join(dir, "stop"))
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		err = amendLaunch(dir, q.Name)
		if err != nil {
			recordUnconfirmed(dir, next, err)
		}
	}
	r.Message = fmt.Sprintf("amended %s: revision %d, attempt %d", q.Name, rev.Number, len(h.Attempts))
	return r, err
}
