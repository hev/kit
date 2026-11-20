package dispatch

import (
	"fmt"
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
//
// Only failures that say the connection never happened count: ssh's own
// connect errors and its exit status 255, unless ssh says the session was
// established and then dropped. "hevd unreachable; using ssh" is
// not one, since peers prints it before every SSH fallback that may then
// reach the inbox. A deadline or a killed sender is not one either: the push
// may have landed, and the pull would deliver it again under another ID.
func ownerUnreachable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "relay pending:") && !strings.Contains(s, "recipient snapshot stale") && !strings.Contains(s, "recipient absent") {
		return false
	}
	// ssh exits 255 too when an established session drops; the remote
	// `hev dispatch rpc` may already have delivered, so that is uncertain,
	// not unreachable.
	for _, fragment := range []string{"server not responding", "closed by remote host"} {
		if strings.Contains(s, fragment) {
			return false
		}
	}
	for _, fragment := range []string{
		"pull required:", "could not resolve hostname", "no such host", "name or service not known",
		"nodename nor servname provided", "connection refused", "connection timed out", "operation timed out",
		"no route to host", "network is unreachable", "host is down", "ssh peers rpc: exit status 255", "relay pending:",
	} {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}

// ownerNoticeEligible reports a run whose completion notice belongs in the
// owner queue. Teammate runs keep theirs in the member store; validators and
// forwards have no owner notice.
func ownerNoticeEligible(m Meta) bool {
	return m.Reply != "" && !memberRun(m) && m.Parent == "" && m.Forward == nil
}

// settleOwnerNotice follows every push of a run notice (notify), so run
// ends, repeat ends, validation verdicts and an adopt's retry all keep the
// same queue. A push that could not reach the owner is queued for pull, or
// re-addressed when the notice is already queued (an adopt re-points the
// reply). A push that succeeds retires the same notice if an earlier push
// queued it, so a pull never delivers it a second time or to the old owner.
// The returned error is pushErr, annotated when the notice was queued.
func settleOwnerNotice(m Meta, r Result, pushErr error) error {
	if !ownerNoticeEligible(m) {
		return pushErr
	}
	unreachable := ownerUnreachable(pushErr)
	if pushErr != nil && !unreachable {
		return pushErr
	}
	dir := ownerNoticeDir()
	if pushErr == nil && !exists(dir) {
		return nil
	}
	n, file := runEndNotice(m, r)
	path := filepath.Join(dir, file)
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return keepFailed(pushErr, e)
	}
	defer unlock()
	var cur OrchestratorNotice
	have := readJSON(path, &cur) == nil
	if pushErr == nil {
		if have && cur.Status == "queued" {
			cur.Status, cur.Error = "delivered", ""
			if e = writeNotice(path, &cur); e != nil {
				return fmt.Errorf("pushed, but retiring the queued copy failed: %v", e)
			}
		}
		return nil
	}
	if have {
		if cur.Status != "queued" {
			// Already delivered by a pull; this push is the duplicate.
			return pushErr
		}
		cur.To, cur.Error = m.Reply, noticeText(m.Name, pushErr.Error())
		n = cur
	} else {
		n.Route = "pull"
		n.Error = noticeText(m.Name, pushErr.Error())
	}
	if e = writeNotice(path, &n); e != nil {
		return keepFailed(pushErr, e)
	}
	if e = pruneMemberNotices(dir); e != nil {
		return keepFailed(pushErr, e)
	}
	return fmt.Errorf("%w; queued for hev orchestrate notices pull", pushErr)
}

func keepFailed(pushErr, e error) error {
	if pushErr == nil {
		return nil
	}
	return fmt.Errorf("%w; keeping the notice for pull also failed: %v", pushErr, e)
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
