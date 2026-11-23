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
	Revision string `json:"revision"`
	EventID  string `json:"event_id"`
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
	Source       Source
	ObservedAt   time.Time
	LastActivity time.Time
	Outcome      string
}

// Attribution is a workspace-owned registration. Corrections must come from
// the registration, independently of any text in the PR or its reviews.
type Attribution struct {
	PullRequest PullRequest  `json:"pull_request"`
	Provenance  Provenance   `json:"provenance"`
	Correction  *Observation `json:"correction,omitempty"`
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
		if !slices.Contains([]string{Superseded, Corrected, Reverted}, link.Correction.Outcome) {
			return Evaluation{}, errInvalid
		}
		observation = *link.Correction
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
		if staleAfter > 0 && !observation.LastActivity.IsZero() && observation.ObservedAt.Sub(observation.LastActivity) >= staleAfter {
			result.Outcome, result.Score, result.Reason = Stale, score(-0.25), "The active pull request exceeded the configured inactivity threshold."
		}
	}
	if learningEnabled && result.Outcome != Active && result.Outcome != Stale {
		result.Learning.Status = "pending"
	}
	if previous == nil {
		return result, nil
	}
	if previous.ID != result.ID {
		return Evaluation{}, errInvalid
	}
	if !result.ObservedAt.After(previous.ObservedAt) ||
		previous.Outcome == Merged && (result.Outcome == Active || result.Outcome == Stale || result.Outcome == Closed) {
		return *previous, nil
	}
	if previous.Outcome == result.Outcome && previous.Source.Revision == result.Source.Revision &&
		previous.Source.PullRequest == result.Source.PullRequest {
		result.ObservedAt, result.Source.EventID, result.Learning = previous.ObservedAt, previous.Source.EventID, previous.Learning
	}
	return result, nil
}
