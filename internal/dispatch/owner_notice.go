package dispatch

import (
	"path/filepath"
	"slices"
	"strings"
)

// A run's completion notice is pushed to its owner (meta reply) by the
// configured notify command. When the owner's machine cannot be reached, the
// notice used to survive only as result.json's notify_error, so a pull from
// the owner found nothing: `hev orchestrate notices pull` reported "delivered
// 0 notices" while a needs-decision run was waiting. The receipt is now kept
// here, queued, and served with the orchestrator notices that pull already
// collects, whatever any notify_route says.

func ownerNoticeDir() string { return filepath.Join(Root(), ".owner-notices") }

// ownerUnreachable reports a push that failed because the owner's machine
// could not be reached, as opposed to a refusal by a reachable inbox. A relay
// that accepted the notice for later delivery is not a failure to reach.
func ownerUnreachable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "relay pending:") && !strings.Contains(s, "recipient snapshot stale") && !strings.Contains(s, "recipient absent") {
		return false
	}
	for _, fragment := range []string{
		"pull required:", "could not resolve hostname", "no such host", "name or service not known",
		"connection refused", "connection timed out", "operation timed out", "no route to host",
		"network is unreachable", "host is down", "connection closed by", "connection reset",
		"ssh peers rpc", "hevd unreachable", "relay pending:", "context deadline exceeded", "signal: killed",
	} {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}

// queueOwnerNotice keeps a run's completion notice for the owner's pull after
// its push failed to reach them. Teammate runs already keep theirs in the
// member store; validators and forwards have no owner notice.
func queueOwnerNotice(m Meta, r Result, pushErr error) (bool, error) {
	if m.Reply == "" || memberRun(m) || m.Parent != "" || m.Forward != nil || !ownerUnreachable(pushErr) {
		return false, nil
	}
	n, file := runEndNotice(m, r)
	n.Route = "pull"
	n.Error = noticeText(m.Name, pushErr.Error())
	dir := ownerNoticeDir()
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return false, e
	}
	defer unlock()
	path := filepath.Join(dir, file)
	if exists(path) {
		return true, nil
	}
	if e = writeNotice(path, &n); e != nil {
		return false, e
	}
	return true, pruneMemberNotices(dir)
}

// pendingOwnerNotices lists queued run notices oldest first.
func pendingOwnerNotices() ([]OrchestratorNotice, error) {
	dir := ownerNoticeDir()
	if !exists(dir) {
		return nil, nil
	}
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return nil, e
	}
	defer unlock()
	all, e := loadMemberNotices(dir)
	if e != nil {
		return nil, e
	}
	var out []OrchestratorNotice
	for _, f := range all {
		if f.n.Status == "queued" {
			out = append(out, f.n)
		}
	}
	return out, nil
}

// ackOwnerNotices marks pulled run notices delivered; unknown IDs are ignored.
func ackOwnerNotices(ids []string) error {
	dir := ownerNoticeDir()
	if len(ids) == 0 || !exists(dir) {
		return nil
	}
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return e
	}
	defer unlock()
	all, e := loadMemberNotices(dir)
	if e != nil {
		return e
	}
	for _, f := range all {
		if f.n.Status != "queued" || !slices.Contains(ids, f.n.ID) {
			continue
		}
		f.n.Status, f.n.Error = "delivered", ""
		if e = writeNotice(f.path, &f.n); e != nil {
			return e
		}
	}
	return nil
}

// ownerNoticeID reports a run notice ID, HOST/d-NAME:dispatch:HASH.
func ownerNoticeID(id string) bool {
	sender, rest, ok := strings.Cut(id, ":")
	_, name, isRun := strings.Cut(sender, "/d-")
	return ok && isRun && ValidName(name) && strings.HasPrefix(rest, "dispatch:")
}
