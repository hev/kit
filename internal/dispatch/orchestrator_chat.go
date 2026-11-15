package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	chat "github.com/hev/kit/internal/notify"
	"github.com/hev/kit/internal/redact"
)

// Chat routes (RFC 0017): an orchestrator spec maps one node-local notifier
// and channel label. kit composes every post from state, queues it as a
// receipt on the owner notice queue with To notify:NOTIFIER/LABEL, and the
// supervisor host posts it. Nothing here writes run or orchestrator state.

const chatPrefix = "notify:"

// OrchestratorNotify is the spec's notify: mapping.
type OrchestratorNotify struct {
	Notifier string   `yaml:"notifier,omitempty" json:"notifier,omitempty"`
	Channel  string   `yaml:"channel" json:"channel"`
	Events   []string `yaml:"events,omitempty" json:"events,omitempty"`
}

func (n *OrchestratorNotify) validate() error {
	if n == nil {
		return nil
	}
	if !ValidLabel(n.Channel) {
		return fmt.Errorf("notify.channel must be a channel label from this node's notify.toml")
	}
	if n.Notifier != "" && !ValidLabel(n.Notifier) {
		return fmt.Errorf("notify.notifier must be a notifier name")
	}
	return chat.ValidateEvents(n.Events)
}

func isChatRecipient(to string) bool { return strings.HasPrefix(to, chatPrefix) }

// Seams for tests: the node config, the secret source, the scrubber, the
// Slack endpoint and the sender. Production reads this node's notify.toml,
// resolves through hev secrets and posts to slack.com.
var (
	notifyLoadConfig                  = chat.Load
	notifyResolveSecret chat.Resolver = chat.ResolveSecret
	notifyScrubber                    = loadNotifyScrubber
	notifySlackBase                   = chat.SlackAPI
	chatNoticeSender                  = sendChatNotice
	notifyApplier                     = LocalSender
)

// chatCredentialExpiry bounds how long a credential failure keeps a receipt
// queued (RFC 0017 open question 5, decided: 24 hours).
const chatCredentialExpiry = 24 * time.Hour

// checkOrchestratorNotify is apply's fail-early check: the notifier exists
// on this node, the label is on its list, and its repos cover manages.repos.
func checkOrchestratorNotify(s OrchestratorSpec) error {
	if s.Notify == nil {
		return nil
	}
	cfg, e := notifyLoadConfig()
	if e != nil {
		return fmt.Errorf("notify: %w", e)
	}
	n, e := cfg.Lookup(s.Notify.Notifier)
	if e != nil {
		return fmt.Errorf("notify: %w", e)
	}
	if _, ok := n.Channels[s.Notify.Channel]; !ok {
		return fmt.Errorf("notify: channel %s not allowed for notifier %s", s.Notify.Channel, n.Name)
	}
	for _, repo := range s.Manages.Repos {
		if !n.AllowsRepo(repo) {
			return fmt.Errorf("notify: notifier %s does not allow repo %s", n.Name, repo)
		}
	}
	return nil
}

func loadNotifyScrubber() (chat.Scrub, error) {
	// Scrub even when [capture] redact = false: redact.Load returns nil then.
	s, e := redact.LoadAlways()
	if e != nil {
		return nil, e
	}
	return func(text string) (string, map[string]int) {
		out, counts := s.Text(text)
		return out, map[string]int(counts)
	}, nil
}

// chatRoute resolves the recipient and notifier at queue time. A non-empty
// failure means the receipt is written failed and no request is ever made.
func chatRoute(s OrchestratorSpec, repo string) (to string, n chat.Notifier, failure string) {
	cfg, e := notifyLoadConfig()
	name := s.Notify.Notifier
	if name == "" && e == nil {
		name = cfg.Default
	}
	if name == "" {
		name = "default"
	}
	to = chatPrefix + name + "/" + s.Notify.Channel
	if e != nil {
		return to, n, "notify: " + e.Error()
	}
	if n, e = cfg.Lookup(s.Notify.Notifier); e != nil {
		return to, n, e.Error()
	}
	if _, ok := n.Channels[s.Notify.Channel]; !ok {
		return to, n, "channel " + s.Notify.Channel + " not allowed"
	}
	if repo != "" && !n.AllowsRepo(repo) {
		return to, n, "repo not allowed"
	}
	return to, n, ""
}

// queueChatEvent composes one event from state and queues it. key is stable
// for the event, so a repeat observation never posts twice. It never sends.
func queueChatEvent(s OrchestratorSpec, st OrchestratorState, key string, ev chat.Event, repo string) error {
	if s.Notify == nil || !chat.Enabled(s.Notify.Events, ev.Kind) {
		return nil
	}
	to, n, failure := chatRoute(s, repo)
	// Unknown audience is treated as external: withhold agent text.
	external := failure != "" || n.External()
	scrub, e := notifyScrubber()
	if e != nil && failure == "" {
		failure, external = "redaction unavailable", true
	}
	msg, e := chat.Compose(ev, external, scrub)
	if errors.Is(e, chat.ErrNotAvailable) {
		return nil
	}
	if e != nil {
		return e
	}
	requestedBy := st.AppliedBy
	if requestedBy == "" {
		requestedBy = notifyApplier()
	}
	return queueNoticeReceipt(s, "notify", key, to, func(r *OrchestratorNotice) {
		r.Message, r.Redactions, r.Withheld = msg.Text, redactionCounts(msg.Redactions), strings.Join(msg.Withheld, ",")
		r.Notifier, _, _ = strings.Cut(strings.TrimPrefix(to, chatPrefix), "/")
		r.Channel, r.NotifierKind, r.Event = s.Notify.Channel, n.Kind, ev.Kind
		r.RequestedBy, r.Lifetime = requestedBy, st.Start.UTC()
		if failure != "" {
			r.Status, r.Error, r.ErrorClass = "failed", noticeText(s.Name, failure), string(chat.ClassConfig)
		}
	})
}

func redactionCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return strings.Join(parts, ",")
}

// chatLifecycle queues an orchestrator lifecycle event, keyed like the peers
// lifecycle notice: once per lifetime start and key.
func chatLifecycle(s OrchestratorSpec, st OrchestratorState, key string, ev chat.Event) {
	if s.Notify == nil {
		return
	}
	ev.Name = s.Name
	_ = queueChatEvent(s, st, st.Start.Format(time.RFC3339Nano)+":"+key, ev, "")
}

func chatStarted(s OrchestratorSpec, st OrchestratorState) {
	chatLifecycle(s, st, "applied", chat.Event{Kind: chat.EventStarted, Host: noticeHost(s), Goal: s.Goal, Mode: s.Mode, Every: s.Every, Until: s.Until})
}

// chatRunEvents queues managed-run events from gathered evidence. Keys name
// the run, its start and the event, so each state is posted once per run
// attempt, whichever round first observes it.
func chatRunEvents(s OrchestratorSpec, st OrchestratorState, evidence []OrchestratorEvidence) {
	if s.Notify == nil {
		return
	}
	for _, ev := range evidence {
		row := ev.Row
		if row.Kind == "orch" || !ValidName(row.Name) {
			continue
		}
		base := row.Name + ":" + row.Start.UTC().Format(time.RFC3339Nano) + ":"
		repo := row.Repo
		resolved := map[string]Decision{}
		for _, d := range ev.Decisions {
			if !d.Resolved.IsZero() && ValidDecisionKey(d.Key) {
				resolved[d.Key] = d
			}
		}
		queue := func(key string, e chat.Event) {
			e.Run = row.Name
			_ = queueChatEvent(s, st, key, e, repo)
		}
		switch {
		case row.Outcome == OutcomeNeedsDecision && row.State == "needs-review":
			if _, done := resolved[row.Key]; !done {
				queue(base+"needs-decision:"+row.Key, chat.Event{Kind: chat.EventNeedsDecision, Key: row.Key, Question: row.Question})
			}
		case row.State == "blocked":
			queue(base+"blocked", chat.Event{Kind: chat.EventBlocked, Reason: row.Reason})
		case row.State == "done":
			queue(base+"done", chat.Event{Kind: chat.EventDone, PR: row.PR})
		case row.State == "failed" || row.State == "timeout":
			var r struct {
				Exit int `json:"exit_code"`
			}
			_ = json.Unmarshal([]byte(ev.Files["result.json"]), &r)
			queue(base+row.State, chat.Event{Kind: chat.EventFailed, State: row.State, Exit: r.Exit})
		}
		for key, d := range resolved {
			// The resolution, not the run attempt, keys this event.
			queue(row.Name+":resolved:"+key+":"+d.Resolved.UTC().Format(time.RFC3339Nano), chat.Event{Kind: chat.EventResolved, Key: key, ResolvedBy: d.ResolvedBy})
		}
		if row.Validation == "pass" || row.Validation == "fail" {
			queue(base+"verdict", chat.Event{Kind: chat.EventVerdict, Verdict: row.Validation, PR: row.PR})
		}
	}
}

// roundCounts summarises evidence for the round event.
func roundCounts(evidence []OrchestratorEvidence) (live, decisions, blocked int) {
	for _, ev := range evidence {
		switch {
		case ev.Row.State == "running" || ev.Row.State == "queued":
			live++
		case ev.Row.State == "needs-review" && ev.Row.Outcome == OutcomeNeedsDecision:
			decisions++
		case ev.Row.State == "blocked":
			blocked++
		}
	}
	return
}

func chatRound(s OrchestratorSpec, st OrchestratorState) {
	if s.Notify == nil {
		return
	}
	c := st.chatRound
	chatLifecycle(s, st, fmt.Sprintf("round:%d", st.Rounds), chat.Event{Kind: chat.EventRound, Round: st.Rounds, Managed: st.ManagedRuns, Live: c.live, Decisions: c.decisions, Blocked: c.blocked, NextWake: st.Next})
}

// chatPosted is what a successful post returns to the receipt.
type chatPosted struct {
	Ref, Thread, Message string
}

// chatExpired reports a credential failure older than the expiry.
func chatExpired(n OrchestratorNotice, now time.Time) bool {
	return n.ErrorClass == string(chat.ClassCredential) && !now.Before(n.Created.Add(chatCredentialExpiry))
}

// applyChatResult maps a post result onto its receipt (RFC 0017 section 3).
func applyChatResult(name string, n *OrchestratorNotice, posted chatPosted, err error) {
	now := noticeNow()
	if err == nil {
		n.Status, n.Error, n.ErrorClass, n.NextAttempt = "delivered", "", "", time.Time{}
		n.Ref, n.Thread = posted.Ref, posted.Thread
		if posted.Message != "" {
			n.Message = posted.Message
		}
		return
	}
	class := chat.Classify(err)
	n.Error, n.ErrorClass = noticeText(name, err.Error()), string(class)
	switch class {
	case chat.ClassChannel, chat.ClassPayload, chat.ClassConfig, chat.ClassThread:
		n.Status, n.NextAttempt = "failed", time.Time{}
	case chat.ClassRateLimited:
		// The later of the backoff and the server's retry time. Rate limits
		// never count toward the credential expiry.
		n.Attempts++
		next := now.Add(noticeBackoff(n.Attempts))
		if at := chat.RetryAt(err); at.After(next) {
			next = at
		}
		n.NextAttempt = next.UTC()
	case chat.ClassCredential:
		n.Attempts++
		if !now.Before(n.Created.Add(chatCredentialExpiry)) {
			n.Status, n.NextAttempt = "failed", time.Time{}
			n.Error = noticeText(name, "expired after 24h: "+err.Error())
			return
		}
		n.NextAttempt = now.Add(noticeBackoff(n.Attempts)).UTC()
	default:
		n.Attempts++
		n.NextAttempt = now.Add(noticeBackoff(n.Attempts)).UTC()
	}
}

// Fresh secret reads after a credential failure, at most once an hour per
// notifier, so a rotated token is picked up without hammering the provider.
var (
	freshReadsMu sync.Mutex
	freshReads   = map[string]time.Time{}
)

func wantFreshRead(n OrchestratorNotice) bool {
	if n.ErrorClass != string(chat.ClassCredential) {
		return false
	}
	freshReadsMu.Lock()
	defer freshReadsMu.Unlock()
	now := noticeNow()
	if last, ok := freshReads[n.Notifier]; ok && now.Before(last.Add(time.Hour)) {
		return false
	}
	freshReads[n.Notifier] = now
	return true
}

// threadKey names one notifier/label thread for one orchestrator lifetime.
func threadKey(n OrchestratorNotice) string {
	return strings.TrimPrefix(n.To, chatPrefix) + "@" + n.Lifetime.UTC().Format(time.RFC3339Nano)
}

type chatThread struct {
	Thread string    `json:"thread"`
	Ref    string    `json:"ref"`
	Since  time.Time `json:"since"`
}

func threadsPath(name string) string {
	// Directly in notices/, which loadOrchestratorNotices skips.
	return filepath.Join(OrchestratorDir(name), "notices", "threads.json")
}

func readThreads(name string) (map[string]chatThread, error) {
	out := map[string]chatThread{}
	b, e := os.ReadFile(threadsPath(name))
	if e != nil {
		return out, e
	}
	if e = json.Unmarshal(b, &out); e != nil {
		return map[string]chatThread{}, e
	}
	return out, nil
}

func saveThread(name, key string, t chatThread) error {
	unlock, e := lockNotices(name)
	if e != nil {
		return e
	}
	defer unlock()
	all, _ := readThreads(name)
	all[key] = t
	if e = os.MkdirAll(filepath.Dir(threadsPath(name)), 0700); e != nil {
		return e
	}
	return writeDurableJSON(threadsPath(name), all)
}

// lookupThread returns the anchor for n, and whether one should exist: a
// receipt for the same route and lifetime was already delivered.
func lookupThread(name string, n OrchestratorNotice) (thread string, lost bool) {
	unlock, e := lockNotices(name)
	if e == nil {
		threads, _ := readThreads(name)
		unlock()
		if t, ok := threads[threadKey(n)]; ok && t.Thread != "" {
			return t.Thread, false
		}
	}
	all, _ := loadOrchestratorNotices(name)
	for _, f := range all {
		if f.n.ID != n.ID && f.n.To == n.To && f.n.Lifetime.Equal(n.Lifetime) && f.n.Status == "delivered" {
			return "", true
		}
	}
	return "", false
}

const newThreadPrefix = "(new thread) "

// sendChatNotice posts one chat receipt. It reads only the node-local
// notify.toml, resolves the credential at send time, and never writes run
// or orchestrator state; only threads.json is updated.
func sendChatNotice(ctx context.Context, s OrchestratorSpec, n OrchestratorNotice) (chatPosted, error) {
	cfg, e := notifyLoadConfig()
	if e != nil {
		return chatPosted{}, chat.LocalError(chat.ClassConfig, "notify: "+e.Error())
	}
	notifier, e := cfg.Lookup(n.Notifier)
	if e != nil {
		return chatPosted{}, chat.LocalError(chat.ClassConfig, e.Error())
	}
	channel, ok := notifier.Channels[n.Channel]
	if !ok {
		return chatPosted{}, chat.LocalError(chat.ClassConfig, "channel "+n.Channel+" not allowed")
	}
	if notifier.Kind != chat.KindSlack {
		return chatPosted{}, chat.LocalError(chat.ClassConfig, "notifier "+notifier.Name+" kind not supported")
	}
	token, e := notifyResolveSecret(ctx, notifier, wantFreshRead(n))
	if e != nil {
		return chatPosted{}, e
	}
	client := chat.Slack{BaseURL: notifySlackBase, Token: token, Now: noticeNow}
	thread, lost := lookupThread(s.Name, n)
	text := n.Message
	if lost {
		text = newThreadPrefix + text
	}
	ref, anchor, e := client.Post(ctx, channel, text, thread)
	if chat.Classify(e) == chat.ClassThread {
		// The platform refused the anchor: start a new thread rather than
		// guess one from channel history (that would be a read).
		text = newThreadPrefix + n.Message
		ref, anchor, e = client.Post(ctx, channel, text, "")
		thread = ""
	}
	if e != nil {
		return chatPosted{}, e
	}
	if thread == "" {
		_ = saveThread(s.Name, threadKey(n), chatThread{Thread: anchor, Ref: ref, Since: noticeNow().UTC()})
	}
	posted := chatPosted{Ref: ref, Thread: anchor}
	if text != n.Message {
		posted.Message = text
	}
	return posted, nil
}

// appendNotifyAudit appends one line per final chat receipt to
// ~/dispatch/.notify/audit.jsonl: no message text, only its SHA-256.
func appendNotifyAudit(n OrchestratorNotice) {
	if !isChatRecipient(n.To) {
		return
	}
	line, e := json.Marshal(map[string]any{
		"id": n.ID, "time": noticeNow().UTC(), "notifier": n.Notifier, "channel": n.Channel,
		"event": n.Event, "status": n.Status, "ref": n.Ref,
		"message_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(n.Message))),
	})
	if e != nil {
		return
	}
	dir := filepath.Join(Root(), ".notify")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
}
