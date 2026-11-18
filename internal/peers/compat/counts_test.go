package compat

import "testing"

func TestCountAgentsSplitsWorkingFromIdle(t *testing.T) {
	rows := []*Row{
		{ID: "%1", Agent: "claude", Role: "boh"},
		{ID: "%2", Agent: "codex", Role: "foh"},
		{ID: "%3", Agent: "claude", Role: "boh"},
		{ID: "%4", Agent: "pi", Role: "boh"},
		{ID: "%5", Agent: "", Role: "boh"},                       // a shell
		{ID: "%6", Agent: "claude", Role: "offline"},             // not a peer
		{ID: "%7", Agent: "claude", Role: "boh", Headless: true}, // a dispatch worker
		{ID: "managed:x", Agent: "claude", Role: "boh", Managed: NewObj()},
		{ID: "%9", Agent: "claude", Role: "boh"}, // pane vanished mid-sample
	}
	states := map[string]string{"%1": "working", "%2": "working", "%3": "prompt", "%4": "?", "%7": "working", "managed:x": "working"}
	got := countAgents(rows, func(r *Row) string {
		if r.ID == "%9" {
			panic(exitCode(1))
		}
		return states[r.ID]
	})
	if got != (AgentCounts{Working: 2, Idle: 3}) {
		t.Fatalf("%+v", got)
	}
}
