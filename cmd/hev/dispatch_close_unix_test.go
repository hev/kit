//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

const oldTrace = "0d98d96d1f5acba898a8b922b80f0a92"
const oldSpan = "f153bb771c2703d7"

// closeHost is a local host whose one slot a live d-busy session holds, so a
// new order is admitted into the queue without a worktree or worker.
func closeHost(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"TRACEPARENT", "HEV_TRACE_ID", "HEV_ORCHESTRATOR"} {
		t.Setenv(k, "")
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho d-busy\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[hosts.here]\nlocal = true\ncap = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// endedRun records an ended blocked run named name on this host.
func endedRun(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(dispatch.Root(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour)
	m := dispatch.Meta{Order: dispatch.Order{Name: name, Repo: "app", Host: "here"}, Branch: "d/" + name, Start: start, TraceID: oldTrace, SpanID: oldSpan}
	if err := dispatch.WriteJSON(filepath.Join(dir, "meta.json"), m); err != nil {
		t.Fatal(err)
	}
	r := dispatch.Result{Start: start, End: start.Add(time.Minute), Branch: m.Branch, Head: "abc", PR: "https://example.test/pr/1", Outcome: dispatch.OutcomeBlocked, Reason: "needs sudo", TraceID: oldTrace, SpanID: oldSpan}
	if err := dispatch.WriteJSON(filepath.Join(dir, "result.json"), r); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runCloseDispatch(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newDispatchCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func readClosed(t *testing.T, dir string) dispatch.Result {
	t.Helper()
	var r dispatch.Result
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err == nil {
		err = json.Unmarshal(b, &r)
	}
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func queuedRow(t *testing.T, name string) dispatch.Row {
	t.Helper()
	list, err := dispatch.Handle(dispatch.Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list.Rows {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("%s not listed: %+v", name, list.Rows)
	return dispatch.Row{}
}

func TestDispatchSupersedesClosesOldRunAndKeepsTrace(t *testing.T) {
	cfg := closeHost(t)
	oldDir := endedRun(t, "old")
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: new\nrepo: app\nhost: here\n---\nFinish it.\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runCloseDispatch(t, "--config", cfg, order, "--supersedes", "here/old")
	if err != nil || !strings.Contains(out, "queued new") || !strings.Contains(out, "closed old: superseded by here/new") || errOut != "" {
		t.Fatalf("%v\nout %s\nerr %s", err, out, errOut)
	}
	r := readClosed(t, oldDir)
	if dispatch.State(&r, false) != dispatch.StateClosed || r.SupersededBy != "here/new" || r.ClosedReason != "superseded by here/new" || r.Reason != "needs sudo" || r.PR != "https://example.test/pr/1" || r.Head != "abc" || r.TraceID != oldTrace {
		t.Fatalf("%+v", r)
	}
	row := queuedRow(t, "new")
	if row.TraceID != oldTrace || row.ParentSpanID == "" || len(row.Asks) != 1 || row.Asks[0].ParentSpanID != oldSpan {
		t.Fatalf("new run did not continue the old trace: %+v", row.Meta)
	}
}

func TestDispatchRunSupersedesFrontmatterAndExplicitTrace(t *testing.T) {
	cfg := closeHost(t)
	oldDir := endedRun(t, "old")
	other := "11111111111111111111111111111111"
	// dispatch run --supersedes with an explicit --trace keeps that trace.
	out, errOut, err := runCloseDispatch(t, "--config", cfg, "--host", "here", "run", "app", "finish", "--name", "new-run", "--supersedes", "old", "--trace", other)
	if err != nil || !strings.Contains(out, "closed old") {
		t.Fatalf("%v\nout %s\nerr %s", err, out, errOut)
	}
	if row := queuedRow(t, "new-run"); row.TraceID != other {
		t.Fatalf("explicit trace replaced: %+v", row.Meta)
	}
	if r := readClosed(t, oldDir); r.SupersededBy != "here/new-run" {
		t.Fatalf("%+v", r)
	}

	// supersedes: in the frontmatter does the same and never reaches the target.
	endedRun(t, "old2")
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: new2\nrepo: app\nhost: here\nsupersedes: old2\n---\nFinish it.\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err = runCloseDispatch(t, "--config", cfg, order); err != nil || !strings.Contains(out, "closed old2: superseded by here/new2") {
		t.Fatalf("%v\nout %s\nerr %s", err, out, errOut)
	}
	if row := queuedRow(t, "new2"); row.TraceID != oldTrace {
		t.Fatalf("frontmatter supersedes did not continue the trace: %+v", row.Meta)
	}
	b, _ := os.ReadFile(filepath.Join(dispatch.Root(), ".queue", "new2.json"))
	if strings.Contains(string(b), "supersedes:") {
		t.Fatalf("supersedes key reached the target: %s", b)
	}
}

// A replacement whose old run cannot be found or closed is still admitted,
// with a warning naming the close to run later.
func TestDispatchSupersedesWarnsWithoutFailing(t *testing.T) {
	cfg := closeHost(t)
	order := filepath.Join(t.TempDir(), "order.md")
	if err := os.WriteFile(order, []byte("---\nname: new\nrepo: app\nhost: here\n---\nFinish it.\n## Acceptance\n- Done\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runCloseDispatch(t, "--config", cfg, order, "--supersedes", "faraway/old")
	if err != nil || !strings.Contains(out, "queued new") {
		t.Fatalf("%v\nout %s\nerr %s", err, out, errOut)
	}
	if !strings.Contains(errOut, "Warning: here/new was admitted but closing superseded run faraway/old failed") || !strings.Contains(errOut, "hev dispatch close faraway/old --superseded-by here/new") {
		t.Fatalf("warning: %s", errOut)
	}
	if !dispatch.ValidTraceID(queuedRow(t, "new").TraceID) {
		t.Fatal("new run lost its own trace")
	}
	if _, _, err := runCloseDispatch(t, "--config", cfg, order, "--supersedes", "a/b/c"); err == nil || !strings.Contains(err.Error(), "--supersedes") {
		t.Fatalf("malformed supersedes: %v", err)
	}
}

func TestDispatchCloseCommand(t *testing.T) {
	cfg := closeHost(t)
	dir := endedRun(t, "old")
	out, errOut, err := runCloseDispatch(t, "--config", cfg, "close", "old", "merged", "by", "hand", "--reason", "Adam", "--superseded-by", "mini/old-b", "--json")
	if err != nil {
		t.Fatalf("%v %s %s", err, out, errOut)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got["state"] != "closed" || got["closed_reason"] != "Adam merged by hand" || got["superseded_by"] != "mini/old-b" || got["reason"] != "needs sudo" || got["host"] != "here" {
		t.Fatalf("%v", got)
	}
	if r := readClosed(t, dir); r.ClosedReason != "Adam merged by hand" {
		t.Fatalf("%+v", r)
	}
	// show prints the disposition; ls hides the run without --all.
	out, _, err = runCloseDispatch(t, "--config", cfg, "show", "old")
	if err != nil || !strings.Contains(out, "Closed ") || !strings.Contains(out, ": Adam merged by hand (superseded by mini/old-b)") {
		t.Fatalf("%v %s", err, out)
	}
	if out, _, err = runCloseDispatch(t, "--config", cfg, "ls"); err != nil || strings.Contains(out, "old ") {
		t.Fatalf("ls shows closed run: %v %s", err, out)
	}
	if out, _, err = runCloseDispatch(t, "--config", cfg, "ls", "--all"); err != nil || !strings.Contains(out, "closed: Adam merged by hand") {
		t.Fatalf("ls --all: %v %s", err, out)
	}
	if _, _, err = runCloseDispatch(t, "--config", cfg, "close", "old"); err == nil || !strings.Contains(err.Error(), "already closed") {
		t.Fatalf("second close: %v", err)
	}
	if _, _, err = runCloseDispatch(t, "--config", cfg, "close", "missing"); err == nil || !strings.Contains(err.Error(), "run missing not found") {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err = runCloseDispatch(t, "--config", cfg, "close", "elsewhere/old"); err == nil || !strings.Contains(err.Error(), "unknown host elsewhere") {
		t.Fatalf("unknown host: %v", err)
	}
}
