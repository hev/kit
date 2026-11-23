//go:build !windows

package dispatch

import (
	"github.com/hev/kit/internal/steer"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkerCandidates(t *testing.T) {
	for _, mode := range []string{"exit", "launch", "clean", "stop", "halt", "timeout", "validator"} {
		t.Run(mode, func(t *testing.T) {
			const name = "candidate"
			dir, prompts, _ := outcomeFixture(t, name)
			bin := filepath.Dir(prompts)
			var m Meta
			if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
				t.Fatal(err)
			}
			m.Fallbacks = FallbackChain{{Harness: "agy", Model: "backup"}}
			primary := "exit 1"
			switch mode {
			case "launch":
				primary = ""
				if err := os.WriteFile(filepath.Join(bin, "claude"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(bin, "claude"), 0600); err != nil {
					t.Fatal(err)
				}
				// PATH holds the fakes and system tools only, so lookup cannot fall through
				// to a real claude installed on the host.
				t.Setenv("PATH", strings.Join([]string{bin, "/usr/bin", "/bin"}, string(os.PathListSeparator)))
			case "clean":
				primary = "exit 0"
			case "stop":
				primary = "touch ../stop; exit 1"
			case "halt":
				store := steer.Store{Dir: filepath.Join(dir, "steer")}
				msg, err := store.Enqueue(steer.HaltL, "end", "", "", time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				_, err = store.Update(msg.ID, func(m *steer.Message) { m.State = steer.Injected })
				if err != nil {
					t.Fatal(err)
				}
			case "timeout":
				m.Timeout = 0
				primary = "exec sleep 5"
			case "validator":
				m.Parent = "parent"
				m.Implementer = &FallbackTarget{Harness: "agy", Model: "backup"}
			}
			if primary != "" {
				if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"+primary+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(bin, "agy"), []byte("#!/bin/sh\npwd > ../fallback-cwd\necho done > ../status.md\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := superviseRound(name); err != nil && mode != "validator" {
				t.Fatal(err)
			}
			r := readResult(t, dir)
			log, _ := os.ReadFile(filepath.Join(dir, "log"))
			cwd, _ := os.ReadFile(filepath.Join(dir, "fallback-cwd"))
			// pwd reports the resolved path (macOS temp dirs sit behind /var).
			wantCwd, _ := filepath.EvalSymlinks(filepath.Join(dir, "wt"))
			gotCwd, _ := filepath.EvalSymlinks(strings.TrimSpace(string(cwd)))
			want := mode == "launch" || mode == "exit"
			if (len(r.Attempts) == 2) != want || strings.Contains(string(log), "dispatch.fallback") != want || (gotCwd != "" && gotCwd == wantCwd) != want {
				t.Fatalf("attempts %+v cwd %q log %s", r.Attempts, cwd, log)
			}
			if want && exists(filepath.Join(dir, "steer", wiredFile)) {
				t.Fatal("fallback advertises stale primary hooks")
			}
			if want && (r.ExitCode != 0 || r.Harness != "agy" || r.Model != "backup") {
				t.Fatalf("unexpected result %+v", r)
			}
		})
	}
}
