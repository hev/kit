package expedite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

type fakeDriver struct {
	events  []string
	row     dispatch.Row
	pr      PR
	err     error
	reports []string
}

func (f *fakeDriver) Inbox(context.Context) error {
	f.events = append(f.events, "inbox-and-chats")
	return nil
}
func (f *fakeDriver) Inspect(context.Context, Run) (dispatch.Row, PR, error) {
	f.events = append(f.events, "inspect-gh")
	return f.row, f.pr, f.err
}
func (f *fakeDriver) Amend(_ context.Context, _ Run, s string) error {
	f.events = append(f.events, "amend:"+s)
	return nil
}
func (f *fakeDriver) Merge(context.Context, Run, string, string) error {
	f.events = append(f.events, "merge")
	return nil
}
func (f *fakeDriver) Report(_ context.Context, s string) error {
	f.events = append(f.events, "report")
	f.reports = append(f.reports, s)
	return nil
}
func green() PR {
	var p PR
	_ = json.Unmarshal([]byte(`{"headRefOid":"head","state":"OPEN","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}`), &p)
	return p
}
func TestCollectExactReplySnapshot(t *testing.T) {
	rows := []dispatch.Row{}
	for _, v := range []struct{ name, reply, state, until string }{{"a", "desk/foh", "done", ""}, {"b", "other/foh", "done", ""}, {"c", "desk/foh", "merged", ""}, {"d", "desk/foh", "abandoned", ""}, {"e", "desk/foh", "running", "all followed runs merged or abandoned"}, {"f", "desk/foh-extra", "done", ""}} {
		rows = append(rows, dispatch.Row{Meta: dispatch.Meta{Order: dispatch.Order{Name: v.name, Reply: v.reply, Until: v.until, Repo: "sample"}}, State: v.state})
	}
	inv := map[string]dispatch.Response{"worker": {Rows: rows}}
	runs, err := Collect(inv, "desk/foh")
	if err != nil || len(runs) != 1 || runs[0].Name != "a" {
		t.Fatalf("%+v %v", runs, err)
	}
	inv["offline"] = dispatch.Response{Error: "offline"}
	if _, err = Collect(inv, "desk/foh"); err == nil {
		t.Fatal("partial snapshot accepted")
	}
}
func TestRoundPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, authority, change, want string
		merged, done                  bool
	}{
		{"review never merges", "review", "", "requires your merge", false, false},
		{"merge verified green", "merge-on-green", "", "merged verified", true, false},
		{"no checks", "merge-on-green", "empty", "waiting for green", false, false},
		{"pending", "merge-on-green", "pending", "waiting for green", false, false},
		{"failed", "merge-on-green", "failed", "waiting for green", false, false},
		{"stale review", "merge-on-green", "stale", "awaiting review", false, false},
		{"no verification", "merge-on-green", "unverified", "awaiting review", false, false},
		{"rework", "review", "rework", "via amend", false, false},
		{"already merged", "review", "merged", "closed", false, true},
		{"explicit abandoned", "review", "abandoned", "closed", false, true},
		{"inaccessible", "merge-on-green", "error", "[needs you]", false, false},
		{"closed unmerged", "merge-on-green", "closed", "not open", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDriver{row: dispatch.Row{PR: "https://example.com/pr/1", State: "done"}, pr: green()}
			ev := Evidence{Head: "head", Reviewed: true, Verified: true}
			switch tc.change {
			case "empty":
				f.pr.Checks = nil
			case "pending":
				f.pr.Checks[0].Status = "IN_PROGRESS"
			case "failed":
				f.pr.Checks[0].Conclusion = "FAILURE"
			case "stale":
				ev.Head = "old"
			case "unverified":
				ev.Verified = false
			case "rework":
				ev.Amend = "complete amended order"
			case "merged":
				f.pr.State = "MERGED"
			case "abandoned":
				ev.Abandoned = true
			case "error":
				f.err = errors.New("gh unavailable")
			case "closed":
				f.pr.State = "CLOSED"
			}
			p := Plan{Authority: tc.authority, Runs: []Run{{Host: "worker", Name: "task"}}}
			done, err := Round(context.Background(), p, map[string]Evidence{"worker/task": ev}, f)
			if err != nil || done != tc.done {
				t.Fatalf("done=%v err=%v", done, err)
			}
			if f.events[0] != "inbox-and-chats" || (tc.change != "abandoned" && (len(f.events) < 3 || f.events[1] != "inspect-gh")) {
				t.Fatalf("ordering %v", f.events)
			}
			merged := false
			for _, event := range f.events {
				merged = merged || event == "merge"
			}
			if merged != tc.merged {
				t.Fatalf("merge calls %v", f.events)
			}
			if !strings.Contains(f.reports[0], tc.want) {
				t.Fatal(f.reports)
			}
			if tc.change == "rework" && !strings.Contains(strings.Join(f.events, " "), "amend:complete amended order") {
				t.Fatal(f.events)
			}
		})
	}
}
func TestUnknownAuthorityFailsBeforeWork(t *testing.T) {
	f := &fakeDriver{}
	if _, err := Round(context.Background(), Plan{Authority: "admin"}, nil, f); err == nil || len(f.events) > 0 {
		t.Fatal("invalid authority ran")
	}
}
