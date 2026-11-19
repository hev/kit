package dispatch

import (
	"os"
	"path/filepath"
)

// Activity is this node's own run and orchestrator count, read from the same
// state hev dispatch ls reads. It contacts no other host and scrapes no
// processes beyond the liveness checks ls already makes.
type Activity struct {
	HeadlessRuns int
	// Orchestrators is LiveOrchestrators: live orchestrator supervisors,
	// Expediters included, as dispatch hosts reports them.
	Orchestrators int
}

// HeadlessRunRow reports whether a dispatch ls row is a run executing on
// this node. Forwarded rows execute (and are counted) on their target host.
func HeadlessRunRow(r Row) bool {
	return r.Kind == "run" && r.Forward == nil && !r.Imported && (r.State == "running" || r.State == "halting")
}

// LocalActivity counts what HeadlessRunRow would count in this node's hev
// dispatch ls, without the forwarded-host RPCs ls makes, and the live
// orchestrators LiveOrchestrators counts.
func LocalActivity() (Activity, error) {
	var a Activity
	live, e := liveNames()
	if e != nil {
		return a, e
	}
	if e = addReservations(live); e != nil && !os.IsNotExist(e) {
		return a, e
	}
	for name := range live {
		if !ValidName(name) {
			continue
		}
		var m Meta
		if readJSON(filepath.Join(Root(), name, "meta.json"), &m) != nil {
			continue
		}
		if HeadlessRunRow(Row{Kind: "run", Meta: m, State: "running"}) {
			a.HeadlessRuns++
		}
	}
	a.Orchestrators, e = LiveOrchestrators()
	return a, e
}
