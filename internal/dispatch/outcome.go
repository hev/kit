package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Final markers a worker writes as the last line of ../status.md.
const (
	OutcomeDone          = "done"
	OutcomeBlocked       = "blocked"
	OutcomeNeedsDecision = "needs-decision"
)

// Decision is one keyed question a worker asked and, once resolved, its answer.
type Decision struct {
	Key        string    `json:"key"`
	Question   string    `json:"question"`
	Asked      time.Time `json:"asked"`
	Answer     string    `json:"answer,omitempty"`
	Resolved   time.Time `json:"resolved,omitempty"`
	ResolvedBy string    `json:"resolved_by,omitempty"`
}

var decisionKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

// ValidDecisionKey reports whether key may name a decision.
func ValidDecisionKey(key string) bool { return decisionKey.MatchString(key) }

// ParseOutcome reads the final marker from the last non-empty line of a
// worker's status. A missing or malformed marker returns an empty outcome, and
// the run keeps the exit-code state it had before markers existed.
func ParseOutcome(status string) (outcome, key, text string) {
	lines := strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n")
	last := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if last = strings.TrimSpace(lines[i]); last != "" {
			break
		}
	}
	// Tolerate a list bullet or inline code around the marker.
	last = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(last, "- "), "* "))
	last = strings.TrimSpace(strings.Trim(last, "`"))
	head, rest, colon := strings.Cut(last, ":")
	rest = strings.TrimSpace(rest)
	switch {
	case head == OutcomeDone:
		return OutcomeDone, "", rest
	case head == OutcomeBlocked && colon && rest != "":
		return OutcomeBlocked, "", rest
	case colon && rest != "" && strings.HasPrefix(head, OutcomeNeedsDecision+" "):
		key = strings.TrimSpace(strings.TrimPrefix(head, OutcomeNeedsDecision+" "))
		if ValidDecisionKey(key) {
			return OutcomeNeedsDecision, key, rest
		}
	}
	return "", "", ""
}

// recordOutcome sets the result's outcome from status.md and opens a decision
// in the run's ledger when the worker asked for one.
func recordOutcome(dir string, r *Result) {
	r.Outcome, r.Key, r.Question, r.Reason = "", "", "", ""
	b, err := os.ReadFile(filepath.Join(dir, "status.md"))
	// A marker counts only on a clean exit; a crash or timeout keeps its state.
	if err != nil || r.ExitCode != 0 || r.Timeout || r.Stopped {
		r.Decisions = resolvedDecisions(dir)
		return
	}
	outcome, key, text := ParseOutcome(string(b))
	r.Outcome = outcome
	switch outcome {
	case OutcomeBlocked:
		r.Reason = text
	case OutcomeNeedsDecision:
		r.Key, r.Question = key, text
		if unlock, err := lockRoot(); err == nil {
			ledger := loadDecisions(dir)
			ledger = append(ledger, Decision{Key: key, Question: text, Asked: time.Now().UTC()})
			_ = WriteJSON(filepath.Join(dir, "decisions.json"), ledger)
			unlock()
		}
	}
	r.Decisions = resolvedDecisions(dir)
}

func loadDecisions(dir string) []Decision {
	var ledger []Decision
	_ = readJSON(filepath.Join(dir, "decisions.json"), &ledger)
	return ledger
}

func resolvedDecisions(dir string) []Decision {
	var out []Decision
	for _, d := range loadDecisions(dir) {
		if !d.Resolved.IsZero() {
			out = append(out, d)
		}
	}
	return out
}

// OpenDecision is the decision a finished run is waiting on, if any.
func OpenDecision(r *Result) bool {
	return r != nil && r.Outcome == OutcomeNeedsDecision && r.Key != "" && State(r, false) == "needs-review"
}

// resolveDecision checks that key names the run's open decision and returns
// it answered, with a commit that records the answer in the ledger. The caller
// holds the root lock and commits only once the rework is admitted. An unknown
// or already-resolved key is refused.
func resolveDecision(dir, key, answer, by string) (Decision, func() error, error) {
	if strings.TrimSpace(answer) == "" {
		return Decision{}, nil, fmt.Errorf("answer required to resolve %s", key)
	}
	ledger := loadDecisions(dir)
	var current Result
	_ = readJSON(filepath.Join(dir, "result.json"), &current)
	if !OpenDecision(&current) || current.Key != key {
		for i := len(ledger) - 1; i >= 0; i-- {
			if ledger[i].Key == key && !ledger[i].Resolved.IsZero() {
				return Decision{}, nil, fmt.Errorf("decision %s already resolved: %s", key, ledger[i].Answer)
			}
		}
		if OpenDecision(&current) {
			return Decision{}, nil, fmt.Errorf("unknown decision key %s; open decision is %s", key, current.Key)
		}
		return Decision{}, nil, fmt.Errorf("unknown decision key %s; no open decision", key)
	}
	i := len(ledger) - 1
	for i >= 0 && (ledger[i].Key != key || !ledger[i].Resolved.IsZero()) {
		i--
	}
	if i < 0 {
		// The ledger predates this result; record the question from it.
		ledger = append(ledger, Decision{Key: key, Question: current.Question, Asked: current.End})
		i = len(ledger) - 1
	}
	ledger[i].Answer = strings.TrimSpace(answer)
	ledger[i].Resolved = time.Now().UTC()
	ledger[i].ResolvedBy = by
	commit := func() error { return WriteJSON(filepath.Join(dir, "decisions.json"), ledger) }
	return ledger[i], commit, nil
}

func decisionNote(d Decision) string {
	return fmt.Sprintf("Decision %s resolved.\n\nQuestion: %s\n\nAnswer: %s", d.Key, d.Question, d.Answer)
}

// decisionPrompt names every resolved decision so a rework starts from the answers.
func decisionPrompt(dir string) string {
	s := ""
	for _, d := range resolvedDecisions(dir) {
		s += fmt.Sprintf(" Decision %s (%s) was resolved: %s.", d.Key, d.Question, d.Answer)
	}
	return s
}

// outcomeSummary is the marker text for notices and ls.
func outcomeSummary(r Result) string {
	switch r.Outcome {
	case OutcomeBlocked:
		return "blocked: " + r.Reason
	case OutcomeNeedsDecision:
		return OutcomeNeedsDecision + " " + r.Key + ": " + r.Question
	}
	return ""
}

// OutcomePrompt asks the worker for exactly one final marker.
const OutcomePrompt = " End by making the last line of ../status.md exactly one of: `done`, `blocked: <reason>`, or `needs-decision <KEY>: <question>` when you need the dispatcher to choose before you can continue (KEY is a short word such as db). Stop after writing needs-decision; the answer arrives as a rework."
