package dispatch

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/attention"
)

// ModelsExhaustedError ends a round whose whole degrade chain was out of
// plan. The supervisor records the orchestrator as blocked, not failed, and
// wakes it again no sooner than ResetsAt.
type ModelsExhaustedError struct {
	ResetsAt time.Time
}

func (e *ModelsExhaustedError) Error() string { return "blocked: " + exhaustedReason(e.ResetsAt) }

// orchestratorModels is a round's model record, kept in the round directory
// (models.json) and, for ls, as the orchestrator's latest (models.json in
// its directory).
type orchestratorModels struct {
	Order    ModelStep      `json:"order"`
	Running  string         `json:"running"`
	Models   []ModelRun     `json:"models"`
	Degrades []DegradeEvent `json:"degrades,omitempty"`
}

func recordOrchestratorModels(name, round string, order ModelStep, runs []ModelRun, events []DegradeEvent) {
	v := orchestratorModels{Order: order, Running: degradeMark(order, runs), Models: runs, Degrades: events}
	_ = WriteJSON(filepath.Join(round, "models.json"), v)
	_ = WriteJSON(filepath.Join(OrchestratorDir(name), "models.json"), v)
}

// orchestratorModelMark is the model column of an orchestrator's ls row.
func orchestratorModelMark(name string) string {
	var v orchestratorModels
	if readJSON(filepath.Join(OrchestratorDir(name), "models.json"), &v) != nil {
		return ""
	}
	return v.Running
}

func orchestratorDegradeEvent(event string, s OrchestratorSpec, from ModelStep, to *ModelStep, hit *LimitHit, reason string) DegradeEvent {
	ev := DegradeEvent{Event: event, Run: "o-" + s.Name, Host: noticeHost(s), Step: "orchestrator", From: from, To: to, Reason: reason, Time: time.Now().UTC(), Reply: s.Reply}
	if hit != nil {
		ev.LimitType, ev.Message = hit.LimitType, hit.Message
		if !hit.ResetsAt.IsZero() {
			t := hit.ResetsAt
			ev.ResetsAt = &t
		}
	}
	return ev
}

// emitOrchestratorDegrade logs an event in the round's trace, runs the
// on_degrade hook, and queues and flushes the reply notice now, while the
// round continues on the next model. An exhausted chain is a needs-you
// attention notice; a degrade is a done-kind record.
func emitOrchestratorDegrade(s OrchestratorSpec, round string, d DegradeConfig, ev DegradeEvent, index int) {
	logDegrade(filepath.Join(round, "trace"), map[string]any{"type": "dispatch." + ev.Event, "degrade": ev})
	hook := runDegradeHook(d, ev, filepath.Join(round, "trace"))
	kind := attention.KindDone
	if ev.Event == "exhausted" {
		kind = attention.KindNeedsYou
	}
	key := fmt.Sprintf("%s:%s:%d", filepath.Base(round), ev.Event, index)
	att := orchestratorAttention(s, kind, key, "degraded", degradeMessage(ev))
	if att != nil && ev.Event == "exhausted" {
		att.State = "blocked"
	}
	if s.Reply != "" {
		st, _ := orchestratorState(s.Name)
		_ = queueOrchestratorNoticeWith(s, "lifecycle", st.Start.Format(time.RFC3339Nano)+":degrade:"+key, s.Reply, degradeMessage(ev), noticeOrchestrator(s, att))
	} else {
		noticeOrchestrator(s, att)
	}
	flushOrchestratorNotices(s, orchestratorNoticeSender)
	select {
	case <-hook:
	case <-time.After(degradeHookGrace):
	}
}
