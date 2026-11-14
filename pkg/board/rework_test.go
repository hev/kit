package board_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHostedReviewBoundary(t *testing.T) {
	st := store(t)
	seed(t, st, "<script>alert(1)</script>")
	h, c := hosted(t, st, false, allow)
	for _, p := range []string{"/team/board//api/threads", "/team/board/../other/api/threads", "/team/board/%2e%2e/other/api/threads", "/team/board/c/1/..%2fsecret", "/team/board/blob/..%2fsecret"} {
		w := request(h, "GET", p, "", "")
		if loc := w.Header().Get("Location"); loc != "" && !strings.HasPrefix(loc, "/team/board/") {
			t.Errorf("%s escaped mount: %d %s", p, w.Code, loc)
		}
	}
	_, _ = c.Save(1, map[string][]byte{"index.html": []byte("<script>parent.document.body.innerHTML='bad'</script>"), "active.svg": []byte(`<svg onload="alert(1)"/>`)})
	for _, p := range []string{"/c/1/", "/c/1/active.svg", "/c/_/canvas.css", "/c/_/canvas.js", "/static/avatars/agent.svg"} {
		w := request(h, "GET", "/team/board"+p, "", "")
		if !strings.Contains(w.Header().Get("Cache-Control"), "private") {
			t.Errorf("canvas cache: %v", w.Header())
		}
		if strings.HasPrefix(p, "/c/1/") && !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Error("canvas lacks sandbox")
		}
	}
	data := []byte{'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0, 0, 0}
	w := request(h, "POST", "/team/board/api/blobs", "image/gif", string(data))
	var m struct {
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(w.Body.Bytes(), &m) != nil || m.SHA256 == "" {
		t.Fatal(w.Body.String())
	}
	w = request(h, "GET", "/team/board/blob/"+m.SHA256, "", "")
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "private") || strings.Contains(cc, "public") {
		t.Errorf("authenticated blob cache=%q", cc)
	}
	w = request(h, "GET", "/team/board/", "", "")
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src-attr 'none'") || !strings.Contains(csp, "sha256-") {
		t.Errorf("document CSP=%q", csp)
	}
}
