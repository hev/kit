package dispatch

import (
	"os"
	"path/filepath"
	"strings"
)

func roundIdentityForAncestors(ancestors map[int]bool) string {
	entries, _ := os.ReadDir(OrchestratorConfigDir())
	for _, f := range entries {
		if !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(f.Name(), ".md")
		path := filepath.Join(OrchestratorDir(name), "harness.json")
		var p pidRecord
		if readJSON(path, &p) == nil && ancestors[p.PID] && processRecordLive(path) {
			return name
		}
	}
	return ""
}

func requestOrchestratorIdentity(q *Request) {
	if workerInternal(q.Action) {
		q.Orchestrator = ""
		return
	}
	if n := roundProcessIdentity(); n != "" {
		q.Orchestrator = n
		return
	}
	if n := os.Getenv("HEV_ORCHESTRATOR"); n != "" {
		q.Orchestrator = n
	}
}

func currentOrchestratorIdentity() string {
	if n := roundProcessIdentity(); n != "" {
		return n
	}
	return os.Getenv("HEV_ORCHESTRATOR")
}
