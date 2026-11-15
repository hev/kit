package kitcapture_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
)

func TestDrainerCredentialErrorStreams(t *testing.T) {
	for _, status := range []int{400, 401, 403, 302, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			credential := "synthetic-drainer-credential"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1/?key="+credential)
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":"%s"}`, credential)
			}))
			defer srv.Close()
			a := newArchive(t)
			capture(t, a, "s1", "queued capture")
			c := kitapi.New(srv.URL, credential)
			c.Retries = 0
			var logs syncBuffer
			ctx, cancel := context.WithCancel(context.Background())
			d := &kitcapture.Drainer{Outbox: a.Outbox, Client: c, Logger: slog.New(slog.NewTextHandler(&logs, nil)), Interval: time.Hour, MaxBackoff: time.Hour}
			done := make(chan struct{})
			go func() { d.Run(ctx); close(done) }()
			defer func() { cancel(); <-done }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				res, err := d.Last()
				if err != nil || res.Failed > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("drainer did not process response")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			<-done
			// Permanent 4xx responses move to failed without a daemon log;
			// transient/auth/redirect failures log a sanitized diagnostic.
			if strings.Contains(logs.String(), credential) {
				t.Fatal("credential in daemon log")
			}
		})
	}
}
