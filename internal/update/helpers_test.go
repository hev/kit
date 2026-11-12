package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain lets a copied test binary stand in for a running hev on Windows.
func TestMain(m *testing.M) {
	if os.Getenv("HEV_UPDATE_TEST_SLEEP") != "" {
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeRepo is a local stand-in for the repository's published builds: the
// two GitHub API routes the updater reads and the release download route.
type fakeRepo struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	files    map[string][]byte // "<tag>/<name>"
	releases []map[string]any
	latest   string
	requests int
	seq      int
}

func newFakeRepo(t *testing.T) *fakeRepo {
	f := &fakeRepo{t: t, files: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.count()
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.latest == "" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"tag_name": f.latest, "draft": false, "prerelease": false})
	})
	mux.HandleFunc("/api/releases", func(w http.ResponseWriter, r *http.Request) {
		f.count()
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(f.releases)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		f.count()
		f.mu.Lock()
		b, ok := f.files[strings.TrimPrefix(r.URL.Path, "/download/")]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRepo) count() { f.mu.Lock(); f.requests++; f.mu.Unlock() }

func (f *fakeRepo) Requests() int { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }

func (f *fakeRepo) source() Source {
	return Source{API: f.srv.URL + "/api", Download: f.srv.URL + "/download"}
}

// fakeBinary is what a published "hev" does in these tests: print its version,
// optionally after waiting for a file, so a test can hold a run open.
func fakeBinary(version string) []byte {
	if runtime.GOOS == "windows" {
		return []byte("fake hev " + version)
	}
	return []byte("#!/bin/sh\nif [ -n \"$HEV_FAKE_WAIT\" ]; then while [ ! -f \"$HEV_FAKE_WAIT\" ]; do sleep 0.05; done; fi\necho " + version + "\n")
}

func archive(t *testing.T, goos string, binary []byte) []byte {
	var buf bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("hev.exe")
		if err != nil {
			t.Fatal(err)
		}
		w.Write(binary)
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "hev", Mode: 0o755, Size: int64(len(binary)), Typeflag: tar.TypeReg})
	tw.Write(binary)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

type publishOpt struct {
	binary      []byte // default fakeBinary(version)
	noManifest  bool   // a release published before manifests
	tamper      bool   // serve an archive other than the one the manifest names
	prerelease  bool
	publishedAt time.Time
}

// publish adds a build for the test's own platform the way the workflows do.
func (f *fakeRepo) publish(version string, o publishOpt) {
	f.t.Helper()
	tag := Tag(version)
	bin := o.binary
	if bin == nil {
		bin = fakeBinary(version)
	}
	name := AssetName(version, runtime.GOOS, runtime.GOARCH)
	arch := archive(f.t, runtime.GOOS, bin)
	sum := sha256.Sum256(arch)
	digest := hex.EncodeToString(sum[:])
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	served := arch
	if o.tamper {
		served = archive(f.t, runtime.GOOS, append(append([]byte{}, bin...), []byte("\n# tampered\n")...))
	}
	f.files[tag+"/"+name] = served
	f.files[tag+"/checksums.txt"] = []byte(digest + "  " + name + "\n")
	assets := []map[string]any{{"name": name}, {"name": "checksums.txt"}}
	if !o.noManifest {
		m := Manifest{Schema: 1, Version: version, Channel: VersionChannel(version), Sequence: f.seq,
			Assets: []Asset{{Name: name, OS: runtime.GOOS, Arch: runtime.GOARCH, SHA256: digest, Size: int64(len(arch))}}}
		raw, _ := json.Marshal(m)
		f.files[tag+"/manifest.json"] = raw
		assets = append(assets, map[string]any{"name": "manifest.json"})
	}
	at := o.publishedAt
	if at.IsZero() {
		at = time.Date(2026, 11, 1, 0, 0, f.seq, 0, time.UTC)
	}
	isEdge := VersionChannel(version) == ChannelEdge
	f.releases = append([]map[string]any{{"tag_name": tag, "draft": false, "prerelease": isEdge || o.prerelease, "published_at": at, "assets": assets}}, f.releases...)
	if !isEdge {
		f.latest = tag
	}
}

type fakeService struct {
	name    string
	present bool
	fail    func() error
	calls   int
}

func (s *fakeService) Name() string  { return s.name }
func (s *fakeService) Present() bool { return s.present }
func (s *fakeService) Restart(context.Context) error {
	s.calls++
	if s.fail != nil {
		return s.fail()
	}
	return nil
}

type harness struct {
	home, link string
	repo       *fakeRepo
	svc        *fakeService
	skills     []string
	hooks      []string
	hookEnv    [][]string
}

// newHarness installs version at a temporary bin/hev, as a user who unpacked
// a release archive into a directory of their own would have it.
func newHarness(t *testing.T, current string) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{home: filepath.Join(root, "home"), repo: newFakeRepo(t), svc: &fakeService{name: "hevd (fake)", present: true}}
	h.link = filepath.Join(root, "bin", BinaryName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(h.link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.link, fakeBinary(current), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", filepath.Join(h.home, ".hev", "config.toml"))
	return h
}

func (h *harness) updater(current string) *Updater {
	return &Updater{
		Home: h.home, Source: h.repo.source(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Current: current, Invoked: h.link, Services: []Service{h.svc},
		Smoke: func(ctx context.Context, bin string) (string, error) {
			if runtime.GOOS == "windows" {
				b, err := os.ReadFile(bin)
				return strings.TrimPrefix(string(b), "fake hev "), err
			}
			return DefaultSmoke(ctx, bin)
		},
		SyncSkills: func(_ context.Context, bin string) error { h.skills = append(h.skills, bin); return nil },
		RunHook: func(_ context.Context, c string, env []string) ([]byte, error) {
			h.hooks, h.hookEnv = append(h.hooks, c), append(h.hookEnv, env)
			return nil, nil
		},
		By: "test",
	}
}

// running is the version the link runs now.
func (h *harness) running(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		b, err := os.ReadFile(h.link)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimPrefix(string(b), "fake hev ")
	}
	out, err := DefaultSmoke(context.Background(), h.link)
	if err != nil {
		t.Fatalf("run %s: %v", h.link, err)
	}
	return strings.TrimSpace(out)
}

func mustErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want it to contain %q", err, want)
	}
}

var _ = errors.Is
var _ = fmt.Sprint
