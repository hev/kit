package layer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestStoredSummaryIsNotReplayedBySourceIngestion(t *testing.T) {
	t.Setenv("HEV_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	secret := "sk-ABCDEFGHIJKLMNOP0123456789"
	writes := 0
	stored := map[string]any{"id": "s", "summary": secret}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "-sessions/schema") {
			io.WriteString(w, `{}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/query") {
			json.NewEncoder(w).Encode(map[string]any{"rows": []any{stored}})
			return
		}
		b, _ := io.ReadAll(r.Body)
		writes++
		if strings.Contains(string(b), secret) || strings.Contains(string(b), `"summary"`) {
			t.Errorf("stored summary was replayed")
		}
		var body struct {
			Rows []map[string]any `json:"patch_rows"`
		}
		json.Unmarshal(b, &body)
		for k, v := range body.Rows[0] {
			stored[k] = v
		}
		io.WriteString(w, `{"rows_affected":1}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "key", "ns", "").WriteSessions([]trace.SessionRow{{ID: "s"}}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
}
