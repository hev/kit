package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/hev/kit/internal/reviewnotes"
)

// ReviewFetch answers review-fetch: where a run's branch stands, and when
// asked, a git bundle of it, so `hev review` can fetch the branch from any
// host over the dispatch RPC path without the branch being pushed.
type ReviewFetch struct {
	Branch string `json:"branch"`
	Base   string `json:"base"`
	// BaseRef is the ref Base resolved to on the host.
	BaseRef   string `json:"base_ref"`
	Head      string `json:"head"`
	MergeBase string `json:"merge_base"`
	// Bundle holds the branch's commits after Since (or all of them).
	Bundle []byte `json:"bundle,omitempty"`
}

var commitID = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// reviewNotesStatus reports a run's own notes for show.
func reviewNotesStatus(dir string) []reviewnotes.Status {
	return []reviewnotes.Status{reviewnotes.Inspect(filepath.Join(dir, reviewnotes.FileName))}
}

func reviewFetch(dir string, m Meta, q Request) (*ReviewFetch, error) {
	if m.Name == "" {
		return nil, fmt.Errorf("order not found: %s", q.Name)
	}
	repo := m.ResolvedRepo
	if repo == "" || !exists(repo) {
		repo = filepath.Join(dir, "wt")
	}
	if m.Branch == "" {
		return nil, fmt.Errorf("run %s has no branch", m.Name)
	}
	f := &ReviewFetch{Branch: m.Branch, Base: m.Base}
	var err error
	if f.Head, err = command(repo, "git", "rev-parse", "--verify", "--end-of-options", "refs/heads/"+m.Branch+"^{commit}"); err != nil {
		return nil, fmt.Errorf("branch %s not found on host: %w", m.Branch, err)
	}
	base := m.Base
	if base == "" {
		base = "main"
	}
	for _, ref := range []string{"refs/remotes/origin/" + base, "refs/heads/" + base, base} {
		if _, e := command(repo, "git", "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}"); e == nil {
			f.BaseRef = ref
			break
		}
	}
	if f.BaseRef == "" {
		return nil, fmt.Errorf("base %s not found on host", base)
	}
	if f.MergeBase, err = command(repo, "git", "merge-base", "--end-of-options", f.BaseRef, f.Head); err != nil {
		return nil, err
	}
	if !q.Bundle {
		return f, nil
	}
	var exclude []string
	if q.Since != "" {
		if !commitID.MatchString(q.Since) {
			return nil, fmt.Errorf("invalid since commit")
		}
		if reviewnotes.SameHead(q.Since, f.Head) {
			return f, nil
		}
		exclude = []string{"^" + q.Since}
	}
	tmp, err := os.MkdirTemp("", "hev-review-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	path := filepath.Join(tmp, "branch.bundle")
	args := append([]string{"git", "bundle", "create", "-q", path}, exclude...)
	if _, err = command(repo, append(args, "refs/heads/"+m.Branch)...); err != nil {
		return nil, err
	}
	f.Bundle, err = os.ReadFile(path)
	return f, err
}
