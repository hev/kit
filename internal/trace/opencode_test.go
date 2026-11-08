package trace

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const openCodeFixture = "testdata/opencode/opencode-1.19.5.sql"

// openCodeDB builds a WAL-mode opencode.db from the 1.19.5 real-session SQL fixture.
func openCodeDB(t *testing.T, statements ...string) (string, *sql.DB) {
	t.Helper()
	return openCodeDBFrom(t, openCodeFixture, statements...)
}

func openCodeDBFrom(t *testing.T, path string, statements ...string) (string, *sql.DB) {
	t.Helper()
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "opencode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, s := range append([]string{"PRAGMA journal_mode = WAL", string(fixture)}, statements...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return root, db
}

func openCodeSessions(t *testing.T, src *OpenCodeSource) map[string][]Turn {
	t.Helper()
	units, err := src.Units()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]Turn{}
	for _, u := range units {
		turns, err := src.Read(u)
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) == 0 {
			t.Fatalf("%s: no turns", u.Key)
		}
		out[turns[0].SessionID] = turns
	}
	return out
}

func TestOpenCodeRealSessions(t *testing.T) {
	root, _ := openCodeDB(t)
	sessions := openCodeSessions(t, &OpenCodeSource{Root: root})
	if len(sessions) != 5 {
		t.Fatalf("sessions=%d", len(sessions))
	}
	first := sessions["ses_e48a2f02bffeHbDXxurkNl3k8y"]
	var roles []string
	for i, turn := range first {
		roles = append(roles, turn.Role)
		if turn.Harness != "opencode" || turn.Workdir != "/tmp/ocproj" || turn.Seq != int64(i) || turn.TS == "" || turn.Summary != "Canary marker retrieval from README" || turn.SourcePath != filepath.Join(root, "opencode.db") || turn.IsSidechain {
			t.Fatalf("identity: %+v", turn)
		}
	}
	want := []string{"user", "assistant", "tool", "assistant", "tool", "tool", "assistant", "user", "assistant", "tool", "assistant"}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("roles=%v", roles)
	}
	if first[0].TS != "2026-11-07T18:54:51.899Z" || !strings.Contains(first[0].Blocks[0].Text, "OPENCODE_CAPTURE_CANARY_ONE") || first[0].Model != "big-pickle" {
		t.Fatalf("user turn: %+v", first[0])
	}
	call := first[1]
	if call.Model != "big-pickle" || call.RequestID != call.TurnUUID || call.Usage != (Usage{Input: 7692, Output: 55, CacheRead: 1792}) || call.ParentUUID != first[0].TurnUUID {
		t.Fatalf("assistant request: %+v", call)
	}
	use := call.Blocks[1]
	if use.Type != "tool_use" || use.ToolName != "skill" || use.Text != `skill {"name":"canary-notes"}` || use.ToolUseID == "" {
		t.Fatalf("tool use: %+v", use)
	}
	result := first[2].Blocks[0]
	if result.Type != "tool_result" || result.ToolUseID != use.ToolUseID || result.ToolName != "skill" || !strings.Contains(result.Text, "PELICAN-7") || result.IsError {
		t.Fatalf("tool result: %+v", result)
	}
	if loads := use.SkillLoads; len(loads) != 1 || loads[0].Name != "canary-notes" || loads[0].TurnUUID != call.TurnUUID {
		t.Fatalf("skill tool load: %+v", loads)
	}
	failed := first[9].Blocks[0]
	if !failed.IsError || failed.ToolName != "read" || !strings.Contains(failed.Text, "File not found") {
		t.Fatalf("tool error: %+v", failed)
	}
	last := first[len(first)-1]
	if last.Blocks[0].Type != "thinking" && last.Blocks[0].Type != "text" {
		t.Fatalf("final answer: %+v", last)
	}
	var thinking int
	for _, turns := range sessions {
		for _, turn := range turns {
			for _, b := range turn.Blocks {
				if b.Type == "thinking" && b.Text != "" {
					thinking++
				}
			}
		}
	}
	if thinking == 0 {
		t.Fatal("reasoning parts were dropped")
	}

	// A subagent's task runs in a child session linked to its parent.
	child := sessions["ses_e489fda16ffewIo99FGGRcf6YR"]
	if !child[0].IsSidechain || child[0].AgentID != "ses_e489fda16ffewIo99FGGRcf6YR" || child[0].ParentAgentID != "ses_e489fe34dffeBLobPUyE5UX8Cn" {
		t.Fatalf("child session: %+v", child[0])
	}
	var task bool
	for _, turn := range sessions["ses_e489fe34dffeBLobPUyE5UX8Cn"] {
		for _, b := range turn.Blocks {
			task = task || (b.Type == "tool_use" && b.ToolName == "task")
		}
	}
	if !task {
		t.Fatal("parent lost its task call")
	}

	// An attached file: the name stays on the prompt, injected contents are system context.
	attached := sessions["ses_e489fb913ffe4AvJqF9B0VpXiP"]
	if attached[0].Role != "user" || len(attached[0].Blocks) != 2 || attached[0].Blocks[1].Text != "[file omitted: README.md]" {
		t.Fatalf("attachment prompt: %+v", attached[0])
	}
	if attached[1].Role != "system" || attached[2].Role != "system" || !strings.Contains(attached[2].Blocks[0].Text, "1: hello") {
		t.Fatalf("synthetic context: %+v %+v", attached[1], attached[2])
	}
	if rows := Sessions(attached, nil, ""); len(rows) != 1 || rows[0].PromptCount != 1 || !strings.Contains(rows[0].FirstPrompt, "CANARY_FOUR") {
		t.Fatalf("session row: %+v", rows)
	}

	// Reading SKILL.md with OpenCode's read tool (filePath) is a skill load with a path.
	var paths []string
	for _, turn := range sessions["ses_e489f888bffe76OQe8QvoisDfo"] {
		for _, b := range turn.Blocks {
			for _, l := range b.SkillLoads {
				paths = append(paths, l.Name+" "+l.Path+" "+l.Source)
			}
		}
	}
	if !reflect.DeepEqual(paths, []string{"canary-notes /tmp/ocproj/.opencode/skills/canary-notes/SKILL.md /tmp/ocproj/.opencode/skills"}) {
		t.Fatalf("read skill loads: %v", paths)
	}
	rows := Sessions(first, nil, "")
	if len(rows) != 1 || rows[0].Harness != "opencode" || !reflect.DeepEqual([]string(rows[0].Skills), []string{"canary-notes"}) || rows[0].InputTokens == 0 {
		t.Fatalf("session row: %+v", rows)
	}
}

func TestOpenCodeIncrementalRescan(t *testing.T) {
	root, db := openCodeDB(t)
	src := &OpenCodeSource{Root: root}
	before, err := src.Units()
	if err != nil {
		t.Fatal(err)
	}
	unit := before[len(before)-1]
	old, err := src.Read(unit)
	if err != nil {
		t.Fatal(err)
	}
	// OpenCode appends a turn, then streams into the part row in place.
	for _, s := range []string{
		`INSERT INTO message VALUES('msg_zz_new','ses_e48a2f02bffeHbDXxurkNl3k8y',1794078000000,1794078000000,'{"role":"user","time":{"created":1794078000000},"agent":"build","model":{"providerID":"opencode","modelID":"big-pickle"}}')`,
		`INSERT INTO part VALUES('prt_zz_new','msg_zz_new','ses_e48a2f02bffeHbDXxurkNl3k8y',1794078000001,1794078000001,'{"type":"text","text":"partial"}')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	signature := func() string {
		units, err := src.Units()
		if err != nil {
			t.Fatal(err)
		}
		for i, u := range units {
			if u.Key != before[i].Key {
				t.Fatalf("unit order changed: %s", u.Key)
			}
			if u.Key != unit.Key && u.Signature != before[i].Signature {
				t.Fatalf("untouched session re-signed: %s", u.Key)
			}
		}
		return units[len(units)-1].Signature
	}
	grown := signature()
	if grown == unit.Signature {
		t.Fatal("new turn did not change the unit signature")
	}
	if _, err := db.Exec(`UPDATE part SET data='{"type":"text","text":"OPENCODE_CAPTURE_INCREMENTAL"}', time_updated=1794078000002 WHERE id='prt_zz_new'`); err != nil {
		t.Fatal(err)
	}
	if signature() == grown {
		t.Fatal("in-place part update did not change the unit signature")
	}
	turns, err := src.Read(Unit{Key: unit.Key})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != len(old)+1 {
		t.Fatalf("turns=%d want %d", len(turns), len(old)+1)
	}
	seen := map[string]bool{}
	for i, turn := range turns {
		if seen[turn.TurnUUID] {
			t.Fatalf("duplicate turn %s", turn.TurnUUID)
		}
		seen[turn.TurnUUID] = true
		if i < len(old) && (turn.TurnUUID != old[i].TurnUUID || turn.Seq != old[i].Seq) {
			t.Fatalf("existing turn %d changed identity", i)
		}
	}
	if added := turns[len(turns)-1]; added.TurnUUID != "msg_zz_new" || added.Blocks[0].Text != "OPENCODE_CAPTURE_INCREMENTAL" {
		t.Fatalf("new turn: %+v", added)
	}
}

func TestOpenCodeReadsWhileOpenCodeWrites(t *testing.T) {
	root, _ := openCodeDB(t)
	path := filepath.Join(root, "opencode.db")
	writer, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`INSERT INTO part VALUES('prt_zz_uncommitted','msg_1b75d0ffb001ZWZQhw71BYwjim','ses_e48a2f02bffeHbDXxurkNl3k8y',1,1,'{"type":"text","text":"UNCOMMITTED"}')`); err != nil {
		t.Fatal(err)
	}
	// An open write transaction neither blocks the scan nor leaks into it.
	for _, turns := range openCodeSessions(t, &OpenCodeSource{Root: root}) {
		for _, turn := range turns {
			for _, b := range turn.Blocks {
				if strings.Contains(b.Text, "UNCOMMITTED") {
					t.Fatal("read an uncommitted write")
				}
			}
		}
	}
	if _, err := writer.Exec("COMMIT"); err != nil {
		t.Fatal(err)
	}
	// No reader snapshot survives the scan: a full checkpoint completes.
	var busy, logFrames, checkpointed int
	if err := writer.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil || busy != 0 {
		t.Fatalf("checkpoint blocked: busy=%d err=%v", busy, err)
	}
}

func TestOpenCodeMalformedAndPartialData(t *testing.T) {
	const session = "ses_e48a2f02bffeHbDXxurkNl3k8y"
	for name, tc := range map[string]struct {
		statement, err string
	}{
		"invalid part JSON":   {`INSERT INTO part VALUES('prt_zz','msg_1b75d0ffb001ZWZQhw71BYwjim','` + session + `',1,1,'{"type":"text","text":"cut off')`, "invalid JSON"},
		"invalid message":     {`UPDATE message SET data='not json' WHERE id='msg_1b75d0ffb001ZWZQhw71BYwjim'`, "invalid JSON"},
		"unknown part type":   {`INSERT INTO part VALUES('prt_zz','msg_1b75d0ffb001ZWZQhw71BYwjim','` + session + `',1,1,'{"type":"hologram"}')`, "unsupported type"},
		"unknown role":        {`UPDATE message SET data='{"role":"narrator"}' WHERE id='msg_1b75d0ffb001ZWZQhw71BYwjim'`, "unsupported role"},
		"session disappeared": {`DELETE FROM session WHERE id='` + session + `'`, "no rows"},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := openCodeDB(t)
			src := &OpenCodeSource{Root: root}
			units, err := src.Units()
			if err != nil {
				t.Fatal(err)
			}
			// Apply the damage to the scanned database after enumeration, as a
			// concurrent OpenCode write would.
			live, err := sql.Open("sqlite", filepath.Join(root, "opencode.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer live.Close()
			if _, err := live.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			var failed error
			for _, u := range units {
				if _, err := src.Read(u); err != nil {
					failed = err
				}
			}
			if failed == nil || !strings.Contains(failed.Error(), tc.err) {
				t.Fatalf("err=%v want %q", failed, tc.err)
			}
			if strings.Contains(failed.Error(), "cut off") {
				t.Fatal("error echoed transcript content")
			}
		})
	}

	t.Run("in-flight tool call", func(t *testing.T) {
		root, _ := openCodeDB(t, `UPDATE part SET data=json_set(data,'$.state.status','running') WHERE id='prt_1b75d1a35001A49IHJu0Drzmzm'`)
		turns := openCodeSessions(t, &OpenCodeSource{Root: root})[session]
		if turns[1].Blocks[1].Type != "tool_use" || turns[2].Role != "assistant" {
			t.Fatalf("running call should have a tool_use and no result yet: %+v", turns[2])
		}
	})
	t.Run("missing data directory", func(t *testing.T) {
		units, err := (&OpenCodeSource{Root: filepath.Join(t.TempDir(), "absent")}).Units()
		if err != nil || len(units) != 0 {
			t.Fatalf("units=%v err=%v", units, err)
		}
	})
	t.Run("corrupt database", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "opencode.db"), []byte("SQLite format 3\x00 truncated"), 0o600)
		os.WriteFile(filepath.Join(root, "unrelated.db"), []byte("ignored"), 0o600)
		src := &OpenCodeSource{Root: root}
		units, err := src.Units()
		if err != nil || len(units) != 1 {
			t.Fatalf("units=%+v err=%v", units, err)
		}
		if _, err := src.Read(units[0]); err == nil || !strings.Contains(err.Error(), "opencode.db") {
			t.Fatalf("corrupt database read: %v", err)
		}
	})
	t.Run("malformed unit key", func(t *testing.T) {
		if _, err := (&OpenCodeSource{}).Read(Unit{Key: "opencode.db"}); err == nil {
			t.Fatal("missing session id accepted")
		}
	})
}

func TestOpenCodeChannelDatabasesAndRoot(t *testing.T) {
	root, db := openCodeDB(t)
	if _, err := db.Exec("VACUUM INTO ?", filepath.Join(root, "opencode-beta.db")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"opencode.db-wal", "opencode.db-shm", "other.db"} {
		os.WriteFile(filepath.Join(root, name+".x"), nil, 0o600)
	}
	units, err := (&OpenCodeSource{Root: root}).Units()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 10 || !strings.HasPrefix(units[0].Key, filepath.Join(root, "opencode-beta.db")+"#") || !units[2].IsSidechain {
		t.Fatalf("units: %+v", units)
	}

	t.Setenv("HEV_OPENCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg"))
	if got := DefaultOpenCodeRoot(); got != filepath.Join(root, "xdg", "opencode") {
		t.Fatalf("xdg root %q", got)
	}
	t.Setenv("HEV_OPENCODE_DATA_DIR", root)
	if DefaultOpenCodeRoot() != root {
		t.Fatal("override ignored")
	}
	t.Setenv("HEV_OPENCODE_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	home, _ := os.UserHomeDir()
	if got := DefaultOpenCodeRoot(); got != filepath.Join(home, ".local", "share", "opencode") {
		t.Fatalf("default root %q", got)
	}
}

func TestOpenCodeWindowsPaths(t *testing.T) {
	// OpenCode on Windows stores directories with forward slashes.
	root, _ := openCodeDB(t,
		`UPDATE session SET directory='C:/Users/test/proj'`,
		`UPDATE message SET data=json_set(data,'$.path.cwd','C:/Users/test/proj') WHERE json_extract(data,'$.path.cwd') IS NOT NULL`,
		`UPDATE part SET data=json_set(data,'$.state.input.filePath','C:/Users/test/proj/.opencode/skills/canary-notes/SKILL.md') WHERE id='prt_1b760822e001mETl6jhaVSEbhs'`,
	)
	var loads []SkillLoad
	for _, turns := range openCodeSessions(t, &OpenCodeSource{Root: root}) {
		for _, turn := range turns {
			if turn.Workdir != `C:\Users\test\proj` || !WindowsPath(turn.Workdir) {
				t.Fatalf("workdir %q", turn.Workdir)
			}
			for _, b := range turn.Blocks {
				for _, l := range b.SkillLoads {
					if l.Path != "" {
						loads = append(loads, l)
					}
				}
			}
		}
	}
	if len(loads) != 1 || loads[0].Path != "C:/Users/test/proj/.opencode/skills/canary-notes/SKILL.md" || loads[0].Source != "C:/Users/test/proj/.opencode/skills" {
		t.Fatalf("windows skill read: %+v", loads)
	}
	for in, want := range map[string]string{
		"C:/Users/test/proj":    `C:\Users\test\proj`,
		"//server/share/proj":   `\\server\share\proj`,
		"/home/test/proj":       "/home/test/proj",
		`D:\already\native`:     `D:\already\native`,
		"relative/not/absolute": "relative/not/absolute",
	} {
		if got := openCodeDir(in); got != want {
			t.Errorf("openCodeDir(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		`C:\Users\test\AppData\opencode.db`: "file:///C:/Users/test/AppData/opencode.db?",
		"/home/test/.local/share/a b/x.db":  "file:///home/test/.local/share/a%20b/x.db?",
	} {
		if got := openCodeDSN(in, false); !strings.HasPrefix(got, want) || !strings.Contains(got, "mode=ro") || strings.Contains(got, "immutable") {
			t.Errorf("openCodeDSN(%q)=%q", in, got)
		}
	}
	if got := openCodeDSN(`\\server\share\opencode\opencode.db`, true); !strings.HasPrefix(got, "file:////server/share/opencode/opencode.db?") || !strings.Contains(got, "immutable=1") {
		t.Errorf("UNC DSN %q", got)
	}
	// Session IDs never contain '#', so a '#' in the data directory is safe.
	if _, err := (&OpenCodeSource{}).Read(Unit{Key: `C:\Users\a#b\opencode.db#ses_missing`}); err == nil || !strings.Contains(err.Error(), "ses_missing") {
		t.Fatalf("windows key parse: %v", err)
	}
}

// The shared shell-read boundary cases, replayed as OpenCode bash tool parts.
func TestOpenCodeSkillCommandBoundaries(t *testing.T) {
	f, err := os.Open("testdata/skills/pi-boundaries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	statements := []string{`INSERT INTO session(id, project_id, slug, directory, title, version, time_created, time_updated) VALUES('ses_boundaries','global','b','/repo','boundaries','1.19.5',1,1)`}
	want := map[string]int{}
	dec := json.NewDecoder(f)
	for i := 0; dec.More(); i++ {
		var e struct {
			ID      string `json:"id"`
			Message struct {
				Content []struct {
					Type      string          `json:"type"`
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := dec.Decode(&e); err != nil {
			t.Fatal(err)
		}
		if len(e.Message.Content) == 0 {
			continue
		}
		c := e.Message.Content[0]
		part, _ := json.Marshal(map[string]any{"type": "tool", "tool": c.Name, "callID": e.ID, "state": map[string]any{"status": "completed", "input": c.Arguments, "output": "", "title": "", "metadata": map[string]any{}, "time": map[string]any{"start": i, "end": i}}})
		statements = append(statements,
			fmt.Sprintf(`INSERT INTO message VALUES('msg_%03d','ses_boundaries',%d,%d,'{"role":"assistant","modelID":"test","providerID":"test","tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}}')`, i, i+10, i+10),
			fmt.Sprintf(`INSERT INTO part VALUES('prt_%03d','msg_%03d','ses_boundaries',%d,%d,'%s')`, i, i, i+10, i+10, strings.ReplaceAll(string(part), "'", "''")))
		want[e.ID] = 0
	}
	for _, id := range []string{"spaces", "read-pipe", "awk", "rg", "sed", "multiline", "nl", "head", "tail", "less", "bat"} {
		want[id] = 1
	}
	root, _ := openCodeDB(t, statements...)
	turns, err := (&OpenCodeSource{Root: root}).Read(Unit{Key: filepath.Join(root, "opencode.db") + "#ses_boundaries"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, turn := range turns {
		for _, b := range turn.Blocks {
			if b.Type != "tool_use" {
				continue
			}
			got[b.ToolUseID] = len(b.SkillLoads)
			for _, l := range b.SkillLoads {
				if l.Name != "actual" || !strings.HasSuffix(l.Path, "/skills/actual/SKILL.md") || l.TurnUUID != turn.TurnUUID {
					t.Fatalf("%s: %+v", b.ToolUseID, l)
				}
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loads %v want %v", got, want)
	}
}

// One database that cannot be scanned costs only its own sessions. An empty or
// never-migrated one is skipped; one whose tables match neither layout (a
// future storage format) is reported, not skipped.
func TestOpenCodeBadDatabaseIsolated(t *testing.T) {
	root, _ := openCodeDB(t)
	os.WriteFile(filepath.Join(root, "opencode-empty.db"), nil, 0o600)
	for name, stmt := range map[string]string{
		"opencode-beta.db":  "CREATE TABLE __drizzle_migrations(id INTEGER)",
		"opencode-gamma.db": "CREATE TABLE session_v3(id TEXT)",
	} {
		db, err := sql.Open("sqlite", filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	os.WriteFile(filepath.Join(root, "opencode-rc.db"), []byte("SQLite format 3\x00 truncated"), 0o600)
	src := &OpenCodeSource{Root: root}
	units, err := src.Units()
	if err != nil {
		t.Fatal(err)
	}
	var good, bad, unknown int
	for _, u := range units {
		_, err := src.Read(u)
		switch {
		case err == nil && strings.HasPrefix(u.Key, filepath.Join(root, "opencode.db")+"#"):
			good++
		case err != nil && u.Key == filepath.Join(root, "opencode-rc.db")+"#":
			bad++
		case err != nil && u.Key == filepath.Join(root, "opencode-gamma.db")+"#" && strings.Contains(err.Error(), "no OpenCode session layout Kit recognises"):
			unknown++
		default:
			t.Errorf("unit %s: %v", u.Key, err)
		}
	}
	if good != 5 || bad != 1 || unknown != 1 {
		t.Fatalf("good=%d bad=%d unknown=%d units=%+v", good, bad, unknown, units)
	}
}

// A closed OpenCode leaves no WAL; scanning then creates no -wal or -shm of
// its own and works in a directory it cannot write.
func TestOpenCodeClosedDatabaseLeavesNoFiles(t *testing.T) {
	root, db := openCodeDB(t)
	db.Close()
	path := filepath.Join(root, "opencode.db")
	if _, err := os.Stat(path + "-wal"); !os.IsNotExist(err) {
		t.Fatalf("clean close left a WAL: %v", err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) })
	if got := len(openCodeSessions(t, &OpenCodeSource{Root: root})); got != 5 {
		t.Fatalf("sessions=%d", got)
	}
	for _, side := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + side); !os.IsNotExist(err) {
			t.Errorf("scan created %s: %v", side, err)
		}
	}
}

// An unfinished assistant message holds back the rest of its session until
// OpenCode completes it, or until the session has been idle long enough that
// OpenCode was evidently killed mid-stream.
func TestOpenCodeStreamingAndSettledMessages(t *testing.T) {
	const session = "ses_e48a2f02bffeHbDXxurkNl3k8y"
	insert := func(updated int64) []string {
		return []string{
			fmt.Sprintf(`INSERT INTO message VALUES('msg_zz_a','%s',%d,%d,'{"role":"user","time":{"created":1},"model":{"providerID":"p","modelID":"m","variant":"high"}}')`, session, updated, updated),
			fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_a','msg_zz_a','%s',%d,%d,'{"type":"text","text":"ASK"}')`, session, updated, updated),
			fmt.Sprintf(`INSERT INTO message VALUES('msg_zz_b','%s',%d,%d,'{"role":"assistant","parentID":"msg_zz_a","modelID":"m","time":{"created":1},"tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}}}')`, session, updated+1, updated+1),
			fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_b','msg_zz_b','%s',%d,%d,'{"type":"text","text":"HALF"}')`, session, updated+1, updated+1),
			fmt.Sprintf(`INSERT INTO message VALUES('msg_zz_c','%s',%d,%d,'{"role":"user","time":{"created":1},"model":{"providerID":"p","modelID":"m"}}')`, session, updated+2, updated+2),
			fmt.Sprintf(`INSERT INTO part VALUES('prt_zz_c','msg_zz_c','%s',%d,%d,'{"type":"text","text":"AFTER"}')`, session, updated+2, updated+2),
		}
	}
	read := func(root string) (Unit, string, map[string]string) {
		t.Helper()
		src := &OpenCodeSource{Root: root}
		units, err := src.Units()
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range units {
			if !strings.HasSuffix(u.Key, "#"+session) {
				continue
			}
			turns, err := src.Read(u)
			if err != nil {
				t.Fatal(err)
			}
			var text []string
			effort := map[string]string{}
			for _, turn := range turns {
				effort[turn.TurnUUID] = turn.Effort
				for _, b := range turn.Blocks {
					text = append(text, b.Text)
				}
			}
			return u, strings.Join(text, "\n"), effort
		}
		t.Fatal("session not listed")
		return Unit{}, "", nil
	}

	root, _ := openCodeDB(t, insert(time.Now().UnixMilli())...)
	u, text, _ := read(root)
	if !strings.HasSuffix(u.Signature, "-streaming") || !strings.Contains(text, "ASK") || strings.Contains(text, "HALF") || strings.Contains(text, "AFTER") {
		t.Fatalf("streaming: sig=%s\n%s", u.Signature, text)
	}

	root, _ = openCodeDB(t, insert(time.Now().Add(-2*openCodeSettle).UnixMilli())...)
	u, text, effort := read(root)
	if strings.HasSuffix(u.Signature, "-streaming") || !strings.Contains(text, "HALF") || !strings.Contains(text, "AFTER") {
		t.Fatalf("settled: sig=%s\n%s", u.Signature, text)
	}
	if effort["msg_zz_a"] != "high" || effort["msg_zz_b"] != "high" || effort["msg_zz_c"] != "" {
		t.Fatalf("effort carried across messages: %v", effort)
	}
}
