//go:build !windows

package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestratorFakeHarnessAndSenderEndToEnd(t *testing.T) {
	orchHome(t)
	bin := t.TempDir()
	log := filepath.Join(bin, "sent")
	sender := filepath.Join(bin, "fake-sender")
	// Model the native peers entry point, checking identity, stable IDs and the
	// backend isolation used for detached dispatch completion notices.
	script := `#!/bin/sh
[ "$HEV_PEERS_BACKEND" = "" ] || exit 1
[ "$PEERS_FROM" = "" ] || exit 2
[ "$1" = peers ] && [ "$2" = send ] && [ "$3" = owner ] || exit 3
[ "$5" = --from ] && [ "$6" = build-1/o-release-train ] || exit 4
[ "$7" = --id ] && [ "$9" = --inbox-only ] || exit 5
printf '%s\n' "$4" >> "$NOTICE_TEST_LOG"
`
	if err := os.WriteFile(sender, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nprintf 'Goal pending. Reply notes: decision needed.\\n' > checkpoint.md\nprintf 'Decision needed: approve the next step.\\n' > reply.md\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NOTICE_TEST_LOG", log)
	t.Setenv("PEERS_FROM", "stale-harness")
	t.Setenv("HEV_PEERS_BACKEND", "absent-backend")
	old := notificationExecutable
	notificationExecutable = func() (string, error) { return sender, nil }
	t.Cleanup(func() { notificationExecutable = old })
	orchestratorNoticeSender = sendOrchestratorNotice
	raw := strings.Replace(orchestratorExample, "harness: claude", "harness: codex\nhost: build-1", 1)
	raw = strings.Replace(raw, "say, rework", "notify, say, rework", 1)
	s, err := ApplyOrchestrator(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := orchestratorState(s.Name)
	d := DefaultOrchestratorDriver()
	d.Gather = noticeDriver(t, nil).Gather
	if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
	if err := OrchestratorControl(s.Name, "stop", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"started/applied", "Decision needed:", "stopped"} {
		if strings.Count(string(b), message) != 1 {
			t.Fatal(string(b))
		}
	}
	rows, err := ListOrchestrators()
	if err != nil || rows[0].LastNotice.Status != "delivered" || !strings.Contains(rows[0].LastNotice.Message, "stopped") {
		t.Fatal(rows, err)
	}
}
