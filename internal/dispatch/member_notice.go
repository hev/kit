package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// Teammates' laptops cannot be reached from the host, so their run notices
// are kept here until the teammate pulls them over their forced-command key.
const (
	memberNoticeLimit  = 100
	memberNoticeMaxAge = 14 * 24 * time.Hour
	memberNoticeBatch  = 50
	memberNoticeText   = 1000
)

// memberSender reports a sender authenticated from a host-owned SSH grant.
// Unrestricted starts always carry the separate local:ACCOUNT namespace.
func memberSender(s string) bool { return s != "" && !strings.Contains(s, ":") }

// memberRun excludes validator children and forwards, as teammate actions do.
func memberRun(m Meta) bool { return memberSender(m.Sender) && m.Parent == "" && m.Forward == nil }

// Hex keeps case-insensitive file systems from merging two senders.
func memberNoticeDir(sender string) string {
	return filepath.Join(Root(), ".member-notices", hex.EncodeToString([]byte(sender)))
}

func lockMemberNotices(dir string) (func(), error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	return platformops.LockPath(filepath.Join(dir, "notices.lock"), false)
}

// queueMemberNotice records the notice a push would carry and reports whether
// the configured push should still run. Templates addressed to {reply} name a
// session on the teammate's laptop, which the host cannot reach (and which
// could collide with an unrelated session here), so they are skipped. Other
// templates notify owner-side sinks and still run.
func queueMemberNotice(args []string, m Meta, r Result) (bool, error) {
	push := len(args) > 0
	for _, a := range args {
		if strings.Contains(a, "{reply}") {
			push = false
		}
	}
	state := State(&r, false)
	key := strings.Join([]string{m.Name, m.Start.Format(time.RFC3339Nano), r.End.Format(time.RFC3339Nano), state, r.Validation, verdictName(r)}, "\x00")
	sum := sha256.Sum256([]byte(key))
	message := []rune(fmt.Sprintf("dispatch %s: %s %s", m.Name, state, noticeSummary(m, r)))
	if len(message) > memberNoticeText {
		message = append(message[:memberNoticeText], []rune(" [truncated; hev dispatch show "+m.Name+"]")...)
	}
	n := OrchestratorNotice{
		ID:      notificationHost() + "/d-" + m.Name + ":dispatch:" + hex.EncodeToString(sum[:16]),
		To:      m.Reply,
		Status:  "queued",
		Message: string(message),
		Created: noticeNow().UTC(),
		Kind:    "dispatch",
	}
	dir := memberNoticeDir(m.Sender)
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return push, e
	}
	defer unlock()
	path := filepath.Join(dir, hex.EncodeToString(sum[:16])+".json")
	// The same event (a retried notify) keeps one receipt and one ID.
	if exists(path) {
		return push, nil
	}
	if e = writeNotice(path, &n); e != nil {
		return push, e
	}
	return push, pruneMemberNotices(dir)
}

func verdictName(r Result) string {
	if r.Verdict == nil {
		return ""
	}
	return r.Verdict.Verdict
}

// loadMemberNotices returns receipts oldest first. Callers hold the lock.
func loadMemberNotices(dir string) ([]noticeFile, error) {
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []noticeFile
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var n OrchestratorNotice
		path := filepath.Join(dir, entry.Name())
		if e = readJSON(path, &n); e != nil {
			return nil, e
		}
		out = append(out, noticeFile{path, n})
	}
	slices.SortFunc(out, func(a, b noticeFile) int {
		if c := a.n.Created.Compare(b.n.Created); c != 0 {
			return c
		}
		return strings.Compare(a.n.ID, b.n.ID)
	})
	return out, nil
}

// pruneMemberNotices drops receipts past the age bound, then the oldest past
// the count bound. Delivered receipts stay until then so a retried event
// cannot be queued a second time.
func pruneMemberNotices(dir string) error {
	all, e := loadMemberNotices(dir)
	if e != nil {
		return e
	}
	cutoff := noticeNow().Add(-memberNoticeMaxAge)
	for i, f := range all {
		if f.n.Created.Before(cutoff) || len(all)-i > memberNoticeLimit {
			if e = os.Remove(f.path); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
	}
	return nil
}

// PendingMemberNotices returns the sender's undelivered notices, oldest first
// and at most one batch. Only ever called with an authenticated sender.
func PendingMemberNotices(sender string) ([]OrchestratorNotice, error) {
	if !memberSender(sender) {
		return nil, fmt.Errorf("member notices require a teammate key")
	}
	dir := memberNoticeDir(sender)
	if !exists(dir) {
		return []OrchestratorNotice{}, nil
	}
	unlock, e := lockMemberNotices(dir)
	if e != nil {
		return nil, e
	}
	defer unlock()
	if e = pruneMemberNotices(dir); e != nil {
		return nil, e
	}
	all, e := loadMemberNotices(dir)
	if e != nil {
		return nil, e
	}
	out := []OrchestratorNotice{}
	for _, f := range all {
		if f.n.Status == "queued" && len(out) < memberNoticeBatch {
			out = append(out, f.n)
		}
	}
	return out, nil
}

// AckMemberNotices marks the sender's own notices delivered. IDs are looked up
// only in that sender's store, so other senders' IDs and unknown IDs are ignored.
func AckMemberNotices(sender string, ids []string) error {
	if !memberSender(sender) {
		return fmt.Errorf("member notices require a teammate key")
	}
	dir := memberNoticeDir(sender)
	if !exists(dir) {
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
		f.n.Status = "delivered"
		if e = writeNotice(f.path, &f.n); e != nil {
			return e
		}
	}
	return nil
}
