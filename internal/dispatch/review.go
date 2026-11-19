package dispatch

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/internal/reviewnotes"
	"github.com/hev/kit/internal/version"
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
	repo := reviewRepoDir(dir, m)
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

// ReviewRecordRequest is what hev review record sends: the reviewer's
// findings for one head. A clean review has no annotations.
type ReviewRecordRequest struct {
	Head     string            `json:"head"`
	Reviewer string            `json:"reviewer,omitempty"`
	Notes    reviewnotes.Notes `json:"notes"`
}

func reviewsDir(dir string) string { return filepath.Join(dir, reviewnotes.ReviewsDir) }

// loadReviewRecords reads a run's recorded reviews, oldest first. Files that
// do not parse are skipped: a torn record never hides the others.
func loadReviewRecords(dir string) ([]reviewnotes.Record, error) {
	entries, err := os.ReadDir(reviewsDir(dir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []reviewnotes.Record
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || filepath.Ext(n) != ".json" {
			continue
		}
		var r reviewnotes.Record
		if readJSON(filepath.Join(reviewsDir(dir), n), &r) == nil && r.ID+".json" == n {
			out = append(out, r)
		}
	}
	reviewnotes.SortRecords(out)
	return out, nil
}

// lockReviews serializes record and send for one run.
func lockReviews(dir string) (func(), error) {
	if err := os.MkdirAll(reviewsDir(dir), 0o700); err != nil {
		return nil, err
	}
	return lockDirectory(reviewsDir(dir))
}

func newReviewID(now time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return "rv-" + now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// reviewRepoDir is the repository holding a run's branch on this host:
// the resolved repository, else the run's worktree.
func reviewRepoDir(dir string, m Meta) string {
	if m.ResolvedRepo != "" && exists(m.ResolvedRepo) {
		return m.ResolvedRepo
	}
	return filepath.Join(dir, "wt")
}

// reviewRecord stores a reviewer's findings (or a clean review) for one head
// beside the run. An orchestrator's records name it as the reviewer, whatever
// the request says.
func reviewRecord(dir string, m Meta, q Request) (Response, error) {
	r := Response{Version: version.Version}
	if m.Name == "" {
		return r, fmt.Errorf("order not found: %s", q.Name)
	}
	rr := q.ReviewRecord
	if rr == nil {
		return r, fmt.Errorf("review-record needs findings")
	}
	reviewer := strings.TrimSpace(rr.Reviewer)
	if q.Orchestrator != "" {
		reviewer = "o-" + q.Orchestrator
	}
	head := strings.TrimSpace(rr.Head)
	if !commitID.MatchString(head) {
		return r, fmt.Errorf("head %q is not a commit SHA", head)
	}
	full, err := command(reviewRepoDir(dir, m), "git", "rev-parse", "--verify", "--quiet", "--end-of-options", head+"^{commit}")
	if err != nil {
		return r, fmt.Errorf("head %s is not a commit in %s's repository", head, m.Name)
	}
	now := time.Now()
	rec, err := reviewnotes.NewRecord(newReviewID(now), reviewer, full, now, rr.Notes)
	if err != nil {
		return r, err
	}
	unlock, err := lockReviews(dir)
	if err != nil {
		return r, err
	}
	defer unlock()
	if err = WriteJSON(filepath.Join(reviewsDir(dir), rec.ID+".json"), rec); err != nil {
		return r, err
	}
	r.Reviews, _ = loadReviewRecords(dir)
	r.Message = fmt.Sprintf("recorded review %s of %s at %s: %s, %d findings", rec.ID, m.Name, reviewnotes.Short(rec.Head), rec.Verdict, rec.Findings)
	if f, err := reviewFetch(dir, m, Request{}); err == nil && !reviewnotes.SameHead(f.Head, rec.Head) {
		r.Message += fmt.Sprintf(" (the branch head is now %s)", reviewnotes.Short(f.Head))
	}
	return r, nil
}

// reviewSend delivers the findings recorded for the branch's current head and
// not yet sent, as one anchored message through say: a live run's inbox, or
// a rework of an ended run on the same branch. Delivered records are marked
// sent; records for other heads are never sent.
func reviewSend(dir string, m Meta, q Request) (Response, error) {
	r := Response{Version: version.Version}
	if m.Name == "" {
		return r, fmt.Errorf("order not found: %s", q.Name)
	}
	f, err := reviewFetch(dir, m, Request{})
	if err != nil {
		return r, err
	}
	unlock, err := lockReviews(dir)
	if err != nil {
		return r, err
	}
	defer unlock()
	recs, err := loadReviewRecords(dir)
	if err != nil {
		return r, err
	}
	r.Reviews = recs
	due, stale := reviewnotes.Unsent(recs, f.Head)
	if len(due) == 0 {
		r.Message = fmt.Sprintf("no unsent findings recorded for %s at %s; nothing to send", m.Name, reviewnotes.Short(f.Head))
		if stale > 0 {
			r.Message += fmt.Sprintf(" (%d unsent for earlier heads, not sent)", stale)
		}
		return r, nil
	}
	msg := reviewnotes.FormatFindings(m.Name, f.Head, due)
	r.ReviewDue = len(due)
	if q.DryRun {
		r.Message = msg
		return r, nil
	}
	sr, err := Handle(Request{Action: "say", Name: q.Name, Note: msg, Orchestrator: q.Orchestrator, Caller: q.Caller})
	if err != nil {
		return r, fmt.Errorf("sending findings to %s: %w (nothing marked sent)", m.Name, err)
	}
	now := time.Now().UTC()
	for _, rec := range due {
		rec.Sent = &reviewnotes.Delivery{Time: now, Message: sr.Message}
		if e := WriteJSON(filepath.Join(reviewsDir(dir), rec.ID+".json"), rec); e != nil && err == nil {
			err = fmt.Errorf("sent, but marking %s sent failed: %w", rec.ID, e)
		}
	}
	r.Reviews, _ = loadReviewRecords(dir)
	r.SteerID, r.Delivery, r.DeliveryStatus = sr.SteerID, sr.Delivery, sr.DeliveryStatus
	r.Message = msg + "\n" + sr.Message
	if stale > 0 {
		r.Message += fmt.Sprintf(" (%d unsent for earlier heads, not sent)", stale)
	}
	return r, err
}
