package dispatch

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BusyWork names what runs on this host that an automatic update must not
// race: every dispatch run with a live session, supervisor or admission
// reservation, and every orchestrator in the middle of a round (an
// orchestrator between rounds is idle; its supervisor only waits). An
// orchestrator is named "orch-NAME".
func BusyWork() ([]string, error) {
	if _, err := os.Stat(Root()); os.IsNotExist(err) {
		return nil, nil
	}
	live, err := liveNames()
	if err != nil {
		return nil, err
	}
	if err := addReservations(live); err != nil {
		return nil, err
	}
	var out []string
	for n := range live {
		if strings.HasPrefix(n, "orch-") {
			continue
		}
		out = append(out, n)
	}
	dirs, err := os.ReadDir(Root())
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "orch-") {
			continue
		}
		dir := filepath.Join(Root(), d.Name())
		var st OrchestratorState
		if readJSON(filepath.Join(dir, "state.json"), &st) != nil || st.State != "running" {
			continue
		}
		if live[d.Name()] || supervisorLive(dir) {
			out = append(out, d.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
