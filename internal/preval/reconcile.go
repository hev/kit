package preval

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Documents binds one workspace's existing source writers.
type Documents interface {
	ReadOrder(context.Context, Provenance) (*Evaluation, error)
	WriteOrder(context.Context, Evaluation) error
	ClaimLearning(context.Context, Evaluation) (bool, error)
	WriteTrace(context.Context, string, Evaluation) error
	WriteEvidence(context.Context, string, Evaluation) error
}

type Registration struct {
	Attribution
	ProviderIndex int `json:"provider_index"`
}

// Reconciler serializes hook and timer observations for its workspace. The
// registration is intentionally bounded and sourced from operator policy.
type Reconciler struct {
	providers  []Provider
	links      []Registration
	documents  Documents
	staleAfter time.Duration
	slot       chan struct{}
	learner    Learner
}

func NewReconciler(providers []Provider, links []Registration, docs Documents, staleAfter time.Duration) (*Reconciler, error) {
	if len(providers) == 0 || len(providers) > 32 || len(links) == 0 || len(links) > 128 || docs == nil || staleAfter < 0 {
		return nil, errInvalid
	}
	seen := make(map[Repository]bool)
	for _, provider := range providers {
		if !provider.Repository.valid() || seen[provider.Repository] || provider.Token == "" || provider.HookSecret == "" ||
			(provider.Repository.Provider == AzureDevOps && provider.HookUser == "") ||
			(provider.Repository.Provider == GitHub && !identifier.MatchString(provider.Name)) {
			return nil, errInvalid
		}
		seen[provider.Repository] = true
	}
	orders := make(map[string]bool)
	for _, link := range links {
		if link.ProviderIndex < 0 || link.ProviderIndex >= len(providers) || link.PullRequest.Repository != providers[link.ProviderIndex].Repository ||
			link.PullRequest.ID <= 0 || orders[link.Provenance.OrderID] || !identifier.MatchString(link.Provenance.OrderID) ||
			!identifier.MatchString(link.Provenance.AttemptID) || !validIDs(link.Provenance.TraceSessionIDs, 32) ||
			!validIDs(link.Provenance.EvidenceIDs, 32) {
			return nil, errInvalid
		}
		orders[link.Provenance.OrderID] = true
	}
	return &Reconciler{providers: providers, links: links, documents: docs, staleAfter: staleAfter, slot: make(chan struct{}, 1)}, nil
}

func (r *Reconciler) Handler() http.Handler {
	mux := http.NewServeMux()
	for i, provider := range r.providers {
		mux.Handle("/hooks/"+strconv.Itoa(i), provider.Hook(func(ctx context.Context, request PullRequest, eventAt time.Time) error {
			return r.reconcile(ctx, request, eventAt, false)
		}))
	}
	return mux
}

// Sweep bounds each pass and services every registered attribution across
// repeated passes even if a provider or a linked document is unavailable.
func (r *Reconciler) Sweep(ctx context.Context, cursor int, limit int) (int, error) {
	if cursor < 0 || cursor >= len(r.links) || limit <= 0 || limit > 16 {
		return 0, errInvalid
	}
	var err error
	for range min(limit, len(r.links)) {
		if ctx.Err() != nil {
			return cursor, ctx.Err()
		}
		pr := r.links[cursor].PullRequest
		cursor = (cursor + 1) % len(r.links)
		call, cancel := context.WithTimeout(ctx, 90*time.Second)
		err = errors.Join(err, r.reconcile(call, pr, time.Time{}, true))
		cancel()
	}
	return cursor, err
}

var errBusy = errors.New("pull request reconciliation already running")
var errPersistence = errors.New("could not reconcile a linked evaluation document")

func (r *Reconciler) reconcile(ctx context.Context, pr PullRequest, activity time.Time, learn bool) error {
	select {
	case r.slot <- struct{}{}:
		defer func() { <-r.slot }()
	default:
		return errBusy
	}
	var observation *Observation
	var failed bool
	for _, link := range r.links {
		if link.PullRequest != pr {
			continue
		}
		if observation == nil {
			snapshot, err := r.providers[link.ProviderIndex].Lookup(ctx, pr)
			if err != nil {
				return err
			}
			if snapshot.Outcome == Active && activity.After(snapshot.LastActivity) && !activity.After(snapshot.ObservedAt) {
				snapshot.LastActivity = activity
			}
			observation = &snapshot
		}
		if err := r.repair(ctx, link.Attribution, *observation, r.providers[link.ProviderIndex], learn); err != nil {
			failed = true
		}
	}
	if failed {
		return errPersistence
	}
	return nil
}

func (r *Reconciler) repair(ctx context.Context, link Attribution, observation Observation, provider Provider, learn bool) error {
	old, err := r.documents.ReadOrder(ctx, link.Provenance)
	if err != nil {
		return err
	}
	evaluation, err := Project(link, observation, old, r.staleAfter, r.learner != nil)
	if err != nil {
		return err
	}
	if err := r.documents.WriteOrder(ctx, evaluation); err != nil {
		return err
	}
	for _, id := range link.Provenance.TraceSessionIDs {
		err = errors.Join(err, r.documents.WriteTrace(ctx, id, evaluation))
	}
	for _, id := range link.Provenance.EvidenceIDs {
		err = errors.Join(err, r.documents.WriteEvidence(ctx, id, evaluation))
	}
	if err != nil || !learn || r.learner == nil {
		return err
	}
	if evaluation.Learning.Status == "failed" {
		if recovery, ok := r.learner.(learningRecovery); ok {
			learning, err := recovery.Recover(ctx, evaluation)
			if err != nil {
				return err
			}
			if learning.Status == "recorded" {
				evaluation.Learning = learning
				return r.documents.WriteOrder(ctx, evaluation)
			}
		}
		return nil
	}
	if evaluation.Learning.Status != "pending" {
		return nil
	}
	// Persist the attempt before invoking the harness. An interruption leaves
	// a failed marker and never repeatedly spends model time on this transition.
	evaluation.Learning = Learning{Status: "failed"}
	claimed, err := r.documents.ClaimLearning(ctx, evaluation)
	if err != nil || !claimed {
		return err
	}
	out, err := r.learner.Learn(ctx, provider, evaluation)
	if err != nil {
		return nil // periodic repair copies the durable failed state
	}
	evaluation.Learning = out
	return r.documents.WriteOrder(ctx, evaluation)
}

type learningRecovery interface {
	Recover(context.Context, Evaluation) (Learning, error)
}

type Learner interface {
	Learn(context.Context, Provider, Evaluation) (Learning, error)
}

// EnableLearning binds a workspace-authorized learner before serving requests.
func (r *Reconciler) EnableLearning(learner Learner) { r.learner = learner }
