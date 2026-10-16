package nativegate

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

type fakeFacade func(context.Context, a.SpawnIntent, a.Check, func(a.Receipt) (a.Started, error)) (a.Started, error)

func (f fakeFacade) WithStart(c context.Context, s a.SpawnIntent, k a.Check, l func(a.Receipt) (a.Started, error)) (a.Started, error) {
	return f(c, s, k, l)
}
func sample() (n.StartCheck, Provenance, a.SpawnIntent) {
	expiry := time.Now().Add(time.Hour)
	i := n.Identity{OrderUID: "order", ScopeDigest: "scope", PlanDigest: "plan", AuthorityDigest: "authority", ExecutionEpoch: "epoch", WorkflowUID: "workflow", NodeID: "native-task", StepID: "build", AttemptID: "attempt", CandidateDigest: "candidate", InputDigest: "input", EnvironmentDigest: "environment", Operation: "exec", HostID: "host", ExecutorID: "executor", ChefID: "chef", Fence: 7}
	r := n.Request{Version: n.Version, IntentKey: "intent", Identity: i, Argv: []string{"/bin/sh", "-c", "printf '<one>'"}, Directory: "/tmp/owned", Environment: []string{}, StartAuthority: &n.StartAuthority{Version: n.StartVersion, NodeUID: "guest", Incarnation: "guest-boot", AdmissionGeneration: 3, ReservationID: "reservation", AttemptAuthority: "reviewed"}}
	r.PayloadDigest = n.RequestDigest(r)
	p := Provenance{OriginalFence: n.Fence{OrderUID: i.OrderUID, ChefID: i.ChefID, Epoch: i.Fence, Expires: expiry}, Workspace: a.Workspace{UID: "workspace", Incarnation: "ws-boot", Reference: "workspace/ref"}, Parent: a.Parent{UID: "parent", Incarnation: "parent-boot", WorkspaceRef: "workspace/ref"}, JournalReference: "native-journal://enrolled-host/original-store"}
	return n.StartCheck{Version: n.StartVersion, JobID: "job-opaque", Request: r}, p, a.SpawnIntent{AttentionID: "decision", Revision: "order-rev", Lease: a.Lease{Owner: i.ChefID, Fence: i.Fence, Expires: expiry}}
}
func receipt(s a.SpawnIntent, c a.Check) a.Receipt {
	return a.Receipt{Version: n.StartVersion, JobID: c.JobID, PayloadDigest: c.PayloadDigest, Authority: c.Authority, DecisionID: s.AttentionID, AuthorityRevision: "maintenance/v2@9;businessflow/v2@8;business-policy@2"}
}
func TestExplicitMappingAndNoRequestMutation(t *testing.T) {
	c, p, _ := sample()
	before := n.Digest(c)
	mapped, err := MapCheck(c, p)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Version != n.StartVersion || mapped.PayloadDigest != c.Request.PayloadDigest || mapped.IntentKey != c.Request.IntentKey || !mapped.NativeFenceExpires.Equal(p.OriginalFence.Expires) || mapped.Parent != p.Parent || mapped.Workspace != p.Workspace {
		t.Fatal(mapped)
	}
	original := reflect.ValueOf(c.Request.Identity)
	copy := reflect.ValueOf(mapped.Identity)
	for i := 0; i < original.NumField(); i++ {
		if !reflect.DeepEqual(original.Field(i).Interface(), copy.FieldByName(original.Type().Field(i).Name).Interface()) {
			t.Fatal("identity mapping", original.Type().Field(i).Name)
		}
	}
	authority := reflect.ValueOf(*c.Request.StartAuthority)
	target := reflect.ValueOf(mapped.Authority)
	for i := 0; i < authority.NumField(); i++ {
		if !reflect.DeepEqual(authority.Field(i).Interface(), target.FieldByName(authority.Type().Field(i).Name).Interface()) {
			t.Fatal("authority mapping")
		}
	}
	if n.Digest(c) != before {
		t.Fatal("original request mutated")
	}
	bad := c
	bad.Request.PayloadDigest = "different"
	if _, err = MapCheck(bad, p); err == nil {
		t.Fatal("bad digest")
	}
	bad = c
	bad.Request.StartAuthority = nil
	if _, err = MapCheck(bad, p); err == nil {
		t.Fatal("missing authority")
	}
	missing := p
	missing.OriginalFence.Expires = time.Time{}
	if _, err = MapCheck(c, missing); err == nil {
		t.Fatal("missing owner expiry inferred")
	}
}
func TestCallbackAndStartedAreExact(t *testing.T) {
	c, p, s := sample()
	calls := 0
	f := fakeFacade(func(ctx context.Context, spawn a.SpawnIntent, check a.Check, launch func(a.Receipt) (a.Started, error)) (a.Started, error) {
		return launch(receipt(spawn, check))
	})
	g, err := New(f, s, p)
	if err != nil {
		t.Fatal(err)
	}
	err = g.WithStart(context.Background(), c, func(r n.StartReceipt) error {
		calls++
		if r.Authority != *c.Request.StartAuthority || r.AuthorityRevision == "" || r.DecisionID != s.AttentionID {
			t.Fatal(r)
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatal(calls, err)
	}
}
func TestBrokenFacadeOrReceiptNeverFakesSuccess(t *testing.T) {
	for _, mode := range []string{"none", "wrong-receipt", "swallowed-native-error", "wrong-started", "duplicate", "late"} {
		t.Run(mode, func(t *testing.T) {
			c, p, s := sample()
			calls := 0
			var late func(a.Receipt) (a.Started, error)
			var saved a.Receipt
			f := fakeFacade(func(ctx context.Context, spawn a.SpawnIntent, check a.Check, launch func(a.Receipt) (a.Started, error)) (a.Started, error) {
				r := receipt(spawn, check)
				switch mode {
				case "none":
					return a.Started{}, nil
				case "late":
					late = launch
					saved = r
					return a.Started{}, nil
				case "wrong-receipt":
					r.Authority.Incarnation = "foreign"
				}
				ack, err := launch(r)
				if mode == "duplicate" {
					_, _ = launch(r)
				}
				if mode == "wrong-started" {
					ack.Evidence = "forged"
				}
				if mode == "swallowed-native-error" || mode == "wrong-receipt" {
					return ack, nil
				}
				return ack, err
			})
			g, err := New(f, s, p)
			if err != nil {
				t.Fatal(err)
			}
			err = g.WithStart(context.Background(), c, func(n.StartReceipt) error {
				calls++
				if mode == "swallowed-native-error" {
					return errors.New("native durable commit failed")
				}
				return nil
			})
			if err == nil {
				t.Fatal("false Started")
			}
			if late != nil {
				if _, err = late(saved); err == nil {
					t.Fatal("late callback accepted")
				}
			}
			if calls > 1 {
				t.Fatal("second native launch")
			}
			if (mode == "none" || mode == "wrong-receipt" || mode == "late") && calls != 0 {
				t.Fatal("unexpected native callback")
			}
		})
	}
}
func TestNoHandoffOrCredentialBearingReference(t *testing.T) {
	_, p, s := sample()
	f := fakeFacade(func(context.Context, a.SpawnIntent, a.Check, func(a.Receipt) (a.Started, error)) (a.Started, error) {
		t.Fatal("unexpected callback")
		return a.Started{}, nil
	})
	old := p
	old.OriginalFence.Epoch--
	if _, err := New(f, s, old); !errors.Is(err, a.ErrUnsupportedPendingHandoff) {
		t.Fatal(err)
	}
	for _, ref := range []string{"", "relative", "https://user:password@host/private", "https://host/path?token=secret", "native-journal://host/store#forged"} {
		bad := p
		bad.JournalReference = ref
		if _, err := New(f, s, bad); err == nil {
			t.Fatal("invalid journal reference", ref)
		}
	}
}
