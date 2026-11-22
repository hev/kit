package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProviderOfAndBadge(t *testing.T) {
	for id, want := range map[string]string{
		"codex/gpt-6-astra":     ProviderCodexSub,
		"codex/gpt-5.3-codex":   ProviderCodexSub,
		"openai/codex-mini":     ProviderOpenRouter,
		"google/gemini-4-neon":  ProviderOpenRouter,
		"anthropic/claude-opus": ProviderOpenRouter,
		"codexish/model":        ProviderOpenRouter,
	} {
		if got := ProviderOf(id); got != want {
			t.Errorf("ProviderOf(%s) = %s, want %s", id, got, want)
		}
	}
	if b := (Model{ID: "codex/gpt-5.4", PromptPrice: -1, CompletionPrice: -1}).Badge(); b != "[codex-sub] sub" {
		t.Fatalf("codex badge = %q", b)
	}
	if b := (Model{ID: "anthropic/claude-sonnet-5.5", PromptPrice: 3, CompletionPrice: 15}).Badge(); b != "[OR] $3.00/$15.00" {
		t.Fatalf("OR badge = %q", b)
	}
}

func TestRouterMissingProvider(t *testing.T) {
	r := NewRouter(&scriptedAs{id: ProviderOpenRouter})
	_, err := r.Stream(context.Background(), ChatRequest{Model: "codex/gpt-5.4"}, nil)
	if err == nil || !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("missing codex-sub err = %v", err)
	}
	r = NewRouter(&scriptedAs{id: ProviderCodexSub})
	if _, err := r.Stream(context.Background(), ChatRequest{Model: "openai/gpt-5"}, nil); err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("missing openrouter err = %v", err)
	}
}

// scriptedAs is a scripted provider with a chosen ID and catalog.
type scriptedAs struct {
	scripted
	id     string
	models []Model
}

func (s *scriptedAs) ID() string { return s.id }
func (s *scriptedAs) Models(context.Context, bool) ([]Model, CatalogSource, error) {
	return s.models, SourceCache, nil
}

// TestCrossProviderHotSwap runs turn 1 on codex-sub (with a tool call) and
// turn 2 on OpenRouter, over real HTTP: the history crosses providers and
// every record carries its provider and model; no credential is written.
func TestCrossProviderHotSwap(t *testing.T) {
	access := fakeJWT(t, time.Now().Add(time.Hour), "live")
	codexHome(t, map[string]string{"access_token": access, "refresh_token": testRefresh, "account_id": testAccountID})
	var mu sync.Mutex
	codexStep := 0
	codexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		codexStep++
		n := codexStep
		mu.Unlock()
		if n == 1 {
			codexSSE(w, `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_cx1","name":"glob"}}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_cx1","name":"glob","arguments":"{\"pattern\":\"*.md\"}"}}`,
				`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}}`)
			return
		}
		// Echo the account id in the text: it must be scrubbed from the trace.
		codexSSE(w, fmt.Sprintf(`{"type":"response.output_text.delta","delta":"found README.md (acct %s)"}`, testAccountID),
			`{"type":"response.completed","response":{}}`)
	}))
	defer codexSrv.Close()
	var orReq ChatRequest
	orSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&orReq)
		sse(w, `{"choices":[{"delta":{"content":"gemini here, key `+testKey+`"}}]}`)
	}))
	defer orSrv.Close()

	codex := newCodex(t, codexSrv.URL)
	or := NewClient(orSrv.URL, testKey)
	router := NewRouter(or, codex)
	session, err := OpenSession("", testKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644)
	a := NewAgent(router, NewToolset(dir, noHev, testKey), "codex/gpt-6-astra", "SYS")
	a.Session = session
	if err := a.Run(context.Background(), "find docs", nil); err != nil {
		t.Fatal(err)
	}
	a.SetModel("google/gemini-4-neon")
	if err := a.Run(context.Background(), "summarize", nil); err != nil {
		t.Fatal(err)
	}
	session.Close()

	// Turn 2 on OpenRouter carries turn 1's codex history, tool call included.
	var roles []string
	for _, m := range orReq.Messages {
		roles = append(roles, m.Role)
	}
	if orReq.Model != "google/gemini-4-neon" || strings.Join(roles, ",") != "system,user,assistant,tool,assistant,user" {
		t.Fatalf("OR request model=%s roles=%v", orReq.Model, roles)
	}
	if orReq.Messages[2].ToolCalls[0].ID != "call_cx1" || orReq.Messages[3].ToolCallID != "call_cx1" || !strings.Contains(orReq.Messages[3].Content, "README.md") {
		t.Fatalf("tool history lost: %+v", orReq.Messages[2:4])
	}

	recs := readSession(t, session.Path)
	raw, _ := os.ReadFile(session.Path)
	for _, s := range []string{access, testRefresh, testAccountID, testKey} {
		if strings.Contains(string(raw), s) {
			t.Fatalf("trace leaked a credential (%.10s…)", s)
		}
	}
	var stamps []string
	var sw *Record
	for i, r := range recs {
		switch r.Type {
		case "user", "assistant", "tool":
			stamps = append(stamps, fmt.Sprintf("%d:%s:%s:%s", r.Turn, r.Type, r.Provider, r.Model))
		case "model_switch":
			sw = &recs[i]
		}
	}
	want := []string{
		"1:user:codex-sub:codex/gpt-6-astra",
		"1:assistant:codex-sub:codex/gpt-6-astra",
		"1:tool:codex-sub:codex/gpt-6-astra",
		"1:assistant:codex-sub:codex/gpt-6-astra",
		"2:user:openrouter:google/gemini-4-neon",
		"2:assistant:openrouter:google/gemini-4-neon",
	}
	if strings.Join(stamps, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stamps:\n%s", strings.Join(stamps, "\n"))
	}
	if sw == nil || sw.From != "codex/gpt-6-astra" || sw.FromProvider != ProviderCodexSub || sw.Model != "google/gemini-4-neon" || sw.Provider != ProviderOpenRouter {
		t.Fatalf("model_switch = %+v", sw)
	}
	// Kept: user messages and model text survive redaction.
	if !strings.Contains(string(raw), "found README.md (acct") || !strings.Contains(string(raw), "gemini here") || !strings.Contains(string(raw), `"content":"summarize"`) {
		t.Fatalf("trace lost content:\n%s", raw)
	}
}

// TestFailedTurnRedaction shows an error from either provider is scrubbed in
// the event, the returned error and the trace.
func TestFailedTurnRedaction(t *testing.T) {
	access := fakeJWT(t, time.Now().Add(time.Hour), "live")
	codexHome(t, map[string]string{"access_token": access, "refresh_token": testRefresh, "account_id": testAccountID})
	codexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"detail":"slow down %s (%s) %s"}`, testAccountID, access, testRefresh)
	}))
	defer codexSrv.Close()
	orSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		fmt.Fprintf(w, `{"error":{"message":"key %s has no credit"}}`, testKey)
	}))
	defer orSrv.Close()
	router := NewRouter(NewClient(orSrv.URL, testKey), newCodex(t, codexSrv.URL))
	// The session is opened with no secrets: the agent hands them over.
	session, _ := OpenSession("")
	a := NewAgent(router, nil, "codex/gpt-5.4", "")
	a.Session = session
	var evErrs []string
	emit := func(ev Event) {
		if ev.Kind == EventError {
			evErrs = append(evErrs, ev.Err.Error())
		}
	}
	err1 := a.Run(context.Background(), "hello codex", emit)
	a.SetModel("openai/gpt-5")
	err2 := a.Run(context.Background(), "hello openrouter", emit)
	session.Close()
	raw, _ := os.ReadFile(session.Path)
	all := strings.Join(append(evErrs, fmt.Sprint(err1), fmt.Sprint(err2), string(raw)), "\n")
	for _, s := range []string{access, testRefresh, testAccountID, testKey} {
		if strings.Contains(all, s) {
			t.Fatalf("credential leaked (%.10s…):\n%s", s, all)
		}
	}
	for _, keep := range []string{"slow down", "has no credit", "codex/gpt-5.4", "openai/gpt-5", "hello codex", "hello openrouter"} {
		if !strings.Contains(all, keep) {
			t.Fatalf("lost %q:\n%s", keep, all)
		}
	}
}

func TestTUICrossProviderPicker(t *testing.T) {
	isolateHome(t)
	orModels := []Model{
		{ID: "anthropic/claude-sonnet-5.5", Name: "Claude Sonnet", ContextLength: 200000, PromptPrice: 3, CompletionPrice: 15},
		{ID: "google/gemini-4-neon", Name: "Gemini 4 Neon", ContextLength: 1048576, PromptPrice: 1.25, CompletionPrice: 10},
	}
	codexModels := []Model{
		{ID: "codex/gpt-6-astra", Name: "Codex (ChatGPT subscription): gpt-6-astra", PromptPrice: -1, CompletionPrice: -1},
		{ID: "codex/gpt-5.3-codex", Name: "Codex (ChatGPT subscription): gpt-5.3-codex", PromptPrice: -1, CompletionPrice: -1},
	}
	orP := &scriptedAs{id: ProviderOpenRouter, models: orModels}
	cxP := &scriptedAs{id: ProviderCodexSub, models: codexModels}
	cxP.replies = []ChatResult{{Content: "from codex"}}
	orP.replies = []ChatResult{{Content: "from gemini"}}
	router := NewRouter(orP, cxP)
	a := NewAgent(router, NewToolset(t.TempDir(), noHev), "codex/gpt-6-astra", "SYS")
	m := NewTUI(TUIOptions{Agent: a, Favorites: LoadFavorites(""), Version: "test", Cwd: "/w",
		Composer: PromptComposer{Cwd: t.TempDir()},
		Catalog:  func(ctx context.Context) ([]Model, CatalogSource, error) { return router.Models(ctx, false) }})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 50})
	m.Update(m.loadCatalog()())

	m.Update(key(tea.KeyCtrlM))
	v := plain(m)
	for _, want := range []string{"codex/gpt-6-astra", "[codex-sub] sub", "google/gemini-4-neon", "[OR] $1.25/$10.00", "[OR] $3.00/$15.00"} {
		if !strings.Contains(v, want) {
			t.Fatalf("picker missing %q:\n%s", want, v)
		}
	}
	// Subscription models list first.
	if m.picker.filtered[0].ProviderID() != ProviderCodexSub {
		t.Fatalf("order = %v", m.picker.filtered)
	}
	// Fuzzy filter reaches both providers, and only the matching rows stay.
	typeText(m, "astra")
	if len(m.picker.filtered) != 1 || m.picker.filtered[0].ID != "codex/gpt-6-astra" {
		t.Fatalf("astra = %v", m.picker.filtered)
	}
	m.Update(key(tea.KeyEsc))
	m.Update(key(tea.KeyCtrlM))
	typeText(m, "neon")
	if len(m.picker.filtered) == 0 || m.picker.filtered[0].ID != "google/gemini-4-neon" {
		t.Fatalf("neon = %v", m.picker.filtered)
	}
	for _, f := range m.picker.filtered {
		if f.ProviderID() == ProviderCodexSub {
			t.Fatalf("neon matched a codex row: %v", m.picker.filtered)
		}
	}
	m.Update(key(tea.KeyCtrlF)) // star the OpenRouter model
	m.Update(key(tea.KeyEsc))
	m.Update(key(tea.KeyCtrlM))
	typeText(m, "53codex")
	if len(m.picker.filtered) == 0 || m.picker.filtered[0].ID != "codex/gpt-5.3-codex" {
		t.Fatalf("53codex = %v", m.picker.filtered)
	}
	m.Update(key(tea.KeyCtrlF)) // star the codex-sub model
	m.Update(key(tea.KeyEsc))

	// Favorites for both providers persist and reload.
	fav := LoadFavorites("")
	if !fav.Has("google/gemini-4-neon") || !fav.Has("codex/gpt-5.3-codex") {
		t.Fatalf("favorites = %v", fav.IDs)
	}
	// A favorite from a provider that isn't loaded still shows with its badge.
	fav.Toggle("codex/gpt-9-future")
	m.opts.Favorites = LoadFavorites("")
	m.Update(key(tea.KeyCtrlM))
	if v := plain(m); !strings.Contains(v, "codex/gpt-9-future") || strings.Count(v, "★") != 3 {
		t.Fatalf("favorites view:\n%s", v)
	}
	m.Update(key(tea.KeyEsc))

	// Hot-swap mid-thread: turn on codex, pick gemini, turn on OpenRouter.
	drain(t, m, submit(m, "first"))
	m.Update(key(tea.KeyCtrlM))
	typeText(m, "neon")
	m.Update(key(tea.KeyEnter))
	if a.Model() != "google/gemini-4-neon" || !strings.Contains(plain(m), "(codex-sub → openrouter)") {
		t.Fatalf("swap: model=%s view:\n%s", a.Model(), plain(m))
	}
	drain(t, m, submit(m, "second"))
	if len(cxP.requests) != 1 || len(orP.requests) != 1 {
		t.Fatalf("requests codex=%d or=%d", len(cxP.requests), len(orP.requests))
	}
	h := orP.requests[0].Messages
	if len(h) != 4 || h[2].Content != "from codex" || h[3].Content != "second" {
		t.Fatalf("history across providers = %+v", h)
	}
	if !strings.Contains(plain(m), "[openrouter]") {
		t.Fatalf("header lacks provider:\n%s", plain(m))
	}
}
