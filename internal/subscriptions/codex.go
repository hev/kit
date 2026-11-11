package subscriptions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/cost"
)

// Codex reading bounds: how many recent day directories and rollout files a
// read opens. A busy machine writes many rollouts a day; the newest few by
// modification time always hold the newest token-count event.
const (
	codexDays  = 7
	codexFiles = 6
	// codexStaleAfter is how old a rollout reading can be before it is marked
	// stale. Codex records one on every turn, so an older one means no Codex
	// work has run here since.
	codexStaleAfter = 6 * time.Hour
)

const codexMainLimit = "codex"

func codexHome(env Env) string {
	if h := env.getenv("CODEX_HOME"); h != "" {
		return h
	}
	return filepath.Join(env.Home, ".codex")
}

// readCodex makes no network call. It reads the newest rate-limit event from
// the last few rollouts under ~/.codex/sessions/YYYY/MM/DD.
func readCodex(ctx context.Context, env Env) Status {
	home := codexHome(env)
	s := Status{Source: "codex rollouts in " + tildePath(env.Home, filepath.Join(home, "sessions"))}
	auth, authErr := codexAuthMode(home)
	readings, err := newestCodexReadings(filepath.Join(home, "sessions"), env.now())
	switch {
	case authErr == nil && auth == "apikey":
		s.State, s.StateReason = NotSignedIn, "Codex is signed in with an API key, not a ChatGPT plan"
	case authErr == nil:
		s.State = SignedIn
	case errors.Is(authErr, os.ErrNotExist) && len(readings) == 0:
		s.State, s.StateReason = NotSignedIn, "no Codex login (auth.json) and no rollouts"
	case errors.Is(authErr, os.ErrNotExist):
		s.State, s.StateReason = Unknown, "no auth.json; the login may be in the OS keyring"
	default:
		s.State, s.StateReason = Unknown, "auth.json is unreadable"
	}
	if err != nil {
		s.ObservedReason = "rollouts unreadable: " + err.Error()
	}
	if len(readings) == 0 {
		if s.ObservedReason == "" {
			s.ObservedReason = "no rate-limit reading in the recent rollouts"
		}
		return s
	}
	if s.State == NotSignedIn && auth == "apikey" {
		s.ObservedReason = "readings exist but the current login is an API key"
	}
	mainID := codexMainLimit
	if _, ok := readings[mainID]; !ok {
		// An older Codex reports no limit id; otherwise fall back to the
		// newest of whatever limits were reported.
		mainID = ""
		if _, ok := readings[""]; !ok {
			var newest time.Time
			for id, r := range readings {
				if r.at.After(newest) || (r.at.Equal(newest) && id < mainID) {
					mainID, newest = id, r.at
				}
			}
		}
	}
	main := readings[mainID]
	now := env.now()
	at := main.at
	s.ObservedAt = &at
	if main.limits.PlanType != "" {
		s.Plan = ptr(main.limits.PlanType)
	}
	s.Meters = codexMeters(main.limits, "", now)
	ids := []string{}
	for id := range readings {
		if id != mainID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := readings[id]
		scope := id
		if r.limits.LimitName != nil && *r.limits.LimitName != "" {
			scope = *r.limits.LimitName
		}
		for _, m := range codexMeters(r.limits, scope, now) {
			if m.Kind == KindCredits {
				continue
			}
			s.Meters = append(s.Meters, m)
		}
	}
	if now.Sub(at) > codexStaleAfter {
		s.Stale, s.StaleReason = true, fmt.Sprintf("newest Codex reading is %s old", humanDuration(now.Sub(at)))
	}
	return s
}

func codexMeters(r cost.CodexRateLimits, scope string, now time.Time) []Meter {
	out := []Meter{}
	for _, w := range r.Windows() {
		m := Meter{WindowMinutes: w.Window, Scope: scope}
		switch {
		case w.Window == cost.WeeklyMinutes && scope == "":
			m.Kind, m.Label = KindWeekly, "weekly"
		case w.Window == cost.WeeklyMinutes:
			m.Kind, m.Label = KindWeeklyScoped, "weekly ("+scope+")"
		case w.Window > 0 && w.Window <= 24*60:
			m.Kind, m.Label = KindSession, fmt.Sprintf("session (%s)", humanDuration(time.Duration(w.Window)*time.Minute))
		default:
			m.Kind, m.Label = KindWindow, fmt.Sprintf("%s window", humanDuration(time.Duration(w.Window)*time.Minute))
		}
		if scope != "" && m.Kind != KindWeeklyScoped {
			m.Label += " (" + scope + ")"
		}
		if w.Used >= 0 && w.Used <= 100 {
			m.Percent = ptr(w.Used)
		} else {
			m.PercentReason = fmt.Sprintf("reported %v%% is out of range", w.Used)
		}
		if w.ResetsAt > 0 {
			reset := time.Unix(w.ResetsAt, 0)
			if !reset.After(now) {
				m.RolledOver = true
				m.Percent, m.PercentReason = ptr(0.0), ""
				m.ResetReason = "window reset at " + reset.UTC().Format(time.RFC3339) + " after this reading; next reset unknown until a newer reading"
			} else {
				m.ResetsAt = &reset
			}
		}
		out = append(out, m)
	}
	if c := r.Credits; c != nil && scope == "" {
		m := Meter{Kind: KindCredits, Label: "credits", PercentReason: "credits are a balance, not a percentage", ResetReason: "credits do not reset"}
		switch {
		case c.Unlimited:
			m.Detail = "unlimited"
		case !c.HasCredits:
			m.Detail = "none"
		case c.BalanceText() != "":
			m.Detail = "balance " + c.BalanceText()
		default:
			m.Detail = "available"
		}
		out = append(out, m)
	}
	return out
}

func codexAuthMode(home string) (string, error) {
	b, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return "", err
	}
	var a struct {
		Mode   string          `json:"auth_mode"`
		APIKey json.RawMessage `json:"OPENAI_API_KEY"`
	}
	if err = json.Unmarshal(b, &a); err != nil {
		return "", fmt.Errorf("auth.json is not JSON")
	}
	mode := strings.ToLower(a.Mode)
	if mode == "" && len(a.APIKey) > 0 && string(a.APIKey) != "null" {
		mode = "apikey"
	}
	return mode, nil
}

type codexReading struct {
	at     time.Time
	limits cost.CodexRateLimits
}

// newestCodexReadings returns the newest rate-limit reading per limit id
// across the most recently modified rollouts of the last few days.
func newestCodexReadings(root string, now time.Time) (map[string]codexReading, error) {
	files, err := recentRollouts(root)
	if err != nil {
		return nil, err
	}
	out := map[string]codexReading{}
	for _, f := range files {
		if err := scanRollout(f.path, f.mod, out); err != nil && !errors.Is(err, os.ErrNotExist) {
			return out, err
		}
	}
	return out, nil
}

type rollout struct {
	path string
	mod  time.Time
}

func recentRollouts(root string) ([]rollout, error) {
	days := []string{}
	years, err := descendingDirs(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	for _, y := range years {
		months, _ := descendingDirs(y)
		for _, m := range months {
			ds, _ := descendingDirs(m)
			for _, d := range ds {
				days = append(days, d)
				if len(days) == codexDays {
					break
				}
			}
			if len(days) == codexDays {
				break
			}
		}
		if len(days) == codexDays {
			break
		}
	}
	files := []rollout{}
	for _, d := range days {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, rollout{filepath.Join(d, e.Name()), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].mod.Equal(files[j].mod) {
			return files[i].mod.After(files[j].mod)
		}
		return files[i].path > files[j].path
	})
	if len(files) > codexFiles {
		files = files[:codexFiles]
	}
	return files, nil
}

// descendingDirs lists numeric subdirectories (YYYY, MM, DD), newest first.
func descendingDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type n struct {
		v    int
		path string
	}
	ns := []n{}
	for _, e := range entries {
		if v, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			ns = append(ns, n{v, filepath.Join(dir, e.Name())})
		}
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].v > ns[j].v })
	out := []string{}
	for _, x := range ns {
		out = append(out, x.path)
	}
	return out, nil
}

var rateLimitsKey = []byte(`"rate_limits"`)

func scanRollout(path string, mod time.Time, out map[string]codexReading) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && bytes.Contains(line, rateLimitsKey) {
			consider(line, mod, out)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func consider(line []byte, mod time.Time, out map[string]codexReading) {
	var ev struct {
		Timestamp string `json:"timestamp"`
		Payload   struct {
			Type       string          `json:"type"`
			RateLimits json.RawMessage `json:"rate_limits"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &ev) != nil || ev.Payload.Type != "token_count" {
		return
	}
	raw := bytes.TrimSpace(ev.Payload.RateLimits)
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	limits, err := cost.ParseCodexRateLimits(raw)
	if err != nil || len(limits.Windows()) == 0 {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, ev.Timestamp)
	if err != nil {
		at = mod
	}
	id := limits.LimitID
	if prev, ok := out[id]; !ok || at.After(prev.at) {
		out[id] = codexReading{at: at, limits: limits}
	}
}

func tildePath(home, p string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return p
}
