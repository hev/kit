package nativejob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func alive(b *Birth) bool {
	if b == nil {
		return false
	}
	got, err := processBirth(b.PID)
	return err == nil && got == *b
}

// Recover never replays an operation. It checks the supervisor's boot and birth
// and holds orphaned running jobs Uncertain, even if the child still exists.
func (s *Store) Recover(id string) (Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	j, err := load(tx, id)
	if err != nil {
		return Job{}, err
	}
	if j.State == "Running" && !alive(j.Owner) {
		j.State = "Uncertain"
		if err = save(tx, j); err != nil {
			return Job{}, err
		}
	}
	return j, tx.Commit()
}

// Execute claims a Pending intent exactly once. A FULL commit of Uncertain plus
// supervisor birth precedes spawn. Every interruption after that commit needs
// observation/reconciliation, even if no child was created. No retry is implied.
func (s *Store) Execute(id string) (Job, error) {
	return s.ExecuteWithGate(context.Background(), id, nil)
}

// ExecuteWithGate enforces bound final admission at the actual process launch.
// Legacy unguarded jobs retain Execute semantics. Guarded jobs cannot bypass
// their gate through Execute or the local wire execute action.
func (s *Store) ExecuteWithGate(ctx context.Context, id string, gate FinalStartGate) (Job, error) {
	return s.ExecuteWithGateObserved(ctx, id, gate, nil)
}

// ExecuteWithGateObserved preserves ExecuteWithGate's execution and completion
// ownership. onRunning observes successful final-gate return and durable Running
// before Wait, outside every launch guard. It runs once synchronously and must
// return promptly even if publishing readiness fails. It must not authorize work
// or reenter execution. A failed or uncertain gate never produces notification.
// Callback panic or supervisor death retains the original recovery semantics.
func (s *Store) ExecuteWithGateObserved(ctx context.Context, id string, gate FinalStartGate, onRunning func(StartedObservation)) (Job, error) {
	return s.executeWithGateObserved(ctx, id, gate, onRunning, 0)
}

func (s *Store) executeWithGateObserved(ctx context.Context, id string, gate FinalStartGate, onRunning func(StartedObservation), workloadTimeout time.Duration) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	owner, err := processBirth(os.Getpid())
	if err != nil {
		return Job{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	j, err := load(tx, id)
	if err != nil {
		return Job{}, err
	}
	if err = s.check(tx, j.Request.Identity); err != nil {
		return Job{}, err
	}
	if (j.Request.StartAuthority != nil && gate == nil) || (j.Request.StartAuthority == nil && gate != nil) {
		return Job{}, ErrStartGate
	}
	if j.State != "Pending" {
		return Job{}, fmt.Errorf("execution requires Pending, got %s; query or reconcile", j.State)
	}
	j.State = "Uncertain"
	j.Owner = &owner
	if err = save(tx, j); err != nil {
		return Job{}, err
	}
	if err = s.hit("ownership-staged"); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	if err = s.hit("ownership-committed"); err != nil {
		return Job{}, err
	}
	// Output files are exclusively created; no truncation of earlier evidence.
	output := filepath.Join(s.root, j.ID)
	if err = journalCreateOutputDir(output); err != nil {
		return Job{}, err
	}
	stdout, err := journalCreateOutputFile(filepath.Join(output, "stdout"))
	if err != nil {
		return Job{}, err
	}
	defer stdout.Close()
	stderr, err := journalCreateOutputFile(filepath.Join(output, "stderr"))
	if err != nil {
		return Job{}, err
	}
	defer stderr.Close()
	// Sync the output directory and its parent before any effect; otherwise a
	// power failure could retain a result but lose its artifact directory entry.
	for _, path := range []string{output, s.root} {
		if err = journalSyncDir(path); err != nil {
			return Job{}, err
		}
	}
	cmd := exec.Command(j.Request.Argv[0], j.Request.Argv[1:]...)
	var workload context.Context
	var deadlineKilled atomic.Bool
	if workloadTimeout > 0 {
		var cancel context.CancelFunc
		workload, cancel = context.WithTimeout(context.Background(), workloadTimeout)
		defer cancel()
		// This opt-in context controls only our own exec.Cmd root handle.
		// The existing synchronous API never inherits this termination policy.
		cmd = exec.CommandContext(workload, j.Request.Argv[0], j.Request.Argv[1:]...)
		cmd.Cancel = func() error {
			err := cmd.Process.Kill()
			if err == nil {
				deadlineKilled.Store(true)
			}
			return err
		}
	}
	cmd.Dir = j.Request.Directory
	cmd.Env = append([]string{}, j.Request.Environment...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err = s.hit("before-spawn"); err != nil {
		return Job{}, err
	}

	launch := func(receipt *StartReceipt, finalCheck func(context.Context) error) error {
		// Serialize the final launch decision with native fence/reconciliation.
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
			return err
		}
		current, err := load(tx, id)
		if err != nil {
			return err
		}
		if current.State != "Uncertain" || current.Owner == nil || *current.Owner != owner {
			return ErrConflict
		}
		if err = s.check(tx, current.Request.Identity); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if receipt != nil {
			if receipt.Version != StartVersion || receipt.JobID != current.ID || receipt.PayloadDigest != current.Request.PayloadDigest || current.Request.StartAuthority == nil || receipt.Authority != *current.Request.StartAuthority || receipt.DecisionID == "" || receipt.AuthorityRevision == "" {
				return ErrStartGate
			}
			current.StartReceipt = receipt
			if err = save(tx, current); err != nil {
				return err
			}
			if err = s.hit("start-receipt-staged"); err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			if err = s.hit("start-receipt-committed"); err != nil {
				return err
			}
			// The authoritative gate lock is still held across this durable gap.
			tx, err = s.db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
				return err
			}
			current, err = load(tx, id)
			if err != nil {
				return err
			}
			if current.State != "Uncertain" || current.Owner == nil || *current.Owner != owner || current.StartReceipt == nil || encode(current.StartReceipt) != encode(receipt) {
				return ErrConflict
			}
			if err = s.check(tx, current.Request.Identity); err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
		}
		if finalCheck != nil {
			if err = finalCheck(ctx); err != nil {
				return err
			}
			// A blocking remote check cannot extend the original local issuance.
			if err = s.check(tx, current.Request.Identity); err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
		}
		if err = cmd.Start(); err != nil {
			return err
		}
		if err = s.hit("spawned"); err != nil {
			return err
		}
		birth, birthErr := processBirth(cmd.Process.Pid)
		j = current
		if birthErr == nil {
			j.Birth = &birth
		}
		j.State = "Running"
		if err = save(tx, j); err != nil {
			return err
		}
		if err = s.hit("running-staged"); err != nil {
			return err
		}
		return tx.Commit()
	}
	if j.Request.StartAuthority == nil {
		err = launch(nil, nil)
	} else {
		// A gate is trusted authority code, but accidental swallowed errors or
		// repeated callbacks must never produce a second spawn or false success.
		var mu sync.Mutex
		calls := 0
		violated := false
		active := true
		var launchErr error
		check := StartCheck{Version: StartVersion, JobID: j.ID, Request: j.Request}
		requiresFinalCheck := false
		invoke := func(receipt StartReceipt, finalCheck func(context.Context) error) error {
			mu.Lock()
			defer mu.Unlock()
			if !active || calls != 0 {
				violated = true
				return ErrStartGate
			}
			calls++
			if requiresFinalCheck && finalCheck == nil {
				launchErr = ErrStartGate
				return launchErr
			}
			launchErr = launch(&receipt, finalCheck)
			return launchErr
		}
		var gateErr error
		if guarded, ok := gate.(FinalSpawnGate); ok {
			requiresFinalCheck = true
			gateErr = guarded.WithSpawn(ctx, check, invoke)
		} else {
			gateErr = gate.WithStart(ctx, check, func(receipt StartReceipt) error { return invoke(receipt, nil) })
		}
		mu.Lock()
		active = false
		if gateErr != nil {
			err = gateErr
		} else if calls != 1 || violated {
			err = ErrStartGate
		} else {
			err = launchErr
		}
		mu.Unlock()
	}
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Wait()
		}
		return Job{}, err
	}
	if err = s.hit("running-committed"); err != nil {
		_ = cmd.Wait()
		return Job{}, err
	}
	if onRunning != nil {
		observation := StartedObservation{JobID: j.ID, PayloadDigest: j.Request.PayloadDigest, Owner: owner}
		if j.Birth != nil {
			child := *j.Birth
			observation.Child = &child
		}
		if j.StartReceipt != nil {
			receipt := *j.StartReceipt
			observation.StartReceipt = &receipt
		}
		onRunning(observation)
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) {
			return Job{}, waitErr
		}
	}
	if err = s.hit("process-exited"); err != nil {
		return Job{}, err
	}
	if err = stdout.Sync(); err != nil {
		return Job{}, err
	}
	if err = stderr.Sync(); err != nil {
		return Job{}, err
	}
	if err = journalSyncDir(output); err != nil {
		return Job{}, err
	}
	result := Result{ExitCode: cmd.ProcessState.ExitCode(), Artifacts: []Artifact{}}
	if deadlineKilled.Load() && !cmd.ProcessState.Success() {
		result.Evidence = WorkloadDeadlineEvidence
	}
	for _, name := range []string{"stdout", "stderr"} {
		a, err := fileArtifact(filepath.Join(output, name), name)
		if err != nil {
			return Job{}, err
		}
		result.Artifacts = append(result.Artifacts, a)
	}
	if err = s.hit("artifacts-synced"); err != nil {
		return Job{}, err
	}
	tx, err = s.db.Begin()
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE metadata SET value=value WHERE key='identity'`); err != nil {
		return Job{}, err
	}
	current, err := load(tx, id)
	if err != nil {
		return Job{}, err
	}
	if current.State != "Running" || current.Owner == nil || *current.Owner != owner {
		return Job{}, ErrConflict
	}
	j = current
	if err = finish(tx, &j, result); err != nil {
		return Job{}, err
	}
	if err = s.hit("result-staged"); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return j, s.hit("result-committed")
}
func fileArtifact(path, name string) (Artifact, error) {
	f, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	return Artifact{Name: name, Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Size: size}, err
}
