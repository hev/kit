package dispatch

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var routeRPC func(context.Context, Host, Request) (Response, error)

func init() { routeRPC = RPC }

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
	request := Request{Action: "amend-check", Name: q.Name, Order: q.Order, Meta: next, Cap: target.Cap}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err = routeRPC(ctx, target, request); err != nil {
		return r, err
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
	if old.Every == "" {
		next.Start = time.Now().UTC()
	}
	h.Attempts = append(h.Attempts, Attempt{Number: len(h.Attempts) + 1, Revision: rev.Number, Meta: next})
	transfer, err := packAttempt(dir, old.Branch, h)
	if err != nil {
		return r, err
	}
	request.Action = "amend-import"
	request.Meta = next
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
		if rel == "meta.json" || rel == "history.json" || rel == "order.md" || rel == "stop" || rel == "admission.json" || rel == "supervisor.json" || rel == "harness.json" {
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

func receiveAmend(q Request, live map[string]bool) (r Response, retErr error) {
	r.ImportRefused = true
	o, err := Parse(q.Order)
	if err != nil {
		return r, err
	}
	if o.Name != q.Name || q.Meta.Name != q.Name || q.Meta.Order != o || q.Meta.Branch != "d/"+q.Name {
		return r, fmt.Errorf("amend order mismatch")
	}
	unlock, err := lockRoot()
	if err != nil {
		return r, err
	}
	defer unlock()
	live, err = liveNames()
	if err != nil {
		return r, err
	}
	if err = addReservations(live); err != nil {
		return r, err
	}
	cfg, err := checkAmendAdmission(q, q.Meta, live, false)
	if err != nil {
		return r, err
	}
	dir := filepath.Join(Root(), q.Name)
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		return r, fmt.Errorf("target order name already exists")
	}
	repo := resolveRepo(o.Repo, cfg.Workspace)
	if _, err = command(repo, "git", "show-ref", "--verify", "refs/heads/"+q.Meta.Branch); err == nil {
		return r, fmt.Errorf("target branch already exists")
	}
	if q.Action == "amend-check" {
		r.Message = "amend admitted"
		return r, nil
	}
	stage, err := os.MkdirTemp(Root(), ".amend-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(stage)
	if err = unpackAttempt(stage, q.Transfer); err != nil {
		return r, err
	}
	branchCreated, dirCreated, launched := false, false, false
	defer func() {
		if retErr == nil || launched {
			return
		}
		if dirCreated {
			if _, e := command(repo, "git", "worktree", "remove", "--force", filepath.Join(dir, "wt")); e != nil {
				// No worktree may have been registered yet.
				_ = os.RemoveAll(dir)
				_, _ = command(repo, "git", "worktree", "prune")
			}
			if e := os.RemoveAll(dir); e != nil {
				r.ImportRefused = false
			}
		}
		if branchCreated {
			if _, e := command(repo, "git", "branch", "-D", q.Meta.Branch); e != nil {
				r.ImportRefused = false
			}
		}
	}()
	bundle := filepath.Join(stage, "branch.bundle")
	if _, err = command(repo, "git", "fetch", bundle, q.Meta.Branch+":refs/heads/"+q.Meta.Branch); err != nil {
		return r, err
	}
	branchCreated = true
	if err = os.Mkdir(dir, 0700); err != nil {
		return r, err
	}
	dirCreated = true
	next := q.Meta
	next.ResolvedRepo = repo
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
	if err = WriteJSON(filepath.Join(dir, "meta.json"), next); err != nil {
		return r, err
	}
	if err = resetValidation(dir); err != nil {
		return r, err
	}
	launched = true
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
