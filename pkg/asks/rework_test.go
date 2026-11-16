package asks_test

import (
	"strings"
	"testing"
)

func TestHostedReviewAsksShell(t *testing.T) {
	h := hosted(t, fixture("fixture", "11111111111111111111111111111111"), ada)
	w := get(h, "/team/asks/")
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src-attr 'none'") || !strings.Contains(csp, "sha256-") {
		t.Errorf("document CSP=%q", csp)
	}
	if strings.Contains(w.Body.String(), `href="${S.repo_url}/tree/`) {
		t.Error("stored URL interpolated into markup")
	}

	// Every remaining innerHTML assignment must be static (empty clearing or SVG defs).
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.Contains(line, "innerHTML") && !strings.HasPrefix(strings.TrimSpace(line), "//") && !strings.Contains(line, "innerHTML=''") && !strings.Contains(line, "defs.innerHTML=`<pattern") {
			t.Errorf("unexpected HTML sink: %s", line)
		}
		if strings.Contains(line, "html:") {
			t.Errorf("markup attribute sink: %s", line)
		}
	}
	for _, p := range []string{"/team/asks//api/asks", "/team/asks/%2e%2e/other", "/team/asks/api/../other"} {
		w := get(h, p)
		if loc := w.Header().Get("Location"); loc != "" && !strings.HasPrefix(loc, "/team/asks/") {
			t.Errorf("%s escaped mount: %d %s", p, w.Code, loc)
		}
	}
}
