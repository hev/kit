package compat

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/hevpaths"
	"os"
	"path/filepath"
	"strings"
)

// steerSummary is the steering receipts line ls shows for a session (RFC
// 0013), read straight from the receipt files so peers does not depend on the
// steer package. It is empty unless something is unacknowledged or halted, so
// a session nobody steered lists exactly as before.
func steerSummary(session string) string {
	home, err := os.UserHomeDir()
	if err != nil || session == "" || strings.ContainsAny(session, `/\`) || session == "." || session == ".." {
		return ""
	}
	dir := filepath.Join(home, ".hev", "steer", "peers", session)
	if name, ok := strings.CutPrefix(session, "d-"); ok {
		orders := hevpaths.Dir(hevpaths.Orders)
		if _, err := os.Stat(filepath.Join(orders, name, "meta.json")); err == nil {
			dir = filepath.Join(orders, name, "steer")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	counts := map[string]int{}
	halting := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "st-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m struct{ State, Level string }
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		counts[m.State]++
		halting = halting || ((m.Level == "halt" || m.Level == "redirect") && m.State != "acked")
	}
	var st struct{ Halted bool }
	if b, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if counts["queued"]+counts["injected"] == 0 && !st.Halted {
		return ""
	}
	s := fmt.Sprintf("steer: %d queued, %d injected, %d acked", counts["queued"], counts["injected"], counts["acked"])
	switch {
	case st.Halted:
		s += ", halted"
	case halting:
		s += ", halting"
	}
	return s
}
