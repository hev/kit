package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hev/kit/pkg/archive"
	"github.com/hev/kit/pkg/dashboard"
)

func TestReaderHandlerOverFileArchive(t *testing.T) {
	st, err := archive.OpenFiles(t.TempDir(), "team")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := st.WriteSessions([]archive.SessionRow{{ID: "s1", SessionID: "s1", Host: "h", Harness: "claude", Start: now, End: now, Summary: "fixture session"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]archive.Row{{ID: "s1:c", SessionID: "s1", Text: "the quick brown fox", Host: "h", Harness: "claude"}}); err != nil {
		t.Fatal(err)
	}
	h := http.StripPrefix("/v1/traces", dashboard.NewReaderHandler(st, dashboard.ReaderOptions{}))
	for _, path := range []string{"/v1/traces/api/sessions", "/v1/traces/api/session/s1", "/v1/traces/api/stats", "/v1/traces/api/values?facet=harness", "/v1/traces/api/search?q=quick+brown&mode=phrase"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !json.Valid(rec.Body.Bytes()) {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
}
