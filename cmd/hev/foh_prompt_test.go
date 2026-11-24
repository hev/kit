package main

import (
	"strings"
	"testing"

	"github.com/hev/kit/internal/foh"
)

func TestBasePromptHevCLIReferenceMatchesRootCmd(t *testing.T) {
	if _, rest, _ := rootCmd.Find([]string{"traces"}); len(rest) != 0 {
		addDirectSurface()
	}
	if !strings.Contains(foh.BasePrompt, "hev CLI quick reference") {
		t.Fatal("BasePrompt missing baked hev CLI quick reference")
	}
	if !strings.Contains(foh.BasePrompt, `running "hev --help"`) {
		t.Fatal("BasePrompt should tell the model not to waste turns on hev --help")
	}

	// Every top-level command and key subcommand advertised in BasePrompt must
	// resolve on rootCmd so the baked prompt cannot drift from the binary.
	paths := [][]string{
		{"dispatch"},
		{"dispatch", "ls"},
		{"dispatch", "show"},
		{"dispatch", "hosts"},
		{"dispatch", "logs"},
		{"dispatch", "stop"},
		{"dispatch", "retry"},
		{"dispatch", "rework"},
		{"dispatch", "amend"},
		{"dispatch", "close"},
		{"dispatch", "adopt"},
		{"dispatch", "say"},
		{"dispatch", "sweep"},
		{"dispatch", "clean"},
		{"dispatch", "compact"},
		{"dispatch", "gc"},
		{"dispatch", "notices"},
		{"dispatch", "validate"},
		{"steer"},
		{"steer", "ls"},
		{"steer", "show"},
		{"steer", "ack"},
		{"orchestrate"},
		{"orchestrate", "ls"},
		{"orchestrate", "show"},
		{"orchestrate", "start"},
		{"orchestrate", "stop"},
		{"orchestrate", "pause"},
		{"orchestrate", "resume"},
		{"orchestrate", "wake"},
		{"orchestrate", "say"},
		{"orchestrate", "logs"},
		{"orchestrate", "adopt"},
		{"orchestrate", "retire"},
		{"orchestrate", "follow"},
		{"orchestrate", "swarm"},
		{"orchestrate", "apply"},
		{"orchestrate", "render"},
		{"orchestrate", "reconcile"},
		{"orchestrate", "notices"},
		{"peers"},
		{"board"},
		{"traces"},
		{"traces", "list"},
		{"traces", "search"},
		{"traces", "show"},
		{"traces", "stats"},
		{"traces", "images"},
		{"cluster"},
		{"cluster", "ls"},
		{"cluster", "status"},
		{"cluster", "pending"},
		{"cluster", "invite"},
		{"cluster", "accept"},
		{"cluster", "join"},
		{"cluster", "leave"},
		{"cluster", "allow"},
		{"cluster", "deny"},
		{"cluster", "hygiene"},
		{"review"},
		{"review", "ls"},
		{"review", "show"},
		{"review", "run"},
		{"review", "put"},
		{"review", "add"},
		{"review", "resolve"},
		{"review", "search"},
		{"evidence"},
		{"evidence", "ls"},
		{"evidence", "show"},
		{"evidence", "get"},
		{"evidence", "create"},
		{"evidence", "add"},
		{"evidence", "claim"},
		{"evidence", "seal"},
		{"evidence", "put"},
		{"evidence", "search"},
		{"cron"},
		{"cron", "ls"},
		{"cron", "show"},
		{"cron", "run"},
		{"cron", "pause"},
		{"cron", "resume"},
		{"service"},
		{"service", "ls"},
		{"service", "show"},
		{"service", "logs"},
		{"service", "start"},
		{"service", "stop"},
		{"service", "restart"},
		{"preview"},
		{"preview", "ls"},
		{"preview", "show"},
		{"preview", "sync"},
		{"preview", "serve"},
		{"secrets"},
		{"secrets", "run"},
		{"secrets", "read"},
		{"secrets", "env"},
		{"secrets", "ping"},
		{"secrets", "status"},
		{"update"},
		{"update", "hold"},
		{"update", "release"},
		{"workspace"},
		{"workspace", "ls"},
		{"workspace", "current"},
		{"workspace", "show"},
		{"workspace", "use"},
		{"workspace", "add"},
		{"workspace", "archive"},
		{"up"},
		{"down"},
		{"s"},
		{"stop"},
		{"config", "show"},
		{"config", "path"},
		{"admission"},
		{"context"},
		{"watch"},
	}
	for _, p := range paths {
		cmd, rest, err := rootCmd.Find(p)
		if err != nil || cmd == nil || len(rest) != 0 {
			t.Errorf("baked CLI command %q did not resolve on rootCmd (cmd=%v rest=%v err=%v)", strings.Join(p, " "), cmd, rest, err)
		}
	}

	// Verify key flags advertised in BasePrompt exist on their commands.
	for _, tc := range []struct {
		path []string
		flag string
	}{
		{[]string{"dispatch"}, "host"},
		{[]string{"dispatch"}, "supersedes"},
		{[]string{"dispatch"}, "trace"},
		{[]string{"steer"}, "host"},
		{[]string{"steer"}, "level"},
		{[]string{"secrets", "status"}, "no-quota"},
		{[]string{"update"}, "channel"},
		{[]string{"update"}, "check"},
		{[]string{"update"}, "to"},
		{[]string{"update"}, "rollback"},
	} {
		cmd, _, err := rootCmd.Find(tc.path)
		if err != nil || cmd == nil {
			t.Fatalf("command %v not found: %v", tc.path, err)
		}
		if cmd.Flags().Lookup(tc.flag) == nil && cmd.PersistentFlags().Lookup(tc.flag) == nil {
			t.Errorf("command %q missing advertised flag --%s", strings.Join(tc.path, " "), tc.flag)
		}
	}
}
