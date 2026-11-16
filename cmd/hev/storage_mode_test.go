package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/storagemode"
)

// storageHome gives a case its own HOME and config with no key anywhere, the
// environment a dispatched worker starts with (HOME, LANG, PATH, SHELL).
func storageHome(t *testing.T, config string) (home, archive string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range layerenv.Names() {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	t.Setenv("HEV_ARCHIVE_DIR", "")
	path := filepath.Join(home, ".hev", "config.toml")
	os.MkdirAll(filepath.Dir(path), 0o700)
	if config != "" {
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HEV_CONFIG", path)
	return home, filepath.Join(home, ".hev", "archive")
}

// fakeDaemon holds daemon.lock, as a running hevd does, and leaves the
// storage record it would write.
func fakeDaemon(t *testing.T, record string) {
	t.Helper()
	release, err := platformops.LockPath(daemon.DaemonLockPath(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if record != "" {
		if err := os.WriteFile(daemon.StorageRecordPath(), []byte(record), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const layerRecord = `{"pid": 4242, "mode": "layer", "namespace": "team", "endpoint": "https://gateway.example", "key_source": "HEVLAYER_API_KEY"}`

func statusOutput(t *testing.T) string {
	t.Helper()
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	st, stErr := daemon.RunningStorage()
	var out bytes.Buffer
	writeStorage(&out, cfg, st, stErr)
	return out.String()
}

func noArchiveAt(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive root %s exists (err=%v)", root, err)
	}
}

// The case that went wrong: the daemon writes to Layer with a key only its
// service unit has; the shell (or a worker) has none.
func TestStatusShowsDaemonModeAndFlagsAShellThatDiffers(t *testing.T) {
	_, archive := storageHome(t, "")
	fakeDaemon(t, layerRecord)
	out := statusOutput(t)
	for _, want := range []string{
		"Storage (daemon): hev layer namespace team at https://gateway.example (pid 4242, key from HEVLAYER_API_KEY)",
		"Storage (this shell): error: this machine is set up for hev layer",
		"capture daemon on this machine (pid 4242)",
		"! the daemon and this shell differ",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status lacks %q:\n%s", want, out)
		}
	}
	noArchiveAt(t, archive)
}

func TestStatusAgreesWhenShellHasTheDaemonsKey(t *testing.T) {
	storageHome(t, "[layer]\nendpoint = \"https://gateway.example\"\nnamespace = \"team\"\n")
	t.Setenv("HEVLAYER_API_KEY", "shell-key")
	fakeDaemon(t, layerRecord)
	out := statusOutput(t)
	if !strings.Contains(out, "Storage (this shell): hev layer namespace team at https://gateway.example (key from HEVLAYER_API_KEY)") || strings.Contains(out, "differ") {
		t.Fatalf("status:\n%s", out)
	}
	if strings.Contains(out, "shell-key") {
		t.Fatal("status printed the key")
	}
}

func TestStatusFlagsALayerShellBesideAFilesDaemon(t *testing.T) {
	storageHome(t, "")
	t.Setenv("HEVLAYER_API_KEY", "shell-key")
	fakeDaemon(t, `{"pid": 7, "mode": "filesystem", "namespace": "hev-traces", "archive": "/somewhere/hev-traces"}`)
	out := statusOutput(t)
	if !strings.Contains(out, "Storage (daemon): files at /somewhere/hev-traces (pid 7)") || !strings.Contains(out, "differ") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestStatusWithNoDaemonAndAnOlderDaemon(t *testing.T) {
	_, archive := storageHome(t, "")
	out := statusOutput(t)
	if !strings.Contains(out, "Storage (daemon): no capture daemon is running") || !strings.Contains(out, "Storage (this shell): files at") || strings.Contains(out, "differ") {
		t.Fatalf("status:\n%s", out)
	}
	fakeDaemon(t, "")
	out = statusOutput(t)
	if !strings.Contains(out, "not reported (the running daemon predates this build") {
		t.Fatalf("status:\n%s", out)
	}
	noArchiveAt(t, archive)
}

// `hev s` in files mode used to create the empty archive directory (through
// the edition check). It creates nothing now.
func TestStatusCreatesNoArchive(t *testing.T) {
	_, archive := storageHome(t, "")
	printStorage()
	printEdition()
	noArchiveAt(t, archive)
}

// A trace command on a machine set up for Layer, with no key in reach,
// fails with the explanation instead of reading an empty file archive.
func TestTraceCommandsRefuseFilesOnALayerMachine(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T){
		"config endpoint": func(t *testing.T) {
			os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[layer]\nendpoint = \"https://gateway.example\"\n"), 0o600)
		},
		"config namespace": func(t *testing.T) {
			os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[layer]\nnamespace = \"team\"\n"), 0o600)
		},
		"daemon in layer mode": func(t *testing.T) { fakeDaemon(t, layerRecord) },
	} {
		t.Run(name, func(t *testing.T) {
			_, archive := storageHome(t, "")
			setup(t)
			_, err := traces("")
			if !storagemode.IsNoKey(err) || !strings.Contains(err.Error(), "set up for hev layer") {
				t.Fatalf("traces: %v", err)
			}
			if err := runLs(lsCmd, nil); !storagemode.IsNoKey(err) {
				t.Fatalf("hev traces ls: %v", err)
			}
			if _, err := client(""); !storagemode.IsNoKey(err) {
				t.Fatalf("client: %v", err)
			}
			noArchiveAt(t, archive)
		})
	}
}

// With nothing pointing at Layer, listing works on files as before and
// leaves no archive behind.
func TestTraceListOnFilesCreatesNothing(t *testing.T) {
	_, archive := storageHome(t, "")
	st, err := traces("")
	if err != nil || !isLexical(st) {
		t.Fatalf("traces = %T, %v", st, err)
	}
	if err := runLs(lsCmd, nil); err != nil {
		t.Fatal(err)
	}
	noArchiveAt(t, archive)
}
