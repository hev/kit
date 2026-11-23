package foh

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/trace"
)

// ResumeSession describes one local session on this host (either a native hev
// session or a transcript from another harness such as claude, codex, or agy).
type ResumeSession struct {
	ID      string
	Harness string // hev, claude, codex, agy, pi, opencode, dot
	Cwd     string
	Title   string
	ModTime time.Time

	fohPath string
	source  trace.Source
	unit    trace.Unit
}

// ShortDir returns the last path component of Cwd for compact picker display.
func (s ResumeSession) ShortDir() string {
	if s.Cwd == "" {
		return "-"
	}
	return filepath.Base(s.Cwd)
}

// AgeLabel formats how long ago the session was active.
func (s ResumeSession) AgeLabel(now time.Time) string {
	if s.ModTime.IsZero() {
		return ""
	}
	d := now.Sub(s.ModTime)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func shortHarnessName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "claude_code", "claude":
		return "claude"
	case "codex":
		return "codex"
	case "agy", "antigravity":
		return "agy"
	case "pi":
		return "pi"
	case "opencode":
		return "opencode"
	case "dot":
		return "dot"
	case "foh", "hev":
		return "hev"
	default:
		if name == "" {
			return "trace"
		}
		return name
	}
}

type rawCandidate struct {
	modTime time.Time
	fohPath string
	source  trace.Source
	srcName string
	unit    trace.Unit
}

// ListLocalSessions discovers recent sessions on this host across native hev
// sessions and all registered local harness trace sources (claude, codex, agy,
// pi, opencode, dot). Only the newest `limit` candidates are parsed for titles.
func ListLocalSessions(fohDir string, sources []trace.Source, limit int) []ResumeSession {
	if fohDir == "" {
		fohDir = SessionsDir()
	}
	if sources == nil {
		sources = trace.Sources(nil)
	}
	if limit <= 0 {
		limit = 30
	}

	var cands []rawCandidate
	// 1. Native hev sessions (~/.hev/foh/sessions/*.jsonl).
	if entries, err := os.ReadDir(fohDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() == 0 {
				continue
			}
			cands = append(cands, rawCandidate{
				modTime: info.ModTime(),
				fohPath: filepath.Join(fohDir, e.Name()),
				srcName: "hev",
			})
		}
	}

	// 2. Other local harnesses via internal/trace (claude, codex, agy, pi, opencode).
	for _, src := range sources {
		if src == nil {
			continue
		}
		_, name := trace.SourceInfo(src)
		units, err := src.Units()
		if err != nil {
			continue
		}
		for _, u := range units {
			if u.IsSidechain {
				continue
			}
			var mt time.Time
			if u.Modified > 0 {
				mt = time.UnixMilli(u.Modified)
			} else if info, err := os.Stat(u.Key); err == nil {
				mt = info.ModTime()
			}
			if mt.IsZero() {
				continue
			}
			cands = append(cands, rawCandidate{
				modTime: mt,
				source:  src,
				srcName: shortHarnessName(name),
				unit:    u,
			})
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		return cands[i].modTime.After(cands[j].modTime)
	})
	if len(cands) > limit*2 {
		cands = cands[:limit*2]
	}

	var out []ResumeSession
	for _, c := range cands {
		if len(out) >= limit {
			break
		}
		if c.fohPath != "" {
			if s, ok := peekFOHSession(c.fohPath, c.modTime); ok {
				out = append(out, s)
			}
			continue
		}
		if s, ok := peekTraceSession(c.source, c.srcName, c.unit, c.modTime); ok {
			out = append(out, s)
		}
	}
	return out
}

func peekFOHSession(path string, modTime time.Time) (ResumeSession, bool) {
	f, err := os.Open(path)
	if err != nil {
		return ResumeSession{}, false
	}
	defer f.Close()
	s := ResumeSession{
		ID:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Harness: "hev",
		ModTime: modTime,
		fohPath: path,
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 2<<20)
	hasUser := false
	for sc.Scan() {
		var rec Record
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if rec.Session != "" {
			s.ID = rec.Session
		}
		if s.Cwd == "" && rec.Cwd != "" {
			s.Cwd = rec.Cwd
		}
		if rec.Type == "user" && strings.TrimSpace(rec.Content) != "" {
			hasUser = true
			if s.Title == "" {
				s.Title = firstLine(rec.Content, 72)
			}
		}
		if hasUser && s.Cwd != "" {
			break
		}
	}
	if !hasUser {
		return ResumeSession{}, false
	}
	return s, true
}

func peekTraceSession(src trace.Source, harnessName string, u trace.Unit, modTime time.Time) (ResumeSession, bool) {
	turns, err := src.Read(u)
	if err != nil || len(turns) == 0 {
		return ResumeSession{}, false
	}
	s := ResumeSession{
		Harness: harnessName,
		ModTime: modTime,
		source:  src,
		unit:    u,
	}
	for _, t := range turns {
		if s.ID == "" && t.SessionID != "" {
			s.ID = t.SessionID
		}
		if s.Cwd == "" && t.Workdir != "" {
			s.Cwd = t.Workdir
		}
		if s.Harness == "trace" && t.Harness != "" {
			s.Harness = shortHarnessName(t.Harness)
		}
		if s.Title == "" && t.Summary != "" {
			s.Title = firstLine(t.Summary, 72)
		}
		if s.Title == "" && t.Role == "user" {
			if txt := turnText(t); txt != "" {
				s.Title = firstLine(txt, 72)
			}
		}
		if s.ID != "" && s.Cwd != "" && s.Title != "" {
			break
		}
	}
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(u.Key), filepath.Ext(u.Key))
	}
	if s.Title == "" {
		return ResumeSession{}, false
	}
	return s, true
}

// LoadSessionMessages reads all user/assistant text turns from a local session
// (native hev JSONL or any other local harness trace) so it can be resumed in
// this harness's Agent history.
func LoadSessionMessages(s ResumeSession) ([]Message, error) {
	if s.fohPath != "" {
		return loadFOHMessages(s.fohPath)
	}
	if s.source == nil {
		return nil, fmt.Errorf("session %s has no readable source", s.ID)
	}
	turns, err := s.source.Read(s.unit)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for _, t := range turns {
		role := strings.ToLower(strings.TrimSpace(t.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		txt := turnText(t)
		if txt == "" {
			continue
		}
		if len(txt) > maxRecordContent {
			txt = txt[:maxRecordContent] + "\n…[truncated]"
		}
		if len(msgs) > 0 && msgs[len(msgs)-1].Role == role {
			msgs[len(msgs)-1].Content += "\n\n" + txt
		} else {
			msgs = append(msgs, Message{Role: role, Content: txt})
		}
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("session %s has no text turns to resume", s.ID)
	}
	return msgs, nil
}

func loadFOHMessages(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var msgs []Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		var rec Record
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		switch rec.Type {
		case "user":
			if txt := strings.TrimSpace(rec.Content); txt != "" {
				msgs = append(msgs, Message{Role: "user", Content: txt})
			}
		case "assistant":
			if txt := strings.TrimSpace(rec.Content); txt != "" {
				msgs = append(msgs, Message{Role: "assistant", Content: txt})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("session %s has no turns", filepath.Base(path))
	}
	return msgs, nil
}

func turnText(t trace.Turn) string {
	var parts []string
	for _, b := range t.Blocks {
		if b.Type == "text" {
			if txt := strings.TrimSpace(b.Text); txt != "" {
				parts = append(parts, txt)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func firstLine(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	line, _, _ := strings.Cut(s, "\n")
	return truncateLine(strings.TrimSpace(line), maxChars)
}
