//go:build !windows

package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDroidWorkerLaunchCapturesSessionAndResult(t *testing.T) {
	const name = "droid-fixture"
	dir, prompts, _ := outcomeFixture(t, name)
	path := filepath.Join(dir, "meta.json")
	var m Meta
	if err := readJSON(path, &m); err != nil {
		t.Fatal(err)
	}
	m.Harness, m.Model, m.Effort = "droid", "sample-model", "high"
	if err := WriteJSON(path, m); err != nil {
		t.Fatal(err)
	}
	fake := `#!/bin/sh
[ "$#" -eq 11 ] || exit 10
[ "$1 $2 $3 $4 $5 $6 $7 $8 $9 ${10}" = "exec --auto high --output-format json --model sample-model --reasoning-effort high --" ] || exit 11
case "${11}" in *"../order.md"*) ;; *) exit 12 ;; esac
[ -z "$HEV_HOOK_INVOCATION" ] || exit 13
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"session_id":"sample-droid","result":"Work completed"}'
printf 'done\n' > ../status.md
`
	if err := os.WriteFile(filepath.Join(filepath.Dir(prompts), "droid"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_HOOK_INVOCATION", "")
	if err := superviseRound(name); err != nil {
		t.Fatal(err)
	}
	result := readResult(t, dir)
	if result.ExitCode != 0 || result.Session != "sample-droid" || exists(filepath.Join(dir, "steer", wiredFile)) {
		t.Fatalf("Droid worker result: %+v", result)
	}
	log, err := os.ReadFile(filepath.Join(dir, "log"))
	if err != nil || !strings.Contains(string(log), `"result":"Work completed"`) {
		t.Fatalf("result absent from run log: %s, %v", log, err)
	}
}
