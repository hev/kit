package trace

// SpanRow is one dispatch trace fact in the archive's spans namespace: an
// ask, or one attempt of a run (implementation or validation). Each host's
// indexer publishes the facts of the run records it holds, so a dashboard on
// any host sharing the archive can derive the same ask-rooted traces.
//
// Only what the Asks view shows is published: identifiers, times, states,
// hosts, harness and model, branch, head and PR, the ask text (scrubbed) and
// the order's title line. Order bodies, Acceptance, reports, notes beyond
// the ask text, errors and environment are never published.
type SpanRow struct {
	// ID is the span ID: the ask's, or the attempt's (derived for an
	// untraced attempt).
	ID string `json:"id"`
	// Kind is "ask" or "attempt".
	Kind    string `json:"kind"`
	TraceID string `json:"trace_id,omitempty"`
	// Run is the run directory name the fact was recorded on, RunKey that
	// run's identity across hosts, Publisher the host whose indexer wrote it.
	Run       string `json:"run"`
	RunKey    string `json:"run_key"`
	Publisher string `json:"publisher"`
	// Start is the ask time, or the attempt's queued time, in Unix ms.
	Start        int64  `json:"start"`
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// Ask facts.
	Action  string `json:"action,omitempty"`
	AskKind string `json:"ask_kind,omitempty"`
	Text    string `json:"text,omitempty"`
	// Caller is the dispatch caller as JSON: session, harness, harness
	// session, host, sender and run.
	Caller string `json:"caller,omitempty"`

	// Attempt facts, with the run-level facts the view needs repeated on
	// each attempt so a run can be rebuilt from its rows alone.
	Attempt   int64  `json:"attempt,omitempty"`
	Revision  int64  `json:"revision,omitempty"`
	Revisions int64  `json:"revisions,omitempty"`
	Source    string `json:"source,omitempty"`
	Title     string `json:"title,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Host      string `json:"host,omitempty"`
	Harness   string `json:"harness,omitempty"`
	Model     string `json:"model,omitempty"`
	// RunStart and End bound the attempt's result; zero when it has none.
	RunStart    int64  `json:"run_start,omitempty"`
	End         int64  `json:"end,omitempty"`
	Result      int64  `json:"result,omitempty"`
	ExitCode    int64  `json:"exit_code,omitempty"`
	Unconfirmed int64  `json:"unconfirmed,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
	Validation  string `json:"validation,omitempty"`
	Head        string `json:"head,omitempty"`
	PR          string `json:"pr,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Untraced    string `json:"untraced,omitempty"`
	// Validations lists the run's validation runs as JSON
	// [{"name","host","state"}], on its latest attempt.
	Validations string `json:"validations,omitempty"`
	// ContextDigest is the context bundle the attempt was given, and
	// ContextRecipe the recipe it was built from; empty without a recipe.
	ContextDigest string `json:"context_digest,omitempty"`
	ContextRecipe string `json:"context_recipe,omitempty"`
}
