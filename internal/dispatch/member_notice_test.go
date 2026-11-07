package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func memberNoticeTest(t *testing.T) time.Time {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PEERS_SELF", "build")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	old := noticeNow
	t.Cleanup(func() { noticeNow = old })
	noticeNow = func() time.Time { return now }
	return now
}

func memberMeta(name, sender string) Meta {
	return Meta{Order: Order{Name: name, Reply: "laptop-session"}, Sender: sender, Start: time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)}
}

var replyTemplate = []string{"absent-notifier", "{reply}", "dispatch {name}: {state} {summary}"}

func TestMemberNoticesAreIsolatedPerSender(t *testing.T) {
	memberNoticeTest(t)
	end := Result{ExitCode: 0, End: time.Date(2026, 11, 7, 11, 30, 0, 0, time.UTC)}
	for _, m := range []Meta{memberMeta("alpha", "alex"), memberMeta("beta", "blair"), memberMeta("gamma", "Alex")} {
		if e := Notify(context.Background(), replyTemplate, m, end); e != nil {
			t.Fatal(e)
		}
	}
	a, e := HandleAs(Request{Action: "member-notices"}, "alex")
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Notices) != 1 || !strings.Contains(a.Notices[0].Message, "dispatch alpha: done exit=0; sender=alex") || a.Notices[0].To != "laptop-session" || !strings.HasPrefix(a.Notices[0].ID, "build/d-alpha:dispatch:") {
		t.Fatalf("alex sees %+v", a.Notices)
	}
	b, e := HandleAs(Request{Action: "member-notices"}, "blair")
	if e != nil || len(b.Notices) != 1 || !strings.Contains(b.Notices[0].Message, "beta") {
		t.Fatalf("blair sees %+v %v", b.Notices, e)
	}
	// Blair cannot acknowledge (hide) Alex's notice by naming its ID.
	if _, e = HandleAs(Request{Action: "member-notice-ack", IDs: []string{a.Notices[0].ID}}, "blair"); e != nil {
		t.Fatal(e)
	}
	if again, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(again.Notices) != 1 {
		t.Fatalf("another sender's ack changed alex's store: %+v", again.Notices)
	}
	// Names differing only in case stay separate on case-insensitive disks.
	if upper, _ := HandleAs(Request{Action: "member-notices"}, "Alex"); len(upper.Notices) != 1 || !strings.Contains(upper.Notices[0].Message, "gamma") {
		t.Fatalf("Alex sees %+v", upper.Notices)
	}
	// Request fields cannot select another sender's store.
	forged, _ := HandleAs(Request{Action: "member-notices", Name: "beta", Meta: Meta{Sender: "blair"}}, "alex")
	if len(forged.Notices) != 1 || !strings.Contains(forged.Notices[0].Message, "alpha") {
		t.Fatalf("request selected a store: %+v", forged.Notices)
	}
	if _, e = Handle(Request{Action: "member-notices"}); e == nil {
		t.Fatal("unrestricted RPC served member notices without a sender")
	}
}

func TestMemberNoticeAckAndDuplicateSafeRedelivery(t *testing.T) {
	memberNoticeTest(t)
	m := memberMeta("alpha", "alex")
	end := Result{ExitCode: 1, End: time.Date(2026, 11, 7, 11, 30, 0, 0, time.UTC)}
	for i := 0; i < 2; i++ {
		// A retried notification for the same event keeps one receipt.
		if e := Notify(context.Background(), replyTemplate, m, end); e != nil {
			t.Fatal(e)
		}
	}
	first, _ := HandleAs(Request{Action: "member-notices"}, "alex")
	if len(first.Notices) != 1 || !strings.Contains(first.Notices[0].Message, "failed") {
		t.Fatalf("%+v", first.Notices)
	}
	// A lost ack redelivers the identical notice, so the laptop's inbox can
	// deduplicate it by ID and payload.
	second, _ := HandleAs(Request{Action: "member-notices"}, "alex")
	if len(second.Notices) != 1 || second.Notices[0] != first.Notices[0] {
		t.Fatalf("redelivery changed: %+v vs %+v", second.Notices, first.Notices)
	}
	if _, e := HandleAs(Request{Action: "member-notice-ack", IDs: []string{first.Notices[0].ID, "unknown"}}, "alex"); e != nil {
		t.Fatal(e)
	}
	if after, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(after.Notices) != 0 {
		t.Fatalf("acked notice redelivered: %+v", after.Notices)
	}
	// An acknowledged event retried later is not queued again.
	if e := Notify(context.Background(), replyTemplate, m, end); e != nil {
		t.Fatal(e)
	}
	if after, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(after.Notices) != 0 {
		t.Fatalf("retried event requeued: %+v", after.Notices)
	}
	// A validation result is a separate event with its own notice.
	end.Validation = "pass"
	if e := notifyValidationResult(context.Background(), replyTemplate, m, end); e != nil {
		t.Fatal(e)
	}
	if v, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(v.Notices) != 1 || !strings.Contains(v.Notices[0].Message, "validation=pass") || v.Notices[0].ID == first.Notices[0].ID {
		t.Fatalf("validation notice: %+v", v.Notices)
	}
}

func TestMemberNoticeBounds(t *testing.T) {
	now := memberNoticeTest(t)
	m := memberMeta("alpha", "alex")
	m.Reply = ""
	for i := 0; i < memberNoticeLimit+20; i++ {
		noticeNow = func() time.Time { return now.Add(time.Duration(i) * time.Second) }
		r := Result{ExitCode: i, End: now.Add(time.Duration(i) * time.Second), RepeatReason: strings.Repeat("x", 3*memberNoticeText)}
		// No reply session: the notice is still kept for printing.
		if e := Notify(context.Background(), nil, m, r); e != nil {
			t.Fatal(e)
		}
	}
	files, _ := filepath.Glob(filepath.Join(memberNoticeDir("alex"), "*.json"))
	if len(files) != memberNoticeLimit {
		t.Fatalf("count bound: %d files", len(files))
	}
	got, _ := HandleAs(Request{Action: "member-notices"}, "alex")
	if len(got.Notices) != memberNoticeBatch {
		t.Fatalf("batch bound: %d", len(got.Notices))
	}
	// The oldest notices were dropped first.
	if !strings.Contains(got.Notices[0].Message, "exit=20;") {
		t.Fatalf("oldest kept: %s", got.Notices[0].Message[:40])
	}
	if n := len([]rune(got.Notices[0].Message)); n > memberNoticeText+60 || !strings.Contains(got.Notices[0].Message, "[truncated; hev dispatch show alpha]") {
		t.Fatalf("text bound: %d", n)
	}
	noticeNow = func() time.Time { return now.Add(memberNoticeMaxAge + time.Hour) }
	if aged, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(aged.Notices) != 0 {
		t.Fatalf("age bound: %d", len(aged.Notices))
	}
	if files, _ = filepath.Glob(filepath.Join(memberNoticeDir("alex"), "*.json")); len(files) != 0 {
		t.Fatalf("aged receipts kept: %d", len(files))
	}
}

func TestMemberNoticeExcludesValidatorsAndKeepsOwnerPush(t *testing.T) {
	memberNoticeTest(t)
	end := Result{End: time.Date(2026, 11, 7, 11, 30, 0, 0, time.UTC)}
	missing := []string{filepath.Join(t.TempDir(), "absent"), "{reply}", "{summary}"}
	// Owners and unrestricted senders still push (the absent command fails).
	for _, sender := range []string{"local:owner", ""} {
		if e := Notify(context.Background(), missing, memberMeta("owned", sender), end); e == nil {
			t.Fatalf("push skipped for %q", sender)
		}
	}
	// A validator child attributed to a teammate never queues a notice.
	child := memberMeta("child", "alex")
	child.Parent = "alpha"
	if e := Notify(context.Background(), nil, child, end); e != nil {
		t.Fatal(e)
	}
	forwarded := memberMeta("forwarded", "alex")
	forwarded.Forward = &Host{SSH: "elsewhere"}
	_ = Notify(context.Background(), missing, forwarded, end)
	if got, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(got.Notices) != 0 {
		t.Fatalf("validator or forward notices: %+v", got.Notices)
	}
	if _, e := os.Stat(filepath.Join(Root(), ".member-notices")); !os.IsNotExist(e) {
		t.Fatalf("owner runs created a member store: %v", e)
	}
	// A teammate's {reply} push is skipped: it names a laptop session.
	if e := Notify(context.Background(), missing, memberMeta("alpha", "alex"), end); e != nil {
		t.Fatalf("reply push attempted: %v", e)
	}
	// Owner-side templates without {reply} still run for teammates' runs.
	if e := Notify(context.Background(), []string{filepath.Join(t.TempDir(), "absent"), "{name}"}, memberMeta("beta", "alex"), end); e == nil {
		t.Fatal("owner-side push skipped")
	}
	if got, _ := HandleAs(Request{Action: "member-notices"}, "alex"); len(got.Notices) != 2 {
		t.Fatalf("teammate notices: %+v", got.Notices)
	}
}
