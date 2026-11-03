package index

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func projectWrite(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hev.toml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestProjectResolution(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	nested := filepath.Join(home, "project", "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	projectWrite(t, root, "[capture]\nnamespace='outside-home'")
	if got := chooseRoute(nested, "default", nil, home); got.Namespace != "default" || got.Blocked {
		t.Fatalf("home boundary: %+v", got)
	}
	projectWrite(t, home, "[capture]\nnamespace='parent'")
	projectWrite(t, filepath.Dir(nested), "[capture]\nnamespace='project'")
	if got := chooseRoute(nested, "default", []string{"project"}, home); got.Namespace != "project" || got.Blocked {
		t.Fatalf("nearest: %+v", got)
	}
	projectWrite(t, nested, "[capture]\narchive=false")
	if got := chooseRoute(nested, "default", nil, home); !got.Skip || got.Blocked {
		t.Fatalf("nested no merging: %+v", got)
	}
	projectWrite(t, nested, "[capture]\nnamespace='replacement'")
	// Explicitly move mtime, including on filesystems with coarse timestamps.
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(filepath.Join(nested, ".hev.toml"), future, future); err != nil {
		t.Fatal(err)
	}
	if got := chooseRoute(nested, "default", nil, home); got.Namespace != "replacement" || !got.Blocked {
		t.Fatalf("mtime invalidation: %+v", got)
	}
	if err := os.Remove(filepath.Join(nested, ".hev.toml")); err != nil {
		t.Fatal(err)
	}
	if got := chooseRoute(nested, "default", []string{"project"}, home); got.Namespace != "project" {
		t.Fatalf("removed winner: %+v", got)
	}
	for _, dir := range []string{"", filepath.Join(root, "missing"), filepath.Join(nested, ".hev.toml")} {
		if got := chooseRoute(dir, "default", nil, home); got.Namespace != "default" || got.Blocked {
			t.Fatalf("missing workdir %q: %+v", dir, got)
		}
	}
}

func TestProjectForbiddenKeysIgnored(t *testing.T) {
	dir := t.TempDir()
	projectWrite(t, dir, "endpoint='ignored'\napi_key='ignored'\n[layer]\nnamespace='ignored'\n[capture]\nnamespace='team-x-traces'\nredact=false\nstore='ignored'")
	got := chooseRoute(dir, "default", []string{"team-x-traces"}, dir)
	if got.Namespace != "team-x-traces" || got.Blocked || got.Skip {
		t.Fatalf("route: %+v", got)
	}
	cfg, invalid := projectConfig(dir, dir)
	if invalid || cfg.Capture.Namespace != "team-x-traces" {
		t.Fatalf("config: %+v invalid=%v", cfg, invalid)
	}
}

func TestProjectIndexerRoutesAndPinsAllRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	root := t.TempDir()
	a, b, blocked, skipped := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "blocked"), filepath.Join(root, "skip")
	projectWrite(t, a, "[capture]\nnamespace='team-a'\nredact=false")
	projectWrite(t, b, "[capture]\nnamespace='team-b'")
	projectWrite(t, blocked, "[capture]\nnamespace='refused'")
	projectWrite(t, skipped, "[capture]\narchive=false")
	units := []trace.Unit{{Key: "mixed", Signature: "1"}}
	var turns []trace.Turn
	for i, dir := range []string{a, b, blocked, skipped} {
		turns = append(turns, trace.Turn{SessionID: []string{"s-a", "s-b", "s-blocked", "s-skip"}[i], Harness: "codex", Workdir: dir, Role: "user", Blocks: []trace.Block{{Type: "text", Text: "synthetic routing test AKIAABCDEFGHIJKLMNOP"}}})
	}
	src := migrationSource{units: units, turns: map[string][]trace.Turn{"mixed": turns}}
	writes := map[string]int{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body["schema"]) == 0 {
			t.Error("missing schema")
		}
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Error("changed machine credential")
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Error(err)
		}
		if strings.Contains(string(encoded), "AKIAABCDEFGHIJKLMNOP") {
			t.Error("project bypassed machine redaction")
		}
		writes[r.URL.Path]++
		io.WriteString(w, `{"rows_upserted":1}`)
	})
	srv := httptest.NewServer(withSessionPersistence(handler))
	defer srv.Close()
	cl := layer.New(srv.URL, "key", "default", "")
	st := &State{Units: map[string]string{}}
	opt := Options{ProjectNamespaces: []string{"team-a", "team-b"}, RepoURL: func(string) string { return "" }}
	rep, err := Run(src, cl, st, opt)
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("report=%+v err=%v", rep, err)
	}
	if rep.BlockedByProjectConfig != 1 {
		t.Fatalf("blocked: %+v", rep)
	}
	for _, ns := range []string{"team-a", "team-b"} {
		for _, suffix := range []string{"", "-blocks", "-sessions"} {
			if writes["/v2/namespaces/"+ns+suffix] != 1 {
				t.Fatalf("missing %s%s: %v", ns, suffix, writes)
			}
		}
	}
	if len(writes) != 6 {
		t.Fatalf("unexpected writes (default or refused leak): %v", writes)
	}
	// Persist/reload, then grow the source and change the project configuration.
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	st = &State{}
	if err := json.Unmarshal(raw, st); err != nil {
		t.Fatal(err)
	}
	projectWrite(t, a, "[capture]\nnamespace='team-b'")
	src.units[0].Signature = "2"
	rep, err = Run(src, cl, st, opt)
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("growth: %+v %v", rep, err)
	}
	if writes["/v2/namespaces/team-a"] != 2 || writes["/v2/namespaces/team-b"] != 2 || len(writes) != 6 {
		t.Fatalf("routing changed: %v", writes)
	}
	// Removing a machine allowlist entry must stop writes even to a pinned
	// destination. Restoring it retries the held content without rerouting.
	src.units[0].Signature = "3"
	opt.ProjectNamespaces = []string{"team-b"}
	rep, err = Run(src, cl, st, opt)
	if err != nil || len(rep.Errors) > 0 || rep.BlockedByProjectConfig != 2 || writes["/v2/namespaces/team-a"] != 2 {
		t.Fatalf("revoked allowlist: %+v %v writes=%v", rep, err, writes)
	}
	opt.ProjectNamespaces = []string{"team-a", "team-b"}
	rep, err = Run(src, cl, st, opt)
	if err != nil || len(rep.Errors) > 0 || rep.BlockedByProjectConfig != 1 || writes["/v2/namespaces/team-a"] != 3 || len(writes) != 6 {
		t.Fatalf("restored allowlist: %+v %v writes=%v", rep, err, writes)
	}
}

func TestProjectUnreadableWorkdirFallsBack(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "project")
	projectWrite(t, dir, "[capture]\nnamespace='refused'")
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	f, err := os.Open(dir)
	if err == nil {
		_, err = f.Readdirnames(1)
		f.Close()
		if err == nil || err == io.EOF {
			t.Skip("platform or user can read permission-denied directories")
		}
	}
	got := chooseRoute(dir, "default", nil, home)
	if got.Namespace != "default" || got.Blocked {
		t.Fatalf("unreadable workdir: %+v", got)
	}
}

func TestProjectMalformedConfigHolds(t *testing.T) {
	dir := t.TempDir()
	projectWrite(t, dir, "[capture]\nnamespace=[invalid")
	if got := chooseRoute(dir, "default", nil, dir); !got.Blocked {
		t.Fatalf("invalid config leaked: %+v", got)
	}
}

func TestProjectExplicitIndexMigrationRoutesNewSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	project := filepath.Join(home, "project")
	projectWrite(t, project, "[capture]\nnamespace='team-x-traces'")
	root := filepath.Join(home, "transcripts")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	raw := `{"type":"user","uuid":"u","sessionId":"s","cwd":` + strconv.Quote(project) + `,"message":{"role":"user","content":"synthetic migration routing"}}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	srv := httptest.NewServer(withSessionPersistence(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || r.Method == "DELETE" || strings.HasSuffix(r.URL.Path, "/query") {
			io.WriteString(w, `{"rows":[]}`)
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body["upsert_rows"]) > 0 {
			paths[r.URL.Path] = true
		}
		io.WriteString(w, `{"rows_upserted":1}`)
	})))
	defer srv.Close()
	rep, err := Run(&trace.ClaudeSource{Root: root}, layer.New(srv.URL, "key", "default", ""), &State{Units: map[string]string{}}, Options{MigrateArchive: true, ProjectNamespaces: []string{"team-x-traces"}, RepoURL: func(string) string { return "" }})
	if err != nil || len(rep.Errors) > 0 {
		t.Fatalf("migration: %+v %v", rep, err)
	}
	for _, suffix := range []string{"", "-blocks", "-sessions"} {
		if !paths["/v2/namespaces/team-x-traces"+suffix] {
			t.Fatalf("missing project rows: %v", paths)
		}
	}
	if len(paths) != 3 {
		t.Fatalf("unexpected destination: %v", paths)
	}
}

func TestProjectAllHarnessesHoldWithoutDefaultWrite(t *testing.T) {
	for _, harness := range []string{"claude_code", "codex", "pi", "dot"} {
		t.Run(harness, func(t *testing.T) {
			dir := t.TempDir()
			projectWrite(t, dir, "[capture]\nnamespace='refused'")
			src := migrationSource{units: []trace.Unit{{Key: "unit", Signature: "1"}}, turns: map[string][]trace.Turn{"unit": {{Harness: harness, SessionID: "s", Workdir: dir, Role: "user", Blocks: []trace.Block{{Type: "text", Text: "synthetic"}}}}}}
			// A nil client would panic on any accidental write.
			rep, err := Run(src, nil, &State{Units: map[string]string{}}, Options{})
			if err != nil || rep.BlockedByProjectConfig != 1 || rep.Chunks != 0 || rep.Blocks != 0 || rep.Sessions != 0 {
				t.Fatalf("%+v %v", rep, err)
			}
		})
	}
}
