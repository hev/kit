// Package tracestats aggregates archived session rows into the report
// hev traces stats prints: run kinds, skill loads, tool use and dispatch
// launch health.
package tracestats

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// Run kinds, derived from what a session row already carries.
const (
	KindWorker       = "worker"
	KindOrchestrator = "orchestrator-round"
	KindLoop         = "loop"
	KindInteractive  = "interactive"
)

// Kinds lists the run kinds in display order.
var Kinds = []string{KindWorker, KindOrchestrator, KindLoop, KindInteractive}

// dispatchDir matches a working directory inside a dispatch root
// (~/.hev/orders/NAME/... or the legacy ~/dispatch/NAME/...), on any host's
// home layout.
var dispatchDir = regexp.MustCompile(`^(?:/home/[^/]+|/Users/[^/]+|/root|[A-Za-z]:\\Users\\[^\\]+)[/\\](?:dispatch|\.hev[/\\]orders)[/\\]([^/\\]+)(?:[/\\]|$)`)

// loopTools are tools that schedule the session's own next run.
var loopTools = map[string]bool{"ScheduleWakeup": true, "CronCreate": true}

// LoopRepeats is how many sessions outside dispatch must start with the
// same prompt in the window for them to count as a loop.
const LoopRepeats = 3

// RunKind derives a session's run kind (see https://hevkit.com/docs/traces):
//
//  1. orchestrator-round: the dispatch order is an orchestrator's (orch-*),
//     or the working directory is under a dispatch root's orch-* directory.
//  2. worker: the row has a dispatch order or trace ID, or its working
//     directory is under a dispatch root (~/dispatch/NAME/...).
//  3. loop: the session scheduled its own next run (ScheduleWakeup or
//     CronCreate), was started by /loop, runs in an expediter's state
//     directory, or its first prompt recurs (repeats[fingerprint] >=
//     LoopRepeats) among sessions outside dispatch in the window.
//  4. interactive: everything else.
func RunKind(r trace.SessionRow, repeats map[string]int) string {
	dir := ""
	if m := dispatchDir.FindStringSubmatch(r.Workdir); m != nil {
		dir = m[1]
	}
	if strings.HasPrefix(r.Order, "orch-") || strings.HasPrefix(dir, "orch-") {
		return KindOrchestrator
	}
	if r.Order != "" || r.TraceID != "" || dir != "" {
		return KindWorker
	}
	for _, t := range r.ToolNames {
		if loopTools[t] {
			return KindLoop
		}
	}
	prompt := strings.TrimSpace(r.FirstPrompt)
	if strings.HasPrefix(prompt, "/loop") || strings.Contains(prompt, "<command-name>/loop</command-name>") || strings.Contains(r.Workdir, "expediter") {
		return KindLoop
	}
	if f := PromptFingerprint(r); f != "" && repeats[f] >= LoopRepeats {
		return KindLoop
	}
	return KindInteractive
}

// PromptFingerprint identifies a session's first prompt for loop
// recurrence: a hash of its whitespace-normalized text, scoped to the
// working directory. Empty for a session with no prompt.
func PromptFingerprint(r trace.SessionRow) string {
	p := r.FirstPromptShort
	if p == "" {
		p = r.FirstPrompt
	}
	p = strings.Join(strings.Fields(p), " ")
	if p == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(r.Workdir + "\x00" + p))
	return hex.EncodeToString(sum[:8])
}

// Repeats counts first-prompt fingerprints among sessions outside dispatch,
// the input RunKind needs for recurrence.
func Repeats(rows []trace.SessionRow) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		if r.Order != "" || r.TraceID != "" || dispatchDir.MatchString(r.Workdir) {
			continue
		}
		if f := PromptFingerprint(r); f != "" {
			out[f]++
		}
	}
	return out
}
