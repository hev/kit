// Package attention is kit's one "needs you" notice (docs/notify.md): a typed
// record of a run, orchestrator, swarm or steer moment. needs-you
// is only an open decision or a halt escalated to a hard stop, keyed by its
// subject; the opt-in ready and done kinds carry everything else. A
// notice is emitted on the host where it happens. Emitting runs the user's
// hev-hook-NAME handlers as a run-lifecycle event and, when [notify.slack] is
// configured there, posts it to Slack. Neither ever fails the caller.
package attention

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/redact"
)

// Schema names the notice and hook event version
// (schemas/attention_event.schema.json).
const Schema = "hev.attention.event/v1"

// Kinds. needs-you is the default Slack kind; ready and done are opt-in.
const (
	KindNeedsYou = "needs-you"
	KindReady    = "ready"
	KindDone     = "done"
)

// Kinds lists every notice kind.
var Kinds = []string{KindNeedsYou, KindReady, KindDone}

// ValidKind reports whether k is a notice kind.
func ValidKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// Sources say what raised a notice.
const (
	SourceRun          = "run"
	SourceOrchestrator = "orchestrator"
	SourceFollow       = "follow"
	SourceSwarm        = "swarm"
	SourceSteer        = "steer"
	SourceJob          = "job"
)

// SourceLegacyExpedite is what edge builds before the orchestrator rename
// wrote for a follow-through orchestrator's notices. Receipts and outbox
// records may still carry it; kit no longer writes it.
const SourceLegacyExpedite = "expedite"

// idSource is the source string a notice ID is derived from. Follow-through
// IDs keep the pre-rename string so an unchanged subject that an older build
// already raised is not raised again after an upgrade.
func idSource(source string) string {
	if source == SourceFollow {
		return SourceLegacyExpedite
	}
	return source
}

// Links point at where to act. Each is optional.
type Links struct {
	PR    string `json:"pr,omitempty"`
	Run   string `json:"run,omitempty"`
	Board string `json:"board,omitempty"`
}

// Notice is one attention notice, and the run-lifecycle hook event handlers
// receive on stdin.
type Notice struct {
	Schema string    `json:"schema"`
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	// Event is the kind: needs-you, ready or done.
	Event  string `json:"event"`
	Source string `json:"source"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	// State is the run or orchestrator state the notice is about.
	State string `json:"state,omitempty"`
	// Text is one line, already redacted and bounded.
	Text  string `json:"text"`
	Links Links  `json:"links,omitempty"`
	// Thread groups notices about one run or orchestrator into one Slack
	// thread.
	Thread string `json:"thread"`
	// Client marks a notice from a client host: delivery names only the run
	// or orchestrator and its state, never text or links into client repos.
	Client bool `json:"client,omitempty"`
	// RequestedBy is who the run or orchestrator works for
	// (see Requester). Reworks, validators and reviews, and runs an
	// orchestrator or a run dispatches, inherit it.
	RequestedBy *Person `json:"requested_by,omitempty"`
	// Addressee lists the persons or roles the notice asks; it defaults to
	// [requested_by.person]. Delivery does not depend on it.
	Addressee []string `json:"addressee,omitempty"`
}

// textLimit bounds Text before the message is assembled.
const textLimit = 240

// personLimit bounds a requested_by or addressee entry; addresseeLimit
// bounds the addressee list.
const (
	personLimit    = 120
	addresseeLimit = 16
)

// MessageLimit is the hard ceiling on a delivered message, in characters.
const MessageLimit = 399

// NewID derives a stable notice ID from what the notice is about, never from
// when it was emitted, so a retry or a restart that re-emits the same moment
// yields the same ID.
func NewID(source, host, name, kind, key string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{source, host, name, kind, key}, "\x00")))
	return "att_" + hex.EncodeToString(sum[:12])
}

// threadKey groups a run with its steers, and an orchestrator with its
// follow-through or swarm flavour, by host and name.
func threadKey(source, host, name string) string {
	switch source {
	case SourceJob:
		return "job:" + host + "/" + name
	case SourceRun, SourceSteer:
		return "run:" + host + "/" + name
	}
	return "orchestrator:" + host + "/" + name
}

// New builds a notice. key identifies the moment within name (for example
// the run attempt and its end); it never appears in the notice.
func New(kind, source, host, name, key, state, text string, links Links) Notice {
	n := Notice{
		Schema: Schema,
		ID:     NewID(idSource(source), host, name, kind, key),
		Time:   time.Now().UTC(),
		Event:  kind,
		Source: source,
		Name:   name,
		Host:   host,
		State:  state,
		Text:   text,
		Links:  links,
		Thread: threadKey(source, host, name),
	}
	return n.Clean()
}

// Clean redacts and bounds the free text and fills derived fields. It is
// idempotent, and is applied again to notices pulled from another host.
func (n Notice) Clean() Notice {
	if n.Schema == "" {
		n.Schema = Schema
	}
	if n.Thread == "" {
		n.Thread = threadKey(n.Source, n.Host, n.Name)
	}
	n.Text = OneLine(n.Text, textLimit)
	if n.RequestedBy != nil {
		p := Person{Person: OneLine(n.RequestedBy.Person, personLimit), Name: OneLine(n.RequestedBy.Name, personLimit)}
		n.RequestedBy = &p
	}
	if len(n.Addressee) > 0 {
		var to []string
		for _, a := range n.Addressee {
			if a = OneLine(a, personLimit); a != "" && len(to) < addresseeLimit {
				to = append(to, a)
			}
		}
		n.Addressee = to
	}
	if n.Client {
		// A client host may name only the run or orchestrator and state.
		n.Text, n.Links.PR, n.Links.Board = "", "", ""
	}
	return n
}

// prefixes are the first-line tags per kind.
var prefixes = map[string]string{KindNeedsYou: "[needs you]", KindReady: "[ready]", KindDone: "[done]"}

// Message renders the delivered text: first line
// "[needs you] NAME on HOST: TEXT", then one line per link, under
// MessageLimit characters. tag (for example "[test]") is prepended when set.
func (n Notice) Message(tag string) string {
	n = n.Clean()
	head := prefixes[n.Event]
	if head == "" {
		head = "[" + n.Event + "]"
	}
	if tag = strings.TrimSpace(tag); tag != "" {
		head = tag + " " + head
	}
	text := n.Text
	if n.Client || text == "" {
		text = n.State
	}
	if text == "" {
		text = n.Event
	}
	var links []string
	if n.Links.PR != "" {
		links = append(links, "PR: "+OneLine(n.Links.PR, 200))
	}
	if n.Links.Run != "" {
		links = append(links, "Run: "+OneLine(n.Links.Run, 160))
	}
	if n.Links.Board != "" {
		links = append(links, "Board: "+OneLine(n.Links.Board, 200))
	}
	first := head + " " + n.Name + " on " + n.Host + ": "
	// Drop links that would not fit beside a minimal first line.
	for len(links) > 0 && runeLen(first)+20+runeLen("\n"+strings.Join(links, "\n")) > MessageLimit {
		links = links[:len(links)-1]
	}
	tail := ""
	if len(links) > 0 {
		tail = "\n" + strings.Join(links, "\n")
	}
	room := MessageLimit - runeLen(first) - runeLen(tail)
	if room < 1 {
		room = 1
	}
	msg := first + truncate(text, room) + tail
	return truncate(msg, MessageLimit)
}

var (
	scrubOnce sync.Once
	scrubber  *redact.Scrubber
)

// OneLine collapses whitespace, redacts credentials and bounds s to limit
// characters.
func OneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	scrubOnce.Do(func() { scrubber, _ = redact.New([]byte("hev-attention-notice-redaction-salt-v1")) })
	if scrubber == nil {
		// Never deliver text that could not be scrubbed.
		return ""
	}
	if s != "" {
		s, _ = scrubber.Text(s)
	}
	return truncate(s, limit)
}

func runeLen(s string) int { return len([]rune(s)) }

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	if limit <= 1 {
		return string(r[:limit])
	}
	return string(r[:limit-1]) + "…"
}
