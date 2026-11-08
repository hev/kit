package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const openCodeV2Fixture = "testdata/opencode/opencode-2.2.6.sql"

const (
	ocV2SkillBash = "ses_e47afe011ffegskH11HxKTuBSG"
	ocV2Canary    = "ses_e47ae7268ffeU0nY0ozzLJ5ThP"
	ocV2Edit      = "ses_e47ae69cbffe6bQkcB1Zth4rKK"
	ocV2Parent    = "ses_e47ae4119ffevkVGAa4xWmDYU4"
	ocV2Child     = "ses_e47ae3966ffe8TEgTeo68BQFpR"
	ocV2Continued = "ses_e48a2f02bffeHbDXxurkNl3k8y" // a 1.19.5 session continued under 2.2.6
)

func openCodeV2DB(t *testing.T, statements ...string) string {
	t.Helper()
	root, _ := openCodeDBFrom(t, openCodeV2Fixture, statements...)
	return root
}

func turnRoles(turns []Turn) []string {
	var out []string
	for _, t := range turns {
		out = append(out, t.Role)
	}
	return out
}

func TestOpenCodeV2RealSessions(t *testing.T) {
	root := openCodeV2DB(t)
	sessions := openCodeSessions(t, &OpenCodeSource{Root: root})
	if len(sessions) != 11 { // 5 upgraded from 1.19.5 + 6 written by 2.2.6
		t.Fatalf("sessions=%d", len(sessions))
	}

	first := sessions[ocV2SkillBash]
	if want := []string{"user", "assistant", "tool", "tool", "tool", "assistant"}; !reflect.DeepEqual(turnRoles(first), want) {
		t.Fatalf("roles=%v", turnRoles(first))
	}
	for i, turn := range first {
		if turn.Harness != "opencode" || turn.Workdir != "/tmp/ocproj" || turn.Seq != int64(i) || turn.TS == "" || turn.Summary != "oc2 skill+bash" || turn.SourcePath != filepath.Join(root, "opencode.db") || turn.IsSidechain {
			t.Fatalf("identity: %+v", turn)
		}
	}
	call := first[1]
	if call.Model != "big-pickle" || call.Effort != "" || call.RequestID != call.TurnUUID || call.ParentUUID != first[0].TurnUUID || call.Usage.Input == 0 || call.Blocks[0].Type != "thinking" {
		t.Fatalf("assistant request: %+v", call)
	}
	var uses []string
	for _, b := range call.Blocks[1:] {
		uses = append(uses, b.Text)
	}
	if want := []string{`skill {"id":"e2e-notes"}`, `shell {"command":"ls","workdir":"/tmp/ocproj"}`, `read {"path":"/tmp/ocproj/README.md"}`}; !reflect.DeepEqual(uses, want) {
		t.Fatalf("tool uses: %q", uses)
	}
	failed := first[2].Blocks[0]
	if failed.Type != "tool_result" || !failed.IsError || failed.ToolUseID != call.Blocks[1].ToolUseID || failed.Text != "[tool.execution] Unable to load skill e2e-notes" {
		t.Fatalf("failed skill: %+v", failed)
	}
	if first[2].TurnUUID != call.TurnUUID+"#"+call.Blocks[1].ToolUseID || first[3].Blocks[0].Text != "README.md\n" || !strings.Contains(first[4].Blocks[0].Text, "1: hello") {
		t.Fatalf("tool results: %+v %+v %+v", first[2], first[3], first[4])
	}

	// The skill tool takes an id in 2.x.
	canary := sessions[ocV2Canary]
	var loads []string
	for _, turn := range canary {
		for _, b := range turn.Blocks {
			for _, l := range b.SkillLoads {
				loads = append(loads, l.Name+"@"+l.TurnUUID)
			}
		}
	}
	if !reflect.DeepEqual(loads, []string{"canary-notes@" + canary[1].TurnUUID}) || !strings.Contains(canary[2].Blocks[0].Text, "PELICAN-7") {
		t.Fatalf("skill loads: %v", loads)
	}
	if rows := Sessions(canary, nil, ""); len(rows) != 1 || !reflect.DeepEqual([]string(rows[0].Skills), []string{"canary-notes"}) || rows[0].PromptCount != 1 {
		t.Fatalf("session row: %+v", rows)
	}

	// Edits, a continued prompt and a failed read.
	edit := sessions[ocV2Edit]
	var prompts, errs int
	var edited bool
	for _, turn := range edit {
		for _, b := range turn.Blocks {
			prompts += map[bool]int{true: 1}[turn.Role == "user" && b.Type == "text"]
			errs += map[bool]int{true: 1}[b.IsError]
			edited = edited || (b.ToolName == "edit" && strings.Contains(b.Text, `"newString":"notes: alpha\nbeta\n"`))
		}
	}
	if prompts != 2 || errs != 1 || !edited || !strings.Contains(edit[0].Blocks[0].Text, `<file name="notes.txt">`) {
		t.Fatalf("edit session: prompts=%d errors=%d edited=%v", prompts, errs, edited)
	}

	// A subagent runs in a child session linked to its parent.
	child := sessions[ocV2Child]
	if !child[0].IsSidechain || child[0].AgentID != ocV2Child || child[0].ParentAgentID != ocV2Parent || child[0].Summary != "Count files with ls" {
		t.Fatalf("child session: %+v", child[0])
	}
	if b := sessions[ocV2Parent][1].Blocks[1]; b.ToolName != "subagent" || !strings.Contains(sessions[ocV2Parent][2].Blocks[0].Text, ocV2Child) {
		t.Fatalf("parent subagent call: %+v", b)
	}
	units, _ := (&OpenCodeSource{Root: root}).Units()
	for _, u := range units {
		if u.IsSidechain != (strings.HasSuffix(u.Key, ocV2Child) || strings.HasSuffix(u.Key, "ses_e489fda16ffewIo99FGGRcf6YR")) {
			t.Fatalf("sidechain flag: %+v", u)
		}
	}
}

// An upgrade keeps every turn ID the 1.x reader produced: the v1 rows are
// read as before, and the messages 2.x copied (or backdated) are skipped.
func TestOpenCodeV2UpgradeKeepsV1Turns(t *testing.T) {
	before := openCodeSessions(t, &OpenCodeSource{Root: func() string { r, _ := openCodeDB(t); return r }()})
	after := openCodeSessions(t, &OpenCodeSource{Root: openCodeV2DB(t)})
	strip := func(turns []Turn) []Turn {
		out := append([]Turn(nil), turns...)
		for i := range out {
			out[i].SourcePath = ""
		}
		return out
	}
	for id, old := range before {
		got := after[id]
		if len(got) < len(old) || !reflect.DeepEqual(strip(got[:len(old)]), strip(old)) {
			t.Fatalf("%s: v1 turns changed after upgrade", id)
		}
		extra := got[len(old):]
		if id != ocV2Continued {
			if len(extra) != 0 {
				t.Fatalf("%s: upgrade added turns: %+v", id, extra)
			}
			continue
		}
		if want := []string{"user", "assistant", "tool", "assistant"}; !reflect.DeepEqual(turnRoles(extra), want) {
			t.Fatalf("continued roles=%v", turnRoles(extra))
		}
		if !strings.Contains(extra[0].Blocks[0].Text, "continued-in-2x") || extra[2].Blocks[0].Text != "continued-in-2x\n" || extra[0].Seq != int64(len(old)) {
			t.Fatalf("continued turns: %+v", extra)
		}
	}
	for _, turns := range after {
		for _, turn := range turns {
			for _, b := range turn.Blocks {
				if strings.Contains(b.Text, "The available tools have changed") {
					t.Fatalf("backdated migration notice archived: %+v", turn)
				}
			}
		}
	}
}

func TestOpenCodeV2IncrementalAndStreaming(t *testing.T) {
	root, db := openCodeDBFrom(t, openCodeV2Fixture)
	src := &OpenCodeSource{Root: root}
	key := filepath.Join(root, "opencode.db") + "#" + ocV2Canary
	unit := func() Unit {
		units, err := src.Units()
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range units {
			if u.Key == key {
				return u
			}
		}
		t.Fatal("unit missing")
		return Unit{}
	}
	exec := func(statements ...string) {
		t.Helper()
		for _, s := range statements {
			if _, err := db.Exec(s); err != nil {
				t.Fatal(err)
			}
		}
	}
	u0 := unit()
	old, err := src.Read(u0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	exec(
		fmt.Sprintf(`INSERT INTO session_message VALUES('msg_zz_ask','%s','user',100,%d,%d,'{"time":{"created":%d},"text":"OPENCODE_V2_PROMPT","files":[]}')`, ocV2Canary, now, now, now),
		fmt.Sprintf(`INSERT INTO session_message VALUES('msg_zz_reply','%s','assistant',101,%d,%d,'{"time":{"created":%d},"agent":"build","model":{"id":"big-pickle","providerID":"opencode","variant":"high"},"content":[{"type":"text","text":"half a repl"},{"type":"tool","id":"call_zz","name":"read","state":{"status":"streaming","input":"{\"pa"},"time":{"created":%d}}]}')`, ocV2Canary, now+1, now+1, now+1, now+1),
	)
	u1 := unit()
	if u1.Signature == u0.Signature || !strings.HasSuffix(u1.Signature, "-streaming") {
		t.Fatalf("signature %q -> %q", u0.Signature, u1.Signature)
	}
	mid, err := src.Read(u1)
	if err != nil {
		t.Fatal(err)
	}
	if len(mid) != len(old)+1 || mid[len(old)].Blocks[0].Text != "OPENCODE_V2_PROMPT" || !reflect.DeepEqual(mid[:len(old)], old) {
		t.Fatalf("streaming reply leaked or prefix moved: %+v", mid[len(old):])
	}

	// The call starts running: still held back with the unfinished message.
	exec(fmt.Sprintf(`UPDATE session_message SET time_updated=%d, data=json_set(data,'$.content[1].state',json('{"status":"running","input":{"path":"/tmp/ocproj/README.md"},"metadata":{}}')) WHERE id='msg_zz_reply'`, now+2))
	if u := unit(); u.Signature == u1.Signature {
		t.Fatal("in-place update did not move the signature")
	}
	// Completed: the reply and its result appear once, with stable IDs.
	exec(fmt.Sprintf(`UPDATE session_message SET time_updated=%d, data=json_set(data,'$.content[0].text','half a reply, done','$.content[1].state',json('{"status":"completed","input":{"path":"/tmp/ocproj/README.md"},"content":[{"type":"text","text":"1: hello"},{"type":"file","uri":"data:image/png;base64,AAAA","mime":"image/png"}]}'),'$.time.completed',%d) WHERE id='msg_zz_reply'`, now+3, now+3))
	u3 := unit()
	if strings.HasSuffix(u3.Signature, "-streaming") {
		t.Fatalf("completed session still streaming: %s", u3.Signature)
	}
	done, err := src.Read(u3)
	if err != nil {
		t.Fatal(err)
	}
	reply, result := done[len(old)+1], done[len(old)+2]
	if len(done) != len(old)+3 || reply.Effort != "high" || reply.Blocks[0].Text != "half a reply, done" || reply.Blocks[1].Text != `read {"path":"/tmp/ocproj/README.md"}` {
		t.Fatalf("completed reply: %+v", done[len(old):])
	}
	if result.TurnUUID != "msg_zz_reply#call_zz" || result.Blocks[0].Text != "1: hello\n[file omitted: image/png]" || strings.Contains(result.Blocks[0].Text, "AAAA") {
		t.Fatalf("tool result: %+v", result)
	}

	// A message OpenCode never completed (killed mid-stream) is archived once
	// the session settles.
	exec(fmt.Sprintf(`UPDATE session_message SET time_updated=%d, data=json_remove(data,'$.time.completed') WHERE id='msg_zz_reply'`, now-2*openCodeSettle.Milliseconds()),
		fmt.Sprintf(`UPDATE session_message SET time_updated=%d WHERE session_id='%s'`, now-2*openCodeSettle.Milliseconds(), ocV2Canary),
		fmt.Sprintf(`UPDATE session_v2 SET time_updated=%d WHERE id='%s'`, now-2*openCodeSettle.Milliseconds(), ocV2Canary))
	settled, err := src.Read(unit())
	if err != nil || len(settled) != len(done) {
		t.Fatalf("settled: %d turns, %v", len(settled), err)
	}
}

// 2.x can keep assistant parts as session_part rows (experimental.part_rows);
// rows come first, then any inline parts an older build appended.
func TestOpenCodeV2PartRows(t *testing.T) {
	inline := openCodeSessions(t, &OpenCodeSource{Root: openCodeV2DB(t)})[ocV2SkillBash]
	moved := openCodeSessions(t, &OpenCodeSource{Root: openCodeV2DB(t,
		`INSERT INTO session_part SELECT m.id, m.session_id, c.key, json_extract(c.value,'$.type'), json_extract(c.value,'$.id'), m.time_created, m.time_updated, c.value FROM session_message m, json_each(m.data,'$.content') c WHERE m.type='assistant'`,
		`UPDATE session_message SET data=json_set(data,'$.content',json('[]')) WHERE type='assistant'`,
	)})[ocV2SkillBash]
	strip := func(turns []Turn) []Turn {
		for i := range turns {
			turns[i].SourcePath = ""
		}
		return turns
	}
	if !reflect.DeepEqual(strip(inline), strip(moved)) {
		t.Fatal("part rows read differently from inline content")
	}
	both := openCodeSessions(t, &OpenCodeSource{Root: openCodeV2DB(t,
		`INSERT INTO session_part SELECT m.id, m.session_id, 0, 'reasoning', NULL, m.time_created, m.time_updated, '{"type":"reasoning","text":"FROM_PART_ROW"}' FROM session_message m WHERE m.id='msg_1b85029a3001JjFHBBl2yEDGea'`,
	)})[ocV2SkillBash]
	last := both[len(both)-1]
	if last.Blocks[0].Text != "FROM_PART_ROW" || last.Blocks[1].Type != "thinking" || last.Blocks[2].Type != "text" {
		t.Fatalf("rows then inline: %+v", last.Blocks)
	}
}

func TestOpenCodeV2MessageTypes(t *testing.T) {
	const s = ocV2Canary
	root := openCodeV2DB(t,
		`INSERT INTO session_message VALUES('msg_zz_1','`+s+`','user',100,1794093800000,1794093800000,'{"time":{"created":1794093800000},"text":"use it","files":[{"data":"QUJD","mime":"image/png","source":{"type":"inline"},"name":"shot.png"}],"agents":[{"name":"general"}],"skills":[{"id":"canary-notes","name":"canary-notes","text":"The marker word is PELICAN-7."}]}')`,
		`INSERT INTO session_message VALUES('msg_zz_2','`+s+`','skill',101,1794093800001,1794093800001,'{"time":{"created":1794093800001},"skill":"review","name":"review","text":"Review carefully."}')`,
		`INSERT INTO session_message VALUES('msg_zz_3','`+s+`','shell',102,1794093800002,1794093800002,'{"time":{"created":1794093800002,"completed":1794093800003},"shellID":"sh_1","command":"cat .opencode/skills/canary-notes/SKILL.md","status":"exited","exit":1,"output":{"output":"denied","cursor":6,"size":6,"truncated":false}}')`,
		`INSERT INTO session_message VALUES('msg_zz_4','`+s+`','compaction',103,1794093800004,1794093800004,'{"time":{"created":1794093800004},"status":"completed","reason":"auto","summary":"COMPACTED"}')`,
		`INSERT INTO session_message VALUES('msg_zz_5','`+s+`','model-switched',104,1794093800005,1794093800005,'{"time":{"created":1794093800005},"model":{"id":"x","providerID":"y"}}')`,
		`INSERT INTO session_message VALUES('msg_zz_6','`+s+`','synthetic',105,1794093800006,1794093800006,'{"time":{"created":1794093800006},"text":"INJECTED"}')`,
		`INSERT INTO session_message VALUES('msg_zz_7','`+s+`','assistant',106,1794093800007,1794093800007,'{"time":{"created":1794093800007,"completed":1794093800008},"agent":"build","model":{"id":"big-pickle","providerID":"opencode"},"content":[],"error":{"type":"provider","message":"rate limited"}}')`,
	)
	turns := openCodeSessions(t, &OpenCodeSource{Root: root})[s]
	extra := turns[4:]
	var got []string
	for _, turn := range extra {
		var texts []string
		for _, b := range turn.Blocks {
			texts = append(texts, b.Type+":"+b.Text)
		}
		got = append(got, turn.Role+" "+strings.Join(texts, " | "))
	}
	want := []string{
		"user text:use it | text:@general | text:[file omitted: shot.png]",
		`system tool_use:skill {"id":"canary-notes"}`,
		`system tool_use:skill {"id":"review"}`,
		`user tool_use:shell {"command":"cat .opencode/skills/canary-notes/SKILL.md"}`,
		"tool tool_result:denied",
		"system text:INJECTED",
		"assistant text:[provider] rate limited",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("turns:\n%s", strings.Join(got, "\n"))
	}
	if !extra[4].Blocks[0].IsError {
		t.Fatal("non-zero shell exit is not an error")
	}
	var loads []string
	for _, turn := range extra {
		for _, b := range turn.Blocks {
			for _, l := range b.SkillLoads {
				loads = append(loads, l.Name)
			}
		}
	}
	if !reflect.DeepEqual(loads, []string{"canary-notes", "review", "canary-notes"}) {
		t.Fatalf("skill loads: %v", loads)
	}
	if rows := Sessions(turns, nil, ""); rows[0].PromptCount != 2 {
		t.Fatalf("prompt count: %+v", rows[0])
	}
}

func TestOpenCodeV2MalformedData(t *testing.T) {
	for name, tc := range map[string]struct{ statement, err string }{
		"invalid message JSON": {`UPDATE session_message SET data='{"text":"cut off' WHERE id='msg_1b8518da4001aPEfmsAIDrbeHV'`, "invalid JSON"},
		"invalid part JSON":    {`INSERT INTO session_part VALUES('msg_1b8518dad001XvoyRksg0aiy9c','` + ocV2Canary + `',0,'text',NULL,1,1,'{"type":"text","text":"cut off')`, "invalid JSON"},
		"unknown part type":    {`UPDATE session_message SET data=json_set(data,'$.content[0].type','hologram') WHERE id='msg_1b8518dad001XvoyRksg0aiy9c'`, "unsupported type"},
		"unknown message type": {`UPDATE session_message SET type='narration' WHERE id='msg_1b8518da4001aPEfmsAIDrbeHV'`, "unsupported type"},
	} {
		t.Run(name, func(t *testing.T) {
			src := &OpenCodeSource{Root: openCodeV2DB(t, tc.statement)}
			units, err := src.Units()
			if err != nil {
				t.Fatal(err)
			}
			var failed error
			var ok int
			for _, u := range units {
				if _, err := src.Read(u); err != nil {
					failed = err
				} else {
					ok++
				}
			}
			if failed == nil || !strings.Contains(failed.Error(), tc.err) || strings.Contains(failed.Error(), "cut off") || ok != len(units)-1 {
				t.Fatalf("err=%v ok=%d/%d", failed, ok, len(units))
			}
		})
	}
	t.Run("pre-release opencode-next.db", func(t *testing.T) {
		// Its v2 table is named session and it has no message table; 2.x
		// imports it into opencode.db, so it is not read on its own.
		root := t.TempDir()
		_, db := openCodeDBFrom(t, openCodeV2Fixture,
			`DROP TABLE session`, `DROP TABLE message`, `DROP TABLE part`, `ALTER TABLE session_v2 RENAME TO session`)
		if _, err := db.Exec("VACUUM INTO ?", filepath.Join(root, "opencode-next.db")); err != nil {
			t.Fatal(err)
		}
		units, err := (&OpenCodeSource{Root: root}).Units()
		if err != nil || len(units) != 0 {
			t.Fatalf("units=%+v err=%v", units, err)
		}
	})
	t.Run("1.19.5 unused session_message", func(t *testing.T) {
		root, _ := openCodeDB(t, `INSERT INTO session_message VALUES('sm_zz','ses_e489fda16ffewIo99FGGRcf6YR','user',1,1,1,'{}')`)
		if n := len(openCodeSessions(t, &OpenCodeSource{Root: root})); n != 5 {
			t.Fatalf("sessions=%d", n)
		}
	})
}

func TestOpenCodeV2WindowsPaths(t *testing.T) {
	root := openCodeV2DB(t, `UPDATE session_v2 SET directory='C:/Users/test/proj' WHERE id='`+ocV2Canary+`'`)
	turns := openCodeSessions(t, &OpenCodeSource{Root: root})[ocV2Canary]
	for _, turn := range turns {
		if turn.Workdir != `C:\Users\test\proj` {
			t.Fatalf("workdir=%q", turn.Workdir)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "opencode.db")); err != nil {
		t.Fatal(err)
	}
}
