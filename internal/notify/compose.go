package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Orchestrator events (RFC 0017 section 6). Every message is one fixed,
// code-owned template over run and orchestrator state.
const (
	EventStarted       = "started"
	EventRound         = "round"
	EventNeedsDecision = "needs-decision"
	EventResolved      = "resolved"
	EventBlocked       = "blocked"
	EventDone          = "done"
	EventFailed        = "failed"
	EventVerdict       = "verdict"
	EventRoundFailures = "round-failures"
	EventBudget        = "budget"
	EventHandoff       = "handoff"
	EventComplete      = "complete"
	EventStopped       = "stopped"
	EventPaused        = "paused"
	EventResumed       = "resumed"
	EventSourceCap     = "source-cap"
	EventIntake        = "intake"
)

// Events lists every event name in template order.
var Events = []string{EventStarted, EventRound, EventNeedsDecision, EventResolved, EventBlocked, EventDone, EventFailed, EventVerdict, EventRoundFailures, EventBudget, EventHandoff, EventComplete, EventStopped, EventPaused, EventResumed, EventSourceCap, EventIntake}

var defaultOff = []string{EventRound, EventPaused, EventResumed, EventSourceCap, EventIntake}

// ValidateEvents checks a spec's events list: names, "default" or "all".
func ValidateEvents(list []string) error {
	for _, e := range list {
		if e != "default" && e != "all" && !slices.Contains(Events, e) {
			return fmt.Errorf("unknown notify event %q", e)
		}
	}
	return nil
}

// Enabled reports whether event is selected by a spec's events list. An
// empty list is [default].
func Enabled(list []string, event string) bool {
	if len(list) == 0 {
		list = []string{"default"}
	}
	for _, e := range list {
		switch e {
		case "all":
			return true
		case "default":
			if !slices.Contains(defaultOff, event) {
				return true
			}
		case event:
			return true
		}
	}
	return false
}

// Event carries the state one template needs. Fields marked agent-written
// are cleaned, scrubbed and capped, and withheld on external notifiers.
type Event struct {
	Kind string
	// Orchestrator fields.
	Name, Host, Goal, Mode, Every, Until     string
	Round, Managed, Live, Decisions, Blocked int
	NextWake                                 time.Time
	Cap, To                                  string
	Rounds, Failures                         int
	Signal, IntakeEvent                      string
	// Managed-run fields.
	Run, Key, ResolvedBy, PR, State, Verdict string
	Exit                                     int
	Question                                 string // agent-written
	Reason                                   string // agent-written
}

// Message is a composed post plus its audit fields.
type Message struct {
	Text       string         `json:"text"`
	Redactions map[string]int `json:"redactions,omitempty"`
	Withheld   []string       `json:"withheld,omitempty"`
}

// Scrub removes secrets from text and returns replacement counts. Notify
// always has one, even when capture redaction is off.
type Scrub func(string) (string, map[string]int)

const (
	agentFieldLimit = 280
	goalLimit       = 200
	nameLimit       = 64
	// MessageLimit bounds every post, the same bound as member notices and
	// well below Slack's ~4,000-character text guidance
	// (https://docs.slack.dev/reference/methods/chat.postMessage).
	MessageLimit = 1000
)

var decisionKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

// ErrNotAvailable means the event has no external variant and is not posted.
var ErrNotAvailable = fmt.Errorf("event not available on an external notifier")

// Compose builds the message for e. It reads only the fields of e, so text
// an agent wrote anywhere else (reply.md, checkpoints, reports, logs, PR
// titles and bodies) cannot reach it.
func Compose(e Event, external bool, scrub Scrub) (Message, error) {
	m := Message{Redactions: map[string]int{}}
	clean := func(text string, limit int) string {
		text = Clean(text)
		if scrub != nil {
			var counts map[string]int
			text, counts = scrub(text)
			for k, v := range counts {
				m.Redactions[k] += v
			}
		}
		return capRunes(text, limit, "…")
	}
	name := capRunes(Clean(e.Name), nameLimit, "")
	run := capRunes(Clean(e.Run), nameLimit, "")
	key := e.Key
	if !decisionKey.MatchString(key) {
		key = "unknown"
	}
	pr := ""
	if u, err := url.Parse(strings.TrimSpace(e.PR)); err == nil && u.Scheme == "https" && u.Host != "" && !strings.ContainsAny(e.PR, " <>\n") {
		pr = " PR: " + u.String()
	}
	withhold := func(field string) { m.Withheld = append(m.Withheld, field) }
	var text string
	switch e.Kind {
	case EventStarted:
		host := ""
		if external {
			if e.Host != "" {
				withhold("host")
			}
		} else if e.Host != "" {
			host = " on " + capRunes(Clean(e.Host), nameLimit, "")
		}
		cadence := e.Mode
		if e.Every != "" {
			cadence += " every " + e.Every
		}
		if e.Until != "" {
			cadence += " until " + e.Until
		}
		text = fmt.Sprintf("Orchestrator %s started%s. Goal: %s %s. Details: hev orchestrate show %s", name, host, sentence(clean(e.Goal, goalLimit)), cadence, name)
	case EventRound:
		text = fmt.Sprintf("Round %d of %s: %d runs (%d live, %d awaiting a decision, %d blocked).", e.Round, name, e.Managed, e.Live, e.Decisions, e.Blocked)
		if !e.NextWake.IsZero() {
			text += " Next wake " + e.NextWake.UTC().Format("2006-01-02 15:04 UTC") + "."
		}
	case EventNeedsDecision:
		if external {
			if e.Question != "" {
				withhold("question")
			}
			text = fmt.Sprintf("%s needs a decision (%s). The question is withheld from this channel; see hev dispatch show %s.", run, key, run)
		} else {
			text = fmt.Sprintf("%s needs a decision (%s): %s Answer from a shell: hev dispatch say %s --resolve %s 'ANSWER'", run, key, sentence(clean(e.Question, agentFieldLimit)), run, key)
		}
	case EventResolved:
		by := ""
		if external {
			if e.ResolvedBy != "" {
				withhold("resolved_by")
			}
		} else if e.ResolvedBy != "" {
			by = " by " + clean(e.ResolvedBy, nameLimit)
		}
		text = fmt.Sprintf("%s: decision %s answered%s; rework started.", run, key, by)
	case EventBlocked:
		if external {
			if e.Reason != "" {
				withhold("reason")
			}
			text = fmt.Sprintf("%s is blocked. The reason is withheld from this channel; see hev dispatch show %s.", run, run)
		} else {
			text = fmt.Sprintf("%s is blocked: %s See hev dispatch show %s.", run, sentence(clean(e.Reason, agentFieldLimit)), run)
		}
	case EventDone:
		text = fmt.Sprintf("%s done.%s", run, pr)
	case EventFailed:
		state := e.State
		if state != "timeout" {
			state = "failed"
		}
		text = fmt.Sprintf("%s %s (exit %d).", run, state, e.Exit)
	case EventVerdict:
		if e.Verdict != "pass" && e.Verdict != "fail" {
			return m, fmt.Errorf("verdict must be pass or fail")
		}
		text = fmt.Sprintf("%s validation %s.%s", run, e.Verdict, pr)
	case EventRoundFailures:
		text = fmt.Sprintf("%s: %d consecutive rounds failed or timed out. See hev orchestrate show %s.", name, e.Failures, name)
	case EventBudget:
		text = fmt.Sprintf("%s stopped admitting runs: %s reached.", name, Clean(e.Cap))
	case EventHandoff:
		if external {
			if e.To != "" {
				withhold("to")
			}
			text = fmt.Sprintf("%s handed off; this supervisor has stopped.", name)
		} else {
			text = fmt.Sprintf("%s handed off to %s; this supervisor has stopped.", name, clean(e.To, nameLimit))
		}
	case EventComplete:
		text = fmt.Sprintf("%s met its goal after %d rounds.", name, e.Rounds)
	case EventStopped:
		text = fmt.Sprintf("%s stopped.", name)
	case EventPaused:
		text = fmt.Sprintf("%s paused.", name)
	case EventResumed:
		text = fmt.Sprintf("%s resumed.", name)
	case EventSourceCap, EventIntake:
		if external {
			return m, ErrNotAvailable
		}
		text = fmt.Sprintf("%s: intake %s", name, Clean(e.Signal))
		if e.IntakeEvent != "" {
			text += " for event " + clean(e.IntakeEvent, nameLimit)
		}
		text += "."
	default:
		return m, fmt.Errorf("unknown notify event %q", e.Kind)
	}
	if len(m.Redactions) == 0 {
		m.Redactions = nil
	}
	m.Text = capRunes(text, MessageLimit-1, "…")
	return m, nil
}

// TestText is the fixed message for a test post (RFC 0017 section 7).
func TestText(notifier string, at time.Time) string {
	return fmt.Sprintf("kit test from notifier %s at %s.", notifier, at.UTC().Format(time.RFC3339))
}

// Clean keeps printable runes and collapses whitespace, the LocalMemberNotice
// treatment (internal/dispatch/member_notice.go).
func Clean(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// sentence ends a field with one full stop unless it already ends a sentence.
func sentence(text string) string {
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "…") {
		return text
	}
	return text + "."
}

func capRunes(text string, limit int, suffix string) string {
	r := []rune(text)
	if len(r) <= limit {
		return text
	}
	return string(r[:limit]) + suffix
}
