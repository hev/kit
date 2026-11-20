package hevpaths

import (
	"os"
	"path/filepath"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range Env {
		t.Setenv(name, "")
	}
	return home
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

// Each rule is checked from both sides for every surface with a legacy
// directory: what a fresh HOME must resolve to, what a legacy HOME must keep,
// and that the environment beats both.
func TestResolveRules(t *testing.T) {
	legacy := map[Surface]string{
		Traces: filepath.Join(".hev", "archive"),
		Orders: "dispatch",
		Peers:  filepath.Join(".local", "state", "peers-v2"),
	}
	for s, rel := range legacy {
		t.Run(string(s), func(t *testing.T) {
			home := isolate(t)
			want := filepath.Join(home, ".hev", string(s))

			// Fresh HOME: the unified directory, never the legacy one.
			if r := Resolve(s); r.Dir != want || r.Source != SourceDefault {
				t.Fatalf("fresh: %+v, want %s", r, want)
			}

			// Legacy HOME: the legacy directory is kept, not abandoned.
			old := filepath.Join(home, rel)
			mkdir(t, old)
			if r := Resolve(s); r.Dir != old || r.Source != SourceLegacy {
				t.Fatalf("legacy: %+v, want %s", r, old)
			}

			// A legacy path that is a file is not a directory to keep.
			if s == Orders {
				os.Remove(old)
				os.WriteFile(old, []byte("x"), 0o600)
				if r := Resolve(s); r.Dir != want {
					t.Fatalf("legacy file: %+v", r)
				}
				os.Remove(old)
				mkdir(t, old)
			}

			// Both present: the unified directory wins (the operator moved).
			mkdir(t, want)
			if r := Resolve(s); r.Dir != want || r.Source != SourceDefault {
				t.Fatalf("both: %+v, want %s", r, want)
			}

			// Explicit environment wins over both.
			explicit := filepath.Join(home, "explicit")
			t.Setenv(Env[s], explicit)
			if r := Resolve(s); r.Dir != explicit || r.Source != SourceEnv {
				t.Fatalf("env: %+v, want %s", r, explicit)
			}
			// ... and over a legacy-only HOME.
			os.RemoveAll(want)
			if r := Resolve(s); r.Dir != explicit {
				t.Fatalf("env over legacy: %+v", r)
			}
			// A blank variable is not an override.
			t.Setenv(Env[s], "  ")
			if r := Resolve(s); r.Dir != old {
				t.Fatalf("blank env: %+v", r)
			}
		})
	}
}

func TestSurfacesWithoutLegacy(t *testing.T) {
	home := isolate(t)
	for _, s := range []Surface{Board, Hooks, Evidence} {
		if Legacy(home, s) != "" {
			t.Fatalf("%s has a legacy dir", s)
		}
		if got := Dir(s); got != filepath.Join(home, ".hev", string(s)) {
			t.Fatalf("%s: %s", s, got)
		}
	}
}

// EnsureAll creates exactly the resolved directories: the unified layout on a
// fresh HOME, and on a legacy HOME the legacy directories, never a second
// unified directory that would silently take over from them.
func TestEnsureAll(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		home := isolate(t)
		if _, err := EnsureAll(); err != nil {
			t.Fatal(err)
		}
		for _, s := range All {
			if !isDir(filepath.Join(home, ".hev", string(s))) {
				t.Fatalf("%s not created", s)
			}
		}
		for _, s := range []Surface{Traces, Orders, Peers} {
			if isDir(Legacy(home, s)) {
				t.Fatalf("fresh install created legacy %s", Legacy(home, s))
			}
		}
		if info, _ := os.Stat(filepath.Join(home, ".hev", "orders")); info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
			t.Fatalf("orders mode %v", info.Mode())
		}
	})
	t.Run("legacy", func(t *testing.T) {
		home := isolate(t)
		for _, s := range []Surface{Traces, Orders, Peers} {
			mkdir(t, Legacy(home, s))
		}
		rs, err := EnsureAll()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			switch r.Surface {
			case Traces, Orders, Peers:
				if r.Source != SourceLegacy || isDir(Default(home, r.Surface)) {
					t.Fatalf("%s: %+v; unified dir created beside legacy", r.Surface, r)
				}
			default:
				if !isDir(r.Dir) {
					t.Fatalf("%s not created", r.Surface)
				}
			}
		}
	})
}
