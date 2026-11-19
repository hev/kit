package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hev/kit/internal/hostlabel"
	"gopkg.in/yaml.v3"
)

// AdoptResult reports an adopt: who the orchestrator or run replied to, who
// it replies to now, and how many undelivered notices moved with it.
type AdoptResult struct {
	Name        string `json:"name"`
	From        string `json:"from"`
	To          string `json:"to"`
	Redelivered int    `json:"redelivered"`
	// Live is true when a running supervisor will pick the change up itself.
	Live bool `json:"live,omitempty"`
	// Retried and RetryError describe a run's retried completion notice.
	Retried    bool   `json:"retried,omitempty"`
	RetryError string `json:"retry_error,omitempty"`
}

// adoptedFile is the marker an adopt writes beside a live orchestrator's
// state. Its write wakes the supervisor's control watcher without starting a
// round; the supervisor then rereads its owner from the stored spec.
const adoptedFile = "adopted.json"

// validOwner accepts a peers address HOST/SESSION or a bare SESSION.
func validOwner(to string) bool {
	if strings.TrimSpace(to) != to || to == "" {
		return false
	}
	if !strings.Contains(to, "/") {
		return validNoticeSender("h/" + to)
	}
	return validNoticeSender(to)
}

// setSpecReply rewrites the reply key of an orchestrator spec's frontmatter,
// keeping every other key and the brief.
func setSpecReply(text, to string) (string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("spec must start with ---")
	}
	front, body, ok := strings.Cut(text[4:], "\n---")
	if !ok {
		return "", fmt.Errorf("unclosed frontmatter")
	}
	var doc yaml.Node
	if e := yaml.Unmarshal([]byte(front), &doc); e != nil {
		return "", e
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("frontmatter must be a mapping")
	}
	m := doc.Content[0]
	found := false
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "reply" {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: to}
			found = true
		}
	}
	if !found {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "reply"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: to})
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if e := enc.Encode(&doc); e != nil {
		return "", e
	}
	if e := enc.Close(); e != nil {
		return "", e
	}
	return "---\n" + out.String() + "---" + body, nil
}

// orchestratorTerminal reports why an orchestrator cannot be adopted.
func orchestratorTerminal(name string, st OrchestratorState) string {
	switch {
	case st.State == "complete" || st.State == "handed-off":
		return st.State
	case exists(filepath.Join(OrchestratorDir(name), "stop")):
		return "stopped"
	}
	return ""
}

// AdoptOrchestrator re-points a live orchestrator's owner. Rounds, checkpoint,
// runs and state are untouched; only reply changes, in the stored spec and in
// whatever a running supervisor reads. Notices still queued or failed for the
// old owner are queued again for the new one.
func AdoptOrchestrator(name, to string) (AdoptResult, error) {
	res := AdoptResult{Name: name, To: to}
	if !validOwner(to) {
		return res, fmt.Errorf("adopt needs a peers address HOST/SESSION, got %q", to)
	}
	if parent := currentOrchestratorIdentity(); parent != "" {
		if e := admitOrchestratorChild(parent, name); e != nil {
			return res, e
		}
	}
	spec, e := LoadOrchestrator(name)
	if e != nil {
		return res, e
	}
	dir := OrchestratorDir(name)
	if exists(filepath.Join(dir, "scope-owner.json")) {
		return res, fmt.Errorf("%s is an admission-only registration; adopt it on its owner host", name)
	}
	unlock, e := lockRoot()
	if e != nil {
		return res, e
	}
	st, e := orchestratorState(name)
	if e != nil {
		unlock()
		return res, e
	}
	if why := orchestratorTerminal(name, st); why != "" {
		unlock()
		return res, fmt.Errorf("orchestrator %s is %s (terminal); adopt only re-points a live orchestrator. To restart it for a new owner, set reply: %s in its spec and run hev orchestrate apply -f SPEC", name, why, to)
	}
	res.From = spec.Reply
	text, e := os.ReadFile(filepath.Join(OrchestratorConfigDir(), name+".md"))
	if e != nil {
		unlock()
		return res, e
	}
	next, e := setSpecReply(string(text), to)
	if e == nil {
		var parsed OrchestratorSpec
		if parsed, e = ParseOrchestrator(next); e == nil && parsed.Reply != to {
			e = fmt.Errorf("reply did not round-trip")
		}
	}
	if e != nil {
		unlock()
		return res, fmt.Errorf("rewrite spec reply: %w", e)
	}
	for _, p := range []string{filepath.Join(OrchestratorConfigDir(), name+".md"), filepath.Join(dir, "spec.md")} {
		if e = writeFileAtomic(p, []byte(next)); e != nil {
			unlock()
			return res, e
		}
	}
	// A running supervisor owns state.json and rewrites it from memory, so
	// only an idle orchestrator's state is written here. A live one adopts
	// the new reply from the spec when the marker below wakes it, and
	// persists it with its own state.
	res.Live = processRecordLive(filepath.Join(dir, "supervisor.json"))
	if !res.Live {
		st.Reply = to
		if e = WriteJSON(filepath.Join(dir, "state.json"), st); e != nil {
			unlock()
			return res, e
		}
	}
	stamp := noticeNow().UTC()
	if e = WriteJSON(filepath.Join(dir, adoptedFile), map[string]string{"from": res.From, "to": to, "at": stamp.Format(time.RFC3339Nano)}); e != nil {
		unlock()
		return res, e
	}
	if e = recordFormerOwner(name, res.From, to); e != nil {
		unlock()
		return res, e
	}
	unlock()
	spec.Reply = to
	if res.Redelivered, e = reconcileOrchestratorOwner(name, to); e != nil {
		return res, e
	}
	_ = orchestratorLifecycle(spec, st, "adopted: owner "+ownerLabel(res.From)+" -> "+to, "adopted:"+stamp.Format(time.RFC3339Nano))
	logSupervisor(name, fmt.Sprintf("%s: adopted from %s to %s\n", name, ownerLabel(res.From), to))
	if !res.Live {
		flushOrchestratorNotices(spec, orchestratorNoticeSender)
	}
	flushAttention()
	return res, nil
}

func ownerLabel(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func logSupervisor(name, line string) {
	f, e := os.OpenFile(filepath.Join(OrchestratorDir(name), "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

// formerOwnersFile lists every owner an orchestrator was adopted away from.
// It outlives each adopt, so a notice whose send held a lease while an adopt
// ran is still moved once that send fails, by whichever of adopt, the
// supervisor's refresh or the flusher reconciles next.
const formerOwnersFile = "former-owners.json"

// recordFormerOwner adds from to the former owners and drops to, which owns
// it again. Callers hold the root lock.
func recordFormerOwner(name, from, to string) error {
	prev := formerOwners(name)
	var next []string
	for _, o := range append(prev, from) {
		if o == "" || hostlabel.SameAddress(o, to) || slices.ContainsFunc(next, func(x string) bool { return hostlabel.SameAddress(x, o) }) {
			continue
		}
		next = append(next, o)
	}
	if slices.Equal(prev, next) {
		return nil
	}
	return WriteJSON(filepath.Join(OrchestratorDir(name), formerOwnersFile), next)
}

func formerOwners(name string) []string {
	var out []string
	_ = readJSON(filepath.Join(OrchestratorDir(name), formerOwnersFile), &out)
	return out
}

// reconcileOrchestratorOwner queues for to a copy of every notice still
// queued or failed for a former owner, and retires the original as stale. A
// notice mid-send keeps its lease and is moved by a later reconcile if that
// send fails; one that was delivered is never copied. The copy's receipt key
// derives from the original's ID, so repeating it is a no-op. Copies keep
// their creation time, so the new owner reads them in order, and a pull-route
// spec keeps them pullable: flush skips pull specs and
// PendingOrchestratorNotices lists any queued receipt.
func reconcileOrchestratorOwner(name, to string) (int, error) {
	var former []string
	for _, o := range formerOwners(name) {
		if !hostlabel.SameAddress(o, to) {
			former = append(former, o)
		}
	}
	if to == "" || len(former) == 0 {
		return 0, nil
	}
	isFormer := func(addr string) bool {
		return slices.ContainsFunc(former, func(o string) bool { return hostlabel.SameAddress(o, addr) })
	}
	all, e := loadOrchestratorNotices(name)
	if e != nil {
		return 0, e
	}
	moved := 0
	for _, f := range all {
		if (f.n.Status != "queued" && f.n.Status != "failed") || !isFormer(f.n.To) {
			continue
		}
		var orig OrchestratorNotice
		var claimed bool
		if orig, claimed, e = updateNotice(name, f.path, func(n *OrchestratorNotice) bool {
			if (n.Status != "queued" && n.Status != "failed") || n.Lease.After(noticeNow()) || !isFormer(n.To) {
				return false
			}
			n.Status, n.Lease = "stale", time.Time{}
			n.Error = noticeText(name, "adopted: re-delivered to "+to)
			return true
		}); e != nil {
			return moved, e
		}
		if !claimed {
			continue // Delivered, acknowledged, or mid-send since the walk.
		}
		sender, _, _ := strings.Cut(orig.ID, ":")
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(orig.ID+"\x00adopted\x00"+to)))
		path := filepath.Join(filepath.Dir(f.path), sum+".json")
		dup := OrchestratorNotice{ID: sender + ":" + orig.Kind + ":" + sum, To: to, Status: "queued", Message: orig.Message, Created: orig.Created, Kind: orig.Kind, Orchestrator: orig.Orchestrator, Attention: orig.Attention}
		if e = writeNoticeOnce(name, path, &dup); e != nil {
			return moved, e
		}
		moved++
	}
	return moved, nil
}

// reconcileLiveOwner reconciles toward the stored reply of a non-terminal
// orchestrator. A terminal one is left alone: a retire queues its handoff
// notice for a session that may once have owned it.
func reconcileLiveOwner(name string) int {
	if len(formerOwners(name)) == 0 {
		return 0
	}
	st, e := orchestratorState(name)
	if e != nil || orchestratorTerminal(name, st) != "" {
		return 0
	}
	cur, e := LoadOrchestrator(name)
	if e != nil {
		return 0
	}
	n, _ := reconcileOrchestratorOwner(name, cur.Reply)
	return n
}

func writeNoticeOnce(name, path string, n *OrchestratorNotice) error {
	unlock, e := lockNotices(name)
	if e != nil {
		return e
	}
	defer unlock()
	if exists(path) {
		return nil
	}
	if e = writeNotice(path, n); e != nil {
		return e
	}
	updateLastNotice(OrchestratorDir(name), n)
	return nil
}

// refreshOrchestratorOwner is the supervisor's side of adopt: it rereads the
// stored reply and moves notices still queued or failed for any former owner
// (including any a round queued while the adopt ran) to the current one. It
// reports whether the reply changed.
func refreshOrchestratorOwner(s *OrchestratorSpec, st *OrchestratorState) bool {
	marker := filepath.Join(OrchestratorDir(s.Name), adoptedFile)
	if exists(marker) {
		// Removed before the reread: an adopt that lands after it leaves a
		// new marker and wakes the supervisor again.
		_ = os.Remove(marker)
	}
	cur, e := LoadOrchestrator(s.Name)
	if e != nil {
		return false
	}
	changed := cur.Reply != s.Reply
	s.Reply = cur.Reply
	st.Reply = cur.Reply
	// Reconcile even when the owner is unchanged: a notice that was mid-send
	// at the last adopt may have failed since and still names the old owner.
	if len(formerOwners(s.Name)) > 0 && orchestratorTerminal(s.Name, *st) == "" {
		_, _ = reconcileOrchestratorOwner(s.Name, cur.Reply)
	}
	return changed
}

// AdoptRun re-points a dispatch run's reply and retries its last failed
// completion notification to the new owner.
func AdoptRun(name, to string) (AdoptResult, error) {
	res := AdoptResult{Name: name, To: to}
	if !validOwner(to) {
		return res, fmt.Errorf("adopt needs a peers address HOST/SESSION, got %q", to)
	}
	if !ValidName(name) {
		return res, fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), name)
	unlock, e := lockRoot()
	if e != nil {
		return res, e
	}
	var m Meta
	if e = readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
		unlock()
		return res, e
	}
	res.From = m.Reply
	m.Reply = to
	if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
		unlock()
		return res, e
	}
	var r Result
	hasResult := readJSON(filepath.Join(dir, "result.json"), &r) == nil
	unlock()
	live, _ := liveNames()
	res.Live = live[name]
	if !hasResult || r.NotifyError == "" || res.Live {
		return res, nil
	}
	cfg, e := Load(m.ConfigPath)
	if e != nil {
		res.RetryError = "target notification config: " + e.Error()
		return res, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res.Retried = true
	sendErr := Notify(ctx, cfg.Notify, m, r)
	unlock, e = lockRoot()
	if e != nil {
		return res, e
	}
	defer unlock()
	if e = readJSON(filepath.Join(dir, "result.json"), &r); e != nil {
		return res, e
	}
	if sendErr != nil {
		res.RetryError = sendErr.Error()
		r.NotifyError = sendErr.Error()
	} else {
		r.NotifyError = ""
	}
	return res, WriteJSON(filepath.Join(dir, "result.json"), r)
}
