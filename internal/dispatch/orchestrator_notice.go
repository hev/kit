package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// OrchestratorNotice is a durable outbound receipt. Delivered means accepted by
// the recipient inbox (or acknowledged by a pull), not read by the owner.
// Queued receipts retry; failed and stale are terminal.
type OrchestratorNotice struct {
	ID           string    `json:"id"`
	To           string    `json:"to"`
	Status       string    `json:"status"`
	Error        string    `json:"error"`
	Message      string    `json:"message"`
	Created      time.Time `json:"created"`
	Kind         string    `json:"kind,omitempty"`
	Orchestrator string    `json:"orchestrator,omitempty"`
	Attempts     int       `json:"attempts,omitempty"`
	NextAttempt  time.Time `json:"next_attempt,omitempty"`
	// Lease marks a send in progress so concurrent flushes do not repeat it.
	Lease time.Time `json:"lease,omitempty"`
}

type OrchestratorNotifier func(context.Context, OrchestratorNotice) error

var orchestratorNoticeSender OrchestratorNotifier = sendOrchestratorNotice

// Tests replace the clock and the send deadline. The deadline matches
// dispatch's: an ssh ConnectTimeout plus remote exec.
var (
	noticeNow                 = time.Now
	orchestratorNoticeTimeout = 20 * time.Second
)

const (
	noticeMaxBackoff  = time.Hour
	noticeTextLimit   = 200
	sourceCapDeferral = "source run cap reached"
)

func init() { IntakeSignalHook = queueIntakeNotice }

func sendOrchestratorNotice(ctx context.Context, n OrchestratorNotice) error {
	exe, err := notificationExecutable()
	if err != nil {
		return err
	}
	// Like detached dispatch completion notices, use the native peers backend
	// and an explicit sender; never inherit a vanished harness's PEERS_FROM.
	sender, _, ok := strings.Cut(n.ID, ":")
	if !ok {
		return fmt.Errorf("invalid notice ID")
	}
	return executeNotification(ctx, []string{exe, "peers", "send", n.To, n.Message, "--from", sender, "--id", n.ID, "--inbox-only"}, true)
}

// SendOrchestratorNotice delivers a pulled notice into the local peers inbox
// under its stable ID, so a redelivery after a lost ack is deduplicated.
func SendOrchestratorNotice(ctx context.Context, n OrchestratorNotice) error {
	ctx, cancel := context.WithTimeout(ctx, orchestratorNoticeTimeout)
	defer cancel()
	return sendOrchestratorNotice(ctx, n)
}

func noticeSender(d OrchestratorDriver) OrchestratorNotifier {
	if d.Notify != nil {
		return d.Notify
	}
	return orchestratorNoticeSender
}

func noticeHost(s OrchestratorSpec) string {
	if s.Host != "" {
		return s.Host
	}
	return notificationHost()
}

// noticeSenderAddress is the HOST/o-NAME peers sender. Apply validates it so a
// bad host label fails early instead of queueing permanently refused notices.
func noticeSenderAddress(s OrchestratorSpec) (string, error) {
	sender := noticeHost(s) + "/o-" + s.Name
	if !validNoticeSender(sender) {
		return "", fmt.Errorf("owner notice sender %q is not a valid peers address; set host", sender)
	}
	return sender, nil
}

// validNoticeSender mirrors the peers network sender check.
func validNoticeSender(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 255 || strings.HasPrefix(part, "-") {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return false
			}
		}
	}
	return true
}

// noticeText collapses whitespace and bounds the text, pointing to show for
// the full record. Errors can be long and also appear in ls --json.
func noticeText(name, text string) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) <= noticeTextLimit {
		return string(r)
	}
	return string(r[:noticeTextLimit]) + " [truncated; hev orchestrate show " + name + "]"
}

func lockNotices(name string) (func(), error) {
	return platformops.LockPath(filepath.Join(OrchestratorDir(name), "notices.lock"), false)
}

// queueOrchestratorNotice durably records one notice and never sends it. A key
// and recipient map to one receipt under notices/KIND/, outside rounds/, so
// the outbound ID, payload and recipient stay fixed across rounds, retries
// and redelivery. A nil error means the receipt is on disk.
func queueOrchestratorNotice(s OrchestratorSpec, kind, key, to, message string) error {
	dir := OrchestratorDir(s.Name)
	// A scope-only target is not the owner and must not speak for it.
	if s.Reply == "" || to == "" || exists(filepath.Join(dir, "scope-owner.json")) {
		return nil
	}
	sender, e := noticeSenderAddress(s)
	if e != nil {
		return e
	}
	unlock, e := lockNotices(s.Name)
	if e != nil {
		return e
	}
	defer unlock()
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(kind+"\x00"+key+"\x00"+to)))
	path := filepath.Join(dir, "notices", kind, sum+".json")
	var n OrchestratorNotice
	if e = readJSON(path, &n); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	n = OrchestratorNotice{ID: sender + ":" + kind + ":" + sum, To: to, Status: "queued", Message: message, Created: noticeNow().UTC(), Kind: kind, Orchestrator: s.Name}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if e = writeNotice(path, &n); e != nil {
		return e
	}
	updateLastNotice(dir, &n)
	return nil
}

type noticeFile struct {
	path string
	n    OrchestratorNotice
}

func loadOrchestratorNotices(name string) ([]noticeFile, error) {
	var out []noticeFile
	root := filepath.Join(OrchestratorDir(name), "notices")
	e := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" || filepath.Dir(path) == root {
			return nil
		}
		var n OrchestratorNotice
		if err = readJSON(path, &n); err != nil {
			return err
		}
		out = append(out, noticeFile{path, n})
		return nil
	})
	slices.SortFunc(out, func(a, b noticeFile) int {
		if c := a.n.Created.Compare(b.n.Created); c != 0 {
			return c
		}
		return strings.Compare(a.n.ID, b.n.ID)
	})
	return out, e
}

// updateNotice applies fn to a receipt under the notices lock.
func updateNotice(name, path string, fn func(*OrchestratorNotice) bool) (OrchestratorNotice, bool, error) {
	var n OrchestratorNotice
	unlock, e := lockNotices(name)
	if e != nil {
		return n, false, e
	}
	defer unlock()
	if e = readJSON(path, &n); e != nil {
		return n, false, e
	}
	if !fn(&n) {
		return n, false, nil
	}
	if e = writeNotice(path, &n); e != nil {
		return n, false, e
	}
	updateLastNotice(OrchestratorDir(name), &n)
	return n, true, nil
}

func noticeBackoff(attempts int) time.Duration {
	d := time.Minute
	for i := 1; i < attempts && d < noticeMaxBackoff; i++ {
		d *= 2
	}
	return min(d, noticeMaxBackoff)
}

// terminalNoticeError is a refusal that no retry can fix: after an owner
// restart the inbox rejects a reused ID for a different recipient.
func terminalNoticeError(e error) bool {
	return strings.Contains(e.Error(), "reused with different")
}

// flushOrchestratorNotices delivers queued receipts. Callers must hold no
// dispatch or orchestrator lock: the notices lock covers reads and receipt
// writes, never a send. Each recipient gets its oldest notices first, stops at
// its first failure and backs off, so an unreachable owner costs at most one
// bounded attempt per wake. The pull route never execs a transport.
func flushOrchestratorNotices(s OrchestratorSpec, send OrchestratorNotifier) {
	if s.NotifyRoute == "pull" {
		return
	}
	all, e := loadOrchestratorNotices(s.Name)
	if e != nil {
		return
	}
	now := noticeNow()
	blocked := map[string]bool{}
	for _, f := range all {
		if f.n.Status != "queued" || blocked[f.n.To] {
			continue
		}
		if f.n.NextAttempt.After(now) || f.n.Lease.After(now) {
			blocked[f.n.To] = true
			continue
		}
		n, claimed, e := updateNotice(s.Name, f.path, func(n *OrchestratorNotice) bool {
			if n.Status != "queued" || n.Lease.After(now) {
				return false
			}
			n.Lease = now.Add(orchestratorNoticeTimeout + notificationWaitDelay + time.Minute)
			return true
		})
		if e != nil || !claimed {
			blocked[f.n.To] = true
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), orchestratorNoticeTimeout)
		sendErr := send(ctx, n)
		cancel()
		_, _, _ = updateNotice(s.Name, f.path, func(n *OrchestratorNotice) bool {
			n.Lease = time.Time{}
			if n.Status != "queued" {
				return true // Acknowledged or made stale while sending.
			}
			switch {
			case sendErr == nil:
				n.Status, n.Error, n.NextAttempt = "delivered", "", time.Time{}
			case terminalNoticeError(sendErr):
				n.Status, n.Error = "failed", noticeText(s.Name, sendErr.Error())
			default:
				n.Attempts++
				n.Error = noticeText(s.Name, sendErr.Error())
				n.NextAttempt = noticeNow().Add(noticeBackoff(n.Attempts)).UTC()
			}
			return true
		})
		if sendErr != nil {
			blocked[n.To] = true
		}
	}
}

func flushOrchestratorNoticesByName(name string, send OrchestratorNotifier) {
	if s, e := LoadOrchestrator(name); e == nil {
		flushOrchestratorNotices(s, send)
	}
}

// staleOrchestratorNotices retires queued notices on reapply, so nothing
// queued before a stop is sent late to the new lineage.
func staleOrchestratorNotices(name string) error {
	all, e := loadOrchestratorNotices(name)
	if e != nil {
		return e
	}
	for _, f := range all {
		if f.n.Status != "queued" {
			continue
		}
		if _, _, e = updateNotice(name, f.path, func(n *OrchestratorNotice) bool {
			if n.Status != "queued" {
				return false
			}
			n.Status, n.Lease = "stale", time.Time{}
			return true
		}); e != nil {
			return e
		}
	}
	return nil
}

// PendingOrchestratorNotices lists queued receipts oldest first for the pull
// route. An empty name lists every orchestrator on this host.
func PendingOrchestratorNotices(name string) ([]OrchestratorNotice, error) {
	names := []string{name}
	if name == "" {
		rows, e := ListOrchestrators()
		if e != nil {
			return nil, e
		}
		names = nil
		for _, row := range rows {
			names = append(names, row.Name)
		}
	} else if _, e := LoadOrchestrator(name); e != nil {
		return nil, e
	}
	out := []OrchestratorNotice{}
	for _, n := range names {
		all, e := loadOrchestratorNotices(n)
		if e != nil {
			return nil, e
		}
		for _, f := range all {
			if f.n.Status == "queued" {
				out = append(out, f.n)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b OrchestratorNotice) int { return a.Created.Compare(b.Created) })
	return out, nil
}

// AckOrchestratorNotices marks pulled notices delivered. IDs name their
// orchestrator (HOST/o-NAME:...); unknown or already final IDs are ignored.
func AckOrchestratorNotices(ids []string) error {
	byName := map[string][]string{}
	for _, id := range ids {
		sender, _, _ := strings.Cut(id, ":")
		_, name, ok := strings.Cut(sender, "/o-")
		if !ok || !ValidName(name) {
			return fmt.Errorf("invalid notice ID %q", id)
		}
		byName[name] = append(byName[name], id)
	}
	for name, want := range byName {
		if _, e := LoadOrchestrator(name); e != nil {
			return e
		}
		all, e := loadOrchestratorNotices(name)
		if e != nil {
			return e
		}
		for _, f := range all {
			if !slices.Contains(want, f.n.ID) || f.n.Status != "queued" {
				continue
			}
			if _, _, e = updateNotice(name, f.path, func(n *OrchestratorNotice) bool {
				if n.Status != "queued" {
					return false
				}
				n.Status, n.Error, n.Lease, n.NextAttempt = "delivered", "", time.Time{}, time.Time{}
				return true
			}); e != nil {
				return e
			}
		}
	}
	return nil
}

func updateLastNotice(dir string, n *OrchestratorNotice) {
	// Do not replace a newer notice when retrying an older one.
	var last OrchestratorNotice
	if readJSON(filepath.Join(dir, "last-notice.json"), &last) != nil || !last.Created.After(n.Created) {
		_ = writeNotice(filepath.Join(dir, "last-notice.json"), n)
	}
}

func lifecycleMessage(s OrchestratorSpec, event string) string {
	return noticeText(s.Name, fmt.Sprintf("Orchestrator %s on %s: %s", s.Name, noticeHost(s), event))
}

// orchestratorLifecycle queues one notice per recipient. Keys are scoped to
// the lineage start, never the round, so a once-per-lineage event (stop,
// handoff, complete) yields one notice even when it races a round.
func orchestratorLifecycle(s OrchestratorSpec, st OrchestratorState, event, key string, recipients ...string) error {
	if len(recipients) == 0 {
		recipients = []string{s.Reply}
	}
	var first error
	for _, to := range recipients {
		if e := queueOrchestratorNotice(s, "lifecycle", st.Start.Format(time.RFC3339Nano)+":"+key, to, lifecycleMessage(s, event)); e != nil && first == nil {
			first = e
		}
	}
	return first
}

func orchestratorReply(s OrchestratorSpec, st OrchestratorState) {
	if !slices.Contains(s.Authority, "notify") || s.Reply == "" {
		return
	}
	b, err := os.ReadFile(filepath.Join(OrchestratorDir(s.Name), "rounds", fmt.Sprintf("%03d", st.Rounds), "reply.md"))
	if err != nil {
		return
	}
	body := strings.TrimSpace(string(b))
	if body == "" {
		return
	}
	r := []rune(body)
	if len(r) > 2000 {
		body = string(r[:1800]) + "\n[truncated; hev orchestrate show " + s.Name + "]"
	}
	// Delivery errors never fail an accepted round.
	_ = queueOrchestratorNotice(s, "reply", st.Start.Format(time.RFC3339Nano)+":"+fmt.Sprint(st.Rounds), s.Reply, body)
}

// orchestratorBudgetNotice runs under admission locks, so it only queues; the
// owner's supervisor delivers on its next wake. Daily keys dedupe the cap.
func orchestratorBudgetNotice(s OrchestratorSpec, cap string) {
	st, err := orchestratorState(s.Name)
	if err != nil {
		return
	}
	_ = orchestratorLifecycle(s, st, "budget exhausted ("+cap+")", "budget:"+cap+":"+noticeNow().UTC().Format("2006-01-02"))
}

// queueIntakeNotice is the IntakeSignalHook. A nil return means the owner
// notice is durably queued under the signal's stable ID, which is what lets
// #155's outbox mark the signal delivered. It never sends and never panics.
func queueIntakeNotice(s OrchestratorSpec, sig IntakeSignal) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("intake notice: %v", r)
		}
	}()
	event := "intake " + sig.Kind + " for event " + sig.Event
	if sig.Run != "" {
		event += " (run " + sig.Run + ")"
	}
	if sig.Error != "" {
		event += ": " + sig.Error
	}
	return queueOrchestratorNotice(s, "intake", sig.ID, s.Reply, lifecycleMessage(s, event))
}

// noteSourceCap queues one informational notice per cap episode: from the
// first capped round until a round runs without a cap deferral. It is not a
// failure and never changes deferral semantics; errors only lose the notice.
func noteSourceCap(s OrchestratorSpec, st OrchestratorState, ran bool) {
	path := filepath.Join(OrchestratorDir(s.Name), "notices", "cap-episode.json")
	var episode struct {
		Since time.Time `json:"since"`
	}
	open := readJSON(path, &episode) == nil
	capped := strings.Contains(st.Deferred, sourceCapDeferral)
	switch {
	case capped && !open:
		episode.Since = noticeNow().UTC()
		if os.MkdirAll(filepath.Dir(path), 0700) != nil || WriteJSON(path, episode) != nil {
			return
		}
		_ = orchestratorLifecycle(s, st, "source run cap reached; events stay pending and retry (deferred, not failed)", "cap:"+episode.Since.Format(time.RFC3339Nano))
	case !capped && open && ran:
		_ = os.Remove(path)
	}
}

// Sync the outbox record and its directory before any send; atomic rename
// keeps readers from observing partial receipts.
func writeNotice(path string, n *OrchestratorNotice) error {
	b, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = platformops.Rename(path+".tmp", path); err != nil {
		return err
	}
	return noticeDirSync(filepath.Dir(path))
}

var noticeDirSync = syncDir

// syncDir makes a rename durable. Windows cannot fsync a directory handle.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}
