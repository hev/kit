package attention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/platformops"
)

// Delivery states of a Slack post.
const (
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	// StatusUncertain is a post that reached Slack without kit learning the
	// answer: its process stopped mid-send, or the connection dropped after
	// the request was written. Slack may have posted it, so kit never posts
	// it again on its own (at most once); hev s and show name it.
	StatusUncertain = "uncertain"
)

// uncertainCrash explains a send interrupted by its process stopping.
const uncertainCrash = "a post was in flight when its process stopped; Slack may have it, so it is not reposted"

// MaxAttempts bounds retries: with backoff capped at an hour, about a day.
const MaxAttempts = 30

const maxBackoff = time.Hour

// Delivery is one notice's Slack post.
type Delivery struct {
	Status      string    `json:"status"`
	Channel     string    `json:"channel"`
	TS          string    `json:"ts,omitempty"`
	Attempts    int       `json:"attempts,omitempty"`
	Error       string    `json:"error,omitempty"`
	NextAttempt time.Time `json:"next_attempt,omitzero"`
	// Lease marks a post in progress so a concurrent flush does not repeat it.
	Lease time.Time `json:"lease,omitzero"`
	// Sending is set, durably, just before the request goes to Slack and
	// cleared with its answer. Found set under an expired lease, it means
	// the sender stopped mid-send: the outcome is unknown and the post
	// becomes uncertain rather than being repeated.
	Sending   time.Time `json:"sending,omitzero"`
	Delivered time.Time `json:"delivered,omitzero"`
}

// Record is a notice's durable outbox entry, keyed by notice ID. Its
// existence is what makes a re-emitted notice a no-op.
type Record struct {
	Notice   Notice    `json:"notice"`
	StateDir string    `json:"state_dir,omitempty"`
	Created  time.Time `json:"created"`
	// Hooks is when handlers ran; they run at most once per notice.
	Hooks time.Time `json:"hooks,omitzero"`
	// Slack is nil when this host has no [notify.slack] or its kinds do not
	// include the notice's; SlackSkip then says which.
	Slack     *Delivery `json:"slack,omitempty"`
	SlackSkip string    `json:"slack_skip,omitempty"`
}

// RootEnv overrides the outbox directory.
const RootEnv = "HEV_ATTENTION_DIR"

// Root is the outbox directory: HEV_ATTENTION_DIR or ~/.hev/attention.
func Root() string {
	if d := os.Getenv(RootEnv); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "hev-attention")
	}
	return filepath.Join(home, ".hev", "attention")
}

// Emitter emits notices on this host.
type Emitter struct {
	Root      string
	Config    Config
	ConfigErr error
	Post      Poster
	Token     func(ctx context.Context, ref string) (string, error)
	// Hooks runs run-lifecycle handlers; nil uses hev-hook-NAME on PATH.
	Hooks func(ctx context.Context, event string, payload []byte)
	Now   func() time.Time
}

// Default is an emitter for this host's config.
func Default() *Emitter {
	if testing.Testing() && os.Getenv(RootEnv) == "" {
		// A test binary never reaches the operator's Slack or outbox unless
		// it set up its own outbox.
		return &Emitter{Root: filepath.Join(os.TempDir(), "hev-attention-test-"+fmt.Sprint(os.Getpid())), Hooks: func(context.Context, string, []byte) {}}
	}
	cfg, err := LoadConfig(ConfigPath())
	return &Emitter{Root: Root(), Config: cfg, ConfigErr: err}
}

func (e *Emitter) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Emitter) poster() Poster {
	if e.Post != nil {
		return e.Post
	}
	return DefaultPoster()
}

func (e *Emitter) token(ctx context.Context, ref string) (string, error) {
	if e.Token != nil {
		return e.Token(ctx, ref)
	}
	return ResolveToken(ctx, ref)
}

func (e *Emitter) runHooks(ctx context.Context, n Notice) {
	b, err := json.Marshal(n)
	if err != nil {
		return
	}
	if e.Hooks != nil {
		e.Hooks(ctx, n.Event, b)
		return
	}
	cfg, _ := hooks.LoadConfig(ConfigPath())
	hooks.RunLifecycle(ctx, n.Event, b, os.Getenv("PATH"), cfg, hooks.DefaultLog())
}

func (e *Emitter) recordPath(id string) string {
	return filepath.Join(e.Root, "outbox", safeName(id)+".json")
}

func safeName(id string) string {
	ok := id != "" && len(id) <= 80
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			ok = false
		}
	}
	if ok {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "h_" + hex.EncodeToString(sum[:16])
}

func (e *Emitter) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Join(e.Root, "outbox"), 0700); err != nil {
		return nil, err
	}
	return platformops.LockPath(filepath.Join(e.Root, "outbox.lock"), false)
}

// Emit records n, runs hook handlers once and posts it to Slack when this host
// is configured for its kind. A notice whose ID was already emitted (a retry,
// a restart, a redelivered pull) is not handled again; a still-queued post of
// it is retried if due. stateDir is the run or orchestrator directory the
// notice is about ("" for none): its Slack thread and delivery status are
// kept there. Emit never panics, and its error is for logging only.
func (e *Emitter) Emit(ctx context.Context, n Notice, stateDir string) error {
	return e.emit(ctx, n, stateDir, true)
}

// Queue is Emit without the post: it records the notice and runs hooks, and
// a later Flush posts it. Callers holding locks use it.
func (e *Emitter) Queue(ctx context.Context, n Notice, stateDir string) error {
	return e.emit(ctx, n, stateDir, false)
}

func (e *Emitter) emit(ctx context.Context, n Notice, stateDir string, post bool) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("attention: %v", p)
		}
	}()
	n = n.Clean()
	if e.Config.Client {
		n.Client = true
		n = n.Clean()
	}
	if n.ID == "" || !ValidKind(n.Event) {
		return fmt.Errorf("attention: notice needs an ID and a kind")
	}
	path := e.recordPath(n.ID)
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		unlock()
		if !post {
			return nil
		}
		return e.deliver(ctx, path)
	}
	// Every recorded notice says who it is for, and so does its hook event.
	n = n.attributed()
	rec := Record{Notice: n, StateDir: stateDir, Created: e.now(), Hooks: e.now()}
	switch s := e.Config.Slack; {
	case s == nil:
		rec.SlackSkip = "not configured"
	case !s.Wants(n.Event):
		rec.SlackSkip = "kind not in [notify.slack] kinds"
	default:
		rec.Slack = &Delivery{Status: StatusQueued, Channel: s.Channel}
	}
	err = writeJSON(path, rec)
	unlock()
	if err != nil {
		return err
	}
	e.runHooks(ctx, n)
	writeStatus(stateDir, rec)
	if !post {
		return nil
	}
	return e.deliver(ctx, path)
}

// Flush retries every queued post that is due. It returns how many it
// delivered.
func (e *Emitter) Flush(ctx context.Context) int {
	entries, err := os.ReadDir(filepath.Join(e.Root, "outbox"))
	if err != nil {
		return 0
	}
	delivered := 0
	for _, ent := range entries {
		if ent.IsDir() || filepath.Ext(ent.Name()) != ".json" {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		path := filepath.Join(e.Root, "outbox", ent.Name())
		var r Record
		if readJSON(path, &r) != nil || r.Slack == nil || r.Slack.Status != StatusQueued {
			continue
		}
		if e.deliver(ctx, path) == nil {
			if readJSON(path, &r) == nil && r.Slack != nil && r.Slack.Status == StatusDelivered {
				delivered++
			}
		}
	}
	return delivered
}

// deliver posts one queued record if it is due and unleased. It is at most
// once per notice ID: a post that may have reached Slack is never sent again,
// even across a crash and restart (see Delivery.Sending).
func (e *Emitter) deliver(ctx context.Context, path string) error {
	s := e.Config.Slack
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	var rec Record
	if err = readJSON(path, &rec); err != nil {
		unlock()
		return err
	}
	now := e.now()
	d := rec.Slack
	if d == nil || d.Status != StatusQueued || d.NextAttempt.After(now) || d.Lease.After(now) {
		unlock()
		return nil
	}
	if !d.Sending.IsZero() {
		// The lease ran out with a send started and no answer recorded;
		// Sending stays as the time it went out.
		d.Status, d.Error, d.Lease, d.NextAttempt = StatusUncertain, uncertainCrash, time.Time{}, time.Time{}
		err = writeJSON(path, rec)
		unlock()
		if err == nil {
			writeStatus(rec.StateDir, rec)
		}
		return err
	}
	if s == nil {
		// Configured when queued, not now: keep it for a host that has it.
		unlock()
		return nil
	}
	lease := now.Add(2 * time.Minute)
	d.Lease = lease
	err = writeJSON(path, rec)
	unlock()
	if err != nil {
		return err
	}

	// Everything that can fail before the request is a definite miss.
	tok, err := e.token(ctx, s.Token)
	if err != nil {
		return e.settle(path, "", "", err)
	}

	// Commit to sending: from here a stopped process leaves an unknown
	// outcome on disk.
	if unlock, err = e.lock(); err != nil {
		return err
	}
	var cur Record
	if err = readJSON(path, &cur); err != nil || cur.Slack == nil || !cur.Slack.Lease.Equal(lease) || cur.Slack.Status != StatusQueued {
		// Another sender took it over; leave it to them.
		unlock()
		return err
	}
	cur.Slack.Sending = e.now()
	err = writeJSON(path, cur)
	unlock()
	if err != nil {
		return err
	}

	channel, ts, postErr := e.post(ctx, s, rec, tok)
	return e.settle(path, channel, ts, postErr)
}

// settle records a post's answer and releases its lease.
func (e *Emitter) settle(path, channel, ts string, postErr error) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	var cur Record
	if err = readJSON(path, &cur); err != nil {
		return err
	}
	d := cur.Slack
	if d == nil {
		return postErr
	}
	d.Lease, d.Sending = time.Time{}, time.Time{}
	var uncertain UncertainError
	switch {
	case postErr == nil:
		d.Status, d.Error, d.NextAttempt, d.TS, d.Delivered = StatusDelivered, "", time.Time{}, ts, e.now()
		if channel != "" {
			d.Channel = channel
		}
	case errors.As(postErr, &uncertain):
		d.Attempts++
		d.Status, d.NextAttempt = StatusUncertain, time.Time{}
		d.Error = OneLine("outcome unknown, not reposted: "+postErr.Error(), 200)
	default:
		d.Attempts++
		d.Error = OneLine(postErr.Error(), 200)
		if d.Attempts >= MaxAttempts {
			d.Status, d.NextAttempt = StatusFailed, time.Time{}
		} else {
			d.NextAttempt = e.now().Add(backoff(d.Attempts))
		}
	}
	if err = writeJSON(path, cur); err != nil {
		return err
	}
	writeStatus(cur.StateDir, cur)
	return postErr
}

func backoff(attempts int) time.Duration {
	d := time.Minute
	for i := 1; i < attempts && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// thread is the Slack thread of one run or orchestrator, kept beside its
// state (slack-thread.json) or, for a notice without local state (one pulled
// from another host), under the outbox's threads/.
type thread struct {
	Key string `json:"key"`
	// Configured is the [notify.slack] channel the thread was started for;
	// Channel is the conversation Slack answered with (a DM's D... ID).
	Configured string `json:"configured_channel"`
	Channel    string `json:"channel"`
	TS         string `json:"ts"`
}

// ThreadFile is the thread record's name in a run or orchestrator directory.
const ThreadFile = "slack-thread.json"

func (e *Emitter) threadPath(rec Record) string {
	if rec.StateDir != "" {
		return filepath.Join(rec.StateDir, ThreadFile)
	}
	sum := sha256.Sum256([]byte(rec.Notice.Thread))
	return filepath.Join(e.Root, "threads", hex.EncodeToString(sum[:16])+".json")
}

func (e *Emitter) post(ctx context.Context, s *Slack, rec Record, tok string) (string, string, error) {
	text := rec.Notice.Message(s.Tag)
	tp := e.threadPath(rec)
	var th thread
	if readJSON(tp, &th) != nil || th.Configured != s.Channel || th.Key != rec.Notice.Thread {
		th = thread{}
	}
	p := e.poster()
	if th.TS != "" {
		ch, ts, err := p.Post(ctx, SlackMessage{Token: tok, Channel: th.Channel, Text: text, ThreadTS: th.TS})
		if err == nil || !threadGone(err) {
			return ch, ts, err
		}
	}
	ch, ts, err := p.Post(ctx, SlackMessage{Token: tok, Channel: s.Channel, Text: text})
	if err != nil {
		return "", "", err
	}
	th = thread{Key: rec.Notice.Thread, Configured: s.Channel, Channel: ch, TS: ts}
	if th.Channel == "" {
		th.Channel = s.Channel
	}
	if mkErr := os.MkdirAll(filepath.Dir(tp), 0700); mkErr == nil {
		_ = writeJSON(tp, th)
	}
	return ch, ts, nil
}

// StatusFile keeps the latest deliveries of one run or orchestrator, for
// dispatch show, orchestrate show and ls.
const StatusFile = "attention.json"

// Entry is one notice's delivery as shown.
type Entry struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	Source      string    `json:"source"`
	Time        time.Time `json:"time"`
	Message     string    `json:"message"`
	Slack       string    `json:"slack,omitempty"`
	Attempts    int       `json:"attempts,omitempty"`
	Error       string    `json:"error,omitempty"`
	NextAttempt time.Time `json:"next_attempt,omitzero"`
}

const statusKeep = 20

func writeStatus(dir string, rec Record) {
	if dir == "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	p := filepath.Join(dir, StatusFile)
	var list []Entry
	_ = readJSON(p, &list)
	ent := Entry{ID: rec.Notice.ID, Event: rec.Notice.Event, Source: rec.Notice.Source, Time: rec.Created, Message: strings.SplitN(rec.Notice.Message(""), "\n", 2)[0], Slack: rec.SlackSkip}
	if d := rec.Slack; d != nil {
		ent.Slack, ent.Attempts, ent.Error, ent.NextAttempt = d.Status, d.Attempts, d.Error, d.NextAttempt
	}
	replaced := false
	for i := range list {
		if list[i].ID == ent.ID {
			list[i], replaced = ent, true
		}
	}
	if !replaced {
		list = append(list, ent)
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Time.Before(list[j].Time) })
	if len(list) > statusKeep {
		list = list[len(list)-statusKeep:]
	}
	_ = writeJSON(p, list)
}

// ReadStatus returns the deliveries recorded in a run or orchestrator
// directory, oldest first.
func ReadStatus(dir string) []Entry {
	var list []Entry
	_ = readJSON(filepath.Join(dir, StatusFile), &list)
	return list
}

// StatusLine summarizes undelivered Slack posts for one directory: "" when
// every post went through (or none was due).
func StatusLine(dir string) string {
	var queued, failed, uncertain int
	var last, unsure Entry
	for _, e := range ReadStatus(dir) {
		switch e.Slack {
		case StatusUncertain:
			uncertain++
			unsure = e
		case StatusQueued:
			if e.Attempts > 0 {
				queued++
				last = e
			}
		case StatusFailed:
			failed++
			last = e
		}
	}
	switch {
	case uncertain > 0:
		return "slack notice uncertain (check Slack; not reposted): " + unsure.Error
	case failed > 0:
		return fmt.Sprintf("slack notice failed after %d attempts: %s", last.Attempts, last.Error)
	case queued > 0:
		return fmt.Sprintf("slack notice retrying (attempt %d, next %s): %s", last.Attempts, last.NextAttempt.Local().Format("15:04"), last.Error)
	}
	return ""
}

// Summary counts this host's outbox, for hev s.
type Summary struct {
	Delivered, Queued, Retrying, Failed, Uncertain int
	LastError                                      string
}

// Summarize reads the outbox.
func (e *Emitter) Summarize() Summary {
	var s Summary
	entries, _ := os.ReadDir(filepath.Join(e.Root, "outbox"))
	var lastTime time.Time
	for _, ent := range entries {
		var r Record
		if readJSON(filepath.Join(e.Root, "outbox", ent.Name()), &r) != nil || r.Slack == nil {
			continue
		}
		switch r.Slack.Status {
		case StatusDelivered:
			s.Delivered++
		case StatusFailed:
			s.Failed++
		case StatusUncertain:
			s.Uncertain++
		case StatusQueued:
			if r.Slack.Attempts > 0 {
				s.Retrying++
			} else {
				s.Queued++
			}
		}
		if r.Slack.Error != "" && r.Created.After(lastTime) {
			lastTime, s.LastError = r.Created, r.Slack.Error
		}
	}
	return s
}

// Describe is the one-line transport status for hev s.
func (e *Emitter) Describe() string {
	if e.ConfigErr != nil {
		return "notify: " + OneLine(e.ConfigErr.Error(), 200)
	}
	s := e.Config.Slack
	if s == nil {
		return "notify: no [notify.slack] on this host; notices reach owners through peers (or notices pull)"
	}
	sum := e.Summarize()
	line := fmt.Sprintf("notify: slack to %s (%s): %d delivered", s.Channel, strings.Join(s.Kinds, ","), sum.Delivered)
	if sum.Queued > 0 {
		line += fmt.Sprintf(", %d queued", sum.Queued)
	}
	if sum.Retrying > 0 {
		line += fmt.Sprintf(", %d retrying", sum.Retrying)
	}
	if sum.Failed > 0 {
		line += fmt.Sprintf(", %d failed", sum.Failed)
	}
	if sum.Uncertain > 0 {
		line += fmt.Sprintf(", %d uncertain (check Slack; not reposted)", sum.Uncertain)
	}
	if sum.LastError != "" && sum.Retrying+sum.Failed+sum.Uncertain > 0 {
		line += "; last error: " + sum.LastError
	}
	return line
}

func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Emit emits n through this host's default emitter, bounded by a short
// deadline so a slow Slack or secret read never holds a run or round.
func Emit(n Notice, stateDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return Default().Emit(ctx, n, stateDir)
}

// Queue records n through this host's default emitter for a later Flush.
func Queue(n Notice, stateDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return Default().Queue(ctx, n, stateDir)
}

// Flush retries this host's due posts under a deadline.
func Flush(ctx context.Context) int {
	e := Default()
	if e.Config.Slack == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return e.Flush(ctx)
}
