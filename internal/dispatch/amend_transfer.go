package dispatch

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

var routeRPC func(context.Context, Host, Request) (Response, error)

func init() { routeRPC = rpcTransport }

func migrateAmend(q Request, old, next Meta, live map[string]bool) (Response, error) {
	var r Response
	dir := filepath.Join(Root(), q.Name)
	cfg, err := Load(old.ConfigPath)
	if err != nil {
		return r, err
	}
	target, ok := cfg.Hosts[next.Host]
	if !ok {
		return r, fmt.Errorf("owner has no route to amended host %q", next.Host)
	}
	next.ConfigPath = target.ConfigPath
	next.Namespace = target.Namespace
	next.HarnessWrapper = nil
	next.LaunchResolved = false
	next.LaunchEnv = nil
	next.Imported = true
	next.Forward = nil
	if old.Every == "" {
		newAttempt(&next)
	}
	request := Request{Action: "amend-check", Name: q.Name, Order: q.Order, Meta: next, Cap: target.Cap}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	checked, err := routeRPC(ctx, target, request)
	if err != nil {
		return r, err
	}
	if checked.AmendToken == "" {
		return r, fmt.Errorf("target does not support amendment admission reservations")
	}
	// Admission refusal above has no effect on the source record or worker.
	if live[q.Name] || processRecordLive(filepath.Join(dir, "harness.json")) {
		if err = amendStop(dir, q.Name); err != nil {
			return r, err
		}
	}
	h := history(dir, old)
	rev := Revision{Number: len(h.Revisions) + 1, Time: time.Now().UTC(), Order: q.Order, Diff: orderDiff(h.Revisions[len(h.Revisions)-1].Order, q.Order)}
	h.Revisions = append(h.Revisions, rev)
	h.Attempts = append(h.Attempts, Attempt{Number: len(h.Attempts) + 1, Revision: rev.Number, Meta: next})
	transfer, err := packAttempt(dir, old.Branch, h)
	if err != nil {
		return r, err
	}
	request.Action = "amend-import"
	request.AmendToken = checked.AmendToken
	request.Transfer = transfer
	// Persist routing before a potentially uncertain launch response. The source
	// retains its full record and worktree for recovery; it never launches twice.
	old.Forward = &target
	if err = WriteJSON(filepath.Join(dir, "meta.json"), old); err != nil {
		return r, err
	}
	r, err = routeRPC(ctx, target, request)
	if err != nil {
		if r.ImportRefused {
			old.Forward = nil
			if restoreErr := WriteJSON(filepath.Join(dir, "meta.json"), old); restoreErr != nil {
				return r, fmt.Errorf("import refused; restoring source routing failed: %w", restoreErr)
			}
			return r, fmt.Errorf("amend import refused; source remains stopped with original record, ready for rework or amend: %w", err)
		}
		return r, fmt.Errorf("amend transfer outcome requires inspection on target: %w", err)
	}
	return r, nil
}

func packAttempt(dir, branch string, h History) ([]byte, error) {
	temp, err := os.MkdirTemp("", "dispatch-bundle-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	bundle := filepath.Join(temp, "branch.bundle")
	if _, err = command(filepath.Join(dir, "wt"), "git", "bundle", "create", bundle, branch); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "wt/.git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "meta.json" || rel == "history.json" || rel == "order.md" || rel == "stop" || rel == "admission.json" || rel == "supervisor.json" || rel == "harness.json" || rel == supervisorLaunchFile || rel == supervisorExitFile {
			return nil
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err = tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, f)
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(bundle)
	if err != nil {
		return nil, err
	}
	if err = tw.WriteHeader(&tar.Header{Name: "branch.bundle", Mode: 0600, Size: int64(len(b))}); err != nil {
		return nil, err
	}
	if _, err = tw.Write(b); err != nil {
		return nil, err
	}
	historyBytes, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	if err = tw.WriteHeader(&tar.Header{Name: "history.json", Mode: 0600, Size: int64(len(historyBytes))}); err != nil {
		return nil, err
	}
	if _, err = tw.Write(historyBytes); err != nil {
		return nil, err
	}
	if err = tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const amendReservationTTL = 5 * time.Minute

type amendReservation struct {
	Token   string
	Digest  [32]byte
	Expires time.Time
}

// The receipt binds the external decision to the exact order, attempt and
// resolved destination. Retain only a digest, never the order or launch data.
func amendDigest(q Request, repo string) ([32]byte, error) {
	raw, err := json.Marshal(struct {
		Order string
		Meta  Meta
		Cap   int
		Repo  string
	}{q.Order, q.Meta, q.Cap, repo})
	return sha256.Sum256(raw), err
}

func pruneAmendReservations(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	active := 0
	for _, entry := range entries {
		var receipt amendReservation
		path := filepath.Join(dir, entry.Name())
		if readJSON(path, &receipt) == nil && now.Before(receipt.Expires) {
			active++
		} else if err := os.Remove(path); err != nil {
			return err
		}
	}
	if active >= 256 {
		return fmt.Errorf("too many pending amendment reservations")
	}
	return nil
}

func receiveAmend(q Request) (r Response, retErr error) {
	r.ImportRefused = true
	o, err := Parse(q.Order)
	if err != nil {
		return r, err
	}
	if !ValidName(q.Name) || o.Name != q.Name || q.Meta.Name != q.Name || !reflect.DeepEqual(q.Meta.Order, o) || q.Meta.Branch != "d/"+q.Name {
		return r, fmt.Errorf("amend order mismatch")
	}
	unlock, err := lockRoot()
	if err != nil {
		return r, err
	}
	var denial error
	defer func() {
		if unlock != nil {
			unlock()
		}
		if denial != nil {
			admissionDenied(q.Meta, "amend", denial)
		}
	}()
	live, err := admissionLive()
	if err != nil {
		return r, err
	}
	dir := filepath.Join(Root(), q.Name)
	if live[q.Name] || exists(queuePath(q.Name)) || exists(dir) {
		return r, fmt.Errorf("target order name already exists")
	}
	cfg, err := checkAmendAdmission(q, q.Meta, live)
	if err != nil {
		return r, err
	}
	repo := resolveRepo(o.Repo, cfg.Workspace)
	digest, err := amendDigest(q, repo)
	if err != nil {
		return r, err
	}
	reservations := filepath.Join(Root(), ".amend-reservations")
	receiptPath := filepath.Join(reservations, q.Name+".json")
	var receipt amendReservation
	if err = readJSON(receiptPath, &receipt); err != nil && !errors.Is(err, os.ErrNotExist) {
		return r, err
	}
	now := time.Now()
	valid := receipt.Token != "" && now.Before(receipt.Expires)
	if q.Action == "amend-import" {
		if !valid || receipt.Digest != digest || receipt.Token != q.AmendToken {
			return r, fmt.Errorf("missing, expired or mismatched amendment admission reservation")
		}
		if err = os.Remove(receiptPath); err != nil {
			return r, err
		}
	} else if valid && receipt.Digest == digest {
		unlock()
		unlock = nil
		if err = admitTargetAllowlist(repo, q.Meta, "amend"); err != nil {
			return r, err
		}
		r.Message, r.AmendToken = "amend admitted", receipt.Token
		return r, nil
	}
	checkBranch := func() error {
		if _, err := command(repo, "git", "show-ref", "--verify", "refs/heads/"+q.Meta.Branch); err == nil {
			return fmt.Errorf("target branch already exists")
		}
		return nil
	}
	if err = checkBranch(); err != nil {
		return r, err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return r, fmt.Errorf("reserve target order: %w", err)
	}
	keepDir := false
	defer func() {
		if !keepDir {
			if err := os.RemoveAll(dir); err != nil {
				r.ImportRefused = false
				if retErr == nil {
					retErr = fmt.Errorf("release target order reservation: %w", err)
				}
			}
		}
	}()
	if err = reserve(dir); err != nil {
		return r, err
	}
	defer os.Remove(filepath.Join(dir, "admission.json"))
	if q.Action == "amend-check" {
		if err = admitWithRootUnlocked(&unlock, repo, q.Meta, "amend"); err != nil {
			return r, err
		}
		live, err = admissionLive()
		if err != nil {
			return r, err
		}
		cfg, err = checkAmendAdmission(q, q.Meta, live)
		if err != nil {
			return r, err
		}
		if resolveRepo(o.Repo, cfg.Workspace) != repo {
			return r, fmt.Errorf("target repository changed during admission; retry")
		}
		if err = checkBranch(); err != nil {
			return r, err
		}
		if err = os.MkdirAll(reservations, 0700); err != nil {
			return r, err
		}
		now = time.Now()
		if err = pruneAmendReservations(reservations, now); err != nil {
			return r, err
		}
		token, err := classToken()
		if err != nil {
			return r, err
		}
		if err = WriteJSON(receiptPath, amendReservation{Token: token, Digest: digest, Expires: now.Add(amendReservationTTL)}); err != nil {
			return r, err
		}
		r.Message, r.AmendToken = "amend admitted", token
		return r, nil
	}
	policy, err := targetPolicy()
	if err != nil {
		return r, err
	}
	if err = admitRepo(policy.Admission, repo, q.Meta.Repo, q.Meta.Host); err != nil {
		// Emit after cleanup and unlock, so observers can inspect dispatch.
		denial = err
		return r, err
	}
	stage, err := os.MkdirTemp(Root(), ".amend-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(stage)
	if err = unpackAttempt(stage, q.Transfer); err != nil {
		return r, err
	}
	branchCreated := false
	defer func() {
		if retErr == nil || keepDir || !branchCreated {
			return
		}
		if _, e := command(repo, "git", "worktree", "remove", "--force", filepath.Join(dir, "wt")); e != nil {
			// No worktree may have been registered yet.
			_ = os.RemoveAll(dir)
			_, _ = command(repo, "git", "worktree", "prune")
		}
		if _, e := command(repo, "git", "branch", "-D", q.Meta.Branch); e != nil {
			r.ImportRefused = false
		}
	}()
	bundle := filepath.Join(stage, "branch.bundle")
	if _, err = command(repo, "git", "fetch", bundle, q.Meta.Branch+":refs/heads/"+q.Meta.Branch); err != nil {
		return r, err
	}
	branchCreated = true
	next := q.Meta
	next.ResolvedRepo = repo
	next.Compacted, next.HeadSHA, next.WorkspaceBase = false, "", ""
	next.WorkspaceCap, err = dispatchWorkspaceCap()
	if err != nil {
		return r, err
	}
	next.WorktreeCreated = false
	next.BranchCreated = true
	resolveLaunch(&next, cfg)
	if err = WriteJSON(filepath.Join(dir, "meta.json"), next); err != nil {
		return r, err
	}
	if _, err = command(repo, "git", "worktree", "add", filepath.Join(dir, "wt"), next.Branch); err != nil {
		return r, err
	}
	next.WorktreeCreated = true
	entries, err := os.ReadDir(filepath.Join(dir, "wt"))
	if err != nil {
		return r, err
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			if err = os.RemoveAll(filepath.Join(dir, "wt", entry.Name())); err != nil {
				return r, err
			}
		}
	}
	entries, err = os.ReadDir(stage)
	if err != nil {
		return r, err
	}
	for _, entry := range entries {
		if entry.Name() == "branch.bundle" {
			continue
		}
		if entry.Name() == "wt" {
			files, err := os.ReadDir(filepath.Join(stage, "wt"))
			if err != nil {
				return r, err
			}
			for _, f := range files {
				if err = os.Rename(filepath.Join(stage, "wt", f.Name()), filepath.Join(dir, "wt", f.Name())); err != nil {
					return r, err
				}
			}
		} else if err = os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(dir, entry.Name())); err != nil {
			return r, err
		}
	}
	if err = archiveAttempt(dir); err != nil {
		return r, err
	}
	if err = atomicOrder(dir, q.Order); err != nil {
		return r, err
	}
	// A recipe that cannot be built here refuses the import before launch.
	// A recipe carried in the order is pinned again from it on this host.
	unpinRecipe(dir, &next)
	if err = prepareContext(dir, &next, q.Order); err != nil {
		return r, err
	}
	if err = WriteJSON(filepath.Join(dir, "meta.json"), next); err != nil {
		return r, err
	}
	if err = resetValidation(dir); err != nil {
		return r, err
	}
	keepDir = true
	r.ImportRefused = false
	if err = amendLaunch(dir, q.Name); err != nil {
		recordUnconfirmed(dir, next, err)
		return r, err
	}
	r.Message = "amended " + q.Name + " on " + o.Host
	return r, nil
}

// Extract regular files before symlinks so archive entries cannot write through
// a link, including links present in the repository's working tree.
func unpackAttempt(dir string, data []byte) error {
	tr := tar.NewReader(bytes.NewReader(data))
	links := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(hdr.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.Contains(name, ":") || strings.Contains(hdr.Name, "\\") {
			return fmt.Errorf("invalid transfer path")
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("invalid transfer path")
		}
		for _, part := range strings.Split(clean, "/") {
			if strings.EqualFold(part, ".git") {
				return fmt.Errorf("git metadata in transfer")
			}
		}
		if clean == "." {
			return fmt.Errorf("invalid transfer path")
		}
		path := filepath.Join(dir, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(hdr.Mode)&0777)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			links[path] = hdr.Linkname
		default:
			return fmt.Errorf("unsupported transfer entry")
		}
	}
	for path, target := range links {
		if err := os.Symlink(target, path); err != nil {
			return err
		}
	}
	return nil
}
