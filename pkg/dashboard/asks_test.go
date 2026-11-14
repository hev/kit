package dashboard_test

import (
	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/tracefixture"
	"github.com/hev/kit/pkg/asks"
	"github.com/hev/kit/pkg/dashboard"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHostedAsksInlineSessionsNeverReadLocalDispatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	info, e := tracefixture.Write(home, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	id := "11111111-0000-4000-8000-000000000003"
	r := &serve.MemoryReader{Sessions: []asks.SessionRow{{ID: id, SessionID: id, TraceID: info.Trace, Summary: "team session"}}}
	h, e := dashboard.NewHostedAsks(r, asks.HostedOptions{BasePath: "/asks", TracesPath: "/traces", Principal: func(*http.Request) (asks.Principal, error) { return asks.Principal{ID: "Ada"}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/asks/api/session/"+id, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "team session") || strings.Contains(w.Body.String(), `"trace_context"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
