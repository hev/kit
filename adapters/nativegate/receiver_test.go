//go:build linux || darwin

package nativegate

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	n "github.com/hev/kit/pkg/nativejob"
)

func TestInspectionHistoricalCompletionAndExactACK(t *testing.T) {
	root := t.TempDir()
	s, err := n.Open(filepath.Join(root, "journal"), n.Enrollment{HostID: "host", ExecutorID: "executor"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, _, _ := sample()
	r := c.Request
	r.StartAuthority = nil
	r.Directory = root
	r.Argv = []string{"/bin/sh", "-c", "printf receiver-effect"}
	r.Environment = []string{"RECEIVER_SECRET=never-serialize-this"}
	r.PayloadDigest = n.RequestDigest(r)
	if err = s.AdvanceFence(n.Fence{OrderUID: r.Identity.OrderUID, ChefID: r.Identity.ChefID, Epoch: r.Identity.Fence, Expires: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	j, err := s.Submit(r)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := Inspect(context.Background(), s, j.ID)
	if err != nil || pending.State != "Pending" || pending.Completion != nil || pending.RequestDigest != r.PayloadDigest {
		t.Fatal(pending, err)
	}
	raw, _ := json.Marshal(pending)
	for _, secret := range []string{"receiver-effect", "never-serialize-this", `"argv":`, `"environment":`} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("inspection exposed executable payload", secret)
		}
	}
	if _, err = Inspect(context.Background(), s, r.IntentKey); !errors.Is(err, n.ErrConflict) {
		t.Fatal("intent alias accepted", err)
	}
	j, err = s.Execute(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Inspect(context.Background(), s, j.ID)
	if err != nil || before.Completion == nil || before.Acknowledged {
		t.Fatal(before, err)
	}
	e := before.Completion
	if err = Acknowledge(context.Background(), s, j.ID, e.Identity, e.EventID, "wrong"); !errors.Is(err, n.ErrConflict) {
		t.Fatal("wrong ACK accepted", err)
	}
	if err = s.AdvanceFence(n.Fence{OrderUID: r.Identity.OrderUID, ChefID: "successor", Epoch: r.Identity.Fence + 1, Expires: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = Acknowledge(context.Background(), s, j.ID, e.Identity, e.EventID, e.ResultDigest); err != nil {
			t.Fatal(err)
		}
	}
	after, err := Inspect(context.Background(), s, j.ID)
	if err != nil || !after.Acknowledged || n.Digest(after.Completion) != n.Digest(before.Completion) {
		t.Fatal(after, err)
	}
	events, err := s.Outbox()
	if err != nil || len(events) != 0 {
		t.Fatal(events, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Inspect(ctx, s, j.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
