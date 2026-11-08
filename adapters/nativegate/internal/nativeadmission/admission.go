// Package nativeadmission implements an opt-in final-spawn authority facade.
// It exposes neutral DTOs so native bridges need not import admission internals.
// It neither schedules work nor waits for native process completion.
package nativeadmission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hev/kit/adapters/nativegate/internal/businessflow"
	"github.com/hev/kit/adapters/nativegate/internal/chef"
	"github.com/hev/kit/adapters/nativegate/internal/maintenance"
)

const Version = "native-start/v1"

var ErrUnsupportedPendingHandoff = errors.New("UnsupportedPendingHandoff: original native chef fence is expired or transferred; inspect original pending identity")
var ErrHeld = errors.New("native admission held; inspect original reservation and native attempt")
var ErrBinding = errors.New("native admission immutable binding conflict")

type Lease struct {
	Owner   string
	Fence   uint64
	Expires time.Time
}
type Workspace struct{ UID, Incarnation, Reference string }
type Parent struct{ UID, Incarnation, WorkspaceRef string }

// Identity mirrors immutable native identity. The adapter must compute and
// verify the native RequestDigest before passing Check; do not rewrite fields.
type Identity struct {
	OrderUID, ScopeDigest, PlanDigest, AuthorityDigest, ExecutionEpoch                      string
	WorkflowUID, NodeID, StepID, AttemptID, CandidateDigest, InputDigest, EnvironmentDigest string
	Operation, HostID, ExecutorID, ChefID                                                   string
	Fence                                                                                   uint64
}
type StartAuthority struct {
	Version                         string
	NodeUID, Incarnation            string
	AdmissionGeneration             uint64
	ReservationID, AttemptAuthority string
}
type Check struct {
	Version, JobID, IntentKey, PayloadDigest string
	Identity                                 Identity
	Authority                                StartAuthority
	Workspace                                Workspace
	Parent                                   Parent
	// NativeFenceExpires is the original receiver fence expiry, supplied by its
	// authenticated fence owner. Zero is Unknown and cannot authorize spawn.
	NativeFenceExpires time.Time
}

// Receipt maps exactly to the accepted native StartReceipt, not a bearer permit.
type Receipt struct {
	Version, JobID, PayloadDigest string
	Authority                     StartAuthority
	DecisionID, AuthorityRevision string
}

// Started is acknowledged only after the synchronous native callback durably
// committed actual spawn+Running. Pending/submitted/queued is never sufficient.
type Started struct{ State, JobID, PayloadDigest, DecisionID, AuthorityRevision, Evidence string }
type Assessment struct{ Evidence, Reason, Action string }
type SpawnIntent struct {
	AttentionID, Revision string
	Lease                 Lease
}
type OrderState struct {
	OrderUID, Revision, Evidence string
	Open                         bool
}

// Authority is an explicitly trusted canonical source. It must verify complete
// immutable native identity, reviewed attempt authority, enrollment, workspace
// access and canonical candidate/scope/plan/input/environment bindings. Methods
// must honor the bounded context, never wait for process completion, and must
// not reenter ANY chef/businessflow/maintenance mutations or advance DAGs.
type Authority interface {
	Current(context.Context, string) (OrderState, error)
	Authorize(context.Context, Check) error
}
type Config struct {
	ChefDir, BusinessDir, MaintenanceDir string
	MaintenanceOwner                     Lease
	Authority                            Authority
}
type Gate struct{ config Config }

func New(c Config) (*Gate, error) {
	for _, p := range []string{c.ChefDir, c.BusinessDir, c.MaintenanceDir} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return nil, errors.New("explicit canonical absolute authority directories required")
		}
	}
	if c.Authority == nil {
		return nil, errors.New("canonical native authority required")
	}
	return &Gate{config: c}, nil
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func valid(c Check) error {
	i := c.Identity
	a := c.Authority
	strings := []string{c.JobID, c.IntentKey, c.PayloadDigest, i.OrderUID, i.ScopeDigest, i.PlanDigest, i.AuthorityDigest, i.ExecutionEpoch, i.WorkflowUID, i.NodeID, i.StepID, i.AttemptID, i.CandidateDigest, i.InputDigest, i.EnvironmentDigest, i.HostID, i.ExecutorID, i.ChefID, a.NodeUID, a.Incarnation, a.ReservationID, a.AttemptAuthority, c.Workspace.UID, c.Workspace.Incarnation, c.Workspace.Reference}
	for _, v := range strings {
		if v == "" {
			return ErrBinding
		}
	}
	if c.Version != Version || a.Version != Version || i.Operation != "exec" || i.Fence == 0 || a.AdmissionGeneration == 0 || c.NativeFenceExpires.IsZero() {
		return ErrBinding
	}
	if c.Parent != (Parent{}) && (c.Parent.UID == "" || c.Parent.Incarnation == "" || c.Parent.WorkspaceRef != c.Workspace.Reference) {
		return ErrBinding
	}
	b, e := json.Marshal(c)
	if e != nil || len(b) > 6000 {
		return ErrBinding
	}
	return nil
}
func (g *Gate) currentLease(ctx context.Context, l Lease, c Check) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if c.Identity.ChefID != l.Owner || c.Identity.Fence != l.Fence || !time.Now().Before(c.NativeFenceExpires) {
		return ErrUnsupportedPendingHandoff
	}
	s, e := (chef.Store{Dir: g.config.ChefDir}).ReadStatus()
	if e != nil {
		return e
	}
	if s.Lease.Owner != l.Owner || s.Lease.Fence != l.Fence || !time.Now().Before(s.Lease.Expires) {
		return ErrUnsupportedPendingHandoff
	}
	return nil
}

type canonical struct {
	ctx       context.Context
	authority Authority
	check     Check
}

func (c canonical) Current(order string) (chef.Observation, error) {
	o, e := c.authority.Current(c.ctx, order)
	return chef.Observation{Order: o.OrderUID, Revision: o.Revision, Open: o.Open, Unfinished: true, Idle: true, Evidence: o.Evidence}, e
}
func (c canonical) Authorize(order, revision string, d chef.Decision) error {
	var check Check
	if json.Unmarshal(d.Payload, &check) != nil || hash(check) != hash(c.check) || order != check.Identity.OrderUID {
		return ErrBinding
	}
	if d.Action != "admit-workflow" && d.Action != "request-validation" {
		return ErrBinding
	}
	return c.authority.Authorize(c.ctx, check)
}

// DecideSpawn persists the reviewed, separate actual-spawn action. It does not
// submit native work, consume a reservation or report a pending job as started.
func (g *Gate) DecideSpawn(ctx context.Context, l Lease, c Check, a Assessment) (SpawnIntent, error) {
	if e := valid(c); e != nil {
		return SpawnIntent{}, e
	}
	if e := g.currentLease(ctx, l, c); e != nil {
		return SpawnIntent{}, e
	}
	if a.Evidence == "" || a.Reason == "" || (a.Action != "admit-workflow" && a.Action != "request-validation") {
		return SpawnIntent{}, ErrBinding
	}
	ca := canonical{ctx, g.config.Authority, c}
	o, e := ca.Current(c.Identity.OrderUID)
	if e != nil {
		return SpawnIntent{}, e
	}
	if o.Order != c.Identity.OrderUID || !o.Open {
		return SpawnIntent{}, ErrBinding
	}
	if e = g.config.Authority.Authorize(ctx, c); e != nil {
		return SpawnIntent{}, e
	}
	if e = g.currentLease(ctx, l, c); e != nil {
		return SpawnIntent{}, e
	}
	o.Release = "actual-spawn/" + hash(c)
	s := chef.Store{Dir: g.config.ChefDir}
	id, e := s.Observe(o)
	if e != nil {
		return SpawnIntent{}, e
	}
	payload, _ := json.Marshal(c)
	e = s.Decide(chef.Lease{Owner: l.Owner, Fence: l.Fence, Expires: l.Expires}, id, chef.Decision{Assessment: a.Evidence, Reason: a.Reason, Action: a.Action, Payload: payload}, ca)
	if e != nil {
		return SpawnIntent{}, e
	}
	return SpawnIntent{AttentionID: id, Revision: o.Revision, Lease: l}, nil
}

type effectFunc func(chef.Intent, chef.Guard) (string, error)

func (f effectFunc) Execute(i chef.Intent, g chef.Guard) (string, error) { return f(i, g) }

// WithStart belongs INSIDE a native FinalStartGate. The native launch callback
// must synchronously commit its receipt, spawn, and Running before returning a
// bound Started ACK. Never wrap native Execute/Wait with this method.
func (g *Gate) WithStart(ctx context.Context, spawn SpawnIntent, c Check, launch func(Receipt) (Started, error)) (Started, error) {
	var ack Started
	if e := valid(c); e != nil {
		return ack, e
	}
	if spawn.AttentionID == "" || spawn.Revision == "" || launch == nil {
		return ack, ErrBinding
	}
	if e := g.currentLease(ctx, spawn.Lease, c); e != nil {
		return ack, e
	}
	ca := canonical{ctx, g.config.Authority, c}
	s := chef.Store{Dir: g.config.ChefDir}
	called := false
	e := s.Apply(chef.Lease{Owner: spawn.Lease.Owner, Fence: spawn.Lease.Fence, Expires: spawn.Lease.Expires}, spawn.AttentionID, ca, effectFunc(func(i chef.Intent, guard chef.Guard) (string, error) {
		return guard.Do(func() (string, error) {
			if i.Revision != spawn.Revision || i.ID != spawn.AttentionID {
				return "", ErrBinding
			}
			if e := g.currentLease(ctx, spawn.Lease, c); e != nil {
				return "", e
			}
			var err error
			ack, err = g.withAdmission(ctx, spawn.AttentionID, i.Decision.Action, "", c, func(ctx context.Context) error {
				return g.currentLease(ctx, spawn.Lease, c)
			}, launch)
			if err != nil {
				return "", err
			}
			called = true
			return ack.Evidence, nil
		})
	}))
	if e != nil {
		return Started{}, e
	}
	if !called {
		return Started{}, ErrHeld
	}
	return ack, nil
}

// withAdmission preserves the business and maintenance mutation guards independently
// of the chef persistence mechanism. Its caller holds the chef authority through
// this synchronous callback and supplies a final authority/expiry recheck.
func (g *Gate) withAdmission(ctx context.Context, decisionID, action, revisionPrefix string, c Check, recheck func(context.Context) error, launch func(Receipt) (Started, error)) (Started, error) {
	var ack Started
	bstore := businessflow.Store{Dir: g.config.BusinessDir}
	b, e := bstore.Snapshot(ctx)
	if e != nil {
		return Started{}, e
	}
	r, ok := b.Reservations[c.Authority.ReservationID]
	entry, exists := b.Entries[c.Identity.OrderUID]
	w := businessflow.WorkspaceIdentity{UID: c.Workspace.UID, Incarnation: c.Workspace.Incarnation}
	if !ok || !exists || r.Order != c.Identity.OrderUID || r.Intent != c.IntentKey || r.Attempt != c.Identity.AttemptID || r.Candidate != c.Identity.CandidateDigest || r.Executor != c.Authority.NodeUID || entry.Scope != c.Identity.ScopeDigest || r.Workspace != w || entry.Workspace != w {
		return Started{}, ErrBinding
	}
	if (action == "request-validation" && r.Kind != "validation") || (action == "admit-workflow" && r.Kind != "execution") {
		return Started{}, ErrBinding
	}
	want := businessflow.ReservationCheck{Workspace: w, Order: r.Order, Obligation: r.Obligation, Candidate: r.Candidate, Scope: entry.Scope, Attempt: r.Attempt, Intent: r.Intent, Executor: r.Executor, PolicyRevision: r.PolicyRevision}
	e = bstore.WithReservationGuard(ctx, r.ID, want, func(ctx context.Context) error {
		mstore := maintenance.Store{Dir: g.config.MaintenanceDir}
		m, e := mstore.ReadSnapshot()
		if e != nil {
			return e
		}
		n, ok := m.Nodes[c.Authority.NodeUID]
		if !ok || n.Incarnation != c.Authority.Incarnation || n.AdmissionGeneration != c.Authority.AdmissionGeneration {
			return ErrBinding
		}
		if n.Parent == nil {
			if c.Parent != (Parent{}) {
				return ErrBinding
			}
		} else if c.Parent != (Parent{UID: n.Parent.UID, Incarnation: n.Parent.Incarnation, WorkspaceRef: n.Parent.WorkspaceRef}) || n.Parent.WorkspaceRef != c.Workspace.Reference {
			return ErrBinding
		}
		owner := maintenance.Owner{ID: g.config.MaintenanceOwner.Owner, Fence: g.config.MaintenanceOwner.Fence, ExpiresAt: g.config.MaintenanceOwner.Expires}
		request := func(kind string) maintenance.Request {
			return maintenance.Request{Key: "actual-spawn/" + decisionID + "/" + kind, ExpectedRevision: m.Revision, Owner: owner, NodeUID: n.UID, Incarnation: n.Incarnation, Reason: "actual native spawn " + c.JobID}
		}
		mr := maintenance.Reservation{ID: r.ID, NodeUID: n.UID, Incarnation: n.Incarnation, OrderUID: r.Order, ObligationID: r.Obligation, AttemptID: r.Attempt, Candidate: r.Candidate, Scope: entry.Scope, AdmissionGeneration: c.Authority.AdmissionGeneration}
		if prior, ok := m.Reservations[r.ID]; ok {
			if prior.State != "Reserved" || prior.NodeUID != mr.NodeUID || prior.Incarnation != mr.Incarnation || prior.OrderUID != mr.OrderUID || prior.ObligationID != mr.ObligationID || prior.AttemptID != mr.AttemptID || prior.Candidate != mr.Candidate || prior.Scope != mr.Scope || prior.AdmissionGeneration != mr.AdmissionGeneration {
				return ErrHeld
			}
		} else {
			m, e = mstore.Reserve(ctx, maintenance.ReservationRequest{Request: request("reserve"), Reservation: mr})
			if e != nil {
				return e
			}
		}
		_, e = mstore.WithStartGuard(ctx, maintenance.StartRequest{Request: request("start"), ReservationID: r.ID}, func(ctx context.Context) error {
			// Both writer guards are held. Read their published Starting
			// images without reentering mutation locks, and bind those
			// exact revisions into the durable native receipt.
			currentM, err := mstore.ReadSnapshot()
			if err != nil {
				return err
			}
			currentB, err := bstore.Snapshot(ctx)
			if err != nil {
				return err
			}
			if currentM.Reservations[r.ID].State != "Starting" || currentB.Reservations[r.ID].Status != "Starting" {
				return ErrHeld
			}
			receipt := Receipt{Version: Version, JobID: c.JobID, PayloadDigest: c.PayloadDigest, Authority: c.Authority, DecisionID: decisionID, AuthorityRevision: revisionPrefix + fmt.Sprintf("%s@%d;%s@%d;business-policy@%d", currentM.Version, currentM.Revision, currentB.Version, currentB.ResourceVersion, r.PolicyRevision)}
			if e := g.config.Authority.Authorize(ctx, c); e != nil {
				return e
			}
			if e := recheck(ctx); e != nil {
				return e
			}
			ack, err = launch(receipt)
			if err != nil {
				return err
			}
			if ack.State != "NativeSupervisorStarted" || ack.JobID != receipt.JobID || ack.PayloadDigest != receipt.PayloadDigest || ack.DecisionID != receipt.DecisionID || ack.AuthorityRevision != receipt.AuthorityRevision || ack.Evidence == "" || len(ack.Evidence) > 2048 {
				return ErrHeld
			}
			if e := recheck(ctx); e != nil {
				return e
			}
			return nil
		})
		return e
	})
	if e != nil {
		return Started{}, e
	}
	return ack, nil
}
