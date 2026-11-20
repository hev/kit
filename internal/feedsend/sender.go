// Package feedsend is the Slack feed's sender (docs/feed.md): it drains the
// spool the hook side writes, summarizes turns with a small model, and posts
// one thread per session, run, orchestrator, board thread or evidence
// record through attention's Slack poster.
package feedsend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/platformops"
)

// Timing defaults.
const (
	// PostGap spaces posts: Slack allows about one a second per channel.
	PostGap = 1100 * time.Millisecond
	// IdleExit ends a sender that has had nothing to do this long.
	IdleExit = 45 * time.Second
	// threadKeep bounds the thread map.
	threadKeep = 1000
	logLimit   = 2 << 20
)

// Sender posts spooled records.
type Sender struct {
	Dir    string
	Config feed.Config
	Post   attention.Poster
	// Upload attaches files; nil uses Post when it can upload.
	Upload   attention.Uploader
	Token    func(ctx context.Context, ref string) (string, error)
	Composer Composer
	Gap      time.Duration
	Idle     time.Duration
	// Settle overrides StopSettle; negative skips the wait.
	Settle time.Duration
	Now    func() time.Time
	Sleep  func(time.Duration)

	token     string
	noUploads string
}

// threadRef is a started Slack thread.
type threadRef struct {
	Channel string    `json:"channel"`
	TS      string    `json:"ts"`
	Time    time.Time `json:"time"`
}

func (s *Sender) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Sender) sleep(d time.Duration) {
	if s.Sleep != nil {
		s.Sleep(d)
		return
	}
	time.Sleep(d)
}

// Log appends a line to sender.log, rotating past 2 MiB.
func (s *Sender) Log(msg string) {
	p := filepath.Join(s.Dir, "sender.log")
	if info, err := os.Stat(p); err == nil && info.Size() > logLimit {
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", s.now().Format(time.RFC3339), strings.ReplaceAll(msg, "\n", " "))
}

func (s *Sender) backoffPath() string { return filepath.Join(s.Dir, "backoff") }

// BackedOff reports a pause after Slack or the token failed.
func (s *Sender) BackedOff() (time.Time, bool) {
	b, err := os.ReadFile(s.backoffPath())
	if err != nil {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil || !until.After(s.now()) {
		return time.Time{}, false
	}
	return until, true
}

func (s *Sender) backoff(d time.Duration, why string) {
	until := s.now().Add(d)
	_ = os.WriteFile(s.backoffPath(), []byte(until.Format(time.RFC3339)+"\n"), 0600)
	s.Log(fmt.Sprintf("pausing until %s: %s", until.Format(time.RFC3339), why))
}

func (s *Sender) threads() map[string]threadRef {
	m := map[string]threadRef{}
	b, err := os.ReadFile(filepath.Join(s.Dir, "threads.json"))
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (s *Sender) saveThreads(m map[string]threadRef) {
	if len(m) > threadKeep {
		type kv struct {
			k string
			v threadRef
		}
		var all []kv
		for k, v := range m {
			all = append(all, kv{k, v})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].v.Time.After(all[j].v.Time) })
		m = map[string]threadRef{}
		for _, e := range all[:threadKeep] {
			m[e.k] = e.v
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = platformops.WriteFileAtomic(filepath.Join(s.Dir, "threads.json"), b, 0600)
}

// Run drains the spool under the sender lock and returns when it has been
// idle for Idle, Slack is unusable, or ctx ends. A second sender returns at
// once.
func (s *Sender) Run(ctx context.Context) error {
	if err := os.MkdirAll(feed.SpoolDir(s.Dir), 0700); err != nil {
		return err
	}
	for {
		unlock, err := platformops.LockPath(feed.LockPath(s.Dir), true)
		if err != nil {
			if platformops.IsLockBusy(err) {
				return nil
			}
			return err
		}
		err = s.drain(ctx)
		unlock()
		if err != nil || ctx.Err() != nil {
			return err
		}
		// A record may have landed between the last look and the unlock.
		if _, paused := s.BackedOff(); paused || len(feed.Pending(s.Dir)) == 0 {
			return nil
		}
	}
}

// errStop ends a drain and keeps the record for a later sender.
var errStop = errors.New("feed: sender paused")

func (s *Sender) drain(ctx context.Context) error {
	idle := s.Idle
	if idle == 0 {
		idle = IdleExit
	}
	idleSince := s.now()
	for ctx.Err() == nil {
		if _, paused := s.BackedOff(); paused {
			return nil
		}
		files := feed.Pending(s.Dir)
		if len(files) == 0 {
			if s.now().Sub(idleSince) >= idle {
				return nil
			}
			s.sleep(500 * time.Millisecond)
			continue
		}
		for _, p := range files {
			if ctx.Err() != nil {
				return nil
			}
			if err := s.One(ctx, p); errors.Is(err, errStop) {
				return nil
			}
		}
		idleSince = s.now()
	}
	return nil
}

// One posts the record at path and removes it, unless Slack or the token
// is unavailable (errStop: the record stays for the next sender).
func (s *Sender) One(ctx context.Context, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if s.token == "" {
		tok, err := s.Token(ctx, s.Config.Token)
		if err != nil {
			s.backoff(10*time.Minute, "token: "+err.Error())
			return errStop
		}
		s.token = tok
	}
	// Claim the record, so no other sender can post it too.
	claimed := filepath.Join(s.Dir, "sending", filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(claimed), 0700); err != nil {
		return nil
	}
	if err := os.Rename(path, claimed); err != nil {
		return nil
	}
	err = s.one(ctx, claimed, b)
	if errors.Is(err, errStop) {
		// Keep it for the next sender.
		_ = os.Rename(claimed, path)
	}
	return err
}

// StopSettle is how long a stop waits before its transcript is read: the
// harness may still be writing the turn's last reply when the hook runs.
const StopSettle = 3 * time.Second

func (s *Sender) one(ctx context.Context, path string, b []byte) error {
	var rec feed.Record
	if json.Unmarshal(b, &rec) != nil || rec.Schema != feed.RecordSchema {
		s.Log("dropping unreadable record " + filepath.Base(path))
		_ = os.Remove(path)
		return nil
	}
	if rec.Kind == feed.KindStop && !rec.Client && s.Settle >= 0 {
		settle := s.Settle
		if settle == 0 {
			settle = StopSettle
		}
		if wait := settle - s.now().Sub(rec.Time); wait > 0 {
			s.sleep(wait)
		}
	}
	c := s.Composer
	c.Warn = func(msg string) { s.Log(rec.Kind + " " + rec.Thread.Key + ": " + msg) }
	msg, ok := c.Compose(ctx, rec)
	if !ok {
		_ = os.Remove(path)
		return nil
	}
	err := s.deliver(ctx, msg)
	switch {
	case err == nil:
		s.Log(fmt.Sprintf("posted %s to %s (summarized: %v)", rec.Kind, rec.Thread.Key, msg.Summarized))
		_ = os.Remove(path)
	case errors.Is(err, errStop):
		return err
	default:
		// A message Slack refused, or one it may already have: never
		// repost it.
		s.Log(fmt.Sprintf("dropped %s %s: %v", rec.Kind, rec.Thread.Key, err))
		_ = os.Remove(path)
	}
	return nil
}

// fatal Slack answers stop the sender: nothing will post until they are
// fixed.
var fatal = map[string]bool{"invalid_auth": true, "not_authed": true, "token_revoked": true, "token_expired": true,
	"account_inactive": true, "channel_not_found": true, "not_in_channel": true, "is_archived": true}

// post sends one message with rate-limit retries and classifies failures.
func (s *Sender) post(ctx context.Context, m attention.SlackMessage) (string, string, error) {
	m.Token = s.token
	for attempt := 0; ; attempt++ {
		ch, ts, err := s.Post.Post(ctx, m)
		if err == nil {
			s.sleep(s.gap())
			return ch, ts, nil
		}
		var se attention.SlackError
		var unsure attention.UncertainError
		switch {
		case errors.As(err, &unsure):
			return "", "", err
		case errors.As(err, &se) && se.Code == "rate_limited" && attempt < 3:
			s.sleep(5 * time.Second)
			continue
		case errors.As(err, &se) && (fatal[se.Code] || se.Code == "rate_limited"):
			s.backoff(10*time.Minute, err.Error())
			return "", "", fmt.Errorf("%w: %v", errStop, err)
		case errors.As(err, &se):
			return "", "", err
		default:
			// Nothing reached Slack (a refused connection): try later.
			s.backoff(time.Minute, err.Error())
			return "", "", fmt.Errorf("%w: %v", errStop, err)
		}
	}
}

func (s *Sender) gap() time.Duration {
	if s.Gap != 0 {
		return s.Gap
	}
	return PostGap
}

func (s *Sender) deliver(ctx context.Context, msg Message) error {
	threads := s.threads()
	th, have := threads[msg.Key]
	text := msg.Text
	start := func() error {
		parent := msg.Parent
		if parent == "" {
			parent, text = text, ""
		}
		ch, ts, err := s.post(ctx, attention.SlackMessage{Channel: s.Config.Channel, Text: parent})
		if err != nil {
			return err
		}
		if ch == "" {
			ch = s.Config.Channel
		}
		th = threadRef{Channel: ch, TS: ts, Time: s.now()}
		threads[msg.Key] = th
		s.saveThreads(threads)
		return nil
	}
	if !have {
		if err := start(); err != nil {
			return err
		}
	}
	if text != "" {
		_, _, err := s.post(ctx, attention.SlackMessage{Channel: th.Channel, Text: text, ThreadTS: th.TS})
		if err != nil && attention.ThreadGone(err) && have {
			// The thread was deleted: start it again.
			text = msg.Text
			if err = start(); err == nil && text != "" {
				_, _, err = s.post(ctx, attention.SlackMessage{Channel: th.Channel, Text: text, ThreadTS: th.TS})
			}
		}
		if err != nil {
			return err
		}
	}
	th.Time = s.now()
	threads[msg.Key] = th
	s.saveThreads(threads)
	s.attach(ctx, th, msg.Files)
	return nil
}

func (s *Sender) uploader() attention.Uploader {
	if s.Upload != nil {
		return s.Upload
	}
	if u, ok := s.Post.(attention.Uploader); ok {
		return u
	}
	return nil
}

// attach uploads files into the thread. A file over the cap, or any file
// when the bot cannot upload, is named with its link instead; an upload
// failure never drops the message it belongs to.
func (s *Sender) attach(ctx context.Context, th threadRef, files []File) {
	up := s.uploader()
	var notes []string
	for _, f := range files {
		why := s.noUploads
		switch {
		case up == nil:
			why = "uploads unavailable"
		case why != "":
		case f.Data == nil:
			why = fmt.Sprintf("%.1f MB, over the %d MB cap", float64(f.Size)/(1<<20), s.Composer.MaxUpload>>20)
		default:
			err := up.Upload(ctx, attention.SlackFile{Token: s.token, Channel: th.Channel, ThreadTS: th.TS, Name: f.Name, Title: f.Title, Data: f.Data})
			if err == nil {
				s.sleep(s.gap())
				continue
			}
			var se attention.SlackError
			if errors.As(err, &se) && (se.Code == "missing_scope" || se.Code == "not_allowed_token_type") {
				// Say it once per sender, then link everything.
				s.noUploads = "the bot token lacks files:write"
				s.Log("uploads off: " + err.Error())
				why = s.noUploads
			} else {
				s.Log("upload " + f.Name + ": " + err.Error())
				why = "upload failed"
			}
		}
		note := fmt.Sprintf(":paperclip: %s not attached (%s)", f.Name, why)
		if f.Link != "" {
			note += ": " + f.Link
		}
		notes = append(notes, note)
	}
	if len(notes) > 0 {
		_, _, _ = s.post(ctx, attention.SlackMessage{Channel: th.Channel, Text: strings.Join(notes, "\n"), ThreadTS: th.TS})
	}
}

// New is a sender for this host's config: hev secrets for the bot token and
// the model key, Slack's Web API (or HEV_SLACK_API_URL), and the Messages
// API (or HEV_ANTHROPIC_API_URL).
func New(cfg feed.Config, dir string) *Sender {
	s := &Sender{Dir: dir, Config: cfg, Post: attention.DefaultPoster()}
	s.Token = func(ctx context.Context, ref string) (string, error) {
		return attention.ResolveSecret(ctx, ref, "SLACK_BOT_TOKEN", "kit feed")
	}
	s.Composer = Composer{MaxUpload: cfg.MaxUploadBytes, Tag: cfg.Tag}
	if cfg.ModelKey != "" {
		var key string
		var keyErr error
		resolved := false
		s.Composer.Model = NewAnthropic(cfg.Model, func(ctx context.Context) (string, error) {
			if !resolved {
				key, keyErr = attention.ResolveSecret(ctx, cfg.ModelKey, "ANTHROPIC_API_KEY", "kit feed")
				resolved = true
			}
			return key, keyErr
		})
	}
	return s
}
