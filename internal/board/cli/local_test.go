package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestMain lets a test run this binary as `hev board ARGS...` in a process of
// its own.
func TestMain(m *testing.M) {
	if args := os.Getenv("BOARD_CLI_ARGS"); args != "" {
		var argv []string
		if err := json.Unmarshal([]byte(args), &argv); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if err := Run(argv); err != nil {
			fmt.Fprintln(os.Stderr, "hev board:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// starterKit is a machine with no Layer key and nothing naming a board
// server: HOME is a temp dir, so the board is ~/.hev/board inside it.
func starterKit(t *testing.T) (home string) {
	t.Helper()
	withoutKey(t)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_BOARD_DIR", "")
	t.Setenv("FACTORY_CANVAS_DIR", filepath.Join(home, ".hev", "canvas"))
	t.Setenv("FACTORY_BOARD_AS", "agent-a")
	return home
}

func TestNoKeyNoServerWorksOnFiles(t *testing.T) {
	home := starterKit(t)
	dir := filepath.Join(home, ".hev", "board")

	var th struct {
		ID   int64    `json:"id"`
		Tags []string `json:"tags"`
	}
	decode(t, must(t, "post", "--board", "tooling", "--title", "Port 5432 collides", "--tags", "ci,redis", "-m", "Port 5432 is already allocated in CI.", "--json"), &th)
	if th.ID != 1 || strings.Join(th.Tags, ",") != "ci,redis" {
		t.Fatalf("post: %+v", th)
	}
	t.Setenv("FACTORY_BOARD_AS", "agent-b")
	if out := must(t, "reply", "1", "-m", "Use a random host port instead."); !strings.Contains(out, "replied #1.2 as agent-b") {
		t.Fatalf("reply: %s", out)
	}
	must(t, "pin", "1")
	must(t, "useful", "1")
	for _, f := range []string{"threads/1.json", "replies/1/2.json", "flags/1.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("one file per post under ~/.hev/board: %v", err)
		}
	}
	var read struct {
		Thread struct {
			Replies int  `json:"replies"`
			Useful  int  `json:"useful"`
			Pinned  bool `json:"pinned"`
		} `json:"thread"`
	}
	decode(t, must(t, "read", "1", "--json"), &read)
	if read.Thread.Replies != 1 || read.Thread.Useful != 1 || !read.Thread.Pinned {
		t.Fatalf("read: %+v", read)
	}
	var list []struct{ ID int64 }
	decode(t, must(t, "ls", "--tag", "redis", "--json"), &list)
	if len(list) != 1 {
		t.Fatalf("ls --tag: %+v", list)
	}

	search := func(t *testing.T) {
		var hits []struct {
			PostN  int `json:"post_n"`
			Thread struct {
				ID int64 `json:"id"`
			} `json:"thread"`
			Snippet string `json:"snippet"`
		}
		decode(t, must(t, "search", "random ports", "--json"), &hits)
		if len(hits) != 1 || hits[0].PostN != 2 || !strings.Contains(hits[0].Snippet, "[random]") {
			t.Fatalf("search: %+v", hits)
		}
		decode(t, must(t, "search", "allocated", "--tag", "ci", "--json"), &hits)
		if len(hits) != 1 || hits[0].PostN != 1 {
			t.Fatalf("search --tag: %+v", hits)
		}
	}
	t.Run("rg", func(t *testing.T) {
		if _, err := exec.LookPath("rg"); err != nil {
			t.Skip("rg is not on PATH")
		}
		search(t)
	})
	t.Run("go", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		search(t)
	})

	var whoami struct {
		Storage struct{ Mode, From, Dir string } `json:"storage"`
	}
	decode(t, must(t, "whoami", "--json"), &whoami)
	if whoami.Storage.Mode != "filesystem" || whoami.Storage.Dir != dir || !strings.Contains(whoami.Storage.From, "no server") {
		t.Fatalf("whoami: %+v", whoami)
	}
}

// Separate `hev board post` processes on one machine never share an id.
func TestNoServerConcurrentPostsGetDistinctIDs(t *testing.T) {
	home := starterKit(t)
	const n = 8
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args, _ := json.Marshal([]string{"post", "--board", "floor", "--title", fmt.Sprintf("post %d", i), "-m", "body"})
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "BOARD_CLI_ARGS="+string(args))
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs[i] = fmt.Errorf("%v: %s", err, out)
				return
			}
			f := strings.Fields(string(out))
			if len(f) < 2 {
				errs[i] = fmt.Errorf("output: %s", out)
				return
			}
			ids[i] = f[1]
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range ids {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if seen[ids[i]] {
			t.Fatalf("two posts got %s: %v", ids[i], ids)
		}
		seen[ids[i]] = true
	}
	files, _ := filepath.Glob(filepath.Join(home, ".hev", "board", "threads", "*.json"))
	if len(files) != n {
		t.Fatalf("%d thread files, want %d", len(files), n)
	}
}

// A board server named in config wins over files even without a key: a
// client of a shared hosted board needs no Layer key of its own.
func TestNamedServerWinsOverFiles(t *testing.T) {
	starterKit(t)
	t.Setenv("FACTORY_BOARD_URL", "http://127.0.0.1:1")
	if _, err := run(t, "ls"); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("ls against a named server: %v", err)
	}
}
