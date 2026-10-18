//go:build linux || darwin

package nativejob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type spawnGateFunc func(context.Context, StartCheck, func(StartReceipt, func(context.Context) error) error) error

func (f spawnGateFunc) WithStart(context.Context, StartCheck, func(StartReceipt) error) error {
	return ErrStartGate
}
func (f spawnGateFunc) WithSpawn(ctx context.Context, c StartCheck, start func(StartReceipt, func(context.Context) error) error) error {
	return f(ctx, c, start)
}

func TestFinalSpawnRecheckAfterDurableReceipt(t *testing.T) {
	for _, mode := range []string{"success", "authority-lost", "connection-lost", "cancel", "nil-checker"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			s := testStore(t, filepath.Join(root, "journal"))
			testFence(t, s)
			r := guardedRequest(root)
			j := mustSubmit(t, s, r)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			receiptDurable := false
			checked := 0
			s.boundary = func(boundary string) error {
				if boundary == "start-receipt-committed" {
					receiptDurable = true
				}
				return nil
			}
			gate := spawnGateFunc(func(ctx context.Context, c StartCheck, start func(StartReceipt, func(context.Context) error) error) error {
				receipt := StartReceipt{Version: StartVersion, JobID: c.JobID, PayloadDigest: c.Request.PayloadDigest, Authority: *c.Request.StartAuthority, DecisionID: "db-action", AuthorityRevision: "db-claim-and-admission"}
				if mode == "nil-checker" {
					return start(receipt, nil)
				}
				return start(receipt, func(ctx context.Context) error {
					checked++
					if !receiptDurable {
						t.Fatal("checker preceded durable receipt")
					}
					if _, err := os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
						t.Fatal("checker ran after process effect", err)
					}
					switch mode {
					case "authority-lost":
						return ErrFence
					case "connection-lost":
						return errors.New("connection lost")
					case "cancel":
						cancel()
					}
					return nil
				})
			})
			got, err := s.ExecuteWithGate(ctx, j.ID, gate)
			if mode == "success" {
				if err != nil || got.State != "Completed" || checked != 1 {
					t.Fatal(got, err, checked)
				}
				return
			}
			if err == nil {
				t.Fatal("failed authority accepted")
			}
			retained, qerr := s.Query(j.ID)
			if qerr != nil || retained.State != "Uncertain" || retained.Request.PayloadDigest != r.PayloadDigest {
				t.Fatal(retained, qerr)
			}
			if mode != "nil-checker" && (!receiptDurable || checked != 1 || retained.StartReceipt == nil) {
				t.Fatal("lost receipt/check", retained, checked)
			}
			if _, err := os.Stat(filepath.Join(root, "effects")); !os.IsNotExist(err) {
				t.Fatal("rejected authority spawned", err)
			}
			if _, err = s.ExecuteWithGate(context.Background(), j.ID, gate); err == nil {
				t.Fatal("uncertain operation replayed")
			}
		})
	}
}
