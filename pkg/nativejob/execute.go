package nativejob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	if j.State != "Pending" {
		return Job{}, fmt.Errorf("execution requires Pending, got %s; query or reconcile", j.State)
	}
	j.State = "Uncertain"
	j.Owner = &owner
	if err = save(tx, j); err != nil {
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
	if err = os.Mkdir(output, 0700); err != nil {
		return Job{}, err
	}
	stdout, err := os.OpenFile(filepath.Join(output, "stdout"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Job{}, err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(output, "stderr"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Job{}, err
	}
	defer stderr.Close()
	cmd := exec.Command(j.Request.Argv[0], j.Request.Argv[1:]...)
	cmd.Dir = j.Request.Directory
	cmd.Env = append([]string{}, j.Request.Environment...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err = s.hit("before-spawn"); err != nil {
		return Job{}, err
	}
	if err = cmd.Start(); err != nil {
		return Job{}, err
	} // ambiguous windows always remain held
	if err = s.hit("spawned"); err != nil {
		_ = cmd.Wait()
		return Job{}, err
	}
	birth, birthErr := processBirth(cmd.Process.Pid)
	if birthErr == nil {
		j.Birth = &birth
	}
	tx, err = s.db.Begin()
	if err != nil {
		_ = cmd.Wait()
		return Job{}, err
	}
	j.State = "Running"
	err = save(tx, j)
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		_ = cmd.Wait()
		return Job{}, err
	}
	if err = s.hit("running-committed"); err != nil {
		_ = cmd.Wait()
		return Job{}, err
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
	dir, err := os.Open(output)
	if err != nil {
		return Job{}, err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return Job{}, err
	}
	result := Result{ExitCode: cmd.ProcessState.ExitCode(), Artifacts: []Artifact{}}
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
