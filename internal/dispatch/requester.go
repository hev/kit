package dispatch

import (
	"os"
	"path/filepath"
)

// Who work is for (requested_by) is inherited: a rework keeps its run's
// meta.json; a validator or review takes its source run's requester; a run
// dispatched from an orchestrator takes the orchestrator's, and one
// dispatched from inside a run's worktree takes that run's. Only an owner
// request may carry it; a teammate's start is always for the teammate
// (HandleAs).

// inheritedRequester is who work started from this process is for: the
// orchestrator round or session running it, else the dispatch run whose
// worker it is (HEV_REQUESTED_BY) or whose worktree it runs in, else this
// account.
func inheritedRequester() string {
	if o := currentOrchestratorIdentity(); o != "" {
		if r := orchestratorRequester(o); r != "" {
			return r
		}
	}
	if r := os.Getenv("HEV_REQUESTED_BY"); r != "" {
		return r
	}
	if cwd, err := os.Getwd(); err == nil {
		if name := attributionName(cwd); name != "" {
			var m Meta
			if readJSON(filepath.Join(Root(), name, "meta.json"), &m) == nil {
				if r := requester(m); r != "" {
					return r
				}
			}
		}
	}
	return LocalSender()
}

// orchestratorRequester is who an orchestrator on this host works for, as
// recorded when it was applied; "" when unknown.
func orchestratorRequester(name string) string {
	st, err := orchestratorState(name)
	if err != nil {
		return ""
	}
	return st.RequestedBy
}
