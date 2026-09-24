package main

import (
	"reflect"
	"testing"
	"time"
)

func TestQueryFilterAndsEveryScope(t *testing.T) {
	defer func(p, w, h, s string) { queryPlan, queryWorkdir, queryHarness, querySince = p, w, h, s }(queryPlan, queryWorkdir, queryHarness, querySince)
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	queryPlan, queryWorkdir, queryHarness, querySince = "", "", "", ""
	if f, err := queryFilter(now); err != nil || f != nil {
		t.Fatalf("no flags: filter=%v err=%v", f, err)
	}

	queryHarness = "codex"
	if f, _ := queryFilter(now); !reflect.DeepEqual(f, []any{"harness", "Eq", "codex"}) {
		t.Errorf("one flag should be a bare clause, got %v", f)
	}

	queryPlan, querySince = "p1", "2d"
	f, err := queryFilter(now)
	want := []any{"And", []any{
		[]any{"plan", "Eq", "p1"},
		[]any{"harness", "Eq", "codex"},
		[]any{"ts", "Gte", "2026-09-22T12:00:00Z"},
	}}
	if err != nil || !reflect.DeepEqual(f, want) {
		t.Errorf("got %v err=%v, want %v", f, err, want)
	}
}
