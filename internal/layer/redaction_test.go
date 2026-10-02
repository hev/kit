package layer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestStoredSummaryIsScrubbedBeforeReplay(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	secret := "sk-ABCDEFGHIJKLMNOP0123456789"
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "-sessions/schema") {
			io.WriteString(w, `{}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[{"id":"s","summary":"`+secret+`"}]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		writes++
		if strings.Contains(string(b), secret) || !strings.Contains(string(b), "[REDACTED:") {
			t.Errorf("stored summary replay was not scrubbed")
		}
		io.WriteString(w, `{"rows_upserted":1}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "key", "ns", "").WriteSessions([]trace.SessionRow{{ID: "s"}}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}
