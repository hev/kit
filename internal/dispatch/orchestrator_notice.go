package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// OrchestratorNotice is a durable outbound receipt. Delivered means accepted by
// the recipient inbox, not read or acknowledged by the person owning it.
type OrchestratorNotice struct {
	ID      string    `json:"id"`
	To      string    `json:"to"`
	Status  string    `json:"status"`
	Error   string    `json:"error"`
	Message string    `json:"message"`
	Created time.Time `json:"created"`
}

type OrchestratorNotifier func(context.Context, OrchestratorNotice) error

var orchestratorNoticeSender OrchestratorNotifier = sendOrchestratorNotice

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

func noticeSender(d OrchestratorDriver) OrchestratorNotifier {
	if d.Notify != nil {
		return d.Notify
	}
	return orchestratorNoticeSender
}

// Each event has an immutable ID and payload. The recipient deduplicates IDs,
// including when a send succeeds but recording the receipt is interrupted.
func orchestratorNotice(s OrchestratorSpec, round int, key, to, message string, send OrchestratorNotifier) {
	if s.Reply == "" || to == "" {
		return
	}
	dir := OrchestratorDir(s.Name)
	unlock, err := platformops.LockPath(filepath.Join(dir, "notices.lock"), false)
	if err != nil {
		return
	}
	defer unlock()
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(key+"\x00"+to)))
	path := filepath.Join(dir, "rounds", fmt.Sprintf("%03d", round), "notices", sum, "notice.json")
	if key == "reply" {
		path = filepath.Join(dir, "rounds", fmt.Sprintf("%03d", round), "notice.json")
	}
	var n OrchestratorNotice
	if err = readJSON(path, &n); os.IsNotExist(err) {
		host := s.Host
		if host == "" {
			host, _ = os.Hostname()
		}
		n = OrchestratorNotice{ID: host + "/o-" + s.Name + ":" + fmt.Sprint(round) + ":" + sum, To: to, Status: "queued", Message: message, Created: time.Now().UTC()}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return
		}
		if err = writeNotice(path, &n); err != nil {
			return
		}
	} else if err != nil {
		return
	}
	updateLastNotice(dir, &n)
	deliverOrchestratorNotice(dir, path, &n, send)
}

func deliverOrchestratorNotice(dir, path string, n *OrchestratorNotice, send OrchestratorNotifier) {
	if n.Status == "delivered" {
		updateLastNotice(dir, n)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n.Status, n.Error = "delivered", ""
	if err := send(ctx, *n); err != nil {
		n.Status, n.Error = "queued", err.Error()
	}
	if writeNotice(path, n) != nil {
		return
	}
	updateLastNotice(dir, n)
}

func updateLastNotice(dir string, n *OrchestratorNotice) {
	// Do not replace a newer notice when retrying an older one.
	var last OrchestratorNotice
	if readJSON(filepath.Join(dir, "last-notice.json"), &last) != nil || !last.Created.After(n.Created) {
		_ = writeNotice(filepath.Join(dir, "last-notice.json"), n)
	}
}

func retryOrchestratorNotices(name string, send OrchestratorNotifier) {
	dir := OrchestratorDir(name)
	unlock, err := platformops.LockPath(filepath.Join(dir, "notices.lock"), false)
	if err != nil {
		return
	}
	defer unlock()
	_ = filepath.WalkDir(filepath.Join(dir, "rounds"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "notice.json" {
			return nil
		}
		var n OrchestratorNotice
		if readJSON(path, &n) == nil {
			deliverOrchestratorNotice(dir, path, &n, send)
		}
		return nil
	})
}

func orchestratorLifecycle(s OrchestratorSpec, st OrchestratorState, event, key string, send OrchestratorNotifier, recipients ...string) {
	host := s.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	message := fmt.Sprintf("Orchestrator %s on %s: %s", s.Name, host, strings.Join(strings.Fields(event), " "))
	if len(recipients) == 0 {
		recipients = []string{s.Reply}
	}
	for _, to := range recipients {
		orchestratorNotice(s, st.Rounds, st.Start.Format(time.RFC3339Nano)+":"+key, to, message, send)
	}
}

func orchestratorReply(s OrchestratorSpec, st OrchestratorState, send OrchestratorNotifier) {
	allowed := false
	for _, a := range s.Authority {
		if a == "notify" {
			allowed = true
		}
	}
	if !allowed || s.Reply == "" {
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
	orchestratorNotice(s, st.Rounds, "reply", s.Reply, body, send)
}

func orchestratorBudgetNotice(s OrchestratorSpec, cap string) {
	st, err := orchestratorState(s.Name)
	if err != nil {
		return
	}
	orchestratorLifecycle(s, st, "budget exhausted ("+cap+")", "budget:"+cap+":"+time.Now().UTC().Format("2006-01-02"), orchestratorNoticeSender)
}

// Sync the immutable outbox record before sending; atomic rename keeps readers
// from observing partial receipts. A send is never attempted without this record.
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
	return os.Rename(path+".tmp", path)
}
