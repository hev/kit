// Package preval evaluates explicitly attributed pull request outcomes.
package preval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"
)

const Schema = "hev.evaluation/v1"
const PolicyVersion = "pr-outcome-1"

const (
	GitHub      = "github"
	AzureDevOps = "azure_devops"

	Active     = "active"
	Merged     = "merged"
	Closed     = "closed_unmerged"
	Stale      = "stale"
	Superseded = "superseded"
	Corrected  = "corrected"
	Reverted   = "reverted"
)

var (
	identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,199}$`)
	errInvalid = errors.New("invalid pull request evaluation data")
)

type Repository struct {
	Provider     string `json:"provider"`
	Organization string `json:"organization"`
	Project      string `json:"project,omitempty"`
	Repository   string `json:"repository"`
}

func (r Repository) valid() bool {
	if !identifier.MatchString(r.Organization) || !identifier.MatchString(r.Repository) {
		return false
	}
	return r.Provider == GitHub && r.Project == "" || r.Provider == AzureDevOps && identifier.MatchString(r.Project)
}

type PullRequest struct {
	Repository
	ID int64 `json:"pr_id,string"`
}

type Source struct {
	PullRequest
	Revision   string    `json:"revision"`
	EventID    string    `json:"event_id"`
	OccurredAt time.Time `json:"occurred_at,omitzero"`
}

type Provenance struct {
	OrderID         string   `json:"order_id"`
	AttemptID       string   `json:"attempt_id"`
	TraceSessionIDs []string `json:"trace_session_ids"`
	EvidenceIDs     []string `json:"evidence_ids"`
}

func validIDs(ids []string, limit int) bool {
	if len(ids) > limit {
		return false
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !identifier.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

type Learning struct {
	Status string   `json:"status"`
	Refs   []string `json:"refs,omitempty"`
}

type Evaluation struct {
	Schema        string     `json:"schema"`
	ID            string     `json:"evaluation_id"`
	PolicyVersion string     `json:"policy_version"`
	ObservedAt    time.Time  `json:"observed_at"`
	Outcome       string     `json:"outcome"`
	Score         *float64   `json:"score"`
	Reason        string     `json:"reason"`
	Source        Source     `json:"source"`
	Provenance    Provenance `json:"provenance"`
	Learning      Learning   `json:"learning"`
}

type Observation struct {
	Source       Source    `json:"source"`
	ObservedAt   time.Time `json:"observed_at"`
	LastActivity time.Time `json:"last_activity,omitempty"`
	Outcome      string    `json:"outcome"`
}

// Attribution is a workspace-owned registration. Corrections must come from
// the registration, independently of any text in the PR or its reviews.
type Attribution struct {
	PullRequest PullRequest  `json:"pull_request"`
	Provenance  Provenance   `json:"provenance"`
	Correction  *Observation `json:"correction,omitempty"`
}

func isCorrection(outcome string) bool {
	return outcome == Superseded || outcome == Corrected || outcome == Reverted
}

func score(value float64) *float64 { return &value }

// Project uses only successful provider observations. A zero LastActivity
// means the provider did not supply enough information to assess inactivity.
func Project(link Attribution, observation Observation, previous *Evaluation, staleAfter time.Duration, learningEnabled bool) (Evaluation, error) {
	p := link.Provenance
	if p.TraceSessionIDs == nil {
		p.TraceSessionIDs = []string{}
	}
	if p.EvidenceIDs == nil {
		p.EvidenceIDs = []string{}
	}
	if !validIDs([]string{p.OrderID}, 1) || !validIDs([]string{p.AttemptID}, 1) ||
		!validIDs(p.TraceSessionIDs, 32) || !validIDs(p.EvidenceIDs, 32) ||
		link.PullRequest != observation.Source.PullRequest || observation.Source.ID <= 0 || !observation.Source.Repository.valid() ||
		!slices.Contains([]string{Active, Merged, Closed}, observation.Outcome) {
		return Evaluation{}, errInvalid
	}
	key, _ := json.Marshal(struct {
		PullRequest        PullRequest
		OrderID, AttemptID string
	}{link.PullRequest, p.OrderID, p.AttemptID})
	id := sha256.Sum256(key)

	if link.Correction != nil {
		observedAt := observation.ObservedAt
		if !slices.Contains([]string{Superseded, Corrected, Reverted}, link.Correction.Outcome) || link.Correction.ObservedAt.After(observation.ObservedAt) {
			return Evaluation{}, errInvalid
		}
		observation = *link.Correction
		if observation.ObservedAt.IsZero() {
			return Evaluation{}, errInvalid
		}
		// Source identifies the correction. The projection revision is the time
		// we observed it, which must follow any previous provider poll.
		observation.Source.OccurredAt = observation.ObservedAt.UTC()
		observation.ObservedAt = observedAt
	}
	if observation.ObservedAt.IsZero() || observation.LastActivity.After(observation.ObservedAt) ||
		observation.Source.ID <= 0 || !observation.Source.Repository.valid() ||
		!identifier.MatchString(observation.Source.Revision) || !identifier.MatchString(observation.Source.EventID) {
		return Evaluation{}, errInvalid
	}
	result := Evaluation{
		Schema: Schema, ID: "pr_" + hex.EncodeToString(id[:]), PolicyVersion: PolicyVersion,
		ObservedAt: observation.ObservedAt.UTC(), Outcome: observation.Outcome,
		Source: observation.Source, Provenance: p, Learning: Learning{Status: "disabled"},
	}
	switch result.Outcome {
	case Merged:
		result.Score, result.Reason = score(1), "Provider confirmed the pull request was merged."
	case Closed:
		result.Score, result.Reason = score(-1), "Provider confirmed the pull request was closed without merging."
	case Superseded, Corrected, Reverted:
		result.Score, result.Reason = score(-1), "An explicit provenance link marked the attributed attempt as "+result.Outcome+"."
	case Active:
		result.Reason = "The pull request is active."
		activity := observation.LastActivity
		if previous != nil && previous.Outcome == Closed {
			// Reactivation is itself activity, even when a provider does not
			// report it in the history used to assess inactivity.
			activity = observation.ObservedAt
		}
		if previous != nil && previous.Outcome == Active && !activity.IsZero() && previous.ObservedAt.After(activity) {
			activity = previous.ObservedAt
		}
		if staleAfter > 0 && !activity.IsZero() && observation.ObservedAt.Sub(activity) >= staleAfter {
			result.Outcome, result.Score, result.Reason = Stale, score(-0.25), "The active pull request exceeded the configured inactivity threshold."
		}
	}
	if learningEnabled && result.Outcome != Active && result.Outcome != Stale {
		result.Learning.Status = "pending"
	}
	if previous == nil {
		return result, nil
	}
	if previous.ID != result.ID ||
		!containsAll(p.TraceSessionIDs, previous.Provenance.TraceSessionIDs) ||
		!containsAll(p.EvidenceIDs, previous.Provenance.EvidenceIDs) {
		return Evaluation{}, errInvalid
	}
	if !result.ObservedAt.After(previous.ObservedAt) ||
		isCorrection(previous.Outcome) && (!isCorrection(result.Outcome) ||
			previous.Source != result.Source && !result.Source.OccurredAt.After(previous.Source.OccurredAt)) ||
		previous.Outcome == Merged && (result.Outcome == Active || result.Outcome == Stale || result.Outcome == Closed) {
		return *previous, nil
	}
	if previous.Outcome == result.Outcome && previous.Source.PullRequest == result.Source.PullRequest &&
		(!isCorrection(result.Outcome) || previous.Source == result.Source) {
		result.Learning = previous.Learning
		sameLinks := slices.Equal(previous.Provenance.TraceSessionIDs, result.Provenance.TraceSessionIDs) &&
			slices.Equal(previous.Provenance.EvidenceIDs, result.Provenance.EvidenceIDs)
		hasNewActivity := result.Outcome == Active && observation.LastActivity.After(previous.ObservedAt)
		if sameLinks && !hasNewActivity {
			return *previous, nil
		}
		if !hasNewActivity {
			result.Source = previous.Source
		}
	}

	return result, nil
}

// UnmarshalJSON accepts the compact string used by Layer and the native object
// used by local source documents and the API.
func (e *Evaluation) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var wire string
		if err := json.Unmarshal(raw, &wire); err != nil {
			return errInvalid
		}
		raw = []byte(wire)
	}
	type projection Evaluation
	return json.Unmarshal(raw, (*projection)(e))
}

func (e Evaluation) Valid() bool {
	p := e.Provenance
	if e.Schema != Schema || e.PolicyVersion != PolicyVersion || e.ObservedAt.IsZero() ||
		e.Source.OccurredAt.After(e.ObservedAt) || !isCorrection(e.Outcome) && !e.Source.OccurredAt.IsZero() ||
		e.Source.ID <= 0 || !e.Source.Repository.valid() || !identifier.MatchString(e.Source.Revision) || !identifier.MatchString(e.Source.EventID) ||
		!identifier.MatchString(e.ID) || !identifier.MatchString(p.OrderID) || !identifier.MatchString(p.AttemptID) ||
		p.TraceSessionIDs == nil || p.EvidenceIDs == nil || !validIDs(p.TraceSessionIDs, 32) || !validIDs(p.EvidenceIDs, 32) ||
		!validIDs(e.Learning.Refs, 16) || !slices.Contains([]string{"pending", "disabled", "failed", "none", "recorded"}, e.Learning.Status) ||
		(e.Learning.Status == "recorded") != (len(e.Learning.Refs) > 0) {
		return false
	}
	var expected float64
	switch e.Outcome {
	case Active:
		return e.Score == nil && e.Reason == "The pull request is active."
	case Merged:
		expected = 1
		if e.Reason != "Provider confirmed the pull request was merged." {
			return false
		}
	case Closed:
		expected = -1
		if e.Reason != "Provider confirmed the pull request was closed without merging." {
			return false
		}
	case Stale:
		expected = -0.25
		if e.Reason != "The active pull request exceeded the configured inactivity threshold." {
			return false
		}
	case Superseded, Corrected, Reverted:
		expected = -1
		if e.Reason != "An explicit provenance link marked the attributed attempt as "+e.Outcome+"." {
			return false
		}
	default:
		return false
	}
	return e.Score != nil && *e.Score == expected
}

func containsAll(list, required []string) bool {
	for _, id := range required {
		if !slices.Contains(list, id) {
			return false
		}
	}
	return true
}
