package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StateClosed is the terminal state of a run a person closed: its work was
// finished another way, or a replacement run superseded it.
const StateClosed = "closed"

// SplitRunAddress splits a [HOST/]NAME run address. HOST is empty when the
// address names only the run.
func SplitRunAddress(s string) (host, name string, err error) {
	s = strings.TrimSpace(s)
	host, name, found := strings.Cut(s, "/")
	if !found {
		host, name = "", s
	}
	if found && (host == "" || strings.ContainsAny(host, " \t/")) {
		return "", "", fmt.Errorf("invalid run address %q: want [HOST/]NAME", s)
	}
	if !ValidName(name) {
		return "", "", fmt.Errorf("invalid run address %q: want [HOST/]NAME", s)
	}
	return host, name, nil
}

// SupersededReason is the close reason recorded on a run a replacement
// superseded.
func SupersededReason(by string) string { return "superseded by " + by }

// closeRun records the closed disposition on an ended run (the close
// action). It never launches a worker and never removes the run's log,
// reports: it rewrites result.json with the closed fields added
// and, when the run is waiting on a decision, settles that decision in
// decisions.json. Pro may then compact the worktree. The caller has checked dir holds a run.
func closeRun(dir string, q Request) (Response, error) {
	r := Response{}
	reason := strings.TrimSpace(q.Note)
	superseded := strings.TrimSpace(q.SupersededBy)
	if superseded != "" {
		h, n, err := SplitRunAddress(superseded)
		if err != nil {
			return r, err
		}
		if h != "" {
			superseded = h + "/" + n
		} else {
			superseded = n
		}
		if reason == "" {
			reason = SupersededReason(superseded)
		}
	}
	by := q.Orchestrator
	if by == "" {
		by = q.Meta.Sender
	}
	if by == "" {
		by = LocalSender()
	}
	unlock, err := lockRoot()
	if err != nil {
		return r, err
	}
	defer unlock()
	live, err := liveNames()
	if err == nil {
		err = addReservations(live)
	}
	if err != nil {
		return r, err
	}
	if live[q.Name] {
		return r, fmt.Errorf("stop running order before close")
	}
	resultPath := filepath.Join(dir, "result.json")
	var result Result
	if err := readJSON(resultPath, &result); err != nil && !os.IsNotExist(err) {
		return r, fmt.Errorf("read result: %w", err)
	}
	reconcileLaunch(dir, &result)
	if result.LaunchUnconfirmed {
		return r, fmt.Errorf("launch unconfirmed; inspect ls/show after launch grace period before close")
	}
	if result.Closed() {
		return r, fmt.Errorf("%s already closed at %s by %s", q.Name, result.ClosedAt.Format(time.RFC3339), result.ClosedBy)
	}
	now := time.Now().UTC()
	if OpenDecision(&result) {
		ledger := loadDecisions(dir)
		i := len(ledger) - 1
		for i >= 0 && (ledger[i].Key != result.Key || !ledger[i].Resolved.IsZero()) {
			i--
		}
		if i < 0 {
			// The ledger predates this result; record the question from it.
			ledger = append(ledger, Decision{Key: result.Key, Question: result.Question, Asked: result.End})
			i = len(ledger) - 1
		}
		answer := reason
		if answer == "" {
			answer = "closed"
		}
		ledger[i].Answer, ledger[i].Resolved, ledger[i].ResolvedBy, ledger[i].Closed = answer, now, by, true
		if err := WriteJSON(filepath.Join(dir, "decisions.json"), ledger); err != nil {
			return r, err
		}
		result.Decisions = resolvedDecisions(dir)
	}
	if result.Start.IsZero() || result.Branch == "" {
		var m Meta
		if readJSON(filepath.Join(dir, "meta.json"), &m) == nil {
			if result.Start.IsZero() {
				result.Start = m.Start
			}
			if result.Branch == "" {
				result.Branch = m.Branch
			}
		}
	}
	result.ClosedAt, result.ClosedBy, result.ClosedReason, result.SupersededBy = now, by, reason, superseded
	if err := WriteJSON(resultPath, result); err != nil {
		return r, err
	}
	r.Message = "closed " + q.Name
	if reason != "" {
		r.Message += ": " + reason
	}
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) == nil {
		if _, pro, _ := workspacePolicy(); pro {
			if err := compactWorkspace(dir, m); err != nil {
				r.Message += "\nworktree kept: " + err.Error()
			}
		}
	}
	r.Files = map[string]string{}
	if b, err := os.ReadFile(resultPath); err == nil {
		r.Files["result.json"] = string(b)
	}
	return r, nil
}
