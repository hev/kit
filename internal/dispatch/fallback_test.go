package dispatch

import (
	"context"
	"errors"
	"github.com/hev/kit/internal/steer"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorCandidates(t *testing.T) {
	for _, mode := range []string{"exit", "launch", "missing", "empty", "all-fail", "stop", "timeout", "halt"} {
		t.Run(mode, func(t *testing.T) {
			orchHome(t)
			s := applyOrch(t)
			s.Fallbacks = FallbackChain{{Harness: "codex", Model: "backup"}}
			st, _ := orchestratorState(s.Name)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := OrchestratorDriver{Gather: func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) { return nil, nil }}
			d.Session = func(_ context.Context, s OrchestratorSpec, round string, _ OrchestratorPacket) error {
				write := func(name, value string) {
					t.Helper()
					if err := os.WriteFile(filepath.Join(round, name), []byte(value), 0600); err != nil {
						t.Fatal(err)
					}
				}
				write("trace", s.Harness)
				if s.Harness == "codex" {
					if mode == "all-fail" {
						return errors.New("also failed")
					}
					write("checkpoint.md", "accepted")
					return nil
				}
				switch mode {
				case "missing":
					return nil
				case "empty":
					write("checkpoint.md", "  ")
					return nil
				case "stop":
					write("../../stop", "stop")
				case "timeout":
					cancel()
				case "halt":
					store := steer.Store{Dir: filepath.Join(OrchestratorDir(s.Name), "steer")}
					now := time.Now()
					if err := store.SaveState(steer.State{Halted: true, HaltedAt: &now}); err != nil {
						t.Fatal(err)
					}
				case "launch":
					return os.ErrNotExist
				}
				write("checkpoint.md", "failed attempt")
				return errors.New("exit 1")
			}
			err := RunOrchestratorRound(ctx, s, &st, d)
			success := mode == "exit" || mode == "launch" || mode == "missing" || mode == "empty"
			if (err == nil) != success || st.Rounds != 1 {
				t.Fatalf("rounds %d, err %v", st.Rounds, err)
			}
			attempts := 1
			if success || mode == "all-fail" {
				attempts = 2
			}
			if len(st.Attempts) != attempts {
				t.Fatalf("attempts %+v", st.Attempts)
			}
			if attempts == 2 {
				round := filepath.Join(OrchestratorDir(s.Name), "rounds", "001")
				b, _ := os.ReadFile(filepath.Join(round, "trace.attempt-1"))
				var trail []HarnessAttempt
				if string(b) != "claude" || readJSON(filepath.Join(round, "attempts.json"), &trail) != nil || len(trail) != 2 || trail[0].Status != "failed" || (trail[1].Status == "succeeded") != success {
					t.Fatalf("trace %q history %+v", b, trail)
				}
			}
		})
	}
}

func TestFallbackInputs(t *testing.T) {
	for _, tc := range []struct {
		h, e  string
		valid bool
	}{{"codex", "high", true}, {"", "high", false}, {"other", "", false}, {"agy", "invalid", false}} {
		o, oe := Parse(strings.Replace(good, "---\n", "---\nfallbacks: ["+tc.h+"/model/"+tc.e+"]\n", 1))
		s, se := ParseOrchestrator(strings.Replace(orchestratorExample, "---\n", "---\nfallbacks: [{harness: '"+tc.h+"', effort: '"+tc.e+"'}]\n", 1))
		path := filepath.Join(t.TempDir(), "dispatch.toml")
		if err := os.WriteFile(path, []byte("[[fallbacks]]\nharness='"+tc.h+"'\neffort='"+tc.e+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		c, ce := Load(path)
		if (oe == nil) != tc.valid || (se == nil) != tc.valid || (ce == nil) != tc.valid {
			t.Fatalf("%+v: order %v, spec %v, config %v", tc, oe, se, ce)
		}
		if tc.valid && (len(o.Fallbacks) != 1 || len(s.Fallbacks) != 1 || len(c.Fallbacks) != 1) {
			t.Fatal("missing fallback")
		}
	}
}

func TestFallbackConfigPersistence(t *testing.T) {
	orchHome(t)
	path := filepath.Join(t.TempDir(), "dispatch.toml")
	t.Setenv("HEV_DISPATCH_CONFIG", path)
	if err := os.WriteFile(path, []byte("[[fallbacks]]\nharness='claude'\n[[orchestrate.fallbacks]]\nharness='codex'\n[orchestrate.follow]\nfallbacks=[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.Orchestrate.Follow.Fallbacks == nil || len(cfg.Fallbacks) != 1 {
		t.Fatalf("%+v, %v", cfg, err)
	}
	for _, input := range []struct {
		line  string
		count int
	}{{"", 1}, {"fallbacks: []\n", 0}} {
		text := strings.Replace(orchestratorExample, "---\n", "---\n"+input.line, 1)
		s, err := ApplyOrchestrator(text, false)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := LoadOrchestrator(s.Name)
		if err != nil || saved.Fallbacks == nil || len(saved.Fallbacks) != input.count {
			t.Fatalf("saved %+v, %v", saved.Fallbacks, err)
		}
		if input.count == 1 && saved.Fallbacks[0].Harness != "codex" {
			t.Fatal(saved.Fallbacks)
		}
	}
	for _, h := range []string{"", "codex", "claude", "agy"} {
		line := ""
		want := h
		if h != "" {
			line = "harness: " + h + "\n"
		} else {
			want = "agy"
		}
		s, err := ParseOrchestrator(strings.Replace(orchestratorExample, "harness: claude\n", line, 1))
		if err != nil || s.Harness != want || s.Model != "" {
			t.Fatalf("harness %q got %q/%q, %v", h, s.Harness, s.Model, err)
		}
	}
}
