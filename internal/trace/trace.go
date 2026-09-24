// Package trace reads coding-agent session transcripts into one shape.
//
// Every harness writes a different file. Claude Code writes JSONL turns under
// ~/.claude/projects, Codex writes rollout JSONL under ~/.codex/sessions, and
// the next one will write something else again. A parser's whole job is to turn
// that into []Turn; everything downstream — chunk, write, search — sees only
// Turn and Block and never learns which harness produced them.
//
// The model is ported from huggingface/funes (Apache-2.0), which had already
// solved this across four harnesses. See NOTICE.
package trace

// Block is one typed piece of a turn. The four types are the union every
// harness expresses, however it spells them: what the model said, what it
// thought, what it called, and what came back.
type Block struct {
	Type      string `json:"type"` // text | thinking | tool_use | tool_result
	Text      string `json:"text"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Usage is the four billable token counters carried by an assistant request.
// Harnesses sometimes repeat the cumulative usage on several transcript lines;
// session aggregation de-duplicates those lines by RequestID.
type Usage struct {
	Input         int64 `json:"input_tokens,omitempty"`
	Output        int64 `json:"output_tokens,omitempty"`
	CacheRead     int64 `json:"cache_read_tokens,omitempty"`
	CacheCreation int64 `json:"cache_creation_tokens,omitempty"`
}

// Turn is one exchange in a session, already attributed to its session and
// working directory so a parser's output stands alone — nothing downstream has
// to remember which file a turn came from.
type Turn struct {
	SessionID  string  `json:"session_id"`
	TurnUUID   string  `json:"turn_uuid"`
	ParentUUID string  `json:"parent_uuid,omitempty"`
	Seq        int64   `json:"seq"`
	TS         string  `json:"ts"`
	Role       string  `json:"role"`
	Blocks     []Block `json:"blocks"`

	Workdir       string `json:"workdir,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Harness       string `json:"harness"` // claude_code | codex
	SourcePath    string `json:"source_path"`
	IsSidechain   bool   `json:"is_sidechain,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`

	// Assistant request metadata. It lives on the turn because Claude can emit
	// several assistant lines for one request, each containing one or more
	// blocks. Non-assistant turns leave these fields empty.
	Model     string  `json:"model,omitempty"`
	Effort    string  `json:"effort,omitempty"`
	RequestID string  `json:"request_id,omitempty"`
	Usage     Usage   `json:"usage,omitempty"`
	Cost      float64 `json:"cost,omitempty"`

	// Summary is a harness-supplied session title. It is repeated on the
	// session's turns so aggregation stays independent of source-specific
	// transcript records such as Claude Code's ai-title line.
	Summary string `json:"summary,omitempty"`
}

// Tier ranks a block by value per byte, which is what decides when it gets
// indexed rather than whether. Prose is what someone searches for; a 500 KB
// test log is one chunk of signal and four hundred of noise, and it is also
// most of the bytes. Nothing is ever dropped by tiering — an unknown block type
// folds into TierText so a harness that invents one is over-indexed, not lost.
type Tier int

const (
	TierText Tier = iota // text, thinking, and anything unrecognized
	TierToolUse
	TierToolResult
)

// AllTiers is index order: cheapest and highest value first.
var AllTiers = [3]Tier{TierText, TierToolUse, TierToolResult}

func (t Tier) String() string {
	switch t {
	case TierToolUse:
		return "tool_use"
	case TierToolResult:
		return "tool_result"
	default:
		return "text"
	}
}

// TierOf maps a block type to its tier.
func TierOf(blockType string) Tier {
	switch blockType {
	case "tool_use":
		return TierToolUse
	case "tool_result":
		return TierToolResult
	default:
		return TierText
	}
}

// Unit is one artifact a source indexes: usually a transcript file, but a
// bulk format may hold many sessions in one file.
//
// It is both granules that matter. Signature is a cheap change-stamp — a unit
// whose signature still matches what was recorded is skipped without being
// opened, which is what makes re-indexing a 110 MB tree nearly free. And a
// unit's turns are written in one commit, so an interrupted index leaves whole
// units behind rather than half a session.
type Unit struct {
	Key         string
	Signature   string
	IsSidechain bool
}

// Source enumerates and parses one kind of transcript store.
//
// Units is cheap: enumerate and stat, never parse. Read is called only for the
// units that were not skipped. Adding a harness is implementing this
// interface — nothing else in kit changes.
type Source interface {
	// Describe names what is being indexed, for the scan banner.
	Describe() string
	// Units lists what to consider, in deterministic order.
	Units() ([]Unit, error)
	// Read parses one unit into turns.
	Read(u Unit) ([]Turn, error)
}
