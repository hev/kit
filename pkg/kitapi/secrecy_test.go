package kitapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Grep every captured error/output/log stream, including an inactive stored key.
func TestCredentialErrorStreams(t *testing.T) {
	for _, status := range []int{400, 401, 403, 302, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			keys := []string{"synthetic-active-credential", "synthetic-stored-credential"}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://127.0.0.1/?key="+keys[0]+keys[1])
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":"refused %s %s"}`, keys[0], keys[1])
			}))
			defer s.Close()
			c := New(s.URL, keys[0], keys[1])
			c.Retries = 0
			_, err := c.Whoami(context.Background())
			if err == nil {
				t.Fatal("missing error")
			}
			var stdout, stderr, logs bytes.Buffer
			fmt.Fprintln(&stdout, err)
			fmt.Fprintln(&stderr, err)
			slog.New(slog.NewTextHandler(&logs, nil)).Warn("upload refused", "err", err)
			for _, stream := range []*bytes.Buffer{&stdout, &stderr, &logs} {
				for _, key := range keys {
					if strings.Contains(stream.String(), key) {
						t.Error("credential in captured stream")
					}
				}
			}
		})
	}
}
