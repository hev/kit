//go:build linux || darwin

package nativegate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	a "github.com/hev/factory/pkg/nativeadmission"
	n "github.com/hev/kit/pkg/nativejob"
)

type fakeDBFacade func(context.Context, a.DBClaim, a.NativeBinding, a.Check, a.DBLaunch) (a.Started, error)

func (f fakeDBFacade) WithDBStart(ctx context.Context, c a.DBClaim, b a.NativeBinding, k a.Check, l a.DBLaunch) (a.Started, error) {
	return f(ctx, c, b, k, l)
}

func TestDBReceiverOriginalIssuanceAndLastChecker(t *testing.T) {
	for _, mode := range []string{"success", "db-lost", "nil-check", "wrong-receipt", "swallowed-callback", "wrong-ack"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			enrollment := n.Enrollment{HostID: "host", ExecutorID: "executor"}
			s, err := n.Open(journal, enrollment)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			c, p, _ := sample()
			r := c.Request
			r.Directory = root
			r.Argv = []string{"/bin/sh", "-c", "printf once > effects"}
			r.PayloadDigest = n.RequestDigest(r)
			if err = s.AdvanceFence(p.OriginalFence); err != nil {
				t.Fatal(err)
			}
			j, err := s.Submit(r)
			if err != nil {
				t.Fatal(err)
			}
			observer, err := n.Open(journal, enrollment)
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close()
			binding := a.NativeBinding{OrderUID: r.Identity.OrderUID, Generation: 3, ScopeDigest: r.Identity.ScopeDigest, JobID: j.ID, RequestDigest: r.PayloadDigest}
			checks := 0
			active := false
			facade := fakeDBFacade(func(ctx context.Context, claim a.DBClaim, b a.NativeBinding, k a.Check, launch a.DBLaunch) (a.Started, error) {
				active = true
				defer func() { active = false }()
				if claim.LeaseFence == int64(k.Identity.Fence) || claim.Holder == k.Identity.ChefID {
					t.Fatal("DB authority replaced native issuance")
				}
				rec := a.Receipt{Version: n.StartVersion, JobID: j.ID, PayloadDigest: r.PayloadDigest, Authority: k.Authority, DecisionID: claim.ActionID, AuthorityRevision: "db-claim;admission"}
				if mode == "wrong-receipt" {
					rec.DecisionID = "other-action"
				}
				checker := func() error {
					checks++
					if !active {
						t.Fatal("detached checker")
					}
					retained, err := observer.Query(j.ID)
					if err != nil || retained.StartReceipt == nil || retained.State != "Uncertain" {
						t.Fatal("no durable receipt before final DB check", retained, err)
					}
					if _, err = os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
						t.Fatal("check followed effect", err)
					}
					if mode == "db-lost" || mode == "swallowed-callback" {
						return errors.New("connection lost")
					}
					return nil
				}
				if mode == "nil-check" {
					checker = nil
				}
				ack, err := launch(rec, checker)
				if mode == "swallowed-callback" {
					return a.Started{}, nil
				}
				if mode == "wrong-ack" {
					ack.JobID = "other-job"
				}
				return ack, err
			})
			got, err := Start(context.Background(), s, facade, dbClaim(), binding, p)
			if mode == "success" {
				if err != nil || got.State != "Completed" || checks != 1 || active {
					t.Fatal(got, err, checks, active)
				}
			} else {
				if err == nil {
					t.Fatal("invalid facade accepted")
				}
				retained, qerr := s.Query(j.ID)
				if qerr != nil || retained.Request.PayloadDigest != r.PayloadDigest {
					t.Fatal(retained, qerr)
				}
				if _, err = Start(context.Background(), s, facade, dbClaim(), binding, p); err == nil {
					t.Fatal("uncertain start replayed")
				}
				// A wrong acknowledgement occurs after actual Start; it is not proof of no effect.
				if mode != "wrong-ack" {
					if _, err = os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
						t.Fatal("denied request spawned", err)
					}
				}
			}
		})
	}
}
