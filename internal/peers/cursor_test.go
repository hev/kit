package peers

import (
	"context"
	"testing"
)

func TestCursorPeerObservation(t *testing.T) {
	var engine Engine
	for _, tc := range []struct {
		command, agent, state string
	}{
		{"/example/.local/bin/cursor-agent --disable-sigusr1 /example/.local/share/cursor-agent/versions/2026.11.19-f005e9d/index.js -p --force --output-format stream-json prompt", "cursor", "working"},
		{"bash /example/.local/bin/cursor-agent", "cursor", "?"},
		{"node /example/cursor-agent-helper.js -p", "", "-"},
	} {
		agent := engineHarness(tc.command)
		if state := engine.paneState(context.Background(), "", agent, agent != "" && headlessCommand(tc.command)); agent != tc.agent || state != tc.state {
			t.Errorf("%q = %q, %q, want %q, %q", tc.command, agent, state, tc.agent, tc.state)
		}
	}
}
