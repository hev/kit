package hostedpeers_test

import (
	"context"
	"fmt"
	"github.com/hev/kit/pkg/kitapi"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewRetrySameLocalIDDoesNotDeadLetter(t *testing.T) {
	c, s, client := fixture(t)
	m := msg()
	m.ID = "123456781234123412341234567890ab"
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	m.Time = m.Time.Add(time.Second) // Record assigns time.Now on every successful local retry.
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	o, _ := testOutbox(c)
	files, _ := os.ReadDir(filepath.Join(o.Dir(), "failed"))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".error") {
			b, _ := os.ReadFile(filepath.Join(o.Dir(), "failed", f.Name()))
			t.Errorf("local retry dead-lettered: %s", b)
		}
	}
	if n := s.Requests("POST /v1/peers/messages"); n != 1 {
		t.Fatalf("unchanged retry uploaded again: %d", n)
	}
}
func TestReviewPermanentPresenceRefusalDoesNotLoop(t *testing.T) {
	c, s, client := fixture(t)
	// Existing edge fixture intentionally receives a permanent host mismatch.
	if e := c.QueuePresence(kitapi.PresenceIn{Host: "wrong-host", ObservedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	_ = c.Flush(context.Background(), client)
	_ = c.Flush(context.Background(), client)
	if n := s.Requests("POST /v1/peers/presence"); n != 1 {
		t.Errorf("permanent refused presence attempted %d times, want 1", n)
	}
}

func TestReviewConflictingLocalRetrySetAsideOnce(t *testing.T) {
	c, s, client := fixture(t)
	m := msg()
	m.ID = "123456781234123412341234567890ab"
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	m.Body = "different"
	for i := 0; i < 2; i++ {
		m.Time = m.Time.Add(time.Second)
		if e := c.QueueMessage(m); e != nil {
			t.Fatal(e)
		}
		if e := c.Flush(context.Background(), client); e != nil {
			t.Fatal(e)
		}
	}
	if n := s.Requests("POST /v1/peers/messages"); n != 2 {
		t.Fatalf("POST count %d, want original and one conflict", n)
	}
	o, _ := testOutbox(c)
	files, _ := os.ReadDir(filepath.Join(o.Dir(), "failed"))
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".error") {
			n++
			b, _ := os.ReadFile(filepath.Join(o.Dir(), "failed", f.Name()))
			if !strings.Contains(string(b), "409") {
				t.Fatal(string(b))
			}
		}
	}
	if n != 1 {
		t.Fatalf("conflict dead letters %d", n)
	}
}

type reviewTransport func(*http.Request) (*http.Response, error)

func (f reviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReviewPermanentPresenceStatusesAndConcurrentReplacement(t *testing.T) {
	for _, status := range []int{400, 403, 413} {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/replace=%v", status, replace), func(t *testing.T) {
				c, _, client := fixture(t)
				in := kitapi.PresenceIn{Host: "host", ObservedAt: time.Now().UTC()}
				if e := c.QueuePresence(in); e != nil {
					t.Fatal(e)
				}
				if e := c.QueueMessage(msg()); e != nil {
					t.Fatal(e)
				}
				base := client.HTTP.Transport
				if base == nil {
					base = http.DefaultTransport
				}
				calls := 0
				client.HTTP.Transport = reviewTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/v1/peers/presence" && r.Method == "POST" {
						calls++
						if calls == 1 {
							if replace {
								in.ObservedAt = in.ObservedAt.Add(time.Second)
								if e := c.QueuePresence(in); e != nil {
									t.Error(e)
								}
							}
							return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"refused"}`)), Request: r}, nil
						}
					}
					return base.RoundTrip(r)
				})
				if e := c.Flush(context.Background(), client); e == nil {
					t.Fatal("expected refusal")
				}
				if len(pending(t, c)) != 0 {
					t.Fatal("messages did not drain")
				}
				if e := c.Flush(context.Background(), client); e != nil {
					t.Fatal(e)
				}
				want := 1
				if replace {
					want = 2
				}
				if calls != want {
					t.Fatalf("presence calls %d want %d", calls, want)
				}
				page, e := client.PeerPresence(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				if replace && (len(page.Machines) != 1 || !page.Machines[0].ObservedAt.Equal(in.ObservedAt)) {
					t.Fatal("new beacon lost", page)
				}
			})
		}
	}
}
