package serve

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

//go:embed template.html
var pageTemplate string

type Reader interface {
	ListSessionRows(topK int, filter any) ([]trace.SessionRow, error)
	ListBlockRows(sessionID string) ([]trace.BlockRow, error)
}

type Server struct {
	reader        Reader
	factoryConfig *FactoryConfig
	// Shared across requests; timing and eval snapshots must not reset it.
	baselines *baselineCache
	// Shared across requests: the newest row per session for value pickers.
	archive *archiveCache
	// Snapshots below belong to one HTTP request, never the shared server.
	allRows    []trace.SessionRow
	evalRows   []layer.EvalRow
	evalLoaded bool
}

func New(reader Reader) *Server {
	s := &Server{reader: reader, baselines: &baselineCache{}, archive: &archiveCache{ttl: DefaultCacheTTL}}
	s.archive.src = s
	return s
}

// WithCacheTTL sets how long the archive answers value pickers before it is
// revalidated against Layer. Zero or less sends every picker to the store.
func (s *Server) WithCacheTTL(ttl time.Duration) *Server {
	s.archive.ttl = ttl
	return s
}

// Warm fills the picker archive in the background, so the first filter a
// visitor opens counts from memory.
func (s *Server) Warm() *Server {
	s.archive.warm()
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range map[string]func(*Server, http.ResponseWriter, *http.Request){
		"GET /": (*Server).page, "GET /api/sessions": (*Server).sessions,
		"GET /api/session/{id}": (*Server).session, "GET /api/stats": (*Server).stats,
		"GET /api/search": (*Server).search, "GET /api/factory": (*Server).factory,
		"GET /api/values": (*Server).values,
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			request := &Server{reader: s.reader, baselines: s.baselines, archive: s.archive, factoryConfig: s.factoryConfig}
			timing := &layer.Timing{}
			if c, ok := s.reader.(*layer.Client); ok {
				request.reader = c.WithTiming(timing)
			}
			handler(request, &timedWriter{ResponseWriter: w, timing: timing}, r)
		})
	}
	mux.Handle("GET /ui/", uiHandler())
	return compressResponses(mux)
}

type timedWriter struct {
	http.ResponseWriter
	timing *layer.Timing
}

func addTiming(w http.ResponseWriter, value any) {
	if tw, ok := w.(*timedWriter); ok {
		w.Header().Set("Server-Timing", fmt.Sprintf("layer;dur=%.3f", tw.timing.LayerMS))
		if m, ok := value.(map[string]any); ok {
			m["timing"] = tw.timing
		}
	}
}

func (s *Server) listRows(filter any) ([]trace.SessionRow, error) {
	if reader, ok := s.reader.(interface {
		ListSlimSessionRows(int, any) ([]trace.SessionRow, error)
	}); ok {
		return reader.ListSlimSessionRows(-1, filter)
	}
	return s.reader.ListSessionRows(-1, filter)
}

func windowFilter(raw string, now time.Time) (any, string, error) {
	if raw == "" {
		raw = "30d"
	}
	if raw == "all" {
		return nil, raw, nil
	}
	days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
	if err != nil || !strings.HasSuffix(raw, "d") || (days != 7 && days != 30 && days != 90) {
		return nil, "", fmt.Errorf("window must be 7d, 30d, 90d, or all")
	}
	return []any{"end", "Gte", now.AddDate(0, 0, -days).UnixMilli()}, raw, nil
}

func (s *Server) rows(r *http.Request) ([]trace.SessionRow, string, error) {
	q := r.URL.Query()
	if !notPoorOnly(q) {
		return s.matchingRows(r)
	}
	// "Not poor" means no poor grade, which includes traces never graded. Joining
	// on eval rows would keep only graded traces, so exclude the poor ones instead.
	q.Del("poor")
	scoped := r.Clone(r.Context())
	scoped.URL.RawQuery = q.Encode()
	rows, window, err := s.matchingRows(scoped)
	if err != nil {
		return nil, "", err
	}
	poor, err := s.evals(url.Values{"poor": {"true"}})
	if err != nil {
		return nil, "", err
	}
	drop := map[string]bool{}
	for _, e := range poor {
		drop[e.SessionID] = true
	}
	kept := make([]trace.SessionRow, 0, len(rows))
	for _, row := range rows {
		if !drop[row.ID] {
			kept = append(kept, row)
		}
	}
	return kept, window, nil
}

// notPoorOnly reports a poor=false filter with no other grade predicate. Role,
// instance and mark ceilings describe a grade, so with any of them present the
// trace must have one and the ordinary eval join is right.
func notPoorOnly(q url.Values) bool {
	if v, err := strconv.ParseBool(q.Get("poor")); err != nil || v {
		return false
	}
	for key := range q {
		if key == "role" || key == "instance" || (strings.HasPrefix(key, "mark_") && strings.HasSuffix(key, "_max")) {
			return false
		}
	}
	return true
}

func (s *Server) matchingRows(r *http.Request) ([]trace.SessionRow, string, error) {
	if _, err := localRows(nil, r.URL.Query()); err != nil {
		return nil, "", err
	}
	// Resolve legacy row identity before applying mutable predicates. Otherwise
	// an old low-cost row could match while the latest state is over the ceiling.
	prefilter, _, err := sessionFilter(r.URL.Query(), nil)
	if err != nil {
		return nil, "", err
	}
	preeval, err := evalFilter(r.URL.Query())
	if err != nil {
		return nil, "", err
	}
	list := s.listRows
	if prefilter != nil || preeval != nil {
		if reader, ok := s.reader.(interface {
			ListSessionIndexRows(int, any) ([]trace.SessionRow, error)
		}); ok {
			list = func(filter any) ([]trace.SessionRow, error) { return reader.ListSessionIndexRows(-1, filter) }
		}
	}
	// The archive grade snapshot is independent of session identity. Fetch it
	// alongside the index, then join before any grade filtering or response.
	var evalDone chan struct{}
	var evalRows []layer.EvalRow
	var evalErr error
	if reader, ok := s.reader.(*layer.Client); ok && (r.URL.Path == "/api/sessions" || r.URL.Path == "/api/search") {
		evalDone = make(chan struct{})
		go func() {
			defer close(evalDone)
			evalRows, evalErr = reader.ListEvalMarks(nil)
		}()
	}
	// Predicates that do not need project-name or grade resolution can run in
	// Layer while the identity scan is in flight. Intersect by newest row ID
	// afterwards: a matching legacy row must never resurrect a stale session.
	var rowsDone chan struct{}
	var candidates []trace.SessionRow
	var candidatesErr error
	if _, ok := s.reader.(*layer.Client); ok && prefilter != nil && preeval == nil && len(r.URL.Query()["project"]) == 0 {
		rowsDone = make(chan struct{})
		go func() {
			defer close(rowsDone)
			candidates, candidatesErr = s.listRows(prefilter)
		}()
	}
	all, err := list(nil)
	if rowsDone != nil {
		<-rowsDone
		if err == nil {
			err = candidatesErr
		}
	}
	if evalDone != nil {
		<-evalDone
		if evalErr != nil {
			return nil, "", evalErr
		}
		s.evalRows, s.evalLoaded = evalRows, true
	}
	if err != nil {
		return nil, "", err
	}
	latest := latestSessions(all)
	s.allRows = latest
	filter, window, err := sessionFilter(r.URL.Query(), latest)
	if err != nil {
		return nil, "", err
	}
	ef, err := evalFilter(r.URL.Query())
	if err != nil {
		return nil, "", err
	}
	if ef != nil {
		evals, err := s.evals(r.URL.Query())
		if err != nil {
			return nil, "", err
		}
		ids := []string{}
		for _, e := range evals {
			ids = append(ids, e.SessionID)
		}
		if len(ids) == 0 {
			return []trace.SessionRow{}, window, nil
		}
		filter = layer.And(filter, []any{"session_id", "In", ids})
	}
	if filter == nil {
		filtered, err := localRows(latest, r.URL.Query())
		return filtered, window, err
	}
	if rowsDone != nil {
		ids := make(map[string]bool, len(latest))
		for _, row := range latest {
			ids[row.ID] = true
		}
		rows := make([]trace.SessionRow, 0, len(candidates))
		for _, row := range candidates {
			if ids[row.ID] {
				rows = append(rows, row)
			}
		}
		filtered, err := localRows(latestSessions(rows), r.URL.Query())
		return filtered, window, err
	}
	rows := []trace.SessionRow{}
	for start := 0; start < len(latest); start += 10000 {
		ids := []string{}
		for _, row := range latest[start:min(start+10000, len(latest))] {
			ids = append(ids, row.ID)
		}
		batch, err := s.listRows(layer.And(filter, []any{"id", "In", ids}))
		if err != nil {
			return nil, "", err
		}
		rows = append(rows, batch...)
	}
	filtered, err := localRows(latestSessions(rows), r.URL.Query())
	return filtered, window, err
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	rows, window, err := s.rows(r)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	out, err := s.joinedCorpus(rows)
	if err != nil {
		writeError(w, err, 502)
		return
	}
	fc, err := s.archiveFacets()
	if err != nil {
		writeError(w, err, 502)
		return
	}
	writeJSON(w, map[string]any{"window": window, "sessions": out, "facets": fc})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	rows, window, err := s.rows(r)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	loc, err := promptLocation(r.URL.Query())
	if err != nil {
		writeError(w, err, 400)
		return
	}
	var grid [7][24]int
	coverage := 0
	var cost float64
	var tokens, prompts, tools, apiMS, wallMS, cacheRead, cacheBase int64
	for _, row := range rows {
		if len(row.PromptTS) > 0 {
			coverage++
		}
		for _, ts := range row.PromptTS {
			d := time.UnixMilli(int64(ts)).In(loc)
			grid[d.Weekday()][d.Hour()]++
		}
		cost += row.Cost
		prompts += row.PromptCount
		tools += row.ToolCount
		apiMS += row.APIMS
		wallMS += row.WallMS
		cacheRead += row.CacheReadTokens
		cacheBase += row.InputTokens + row.CacheReadTokens + row.CacheCreationTokens
		tokens += row.InputTokens + row.OutputTokens + row.CacheReadTokens + row.CacheCreationTokens
	}
	hit := float64(0)
	if cacheBase > 0 {
		hit = float64(cacheRead) / float64(cacheBase)
	}
	perPrompt := float64(0)
	if prompts > 0 {
		perPrompt = cost / float64(prompts)
	}
	writeJSON(w, map[string]any{"window": window, "spend": cost, "tokens": tokens, "traces": len(rows),
		"api_hours": float64(apiMS) / 3600000, "wall_hours": float64(wallMS) / 3600000,
		"cache_hit": hit, "cost_per_prompt": perPrompt, "prompts": prompts, "tools": tools, "prompt_grid": grid, "prompt_coverage": map[string]int{"with": coverage, "total": len(rows)}})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, blocks, err := s.loadSession(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, err, status)
		return
	}
	data, err := s.sessionData(row, blocks)
	if err != nil {
		writeError(w, err, 502)
		return
	}
	writeJSON(w, data)
}

var errNotFound = errors.New("session not found")

func (s *Server) loadSession(id string) (trace.SessionRow, []trace.BlockRow, error) {
	rows, err := s.reader.ListSessionRows(-1, []any{"session_id", "Eq", id})
	if err != nil {
		return trace.SessionRow{}, nil, err
	}
	if len(rows) == 0 {
		return trace.SessionRow{}, nil, errNotFound
	}
	blocks, err := s.reader.ListBlockRows(id)
	return latestSessions(rows)[0], blocks, err
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	payload, _ := json.Marshal(map[string]any{"facets": map[string]any{}, "session": emptySession(), "corpus": []any{}, "rates": rates, "generated": time.Now().UTC().Format(time.RFC3339), "days": 30})
	body := strings.Replace(pageTemplate, "/*__DATA__*/null", string(payload), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	addTiming(w, nil)
	_, _ = w.Write([]byte(body))
}

func corpus(rows []trace.SessionRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range latestSessions(rows) {
		project := projectName(r.RepoURL)
		usage := map[string]int64{"input": r.InputTokens, "output": r.OutputTokens, "cache_read": r.CacheReadTokens, "cache_write": r.CacheCreationTokens}
		out = append(out, map[string]any{"id": r.SessionID, "summary": r.Summary, "first_prompt_short": r.FirstPromptShort,
			"tool_counts": r.ToolCounts, "summary_backfilled": r.Summary == "", "harness": r.Harness, "model": r.Model, "repo_url": r.RepoURL, "project": project,
			"branch": r.Branch, "host": r.Host, "start": r.Start, "end": r.End, "wall_ms": r.WallMS, "api_ms": r.APIMS,
			"prompts": r.PromptCount, "total_tokens": r.TotalTokens, "tools": r.ToolCount, "requests": r.RequestCount,
			"usage_by_model": map[string]any{r.Model: usage}, "cost": r.Cost, "has_subagents": r.HasSubagents})
	}
	return out
}

func projectName(repo string) string {
	repo = strings.TrimSuffix(repo, ".git")
	name := path.Base(repo)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func writeJSON(w http.ResponseWriter, value any) {
	addTiming(w, value)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error, status int) {
	value := map[string]any{"error": err.Error()}
	addTiming(w, value)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var rates = map[string][4]float64{
	"claude-sonnet-5": {3, 15, 6, .3}, "claude-opus-5": {15, 75, 30, 1.5}, "claude-fable-5": {15, 75, 30, 1.5},
	"claude-fable-5-1": {15, 75, 30, 1.5}, "claude-haiku-4-5-20251001": {1, 5, 2, .1},
}

func emptySession() map[string]any { return buildSession(trace.SessionRow{}, nil) }

func buildSession(s trace.SessionRow, rows []trace.BlockRow) map[string]any {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
	usage := func(r trace.BlockRow) map[string]int64 {
		return map[string]int64{"input": r.InputTokens, "output": r.OutputTokens, "cache_read": r.CacheReadTokens, "cache_write": r.CacheCreationTokens, "thinking": 0}
	}
	type turnBuild struct {
		uuid            string
		ts              int64
		prompt          string
		items           []any
		requests        map[string]map[string]any
		cost            float64
		u               map[string]int64
		modelMS, toolMS int64
	}
	turns := []*turnBuild{}
	turnMap := map[string]string{}
	var current *turnBuild
	tools := map[string]map[string]any{}
	agentsByID := map[string][]trace.BlockRow{}
	requestsOut := []map[string]any{}
	for _, b := range rows {
		if b.AgentID != "" {
			agentsByID[b.AgentID] = append(agentsByID[b.AgentID], b)
			continue
		}
		if b.Role == "user" && b.BlockType == "text" {
			if current == nil || current.uuid != b.TurnUUID {
				current = &turnBuild{uuid: b.TurnUUID, ts: b.Start, requests: map[string]map[string]any{}, u: map[string]int64{}}
				turns = append(turns, current)
			}
			turnMap[b.TurnUUID] = current.uuid
			current.prompt += b.Text
			continue
		}
		if current == nil {
			current = &turnBuild{uuid: "pre", ts: b.Start, requests: map[string]map[string]any{}, u: map[string]int64{}}
			turns = append(turns, current)
		}
		turnMap[b.TurnUUID] = current.uuid
		t := current
		if b.BlockType == "tool_use" {
			var input any = map[string]any{}
			if json.Unmarshal([]byte(b.Text), &input) != nil {
				input = map[string]any{"value": b.Text}
			}
			tool := map[string]any{"kind": "tool", "id": b.ToolUseID, "name": b.ToolName, "start": b.Start, "end": b.End, "ms": b.MS, "summary": toolSummary(b.ToolName, input), "input": input, "result": "", "error": !b.OK, "req": b.RequestID, "uuid": b.TurnUUID}
			tools[b.ToolUseID] = tool
			t.items = append(t.items, tool)
			t.toolMS += b.MS
		}
		if b.BlockType == "tool_result" {
			if tool := tools[b.ToolUseID]; tool != nil {
				tool["result"] = b.Text
				tool["error"] = !b.OK
				tool["end"] = b.End
			}
		}
		if b.Role == "assistant" && (b.BlockType == "text" || b.BlockType == "thinking" || b.BlockType == "tool_use") {
			rid := b.RequestID
			if rid == "" {
				rid = b.TurnUUID
			}
			req := t.requests[rid]
			if req == nil {
				req = map[string]any{"kind": "request", "id": rid, "start": b.Start, "first": b.End, "end": b.End, "ms": b.MS, "ttft": max64(0, b.End-b.Start), "model": b.Model, "effort": b.Effort, "usage": usage(b), "blocks": []map[string]any{}, "tools": []string{}, "cost": b.Cost, "context": b.InputTokens + b.CacheReadTokens + b.CacheCreationTokens, "stop": "", "uuid": b.TurnUUID}
				t.requests[rid] = req
				t.items = append(t.items, req)
				requestsOut = append(requestsOut, req)
				t.cost += b.Cost
				addUsage(t.u, usage(b))
				t.modelMS += b.MS
			} else if end, ok := req["end"].(int64); !ok || b.End > end {
				req["end"] = b.End
				req["ms"] = b.End - req["start"].(int64)
			}
			blocks := req["blocks"].([]map[string]any)
			if b.BlockType == "tool_use" {
				blocks = append(blocks, map[string]any{"type": "tool_use", "ts": b.Start, "tool": b.ToolUseID})
				req["tools"] = append(req["tools"].([]string), b.ToolUseID)
			} else {
				blocks = append(blocks, map[string]any{"type": b.BlockType, "ts": b.End, "text": b.Text, "uuid": b.TurnUUID})
			}
			req["blocks"] = blocks
		}
	}
	turnOut := []map[string]any{}
	var priorEnd int64
	for _, t := range turns {
		end := t.ts
		for _, it := range t.items {
			if m, ok := it.(map[string]any); ok {
				if e, ok := m["end"].(int64); ok && e > end {
					end = e
				}
			}
		}
		idle := int64(0)
		if priorEnd > 0 && t.ts > priorEnd {
			idle = t.ts - priorEnd
		}
		priorEnd = end
		turnOut = append(turnOut, map[string]any{"uuid": t.uuid, "ts": t.ts, "end": end, "ms": end - t.ts, "prompt": t.prompt, "items": nonNil(t.items), "usage": t.u, "cost": t.cost, "idle_ms": idle, "model_ms": t.modelMS, "tool_ms": t.toolMS})
	}
	agents := buildAgents(agentsByID)
	// Step 1 preserves agent identity but Claude's subagent file does not carry
	// the spawning tool-use id. Match the same way the reviewed mock falls back:
	// each child goes on the nearest preceding, otherwise next, Agent call.
	agentCalls := []map[string]any{}
	for _, tool := range tools {
		if tool["name"] == "Agent" {
			agentCalls = append(agentCalls, tool)
		}
	}
	sort.Slice(agentCalls, func(i, j int) bool { return agentCalls[i]["start"].(int64) < agentCalls[j]["start"].(int64) })
	sort.Slice(agents, func(i, j int) bool { return agents[i]["start"].(int64) < agents[j]["start"].(int64) })
	for i, agent := range agents {
		if i < len(agentCalls) {
			agentCalls[i]["agent"] = agent["id"]
			agent["call"] = agentCalls[i]["id"]
		}
	}
	allUsage := map[string]int64{"input": s.InputTokens, "output": s.OutputTokens, "cache_read": s.CacheReadTokens, "cache_write": s.CacheCreationTokens}
	return map[string]any{"id": s.SessionID, "title": s.Summary, "summary": s.Summary, "first_prompt": s.FirstPrompt, "prompt_ts": nonNil(s.PromptTS), "tool_names": nonNil(s.ToolNames), "project": projectName(s.RepoURL), "repo_url": s.RepoURL, "branch": s.Branch, "harness": s.Harness, "model": s.Model, "host": s.Host, "size": 0, "start": s.Start, "end": s.End, "wall_ms": s.WallMS, "model_ms": s.APIMS, "tool_ms": sumToolMS(rows), "idle_ms": s.IdleMS, "agent_ms": int64(0), "usage": allUsage, "agent_usage": map[string]int64{}, "cost": s.Cost, "rates_known": true, "n_requests": s.RequestCount, "n_tools": s.ToolCount, "n_prompts": s.PromptCount, "n_edits": countEdits(rows), "turn_map": turnMap, "turns": nonNil(turnOut), "requests": nonNil(requestsOut), "agents": nonNil(agents), "commits": []any{}, "serial_reads": []any{}, "tool_table": nonNil(toolTable(rows)), "model_table": nonNil(modelTable(s))}
}

func buildAgents(groups map[string][]trace.BlockRow) []map[string]any {
	out := []map[string]any{}
	for id, rows := range groups {
		sort.Slice(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
		start, end := int64(0), int64(0)
		var cost float64
		var tools int
		u := map[string]int64{}
		model := ""
		parentID := ""
		items := []any{}
		toolRows := []map[string]any{}
		toolByID := map[string]map[string]any{}
		requests := []map[string]any{}
		for _, r := range rows {
			if parentID == "" {
				parentID = r.ParentAgentID
			}
			if start == 0 || r.Start < start {
				start = r.Start
			}
			if r.End > end {
				end = r.End
			}
			if r.BlockType == "tool_use" {
				tools++
				var input any = map[string]any{}
				if json.Unmarshal([]byte(r.Text), &input) != nil {
					input = map[string]any{"value": r.Text}
				}
				tool := map[string]any{"kind": "tool", "id": r.ToolUseID, "name": r.ToolName, "start": r.Start, "end": r.End, "ms": r.MS, "summary": toolSummary(r.ToolName, input), "input": input, "result": "", "error": !r.OK, "req": r.RequestID, "uuid": r.TurnUUID}
				toolRows = append(toolRows, tool)
				toolByID[r.ToolUseID] = tool
				items = append(items, tool)
			}
			if r.BlockType == "tool_result" {
				if tool := toolByID[r.ToolUseID]; tool != nil {
					tool["result"] = r.Text
					tool["error"] = !r.OK
					tool["end"] = r.End
				}
			}
			if r.Role == "user" && r.BlockType == "text" {
				items = append(items, map[string]any{"kind": "prompt", "ts": r.Start, "uuid": r.TurnUUID, "text": r.Text})
			}
			if r.Role == "assistant" && (r.BlockType == "text" || r.BlockType == "thinking" || r.BlockType == "tool_use") {
				block := map[string]any{"type": r.BlockType, "ts": r.End, "text": r.Text, "uuid": r.TurnUUID}
				if r.BlockType == "tool_use" {
					block = map[string]any{"type": "tool_use", "ts": r.Start, "tool": r.ToolUseID}
				}
				req := map[string]any{"kind": "request", "id": r.RequestID, "start": r.Start, "first": r.End, "end": r.End, "ms": r.MS, "ttft": max64(0, r.End-r.Start), "model": r.Model, "effort": r.Effort, "usage": map[string]int64{"input": r.InputTokens, "output": r.OutputTokens, "cache_read": r.CacheReadTokens, "cache_write": r.CacheCreationTokens}, "blocks": []map[string]any{block}, "tools": []string{}, "cost": r.Cost, "context": r.InputTokens + r.CacheReadTokens + r.CacheCreationTokens, "uuid": r.TurnUUID}
				requests = append(requests, req)
				items = append(items, req)
			}
			if r.Role == "assistant" {
				model = r.Model
				cost += r.Cost
				addUsage(u, map[string]int64{"input": r.InputTokens, "output": r.OutputTokens, "cache_read": r.CacheReadTokens, "cache_write": r.CacheCreationTokens})
			}
		}
		out = append(out, map[string]any{"id": id, "parent_agent_id": parentID, "name": id, "type": "agent", "desc": "", "model": model, "start": start, "end": end, "ms": end - start, "call": "", "usage": u, "cost": cost, "n_tools": tools, "tools": nonNil(toolRows), "items": nonNil(items), "requests": nonNil(requests)})
	}
	return out
}
func addUsage(a, b map[string]int64) {
	for k, v := range b {
		a[k] += v
	}
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func sumToolMS(rows []trace.BlockRow) int64 {
	var n int64
	for _, r := range rows {
		if r.BlockType == "tool_use" && r.ToolName != "Agent" {
			n += r.MS
		}
	}
	return n
}
func countEdits(rows []trace.BlockRow) int {
	n := 0
	for _, r := range rows {
		if r.BlockType == "tool_use" && (r.ToolName == "Edit" || r.ToolName == "Write" || r.ToolName == "NotebookEdit") {
			n++
		}
	}
	return n
}
func toolSummary(name string, input any) string {
	m, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range []string{"description", "command", "file_path", "pattern", "query", "prompt", "path"} {
		if v, ok := m[k]; ok {
			return strings.Split(fmt.Sprint(v), "\n")[0]
		}
	}
	return ""
}
func toolTable(rows []trace.BlockRow) []map[string]any {
	type agg struct {
		n, fail int
		ms      int64
		ds      []int64
	}
	by := map[string]*agg{}
	for _, r := range rows {
		if r.BlockType != "tool_use" {
			continue
		}
		a := by[r.ToolName]
		if a == nil {
			a = &agg{}
			by[r.ToolName] = a
		}
		a.n++
		a.ms += r.MS
		a.ds = append(a.ds, r.MS)
		if !r.OK {
			a.fail++
		}
	}
	out := []map[string]any{}
	for name, a := range by {
		sort.Slice(a.ds, func(i, j int) bool { return a.ds[i] < a.ds[j] })
		out = append(out, map[string]any{"name": name, "n": a.n, "ms": a.ms, "fail": a.fail, "p50": a.ds[len(a.ds)/2]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["ms"].(int64) > out[j]["ms"].(int64) })
	return out
}
func modelTable(s trace.SessionRow) []map[string]any {
	return []map[string]any{{"model": s.Model, "n": s.RequestCount, "usage": map[string]int64{"input": s.InputTokens, "output": s.OutputTokens, "cache_read": s.CacheReadTokens, "cache_write": s.CacheCreationTokens}, "cost": s.Cost, "ms": s.APIMS}}
}

// nonNil keeps an empty list an empty list on the wire. A nil slice marshals
// to JSON null, and the page iterates every one of these.
func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// Latest state wins even while content-addressed legacy rows remain in Layer.
func latestSessions(rows []trace.SessionRow) []trace.SessionRow {
	byID := map[string]trace.SessionRow{}
	for _, r := range rows {
		old, ok := byID[r.SessionID]
		if !ok || r.End > old.End || (r.End == old.End && (r.ID == r.SessionID || (old.ID != old.SessionID && r.ID > old.ID))) {
			byID[r.SessionID] = r
		}
	}
	out := make([]trace.SessionRow, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].End == out[j].End {
			return out[i].SessionID < out[j].SessionID
		}
		return out[i].End > out[j].End
	})
	return out
}
