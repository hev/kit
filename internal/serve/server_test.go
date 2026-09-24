package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/hev/kit/internal/trace"
)

type fakeReader struct {
	sessions      []trace.SessionRow
	blocks        []trace.BlockRow
	sessionFilter any
}

func (f *fakeReader) ListSessionRows(_ int, filter any) ([]trace.SessionRow, error) {
	f.sessionFilter = filter
	if reflect.DeepEqual(filter, []any{"session_id", "Eq", "s1"}) {
		return f.sessions[:1], nil
	}
	return f.sessions, nil
}
func (f *fakeReader) ListBlockRows(string) ([]trace.BlockRow, error) { return f.blocks, nil }

func TestStats(t *testing.T) {
	f := &fakeReader{sessions: []trace.SessionRow{{SessionID: "s1", Cost: 12, PromptCount: 4, ToolCount: 3, APIMS: 3600000, WallMS: 7200000, InputTokens: 10, OutputTokens: 5, CacheReadTokens: 80, CacheCreationTokens: 10}}}
	r := httptest.NewRequest("GET", "/api/stats?window=7d", nil)
	w := httptest.NewRecorder()
	New(f).Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["spend"] != 12.0 || got["tokens"] != 105.0 || got["api_hours"] != 1.0 || got["cost_per_prompt"] != 3.0 {
		t.Fatalf("unexpected stats: %#v", got)
	}
	filter := f.sessionFilter.([]any)
	if filter[0] != "And" {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestSessionReassemblesOrderedBlocks(t *testing.T) {
	f := &fakeReader{sessions: []trace.SessionRow{{SessionID: "s1", Summary: "hello", PromptCount: 1, ToolCount: 1}}, blocks: []trace.BlockRow{
		{Seq: 2, SessionID: "s1", TurnUUID: "u1", Role: "user", BlockType: "tool_result", ToolUseID: "x", Text: "done", Start: 30, End: 30, OK: true},
		{Seq: 0, SessionID: "s1", TurnUUID: "u1", Role: "user", BlockType: "text", Text: "prompt", Start: 10, End: 10, OK: true},
		{Seq: 1, SessionID: "s1", TurnUUID: "u1", Role: "assistant", BlockType: "tool_use", ToolUseID: "x", ToolName: "Bash", Text: `{"command":"true"}`, RequestID: "r1", Start: 10, End: 30, MS: 20, OK: true},
	}}
	r := httptest.NewRequest("GET", "/api/session/s1", nil)
	r.SetPathValue("id", "s1")
	w := httptest.NewRecorder()
	New(f).Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	turn := got["turns"].([]any)[0].(map[string]any)
	if turn["prompt"] != "prompt" {
		t.Fatalf("turn: %#v", turn)
	}
	items := turn["items"].([]any)
	tool := items[0].(map[string]any)
	if tool["result"] != "done" || tool["name"] != "Bash" {
		t.Fatalf("tool: %#v", tool)
	}
}

func TestBadWindow(t *testing.T) {
	w := httptest.NewRecorder()
	New(&fakeReader{}).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?window=1d", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
}

func TestSessionsLatestAndPromptData(t *testing.T) {
	f := &fakeReader{sessions: []trace.SessionRow{
		{ID: "old", SessionID: "s", End: 1, PromptCount: 1},
		{ID: "new", SessionID: "s", End: 2, PromptCount: 2, PromptTS: []uint64{100, 200}, ToolNames: []string{"Bash"}, TotalTokens: 42},
	}}
	w := httptest.NewRecorder()
	New(f).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/sessions?window=7d", nil))
	var got struct {
		Sessions []struct {
			ID          string
			End         int
			Prompts     int
			PromptTS    []uint64 `json:"prompt_ts"`
			TotalTokens int      `json:"total_tokens"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].End != 2 || len(got.Sessions[0].PromptTS) != 0 || got.Sessions[0].TotalTokens != 42 {
		t.Fatalf("latest row or prompt data lost: %s", w.Body.String())
	}
	t.Log("session rows = 1; unique session ids = 1; list rows omit prompt_ts")
}
