package main

import (
	"testing"

	"github.com/hev/kit/internal/trace"
)

func TestMatchPickerTrace(t *testing.T) {
	rows := []trace.SessionRow{
		{SessionID: "one", Workdir: "/project", Harness: "codex", Start: 103000},
		{SessionID: "two", Workdir: "/other", Harness: "codex", Start: 102000},
	}
	pane := pickerPane{Dir: "/project", Harness: "codex", AgentStarted: 100}
	if match := matchPickerTrace(pane, rows); match == nil || match.SessionID != "one" {
		t.Fatalf("unique launch match = %+v", match)
	}
	rows = append(rows, trace.SessionRow{SessionID: "three", Workdir: "/project", Harness: "codex", Start: 104000})
	if match := matchPickerTrace(pane, rows); match != nil {
		t.Fatalf("ambiguous launch matched %+v", match)
	}
	pane.Trace = "three"
	if match := matchPickerTrace(pane, rows); match == nil || match.SessionID != "three" {
		t.Fatalf("exact ID match = %+v", match)
	}
}
