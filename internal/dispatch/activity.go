package dispatch

import (
	"os"
	"path/filepath"
)

// expediteUntil marks an orchestrator row as an Expediter in dispatch ls and
// hev expedite status.
const expediteUntil = "all followed runs merged or abandoned"

// Activity is this node's own run, Expediter and orchestrator count, read from the same
// state hev dispatch ls reads. It contacts no other host and scrapes no
// processes beyond the liveness checks ls already makes.
type Activity struct {
	HeadlessRuns int
	Expediters   int
	// Orchestrators is LiveOrchestrators: live orchestrator supervisors,
	// Expediters included, as dispatch hosts reports them.
	Orchestrators int
}

// HeadlessRunRow reports whether a dispatch ls row is a run executing on
// this node. Forwarded rows execute (and are counted) on their target host.
func HeadlessRunRow(r Row) bool {
	return r.Kind == "run" && r.Forward == nil && !r.Imported && (r.State == "running" || r.State == "halting")
}

// ExpediterRow reports whether a dispatch ls row is an Expediter that is
// still following runs, as hev expedite status lists it: stopped, paused,
// complete and handed-off Expediters are not counted.
func ExpediterRow(r Row) bool {
	if r.Kind != "orch" || r.Until != expediteUntil {
		return false
	}
	switch r.State {
	case "stopped", "paused", "complete", "handed-off":
		return false
	}
	return true
}

// LocalActivity counts what HeadlessRunRow and ExpediterRow would count in
// this node's hev dispatch ls, without the forwarded-host RPCs ls makes, and
// the live orchestrators LiveOrchestrators counts.
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
	var r Response
	if e = addOrchestratorRows(&r); e != nil {
		return a, e
	}
	for _, row := range r.Rows {
		if ExpediterRow(row) {
			a.Expediters++
		}
	}
	a.Orchestrators, e = LiveOrchestrators()
	return a, e
}
