package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

func layerKey() (string, error) { return daemon.LayerKey() }

func client(namespace string) (*layer.Client, error) {
	cfg, _ := daemon.LoadConfig()
	key, store := "", ""
	endpoint := os.Getenv("LAYER_ENDPOINT")
	if cfg != nil {
		key, store = cfg.LayerAPIKey, cfg.LayerStore
		if endpoint == "" {
			endpoint = cfg.LayerEndpoint
		}
		if namespace == "" {
			namespace = cfg.LayerNamespace
		}
	}
	if key == "" {
		var err error
		key, err = layerKey()
		if err != nil {
			return nil, err
		}
	}
	if namespace == "" {
		namespace = envOr("LAYER_NAMESPACE", "hev-traces")
	}
	return layer.New(endpoint, key, namespace, os.Getenv("LAYER_EMBED_MODEL")).WithStore(layer.StoreKind(store))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	queryNamespace string
	queryTopK      int
	queryPlan      string
	queryWorkdir   string
	queryHost      string
	queryHarness   string
	querySince     string
	queryJSON      bool
	queryAlso      []string
)

// queryHit is the --json shape: the full chunk text plus the coordinates an
// agent needs to open the turn with hev trace.
type queryHit struct {
	SessionID string `json:"session_id"`
	TurnUUID  string `json:"turn_uuid"`
	TS        string `json:"ts"`
	Harness   string `json:"harness"`
	Role      string `json:"role"`
	BlockType string `json:"block_type"`
	ToolName  string `json:"tool_name,omitempty"`
	Sidechain bool   `json:"is_sidechain,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
	Plan      string `json:"plan,omitempty"`
	PR        string `json:"pr,omitempty"`
	Text      string `json:"text"`
}

// queryFilter ANDs every scoping flag that was set. Chunk ts is RFC 3339 UTC,
// so a string Gte is a time bound.
func queryFilter(now time.Time) (any, error) {
	var clauses []any
	if queryPlan != "" {
		clauses = append(clauses, []any{"plan", "Eq", queryPlan})
	}
	if queryWorkdir != "" {
		clauses = append(clauses, []any{"workdir", "Eq", queryWorkdir})
	}
	if queryHarness != "" {
		clauses = append(clauses, []any{"harness", "Eq", queryHarness})
	}
	if querySince != "" {
		d, err := parseSinceDuration(querySince)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, []any{"ts", "Gte", now.Add(-d).UTC().Format(time.RFC3339)})
	}
	switch len(clauses) {
	case 0:
		return nil, nil
	case 1:
		return clauses[0], nil
	default:
		return []any{"And", clauses}, nil
	}
}

var queryCmd = &cobra.Command{
	Use:     "query <query>",
	Aliases: []string{"find", "q"},
	Short:   "Search your traces (hybrid: semantic + full-text)",
	Long: `Search the trace archive.

Retrieval is hybrid and runs entirely in the store: the query is embedded
server-side for the semantic leg, BM25 scores the same text for the lexical
leg, and the two are fused by reciprocal rank before anything comes back.
Nothing is embedded, fused, or reranked on this machine.

--also adds another phrasing of the same question; up to eight ride in one
request as extra legs, and the store fuses them all. Mix a natural-language
phrasing with the literal tokens you expect (an error string, a flag, a file
name): the dense legs match meaning, the BM25 legs match exact words.

Scoping flags combine: --plan, --workdir, --harness and --since are ANDed.
--json prints full chunk text with the session and turn ids, for agents and
scripts; open a hit's context with hev trace --json <session-id>.

  hev query "why did the turbopuffer preflight fail"
  hev query "why did the preflight fail" --also "TURBOPUFFER_API_KEY 401"
  hev query --plan scoped-key-leak "what did the worker try"
  hev query --since 7d --harness codex --json "flaky deploy"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if queryHost != "" {
			return fmt.Errorf("--host is not supported yet: chunk rows do not record the capturing machine")
		}
		filter, err := queryFilter(time.Now())
		if err != nil {
			return err
		}
		cl, err := client(queryNamespace)
		if err != nil {
			return err
		}
		hits, err := cl.SearchPhrasings(append([]string{strings.Join(args, " ")}, queryAlso...), queryTopK, filter)
		if err != nil {
			return err
		}
		// The fused set can hold up to one top_k per leg; --top is the
		// caller's budget.
		if queryTopK > 0 && len(hits) > queryTopK {
			hits = hits[:queryTopK]
		}
		if queryJSON {
			out := make([]queryHit, len(hits))
			for i, h := range hits {
				out[i] = queryHit{SessionID: h.SessionID, TurnUUID: h.TurnUUID, TS: h.TS, Harness: h.Harness,
					Role: h.Role, BlockType: h.BlockType, ToolName: h.ToolName, Sidechain: h.Sidechain,
					Workdir: h.Workdir, Plan: h.Plan, PR: h.PR, Text: h.Text}
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}
		if len(hits) == 0 {
			fmt.Println("nothing found")
			return nil
		}
		bold, dim, reset := "\033[1m", "\033[2m", "\033[0m"
		if !isTerminal(os.Stdout) || os.Getenv("NO_COLOR") != "" {
			bold, dim, reset = "", "", ""
		}
		for _, h := range hits {
			when := h.TS
			if t, err := time.Parse(time.RFC3339, h.TS); err == nil {
				when = t.Local().Format("2006-01-02 15:04")
			}
			head := fmt.Sprintf("%s  %s  %s/%s", when, h.Harness, h.Role, h.BlockType)
			if h.ToolName != "" {
				head += "  " + h.ToolName
			}
			if h.Plan != "" {
				head += "  plan=" + h.Plan
			}
			if h.Workdir != "" {
				head += "  " + h.Workdir
			}
			fmt.Printf("\n%s%s%s\n", bold, head, reset)
			fmt.Printf("  %s\n", oneParagraph(h.Text, 400))
			fmt.Printf("  %ssession %s  turn %s%s\n", dim, h.SessionID, h.TurnUUID, reset)
		}
		return nil
	},
}

func oneParagraph(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

var (
	indexNamespace string
	indexTiers     []string
	indexLimit     int
	indexForce     bool
	indexDryRun    bool
	indexReadSide  bool
	indexWorkers   int
	indexRoot      string
	indexSummarize bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index local agent transcripts into your Layer namespace",
	Long: `Read agent session transcripts and upsert them as searchable chunks.

Units whose size and mtime have not moved are skipped, so re-running is cheap.
Chunk ids are content-addressed, so re-running is also safe: a chunk that has
already landed upserts over itself rather than duplicating.

Tiers rank blocks by value per byte. text and tool_use are indexed by default;
tool_result is most of the bytes and the least of what anyone searches for, so
it is opt-in.

  hev index                          text + tool_use
  hev index --tier all               everything
  hev index --limit 20 --dry-run     see what it would do`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if indexWorkers < 1 || indexWorkers > 8 || (indexWorkers > 1 && (!indexReadSide || indexSummarize)) {
			return fmt.Errorf("--workers must be 1–8; values above 1 require --read-side without --summarize")
		}
		if indexSummarize && indexDryRun {
			return fmt.Errorf("--summarize and --dry-run cannot be used together")
		}
		claudeRoot := indexRoot
		if claudeRoot == "" {
			claudeRoot = trace.DefaultClaudeRoot()
		}

		var tiers []trace.Tier
		for _, t := range indexTiers {
			switch strings.ToLower(t) {
			case "all":
				tiers = append(tiers, trace.TierText, trace.TierToolUse, trace.TierToolResult)
			case "text":
				tiers = append(tiers, trace.TierText)
			case "tool_use":
				tiers = append(tiers, trace.TierToolUse)
			case "tool_result":
				tiers = append(tiers, trace.TierToolResult)
			default:
				return fmt.Errorf("unknown tier %q (text, tool_use, tool_result, all)", t)
			}
		}

		cl, err := client(indexNamespace)
		if err != nil && !indexDryRun {
			return err
		}
		st := index.LoadState()
		sources := []trace.Source{
			&trace.ClaudeSource{Root: claudeRoot},
			&trace.CodexSource{Root: trace.DefaultCodexRoot()},
		}
		rep := &index.Report{}
		existingSummaries := map[string]string{}
		existingSessions := map[string]bool{}
		if indexSummarize {
			rows, err := cl.ListSessionRows(10000, nil)
			if err != nil {
				return fmt.Errorf("list sessions before summarizing: %w", err)
			}
			for _, row := range rows {
				existingSessions[row.SessionID] = true
				if summary := strings.TrimSpace(row.Summary); summary != "" {
					existingSummaries[row.SessionID] = summary
				}
			}
		}
		found := false
		for _, src := range sources {
			if _, err := os.Stat(sourceRoot(src)); err != nil {
				if indexRoot != "" && sourceRoot(src) == claudeRoot {
					return fmt.Errorf("no transcripts at %s", claudeRoot)
				}
				continue
			}
			found = true
			fmt.Fprintf(os.Stderr, "indexing %s\n", src.Describe())
			var one *index.Report
			if indexSummarize {
				one, err = index.Summarize(src, cl, 200, nil, existingSessions, func(session trace.SessionRow) (string, error) {
					if summary := existingSummaries[session.SessionID]; summary != "" {
						return summary, nil
					}
					// Generate after local harness titles have landed, from the
					// authoritative archive rows and with bounded concurrency.
					return "", nil
				}, func(done, total int, unit string) {
					fmt.Fprintf(os.Stderr, "\r\033[K  %d/%d %s", done, total, truncLeft(unit, 60))
				})
			} else {
				one, err = index.Run(src, cl, st, index.Options{
					Tiers:    tiers,
					Limit:    indexLimit,
					Force:    indexForce,
					DryRun:   indexDryRun,
					ReadSide: indexReadSide,
					Workers:  indexWorkers,
					Progress: func(done, total int, unit string) {
						fmt.Fprintf(os.Stderr, "\r\033[K  %d/%d %s", done, total, truncLeft(unit, 60))
					},
				})
			}
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return err
			}
			fmt.Printf("%-11s %d units seen, %d indexed, %d unchanged, %d embedding tokens\n",
				sourceHarness(src), one.UnitsSeen, one.UnitsIndexed, one.UnitsSkipped, one.EmbeddingTokens)
			addIndexReport(rep, one)
		}
		if indexSummarize {
			// A session may outlive its local transcript; that is the point of the
			// archive. Re-read the session namespace after applying local harness
			// titles and generate from the stored first prompt for anything left.
			rows, err := cl.ListSessionRows(10000, nil)
			if err != nil {
				return fmt.Errorf("list sessions after harness titles: %w", err)
			}
			changed, summaryErrors := claudeSummaries(rows)
			rep.Errors = append(rep.Errors, summaryErrors...)
			for start := 0; start < len(changed); start += 200 {
				end := start + 200
				if end > len(changed) {
					end = len(changed)
				}
				result, err := cl.PatchSessionSummaries(changed[start:end])
				if err != nil {
					return fmt.Errorf("write generated session summaries: %w", err)
				}
				rep.SessionRowsUpserted += result.RowsUpserted
			}
		}
		if !found {
			return fmt.Errorf("no transcripts at %s or %s", claudeRoot, trace.DefaultCodexRoot())
		}
		if !indexDryRun && !indexSummarize {
			if err := st.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not save index state: %v\n", err)
			}
		}

		fmt.Printf("units   %d seen, %d indexed, %d unchanged\n", rep.UnitsSeen, rep.UnitsIndexed, rep.UnitsSkipped)
		fmt.Printf("content %d turns → %d chunks, %d blocks, %d sessions\n", rep.Turns, rep.Chunks, rep.Blocks, rep.Sessions)
		if indexDryRun {
			fmt.Println("dry run — nothing written")
		} else {
			fmt.Printf("written %d chunk, %d block, %d session rows; %d embedding tokens\n",
				rep.RowsUpserted, rep.BlockRowsUpserted, rep.SessionRowsUpserted, rep.EmbeddingTokens)
		}
		for _, e := range rep.Errors {
			fmt.Fprintf(os.Stderr, "error: %s\n", e)
		}
		if indexSummarize && len(rep.Errors) > 0 {
			return fmt.Errorf("could not summarize %d session(s)", len(rep.Errors))
		}
		return nil
	},
}

func sourceRoot(src trace.Source) string {
	switch s := src.(type) {
	case *trace.ClaudeSource:
		return s.Root
	case *trace.CodexSource:
		return s.Root
	default:
		return ""
	}
}

func sourceHarness(src trace.Source) string {
	switch src.(type) {
	case *trace.ClaudeSource:
		return "claude_code"
	case *trace.CodexSource:
		return "codex"
	default:
		return "unknown"
	}
}

func addIndexReport(dst, src *index.Report) {
	dst.UnitsSeen += src.UnitsSeen
	dst.UnitsIndexed += src.UnitsIndexed
	dst.UnitsSkipped += src.UnitsSkipped
	dst.Turns += src.Turns
	dst.Chunks += src.Chunks
	dst.Blocks += src.Blocks
	dst.Sessions += src.Sessions
	dst.RowsUpserted += src.RowsUpserted
	dst.BlockRowsUpserted += src.BlockRowsUpserted
	dst.SessionRowsUpserted += src.SessionRowsUpserted
	dst.EmbeddingTokens += src.EmbeddingTokens
	dst.Errors = append(dst.Errors, src.Errors...)
}

func truncLeft(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max:])
}

func init() {
	queryCmd.Flags().StringVar(&queryNamespace, "namespace", "", "Layer namespace (default $LAYER_NAMESPACE or hev-traces)")
	queryCmd.Flags().IntVar(&queryTopK, "top", 8, "results to return")
	queryCmd.Flags().StringVar(&queryPlan, "plan", "", "only chunks from this plan")
	queryCmd.Flags().StringVar(&queryWorkdir, "workdir", "", "only chunks from this working directory")
	queryCmd.Flags().StringVar(&queryHost, "host", "", "only chunks captured on this machine")
	_ = queryCmd.Flags().MarkHidden("host")
	queryCmd.Flags().StringVar(&queryHarness, "harness", "", "only chunks from this harness (claude_code or codex)")
	queryCmd.Flags().StringVar(&querySince, "since", "", "only chunks from within this window (e.g. 2h, 7d)")
	queryCmd.Flags().StringArrayVar(&queryAlso, "also", nil, "another phrasing of the same question, fused in the same request (repeatable, up to 8 in all)")
	queryCmd.Flags().BoolVar(&queryJSON, "json", false, "print hits as JSON with full text and turn ids")

	indexCmd.Flags().StringVar(&indexNamespace, "namespace", "", "Layer namespace (default $LAYER_NAMESPACE or hev-traces)")
	indexCmd.Flags().StringSliceVar(&indexTiers, "tier", nil, "text, tool_use, tool_result, or all (default text,tool_use)")
	indexCmd.Flags().IntVar(&indexLimit, "limit", 0, "stop after this many transcripts")
	indexCmd.Flags().BoolVar(&indexForce, "force", false, "re-index units even if unchanged")
	indexCmd.Flags().BoolVar(&indexDryRun, "dry-run", false, "parse and chunk without writing")
	indexCmd.Flags().IntVar(&indexWorkers, "workers", 1, "concurrent read-side transcripts (1–8; requires --read-side above 1)")
	indexCmd.Flags().BoolVar(&indexReadSide, "read-side", false, "write only the blocks and sessions namespaces; no chunks, no embedding (use with --force to backfill)")
	indexCmd.Flags().StringVar(&indexRoot, "root", "", "Claude transcript root (default ~/.claude/projects; Codex is always ~/.codex/sessions)")
	indexCmd.Flags().BoolVar(&indexSummarize, "summarize", false, "fill missing session summaries using harness titles or local Claude Haiku")
}

func claudeSummary(session trace.SessionRow) (string, error) {
	titles, err := claudeSummaryBatch([]trace.SessionRow{session})
	if err != nil {
		return "", err
	}
	return titles[0], nil
}

func claudeSummaryBatch(sessions []trace.SessionRow) ([]string, error) {
	type promptSession struct {
		FirstPrompt string `json:"first_prompt"`
	}
	input := make([]promptSession, len(sessions))
	for i, session := range sessions {
		firstPrompt := []rune(session.FirstPrompt)
		if len(firstPrompt) > 1000 {
			firstPrompt = firstPrompt[:1000]
		}
		input[i].FirstPrompt = string(firstPrompt)
	}
	rawInput, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	schema := fmt.Sprintf(`{"type":"object","properties":{"titles":{"type":"array","items":{"type":"string"},"minItems":%d,"maxItems":%d}},"required":["titles"]}`, len(sessions), len(sessions))
	prompt := "Write a concise one-line title for each coding-agent session in the JSON array, preserving array order. Return exactly one title per input through the requested schema.\n\nSessions:\n" + string(rawInput)
	// Title generation is plumbing, not an operator trace. Persistence would
	// let an index loop archive each summarizer invocation as another empty
	// session, making backfill manufacture the work it is trying to finish.
	command := exec.Command("claude", "-p", "--model", "haiku", "--no-session-persistence", "--tools", "", "--output-format", "json", "--json-schema", schema)
	command.Stdin = strings.NewReader(prompt)
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("claude -p --model haiku: %w", err)
	}
	var response struct {
		StructuredOutput struct {
			Titles []string `json:"titles"`
		} `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, fmt.Errorf("decode claude titles: %w", err)
	}
	if len(response.StructuredOutput.Titles) != len(sessions) {
		return nil, fmt.Errorf("claude returned %d titles for %d sessions", len(response.StructuredOutput.Titles), len(sessions))
	}
	for i, summary := range response.StructuredOutput.Titles {
		summary = strings.Join(strings.Fields(summary), " ")
		runes := []rune(summary)
		if len(runes) > 200 {
			summary = string(runes[:200])
		}
		response.StructuredOutput.Titles[i] = summary
	}
	return response.StructuredOutput.Titles, nil
}

func claudeSummaries(rows []trace.SessionRow) ([]trace.SessionRow, []string) {
	const batchSize = 40
	type batchResult struct {
		start, end int
		titles     []string
		err        error
	}
	var missing []trace.SessionRow
	for _, row := range rows {
		if strings.TrimSpace(row.Summary) == "" {
			missing = append(missing, row)
		}
	}
	results := make(chan batchResult, (len(missing)+batchSize-1)/batchSize)
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for start := 0; start < len(missing); start += batchSize {
		end := start + batchSize
		if end > len(missing) {
			end = len(missing)
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			titles, err := claudeSummaryBatch(missing[start:end])
			results <- batchResult{start: start, end: end, titles: titles, err: err}
		}(start, end)
	}
	wg.Wait()
	close(results)

	var errors []string
	for result := range results {
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("sessions %s..%s summaries: %v", missing[result.start].SessionID, missing[result.end-1].SessionID, result.err))
			continue
		}
		for i, summary := range result.titles {
			if summary == "" {
				errors = append(errors, fmt.Sprintf("%s summary: claude returned an empty title", missing[result.start+i].SessionID))
				continue
			}
			missing[result.start+i].Summary = summary
		}
	}
	changed := missing[:0]
	for _, row := range missing {
		if row.Summary != "" {
			changed = append(changed, row)
		}
	}
	return changed, errors
}

func fillMissingSummaries(rows []trace.SessionRow, summarize func(trace.SessionRow) (string, error)) ([]trace.SessionRow, []string) {
	type result struct {
		summary string
		err     error
	}
	results := make([]result, len(rows))
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range rows {
		if strings.TrimSpace(rows[i].Summary) != "" {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			results[i].summary, results[i].err = summarize(rows[i])
		}(i)
	}
	wg.Wait()

	var changed []trace.SessionRow
	var errors []string
	for i, result := range results {
		if strings.TrimSpace(rows[i].Summary) != "" {
			continue
		}
		if result.err != nil {
			errors = append(errors, fmt.Sprintf("%s summary: %v", rows[i].SessionID, result.err))
			continue
		}
		rows[i].Summary = result.summary
		changed = append(changed, rows[i])
	}
	return changed, errors
}
