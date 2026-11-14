package hostedpeers_test

import (
	"context"
	"github.com/hev/kit/internal/hostedpeers"
	"github.com/hev/kit/pkg/kitapi"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReceiptFullOutboxAddsNothing(t *testing.T) {
	c, _, _ := fixture(t)
	o, _ := testOutbox(c)
	f, e := os.Create(filepath.Join(o.Dir(), "000-sessions.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate(1 << 30); e != nil {
		t.Fatal(e)
	}
	f.Close()
	m := msg()
	m.ID = hostedpeers.UUID()
	if e = c.QueueMessage(m); e == nil {
		t.Fatal("full queue accepted")
	}
	entries, _ := os.ReadDir(filepath.Join(o.Dir(), "observed"))
	if len(entries) != 0 {
		t.Fatalf("refused enqueue wrote %d receipts", len(entries))
	}
}
func TestReceiptBodyRemovedAndRetryAfterPrune(t *testing.T) {
	c, s, client := fixture(t)
	m := msg()
	m.ID = hostedpeers.UUID()
	m.Body = "receipt-private-body /home/test/private.txt"
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	o, _ := testOutbox(c)
	dir := filepath.Join(o.Dir(), "observed")
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(b), m.Body) || strings.Contains(string(b), "private.txt") {
			t.Fatal("receipt retains body")
		}
	}
	c.Now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	if n := s.Requests("POST /v1/peers/messages"); n != 2 {
		t.Fatalf("POSTs %d", n)
	}
	page, e := client.PeerMessages(context.Background(), nil)
	if e != nil || len(page.Messages) != 1 {
		t.Fatalf("rows %+v %v", page, e)
	}
}
func TestReceiptPruneAgeAndCountWithClock(t *testing.T) {
	c, _, client := fixture(t)
	o, _ := testOutbox(c)
	dir := filepath.Join(o.Dir(), "observed")
	os.MkdirAll(dir, 0700)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.ReceiptLimit = 2
	files := []string{filepath.Join(dir, "expired"), filepath.Join(o.Dir(), "presence-refused-expired"), filepath.Join(dir, "oldest"), filepath.Join(o.Dir(), "presence-refused-newer"), filepath.Join(dir, "newest")}
	for i, p := range files {
		if e := os.WriteFile(p, []byte("marker"), 0600); e != nil {
			t.Fatal(e)
		}
		at := now.Add(-time.Duration(5-i) * time.Hour)
		if i < 2 {
			at = now.Add(-31 * 24 * time.Hour)
		}
		os.Chtimes(p, at, at)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	for i, p := range files {
		_, e := os.Stat(p)
		if i < 3 && !os.IsNotExist(e) {
			t.Errorf("not pruned %s", p)
		}
		if i >= 3 && e != nil {
			t.Errorf("new receipt removed %s", p)
		}
	}
}

func TestReceiptCrashBeforeReceiptThenFlush(t *testing.T) {
	c, s, client := fixture(t)
	m := msg()
	m.ID = hostedpeers.UUID()
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	o, _ := testOutbox(c)
	dir := filepath.Join(o.Dir(), "observed")
	// Simulate death after durable enqueue and before either receipt write.
	if e := os.RemoveAll(dir); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	m.Time = m.Time.Add(time.Hour)
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	if n := s.Requests("POST /v1/peers/messages"); n != 1 {
		t.Fatalf("retry posted %d times", n)
	}
}

func TestReceiptPresenceOnlyCountBound(t *testing.T) {
	c, _, client := fixture(t)
	c.Messages = false
	c.ReceiptLimit = 2
	for i := 0; i < 3; i++ {
		if e := c.QueuePresence(kitapi.PresenceIn{Host: "wrong-host", ObservedAt: time.Now().Add(time.Duration(i) * time.Millisecond)}); e != nil {
			t.Fatal(e)
		}
		if e := c.Flush(context.Background(), client); e == nil {
			t.Fatal("expected permanent refusal")
		}
	}
	o, _ := testOutbox(c)
	entries, _ := os.ReadDir(o.Dir())
	n := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "presence-refused-") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("refusal markers %d want cap 2", n)
	}
}
