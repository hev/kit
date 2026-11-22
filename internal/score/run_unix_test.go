package score_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/score"
)

func TestTimeoutOutputBoundsEnvIsolationAndSecretRedaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell child-process group and signal semantics")
	}
	home := isolateHome(t)
	repo, baseSHA := initGitRepo(t, map[string]string{"main.go": "package main\n"})

	t.Run("subprocess timeout kills process group promptly", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "slow-scorer")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		// Spawn a background sleep holding stdout open to prove process-group kill
		// prevents cmd.Wait from hanging on inherited pipes.
		cmdPath := writeTestScript(t, dir, "slow",
			`sleep 30 &
wait`, "")
		specPath := filepath.Join(dir, "scorer.md")
		spec := "---\nkind: metric\ncommand: " + filepath.Base(cmdPath) + "\ndirection: max\ntimeout: 150ms\n---\n"
		if err := os.WriteFile(specPath, []byte(spec), 0600); err != nil {
			t.Fatal(err)
		}
		pinStore := filepath.Join(t.TempDir(), "pins.toml")
		if _, err := score.Pin(pinStore, specPath); err != nil {
			t.Fatal(err)
		}

		start := time.Now()
		_, err := score.RunCommit(context.Background(), score.RunOptions{
			ScorerPath:   specPath,
			PinStorePath: pinStore,
			RepoPath:     repo,
			Commit:       baseSHA,
		})
		elapsed := time.Since(start)
		if !errors.Is(err, score.ErrTimeout) {
			t.Fatalf("expected ErrTimeout, got %v", err)
		}
		if elapsed > 5*time.Second {
			t.Fatalf("timeout took %s; child process group was not killed promptly", elapsed)
		}
	})

	t.Run("excessive stdout output is bounded and refused", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "noisy-scorer")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		// Write ~1.5 MiB (> MaxStdoutBytes = 1 MiB).
		cmdPath := writeTestScript(t, dir, "noisy",
			`dd if=/dev/zero bs=1024 count=1536 2>/dev/null | tr '\0' '9'`, "")
		specPath := filepath.Join(dir, "scorer.md")
		spec := "---\nkind: metric\ncommand: " + filepath.Base(cmdPath) + "\ndirection: max\ntimeout: 10s\n---\n"
		if err := os.WriteFile(specPath, []byte(spec), 0600); err != nil {
			t.Fatal(err)
		}
		pinStore := filepath.Join(t.TempDir(), "pins.toml")
		if _, err := score.Pin(pinStore, specPath); err != nil {
			t.Fatal(err)
		}

		_, err := score.RunCommit(context.Background(), score.RunOptions{
			ScorerPath:   specPath,
			PinStorePath: pinStore,
			RepoPath:     repo,
			Commit:       baseSHA,
		})
		if err == nil || !strings.Contains(err.Error(), "exceeded") {
			t.Fatalf("expected stdout cap error, got %v", err)
		}
	})

	t.Run("minimal env strips host credentials and redacts secrets and local paths in errors", func(t *testing.T) {
		hostEnvSecret := "ultraSecretHostEnvToken_99887766"
		openAISecret := "sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"
		t.Setenv("MY_CUSTOM_API_TOKEN", hostEnvSecret)
		t.Setenv("OPENAI_API_KEY", openAISecret)
		t.Setenv("GH_TOKEN", "ghp_1234567890abcdefghijklmnopqrstuvwxyzABCD")

		// 1. Verify a passing scorer does NOT inherit host secrets and runs with isolated scratch HOME.
		passDir := filepath.Join(t.TempDir(), "env-check")
		if err := os.MkdirAll(passDir, 0700); err != nil {
			t.Fatal(err)
		}
		passCmd := writeTestScript(t, passDir, "checkenv",
			`if [ -n "${MY_CUSTOM_API_TOKEN:-}" ] || [ -n "${OPENAI_API_KEY:-}" ] || [ -n "${GH_TOKEN:-}" ]; then
  echo "host secret leaked into scorer env" >&2
  exit 1
fi
if [ "$HOME" = "`+home+`" ]; then
  echo "scorer inherited real HOME" >&2
  exit 1
fi
echo "42"`, "")
		passSpec := filepath.Join(passDir, "scorer.md")
		if err := os.WriteFile(passSpec, []byte("---\nkind: metric\ncommand: "+filepath.Base(passCmd)+"\ndirection: max\nscale: 10\ntimeout: 5s\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		pinStore := filepath.Join(t.TempDir(), "pins.toml")
		if _, err := score.Pin(pinStore, passSpec); err != nil {
			t.Fatal(err)
		}
		res, err := score.RunCommit(context.Background(), score.RunOptions{
			ScorerPath:   passSpec,
			PinStorePath: pinStore,
			RepoPath:     repo,
			Commit:       baseSHA,
		})
		if err != nil {
			t.Fatalf("env isolation check failed: %v", err)
		}
		if res.Mark == nil || *res.Mark != 420 {
			t.Fatalf("mark = %v, want 420", res.Mark)
		}

		// 2. Verify error output scrubs printed secrets, host env secret values, and local paths.
		failDir := filepath.Join(t.TempDir(), "leak-check")
		if err := os.MkdirAll(failDir, 0700); err != nil {
			t.Fatal(err)
		}
		failCmd := writeTestScript(t, failDir, "leak",
			`echo "crash in $(pwd) home=$HOME key=`+openAISecret+` custom=`+hostEnvSecret+`" >&2
exit 1`, "")
		failSpec := filepath.Join(failDir, "scorer.md")
		if err := os.WriteFile(failSpec, []byte("---\nkind: metric\ncommand: "+filepath.Base(failCmd)+"\ndirection: min\ntimeout: 5s\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := score.Pin(pinStore, failSpec); err != nil {
			t.Fatal(err)
		}
		_, err = score.RunCommit(context.Background(), score.RunOptions{
			ScorerPath:   failSpec,
			PinStorePath: pinStore,
			RepoPath:     repo,
			Commit:       baseSHA,
		})
		if err == nil {
			t.Fatalf("expected error from failing scorer")
		}
		msg := err.Error()
		for _, forbidden := range []string{openAISecret, hostEnvSecret, home, repo, failDir} {
			if strings.Contains(msg, forbidden) {
				t.Fatalf("error leaked sensitive value or local path %q in %q", forbidden, msg)
			}
		}
		if !strings.Contains(msg, "[REDACTED:") {
			t.Fatalf("expected redaction marker in error message, got %q", msg)
		}
	})

	t.Run("post-run pin check refuses scorer that mutates its own data file", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "mutating-scorer")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		dataPath := filepath.Join(dir, "fixture.txt")
		if err := os.WriteFile(dataPath, []byte("clean\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cmdPath := writeTestScript(t, dir, "mutate",
			`echo "mutated" > "$HEV_SCORE_DATA_0"
echo '{"value": 0.99}'`, "")
		specPath := filepath.Join(dir, "scorer.md")
		if err := os.WriteFile(specPath, []byte("---\nkind: metric\ncommand: "+filepath.Base(cmdPath)+"\ndirection: max\ntimeout: 5s\ndata:\n  - fixture.txt\n---\n"), 0600); err != nil {
			t.Fatal(err)
		}
		pinStore := filepath.Join(t.TempDir(), "pins.toml")
		if _, err := score.Pin(pinStore, specPath); err != nil {
			t.Fatal(err)
		}
		_, err := score.RunCommit(context.Background(), score.RunOptions{
			ScorerPath:   specPath,
			PinStorePath: pinStore,
			RepoPath:     repo,
			Commit:       baseSHA,
		})
		if !errors.Is(err, score.ErrStalePin) {
			t.Fatalf("expected post-run ErrStalePin when data mutated during run, got %v", err)
		}
	})
}
