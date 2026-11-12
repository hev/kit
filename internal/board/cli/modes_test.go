package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/layerenv"
)

// run is one `hev board` command with its stdout.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	err := Run(args)
	w.Close()
	os.Stdout = old
	out := <-done
	r.Close()
	return string(out), err
}

func must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("hev board %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func decode(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// firstLine collects what serve writes and signals once it has written.
type firstLine struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (f *firstLine) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer f.once.Do(func() { close(f.ready) })
	return f.buf.Write(p)
}

func (f *firstLine) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}

// startBoard runs `hev board serve` in-process, waits for its startup line,
// and points the client at it.
func startBoard(t *testing.T) (stderr *firstLine) {
	t.Helper()
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stderr = &firstLine{ready: make(chan struct{})}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serve(ctx, ln, stderr); err != nil {
			t.Error(err)
		}
	}()
	t.Cleanup(func() { cancel(); wg.Wait() })
	select {
	case <-stderr.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not start")
	}
	t.Setenv("FACTORY_BOARD_URL", "http://"+ln.Addr().String())
	return stderr
}

// layerTrap is a Layer endpoint that fails the test on any request at all.
func layerTrap(t *testing.T) string {
	trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("filesystem mode called Layer: %s %s", r.Method, r.URL.Path)
		http.Error(w, "no", http.StatusTeapot)
	}))
	t.Cleanup(trap.Close)
	return trap.URL
}

func withoutKey(t *testing.T) {
	t.Helper()
	for _, name := range append(layerenv.Names(), "FACTORY_BOARD_LAYER_API_KEY", "FACTORY_BOARD_URL", "FACTORY_SESSION") {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	// A config file with a [board] table and an empty [layer] key, as `hev init` writes.
	config := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(config, []byte("[layer]\napi_key = \"\"\n\n[board]\naddr = \"127.0.0.1:0\"\n"), 0o600)
	t.Setenv("HEV_CONFIG", config)
	t.Cleanup(func() { settings.Current = settings.Config{} })
}

func TestServeWithoutAKeyRunsOnFiles(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	t.Setenv("FACTORY_CANVAS_DIR", filepath.Join(t.TempDir(), "canvas"))
	// Layer settings are present but there is no key: nothing may reach them.
	trap := layerTrap(t)
	t.Setenv("FACTORY_BOARD_LAYER_ENDPOINT", trap)
	t.Setenv("HEVLAYER_ENDPOINT", trap)
	t.Setenv("FACTORY_BOARD_NAMESPACE", "must-not-be-used")
	t.Setenv("FACTORY_BOARD_AS", "agent-a")

	stderr := startBoard(t)
	if line := stderr.String(); !strings.Contains(line, "filesystem mode") || !strings.Contains(line, dir) {
		t.Fatalf("startup line does not name the mode: %q", line)
	}

	var th struct {
		ID int64 `json:"id"`
	}
	decode(t, must(t, "post", "--board", "tooling", "--title", "Port 5432 collides", "--tags", "ci,redis", "-m", "Port 5432 is already allocated in CI.", "--json"), &th)
	if th.ID != 1 {
		t.Fatalf("thread id %d", th.ID)
	}
	id := strconv.FormatInt(th.ID, 10)
	t.Setenv("FACTORY_BOARD_AS", "agent-b")
	must(t, "reply", id, "-m", "Use a random host port instead.")

	var read struct {
		Thread struct {
			Replies int  `json:"replies"`
			Useful  int  `json:"useful"`
			Pinned  bool `json:"pinned"`
			Locked  bool `json:"locked"`
		} `json:"thread"`
		Posts []struct {
			Author string `json:"author"`
			Body   string `json:"body"`
		} `json:"posts"`
	}
	decode(t, must(t, "read", id, "--json"), &read)
	if read.Thread.Replies != 1 || len(read.Posts) != 2 || read.Posts[1].Author != "agent-b" {
		t.Fatalf("read: %+v", read)
	}

	var list []struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	decode(t, must(t, "ls", "--json"), &list)
	if len(list) != 1 || list[0].Title != "Port 5432 collides" {
		t.Fatalf("ls: %+v", list)
	}
	decode(t, must(t, "ls", "--board", "learnings", "--json"), &list)
	if len(list) != 0 {
		t.Fatalf("ls --board: %+v", list)
	}

	var hits []struct {
		PostN  int `json:"post_n"`
		Thread struct {
			ID int64 `json:"id"`
		} `json:"thread"`
		Snippet string `json:"snippet"`
	}
	decode(t, must(t, "search", "random port", "--json"), &hits)
	if len(hits) != 1 || hits[0].Thread.ID != th.ID {
		t.Fatalf("search: %+v", hits)
	}
	decode(t, must(t, "search", "allocated", "--by", "agent-b", "--json"), &hits)
	if len(hits) != 0 {
		t.Fatalf("search --by: %+v", hits)
	}
	decode(t, must(t, "search", "redis", "--tag", "ci", "--board", "tooling", "--since", "1h", "--json"), &hits)
	if len(hits) != 1 {
		t.Fatalf("search filters: %+v", hits)
	}

	must(t, "useful", id)
	must(t, "pin", id)
	must(t, "lock", id)
	decode(t, must(t, "read", id, "--json"), &read)
	if read.Thread.Useful != 1 || !read.Thread.Pinned || !read.Thread.Locked {
		t.Fatalf("flags: %+v", read.Thread)
	}
	if _, err := run(t, "reply", id, "-m", "too late"); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("reply to a locked thread: %v", err)
	}

	page := filepath.Join(t.TempDir(), "page.html")
	os.WriteFile(page, []byte("<title>Proof sheet</title><p>hello canvas</p>"), 0o600)
	var cv struct {
		ID   int64  `json:"id"`
		Path string `json:"path"`
	}
	decode(t, must(t, "canvas", page, "--json"), &cv)
	if cv.ID != 2 {
		t.Fatalf("canvas: %+v", cv)
	}
	resp, err := http.Get(os.Getenv("FACTORY_BOARD_URL") + cv.Path)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "hello canvas") {
		t.Fatalf("canvas page: %d %s", resp.StatusCode, body)
	}
	decode(t, must(t, "canvas", "ls", "--json"), &list)
	if len(list) != 1 || list[0].ID != cv.ID {
		t.Fatalf("canvas ls: %+v", list)
	}

	var whoami struct {
		Author  string `json:"author"`
		Storage struct {
			Mode string `json:"mode"`
			From string `json:"from"`
		} `json:"storage"`
	}
	decode(t, must(t, "whoami", "--json"), &whoami)
	if whoami.Author != "agent-b" || whoami.Storage.Mode != "filesystem" || whoami.Storage.From != "server" {
		t.Fatalf("whoami: %+v", whoami)
	}
	if out := must(t, "whoami"); !strings.Contains(out, "storage: filesystem (server)") {
		t.Fatalf("whoami: %s", out)
	}

	for _, name := range []string{"1.json", "2.json"} {
		if _, err := os.Stat(filepath.Join(dir, "threads", name)); err != nil {
			t.Errorf("data is not under the board directory: %v", err)
		}
	}
}

func TestServeWithAKeyUsesLayer(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	var mu sync.Mutex
	var paths []string
	layer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-layer-key" {
			t.Errorf("missing Layer authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") {
			w.Write([]byte(`{"rows":[]}`))
			return
		}
		w.Write([]byte(`{"status":"OK"}`))
	}))
	defer layer.Close()
	t.Setenv("FACTORY_BOARD_LAYER_ENDPOINT", layer.URL)
	t.Setenv("FACTORY_BOARD_NAMESPACE", "board-test")
	t.Setenv("HEVLAYER_API_KEY", "test-layer-key")
	t.Setenv("FACTORY_BOARD_AS", "agent-a")

	stderr := startBoard(t)
	if line := stderr.String(); !strings.Contains(line, layer.URL+"/board-test") || !strings.Contains(line, "layer mode") || strings.Contains(line, "test-layer-key") {
		t.Fatalf("startup line: %q", line)
	}
	must(t, "ls", "--json")
	run(t, "post", "--board", "tooling", "--title", "t", "-m", "b")
	var whoami struct {
		Storage struct{ Mode, From string } `json:"storage"`
	}
	decode(t, must(t, "whoami", "--json"), &whoami)
	if whoami.Storage.Mode != "layer" || whoami.Storage.From != "server" {
		t.Fatalf("whoami: %+v", whoami)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 0 || !strings.HasPrefix(paths[0], "POST /v2/namespaces/board-test") {
		t.Fatalf("Layer requests: %v", paths)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Layer mode wrote to the board directory: %v", err)
	}
}

func TestServeRefusesAnUnreadableKey(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HEVLAYER_API_KEY_OP_REF", "op://vault/item/field")
	if err := settings.Load(); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := serve(context.Background(), ln, io.Discard); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("serve: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("an unreadable key fell back to files")
	}
}

func TestWhoamiWithoutAServerReportsLocalMode(t *testing.T) {
	withoutKey(t)
	dir := filepath.Join(t.TempDir(), "board")
	t.Setenv("HEV_BOARD_DIR", dir)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	t.Setenv("FACTORY_BOARD_URL", "http://"+addr)
	var whoami struct {
		Storage struct{ Mode, From, Dir string } `json:"storage"`
	}
	decode(t, must(t, "whoami", "--json"), &whoami)
	if whoami.Storage.Mode != "filesystem" || whoami.Storage.From != "local config" || whoami.Storage.Dir != dir {
		t.Fatalf("whoami: %+v", whoami)
	}
}
