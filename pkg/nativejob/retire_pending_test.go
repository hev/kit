//go:build linux || darwin

package nativejob

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func retirementFixture(t *testing.T) (*Store, Job, Identity, PendingRetirement) {
	t.Helper()
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	j := mustSubmit(t, s, testRequest(root))
	current := j.Request.Identity
	current.Fence = 5
	if e := s.AdvanceFence(Fence{OrderUID: current.OrderUID, ChefID: current.ChefID, Epoch: 5, Expires: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	return s, j, current, PendingRetirement{ID: "retire-1", Evidence: "immutable-never-started-proof", Inspector: "authenticated-owner"}
}
func TestRetirePendingOriginalEventReentryACK(t *testing.T) {
	s, j, current, in := retirementFixture(t)
	before := encode(j.Request)
	retired, e := s.RetirePending(j.ID, current, in)
	if e != nil {
		t.Fatal(e)
	}
	if retired.State != "Completed" || retired.Result.ExitCode != 125 || retired.Result.Artifacts == nil || len(retired.Result.Artifacts) != 0 || retired.PendingRetirement == nil || retired.Owner != nil || retired.Birth != nil || retired.StartReceipt != nil || len(retired.Inspections) != 0 || encode(retired.Request) != before {
		t.Fatal(retired)
	}
	events, e := s.Outbox()
	if e != nil || len(events) != 1 || encode(events[0].Identity) != encode(j.Request.Identity) || events[0].ResultDigest != retired.ResultDigest {
		t.Fatal(events, e)
	}
	if _, e = s.Execute(j.ID); e == nil {
		t.Fatal("retired job executed")
	}
	if _, e = s.Reconcile(j.ID, current, Inspection{ID: "inspect", Evidence: "x", Inspector: "x"}); e == nil {
		t.Fatal("retirement relaxed reconcile")
	}
	if e = s.Ack(j.ID, j.Request.Identity, retired.EventID, retired.ResultDigest); e != nil {
		t.Fatal(e)
	}
	again, e := s.RetirePending(j.ID, current, in)
	if e != nil || !again.Acknowledged || again.EventID != retired.EventID || encode(again.Request) != before {
		t.Fatal(again, e)
	}
	in.Evidence = "changed"
	if _, e = s.RetirePending(j.ID, current, in); e == nil {
		t.Fatal("changed proof accepted")
	}
	events, e = s.Outbox()
	if e != nil || len(events) != 0 {
		t.Fatal(events, e)
	}
}
func TestRetirePendingRefusesNonPristineAndForeign(t *testing.T) {
	mutations := map[string]func(*Job){"uncertain": func(j *Job) { j.State = "Uncertain" }, "running": func(j *Job) { j.State = "Running" }, "owner": func(j *Job) { j.Owner = &Birth{} }, "birth": func(j *Job) { j.Birth = &Birth{} }, "start": func(j *Job) { j.StartReceipt = &StartReceipt{} }, "result": func(j *Job) { j.Result = &Result{ExitCode: 125} }, "resultdigest": func(j *Job) { j.ResultDigest = "x" }, "event": func(j *Job) { j.EventID = "x" }, "ack": func(j *Job) { j.Acknowledged = true }, "inspection": func(j *Job) { j.Inspections = []Inspection{{ID: "old"}} }, "forgedterminal": func(j *Job) { j.State = "Completed"; j.Result = &Result{ExitCode: 125, Artifacts: []Artifact{}} }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s, j, current, in := retirementFixture(t)
			mutate(&j)
			if _, e := s.db.Exec(`UPDATE jobs SET body=? WHERE id=?`, encode(j), j.ID); e != nil {
				t.Fatal(e)
			}
			before := encode(j)
			if _, e := s.RetirePending(j.ID, current, in); e == nil {
				t.Fatal("accepted")
			}
			after, e := s.Query(j.ID)
			if e != nil || encode(after) != before {
				t.Fatal("mutated", e)
			}
			out, e := s.Outbox()
			if e != nil || len(out) != 0 {
				t.Fatal(out, e)
			}
		})
	}
	for _, name := range []string{"same-fence", "unissued", "other-chef", "other-node", "empty-proof", "expired"} {
		t.Run(name, func(t *testing.T) {
			s, j, current, in := retirementFixture(t)
			switch name {
			case "same-fence":
				current.Fence = j.Request.Identity.Fence
			case "unissued":
				current.Fence = 6
			case "other-chef":
				current.ChefID = "foreign"
			case "other-node":
				current.NodeID = "foreign"
			case "empty-proof":
				in.Evidence = ""
			case "expired":
				s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
			}
			if _, e := s.RetirePending(j.ID, current, in); e == nil {
				t.Fatal("accepted")
			}
			after, _ := s.Query(j.ID)
			if encode(after) != encode(j) {
				t.Fatal("mutated")
			}
		})
	}
}
func TestRetirePendingAtomicBoundariesExpiryAndConcurrentReentry(t *testing.T) {
	for _, boundary := range []string{"pending-retirement-staged", "pending-retirement-committed", "expire-before-commit"} {
		t.Run(boundary, func(t *testing.T) {
			s, j, current, in := retirementFixture(t)
			injected := errors.New("injected")
			s.boundary = func(at string) error {
				if at == "pending-retirement-staged" && boundary == "expire-before-commit" {
					s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
				}
				if at == boundary {
					return injected
				}
				return nil
			}
			if _, e := s.RetirePending(j.ID, current, in); e == nil {
				t.Fatal("expected error")
			}
			s.boundary = nil
			s.now = time.Now
			after, _ := s.Query(j.ID)
			out, _ := s.Outbox()
			if boundary == "pending-retirement-committed" {
				if after.State != "Completed" || len(out) != 1 {
					t.Fatal(after, out)
				}
				if _, e := s.RetirePending(j.ID, current, in); e != nil {
					t.Fatal(e)
				}
			} else if encode(after) != encode(j) || len(out) != 0 {
				t.Fatal("partial commit", after, out)
			}
		})
	}
	s, j, current, in := retirementFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for k := 0; k < 8; k++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.RetirePending(j.ID, current, in); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	out, e := s.Outbox()
	if e != nil || len(out) != 1 {
		t.Fatal(out, e)
	}
}
