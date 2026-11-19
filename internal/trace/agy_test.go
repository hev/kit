package trace

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// agyFixtures names the real agy 1.5.12 conversations in testdata/agy (see
// provenance.json and scripts/generate-agy-fixture.py).
func agyFixtures(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile("testdata/agy/provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Conversations map[string]string `json:"conversations"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p.Conversations
}

// agyRoot builds an agy data directory from the fixtures: one WAL-mode
// database per conversation and conversation_summaries.db. It returns the
// root and each fixture's conversation ID.
func agyRoot(t *testing.T) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conversations"), 0o700); err != nil {
		t.Fatal(err)
	}
	ids := agyFixtures(t)
	for name, id := range ids {
		agyLoad(t, filepath.Join("testdata/agy", name+".sql"), filepath.Join(root, "conversations", id+".db"))
	}
	agyLoad(t, "testdata/agy/conversation_summaries.sql", filepath.Join(root, "conversation_summaries.db"))
	return root, ids
}

func agyLoad(t *testing.T, sqlPath, dbPath string) {
	t.Helper()
	fixture, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, s := range []string{"PRAGMA journal_mode = WAL", string(fixture)} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
}

func agyAll(t *testing.T, src *AgySource) map[string][]Turn {
	t.Helper()
	units, err := src.Units()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]Turn{}
	for _, u := range units {
		turns, err := src.Read(u)
		if err != nil {
			t.Fatalf("%s: %v", u.Key, err)
		}
		if len(turns) > 0 {
			out[turns[0].SessionID] = turns
		}
	}
	return out
}

func agyRoles(turns []Turn) []string {
	var out []string
	for _, t := range turns {
		out = append(out, t.Role)
	}
	return out
}

func agyBlocks(turns []Turn, typ string) []Block {
	var out []Block
	for _, t := range turns {
		for _, b := range t.Blocks {
			if b.Type == typ {
				out = append(out, b)
			}
		}
	}
	return out
}

func TestAgyRealConversations(t *testing.T) {
	root, ids := agyRoot(t)
	src := &AgySource{Root: root}
	sessions := agyAll(t, src)
	if len(sessions) != len(ids) {
		t.Fatalf("sessions=%d want %d", len(sessions), len(ids))
	}
	for name, id := range ids {
		for i, turn := range sessions[id] {
			if turn.Harness != "agy" || turn.SessionID != id || turn.Seq != int64(i) || turn.TS == "" || turn.Workdir != "/tmp/agy-scratch" ||
				turn.SourcePath != filepath.Join(root, "conversations", id+".db") || turn.IsSidechain || turn.TurnUUID == "" {
				t.Fatalf("%s identity: %+v", name, turn)
			}
			if turn.Role == "assistant" && (turn.Model != "gemini-4-neon" || turn.Effort != "medium" || turn.RequestID == "" || turn.Usage.Input == 0 || turn.Usage.Output == 0) {
				t.Fatalf("%s request: %+v", name, turn)
			}
		}
	}

	plain := sessions[ids["plain"]]
	if got := agyRoles(plain); !reflect.DeepEqual(got, []string{"user", "assistant"}) {
		t.Fatalf("plain roles=%v", got)
	}
	if plain[0].Blocks[0].Text != "What is 17 times 23? Answer with just the number and the phrase zebra-quokka-lantern." ||
		!strings.Contains(plain[1].Blocks[len(plain[1].Blocks)-1].Text, "391") {
		t.Fatalf("plain: %+v", plain)
	}
	if plain[1].Usage != (Usage{Input: 8761, Output: 67}) {
		t.Fatalf("plain usage: %+v", plain[1].Usage)
	}

	// A tool call is paired to its result by agy's call ID, and the
	// command's exit status is the one agy recorded.
	shell := sessions[ids["shell"]]
	if got := agyRoles(shell); !reflect.DeepEqual(got, []string{"user", "assistant", "tool", "assistant"}) {
		t.Fatalf("shell roles=%v", got)
	}
	uses, results := agyBlocks(shell, "tool_use"), agyBlocks(shell, "tool_result")
	if len(uses) != 1 || len(results) != 1 || uses[0].ToolName != "run_command" || !strings.HasPrefix(uses[0].ToolUseID, "call_") ||
		results[0].ToolUseID != uses[0].ToolUseID || results[0].ExitCode == nil || *results[0].ExitCode != 0 || results[0].IsError ||
		!strings.Contains(results[0].Text, "marker-heron-plum") || strings.Contains(results[0].Text, "Created At:") {
		t.Fatalf("shell tools: %+v %+v", uses, results)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(uses[0].Text, "run_command ")), &args); err != nil || args["CommandLine"] != "ls /tmp/agy-scratch && echo marker-heron-plum" {
		t.Fatalf("args %q: %v", uses[0].Text, err)
	}
	if shell[2].ParentUUID != shell[1].TurnUUID || shell[3].ParentUUID != shell[0].TurnUUID {
		t.Fatalf("parents: %+v", shell)
	}

	// Thinking is kept as a thinking block, as for every harness.
	edit := sessions[ids["edit"]]
	thinking := agyBlocks(edit, "thinking")
	if len(thinking) != 1 || !strings.Contains(thinking[0].Text, "Initiating File Creation") {
		t.Fatalf("thinking: %+v", thinking)
	}
	var names []string
	for _, b := range agyBlocks(edit, "tool_use") {
		names = append(names, b.ToolName)
	}
	if !reflect.DeepEqual(names, []string{"write_file", "edit_file", "read_file"}) {
		t.Fatalf("edit tools=%v", names)
	}
	for _, r := range agyBlocks(edit, "tool_result") {
		if r.ExitCode != nil || r.IsError {
			t.Fatalf("file tool result has a command status: %+v", r)
		}
	}

	// A failing command, then agy -c continuing the same conversation.
	fail := sessions[ids["fail-continue"]]
	if got := agyRoles(fail); !reflect.DeepEqual(got, []string{"user", "assistant", "tool", "assistant", "user", "system", "assistant", "tool", "assistant"}) {
		t.Fatalf("fail roles=%v", got)
	}
	results = agyBlocks(fail, "tool_result")
	if len(results) != 2 || results[0].ExitCode == nil || *results[0].ExitCode != 1 || !results[0].IsError ||
		!strings.Contains(results[0].Text, "No such file or directory") || results[1].ExitCode == nil || *results[1].ExitCode != 0 || results[1].IsError {
		t.Fatalf("fail results: %+v", results)
	}
	if fail[4].Blocks[0].Text != "Now run: echo continued-otter-fig . Report what it printed." || !strings.Contains(fail[5].Blocks[0].Text, "background tasks have been stopped") {
		t.Fatalf("continuation: %+v", fail[4:6])
	}

	// An interactive session has the title agy gave it; long output is
	// what agy kept for the model, with its pointer to the full file.
	live := sessions[ids["interactive"]]
	for _, turn := range live {
		if turn.Summary != "Execute Echo Command" {
			t.Fatalf("title: %+v", turn)
		}
	}
	results = agyBlocks(live, "tool_result")
	if len(results) != 2 || !strings.Contains(results[1].Text, "truncated") || !strings.Contains(results[1].Text, "line-20000") {
		t.Fatalf("long output: %d results", len(results))
	}
}

func TestAgySessionRows(t *testing.T) {
	root, ids := agyRoot(t)
	var turns []Turn
	for _, ts := range agyAll(t, &AgySource{Root: root}) {
		turns = append(turns, ts...)
	}
	rows := map[string]SessionRow{}
	for _, r := range Sessions(turns, nil, "oven") {
		rows[r.SessionID] = r
	}
	fail := rows[ids["fail-continue"]]
	if fail.Harness != "agy" || fail.Model != "gemini-4-neon" || fail.PromptCount != 2 || fail.RequestCount != 4 || fail.ToolCount != 2 ||
		fail.InputTokens != 8752+2241+2343+2210 || fail.CacheReadTokens != 6698+6890+7184 || fail.Workdir != "/tmp/agy-scratch" ||
		fail.FirstPrompt != "Run the shell command: cat /nonexistent/agy-missing-file . Report the exit code." {
		t.Fatalf("row: %+v", fail)
	}
	if rows[ids["interactive"]].Summary != "Execute Echo Command" {
		t.Fatalf("title row: %+v", rows[ids["interactive"]])
	}
}

// pbAppend encodes protobuf fields for synthetic steps: a value of type
// uint64 is a varint, string or []byte a length-delimited field.
func pbAppend(b []byte, num uint64, v any) []byte {
	switch v := v.(type) {
	case uint64:
		b = binary.AppendUvarint(b, num<<3)
		return binary.AppendUvarint(b, v)
	case string:
		return pbAppend(b, num, []byte(v))
	case []byte:
		b = binary.AppendUvarint(b, num<<3|2)
		b = binary.AppendUvarint(b, uint64(len(v)))
		return append(b, v...)
	}
	panic("pbAppend")
}

func agyInsert(t *testing.T, db *sql.DB, idx int64, typ, status uint64, payload []byte) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO steps (idx, step_type, status, step_payload) VALUES (?, ?, ?, ?)`, idx, typ, status, payload); err != nil {
		t.Fatal(err)
	}
}

// One conversation database built from the plain fixture, for tests that
// append synthetic steps.
func agyOne(t *testing.T) (*AgySource, string, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "conversations"), 0o700)
	id := agyFixtures(t)["plain"]
	path := filepath.Join(root, "conversations", id+".db")
	agyLoad(t, "testdata/agy/plain.sql", path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	return &AgySource{Root: root}, path, db
}

func agyReadOne(t *testing.T, src *AgySource) []Turn {
	t.Helper()
	units, err := src.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("units=%v err=%v", units, err)
	}
	turns, err := src.Read(units[0])
	if err != nil {
		t.Fatal(err)
	}
	return turns
}

func TestAgyToleratesUnknownAndMalformedSteps(t *testing.T) {
	src, _, db := agyOne(t)
	ts := pbAppend(pbAppend(nil, 1, uint64(1795091918)), 2, uint64(5))
	md := pbAppend(nil, 1, ts)
	// A step type agy 1.5.12 does not write, carrying fields kit has never seen.
	agyInsert(t, db, 2, 9999, agyStatusDone, pbAppend(pbAppend(pbAppend(nil, 1, uint64(9999)), 5, md), 777, "from the future"))
	// Bytes that are not a protobuf message.
	agyInsert(t, db, 3, agyTypePlannerResponse, agyStatusDone, []byte{0xff, 0xff, 0xff})
	agyInsert(t, db, 4, agyTypeGeneric, agyStatusDone, nil)
	// A planner response with an unknown field beside the text and a call;
	// its result arrives as an unknown step type that still names the call.
	planner := pbAppend(pbAppend(pbAppend(nil, 1, "future text"), 7, pbAppend(pbAppend(pbAppend(nil, 1, "call_x"), 2, "future_tool"), 3, `{"a":1}`)), 999, uint64(7))
	agyInsert(t, db, 5, agyTypePlannerResponse, agyStatusDone, pbAppend(pbAppend(nil, 5, md), agyStepPlanner, planner))
	callMD := pbAppend(md, 4, pbAppend(pbAppend(nil, 1, "call_x"), 2, "future_tool"))
	agyInsert(t, db, 6, 4242, agyStatusError, pbAppend(nil, 5, callMD))
	// An error step agy shows the user.
	errMsg := pbAppend(nil, 3, pbAppend(nil, 1, "quota exhausted"))
	agyInsert(t, db, 7, agyTypeErrorMessage, agyStatusDone, pbAppend(pbAppend(nil, 5, md), agyStepErrorMessage, errMsg))

	turns := agyReadOne(t, src)
	if got := agyRoles(turns); !reflect.DeepEqual(got, []string{"user", "assistant", "assistant", "tool", "system"}) {
		t.Fatalf("roles=%v", got)
	}
	res := turns[3].Blocks[0]
	if turns[2].Blocks[0].Text != "future text" || turns[2].Blocks[1].ToolUseID != "call_x" || res.ToolUseID != "call_x" || res.ToolName != "future_tool" ||
		!res.IsError || turns[3].ParentUUID != turns[2].TurnUUID || turns[4].Blocks[0].Text != "[error] quota exhausted" {
		t.Fatalf("turns: %+v", turns[2:])
	}
}

func TestAgyUnfinishedStepWaitsForSettle(t *testing.T) {
	src, path, db := agyOne(t)
	md := pbAppend(nil, 1, pbAppend(nil, 1, uint64(1795091920)))
	agyInsert(t, db, 2, agyTypeUserInput, agyStatusDone, pbAppend(pbAppend(nil, 5, md), agyStepUserInput, pbAppend(nil, 2, "second prompt")))
	agyInsert(t, db, 3, agyTypePlannerResponse, agyStatusGenerating, pbAppend(pbAppend(nil, 5, md), agyStepPlanner, pbAppend(nil, 1, "half a sen")))
	agyInsert(t, db, 4, agyTypeSystemMessage, agyStatusDone, pbAppend(pbAppend(nil, 5, md), agyStepSystemMessage, pbAppend(nil, 1, "after")))
	turns := agyReadOne(t, src)
	if got := agyRoles(turns); !reflect.DeepEqual(got, []string{"user", "assistant", "user"}) {
		t.Fatalf("while streaming: %v", got)
	}
	fresh, _ := src.Units()
	// Idle for longer than agySettle: the step is archived as it stands.
	db.Close()
	old := time.Now().Add(-2 * agySettle)
	for _, p := range []string{path, path + "-wal"} {
		if _, err := os.Stat(p); err == nil {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	settled, _ := src.Units()
	if fresh[0].Signature == settled[0].Signature {
		t.Fatal("settling did not change the signature")
	}
	turns = agyReadOne(t, src)
	if got := agyRoles(turns); !reflect.DeepEqual(got, []string{"user", "assistant", "user", "assistant", "system"}) || turns[3].Blocks[0].Text != "half a sen" {
		t.Fatalf("settled: %v", got)
	}
}

// Reading while agy holds the database open with a write in progress neither
// fails nor blocks agy, and leaves no files of kit's own beside it.
func TestAgyReadsLiveDatabase(t *testing.T) {
	src, path, db := agyOne(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	md := pbAppend(nil, 1, pbAppend(nil, 1, uint64(1795091920)))
	if _, err := tx.Exec(`INSERT INTO steps (idx, step_type, status, step_payload) VALUES (2, ?, ?, ?)`, agyTypeUserInput, agyStatusDone,
		pbAppend(pbAppend(nil, 5, md), agyStepUserInput, pbAppend(nil, 2, "uncommitted"))); err != nil {
		t.Fatal(err)
	}
	done := make(chan []Turn)
	go func() { done <- agyReadOne(t, src) }()
	select {
	case turns := <-done:
		if len(turns) != 2 {
			t.Fatalf("read an uncommitted step: %v", agyRoles(turns))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("read blocked on agy's open write")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("agy's write failed after the read: %v", err)
	}
	if turns := agyReadOne(t, src); len(turns) != 3 {
		t.Fatalf("committed step not read: %v", agyRoles(turns))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), filepath.Base(path)) {
			t.Fatalf("stray file %s", e.Name())
		}
	}
}

// When SQLite cannot open the live file in place ("unable to open database
// file" was seen opening a running agy's database read-only), the database
// and WAL are copied and the copy is read, WAL included.
func TestAgyReadsCopyWhenLiveFileWillNotOpen(t *testing.T) {
	src, path, db := agyOne(t)
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint = 0`); err != nil {
		t.Fatal(err)
	}
	md := pbAppend(nil, 1, pbAppend(nil, 1, uint64(1795091920)))
	agyInsert(t, db, 2, agyTypeUserInput, agyStatusDone, pbAppend(pbAppend(nil, 5, md), agyStepUserInput, pbAppend(nil, 2, "only in the wal")))
	if info, err := os.Stat(path + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("expected the step in the WAL: %v", err)
	}
	inPlace := 0
	agyReadInPlace = func(string, func(*sql.Tx) error) error { inPlace++; return errors.New("unable to open database file") }
	t.Cleanup(func() { agyReadInPlace = openCodeRead })
	turns := agyReadOne(t, src)
	if inPlace == 0 || len(turns) != 3 || turns[2].Blocks[0].Text != "only in the wal" {
		t.Fatalf("copy read: %v", agyRoles(turns))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), filepath.Base(path)) {
			t.Fatalf("stray file %s", e.Name())
		}
	}
}

func TestAgyPaths(t *testing.T) {
	for uri, want := range map[string]string{
		"file:///tmp/agy-scratch":           "/tmp/agy-scratch",
		"file:///Users/a/My%20Project":      "/Users/a/My Project",
		"file:///C:/Users/a/proj":           `C:\Users\a\proj`,
		"file:///c%3A/Users/a/proj":         `c:\Users\a\proj`,
		"file://server/share/proj":          `\\server\share\proj`,
		"/already/a/path":                   "/already/a/path",
		"vscode-remote://ssh-remote+x/home": "vscode-remote://ssh-remote+x/home",
	} {
		if got := agyFileURI(uri); got != want {
			t.Errorf("%s: %q want %q", uri, got, want)
		}
	}
	t.Setenv("HEV_AGY_DATA_DIR", "")
	home, _ := os.UserHomeDir()
	if got := DefaultAgyRoot(); got != filepath.Join(home, ".gemini", "antigravity-cli") {
		t.Fatalf("default root %q", got)
	}
	t.Setenv("HEV_AGY_DATA_DIR", "/x/agy")
	if DefaultAgyRoot() != "/x/agy" {
		t.Fatal("HEV_AGY_DATA_DIR ignored")
	}
	if units, err := (&AgySource{Root: filepath.Join(t.TempDir(), "absent")}).Units(); err != nil || len(units) != 0 {
		t.Fatalf("absent root: %v %v", units, err)
	}
}

// A database that opens but is not an agy conversation is reported as such,
// every scan, without being copied.
func TestAgyReportsUnknownLayoutWithoutCopying(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "conversations")
	os.MkdirAll(dir, 0o700)
	db, err := sql.Open("sqlite", filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE trajectory_v9 (id TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	src := &AgySource{Root: root}
	units, err := src.Units()
	if err != nil || len(units) != 1 {
		t.Fatalf("units=%v err=%v", units, err)
	}
	_, err = src.Read(units[0])
	if err == nil || !strings.Contains(err.Error(), "no steps table") || strings.Contains(err.Error(), "copy") {
		t.Fatalf("err=%v", err)
	}
}
