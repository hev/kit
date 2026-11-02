//go:build !windows

package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/expedite"
)

func TestExpediteGHAdapterHeadAndDraft(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GH_FAKE_LOG"
case "$*" in
 *statusCheckRollup*) if [ "$GH_FAKE_PENDING" = yes ]; then printf '%s\n' '{"headRefOid":"head","state":"OPEN","statusCheckRollup":[{"status":"IN_PROGRESS","conclusion":""}]}'; exit 0; fi; printf '%s\n' '{"headRefOid":"head","state":"OPEN","statusCheckRollup":[{"status":"COMPLETED","conclusion":"SUCCESS"}]}' ;;
 *headRefOid,isDraft*) printf '{"headRefOid":"%s","isDraft":true}\n' "$GH_FAKE_HEAD" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_FAKE_LOG", log)
	t.Setenv("GH_FAKE_HEAD", "head")
	old := expediteRPC
	t.Cleanup(func() { expediteRPC = old })
	expediteRPC = func(_ context.Context, _ dispatch.Host, q dispatch.Request) (dispatch.Response, error) {
		if q.Action != "list" {
			t.Fatal(q.Action)
		}
		return dispatch.Response{Rows: []dispatch.Row{{Meta: dispatch.Meta{Order: dispatch.Order{Name: "task"}}, State: "done", PR: "https://example.com/pull/1"}}}, nil
	}
	d := &expediteDriver{cfg: dispatch.Config{Hosts: map[string]dispatch.Host{"worker": {Local: true}}}, out: io.Discard}
	run := expedite.Run{Host: "worker", Name: "task"}
	row, pr, err := d.Inspect(context.Background(), run)
	if err != nil || !pr.Green() {
		t.Fatal(row, pr, err)
	}
	if err = d.Merge(context.Background(), run, row.PR, pr.Head); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	s := string(b)
	if !strings.Contains(s, "pr ready https://example.com/pull/1") || !strings.Contains(s, "pr merge https://example.com/pull/1 --squash --match-head-commit head") {
		t.Fatal(s)
	}
	if err = os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_FAKE_HEAD", "new-head")
	if err = d.Merge(context.Background(), run, row.PR, "head"); err == nil {
		t.Fatal("changed head merged")
	}
	b, _ = os.ReadFile(log)
	if strings.Contains(string(b), "pr ready") || strings.Contains(string(b), "pr merge") {
		t.Fatal(string(b))
	}
	// A newly pending check after draft readiness blocks the merge command.
	if err = os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_FAKE_HEAD", "head")
	t.Setenv("GH_FAKE_PENDING", "yes")
	if err = d.Merge(context.Background(), run, row.PR, "head"); err == nil {
		t.Fatal("pending CI after readiness merged")
	}
	b, _ = os.ReadFile(log)
	if !strings.Contains(string(b), "pr ready") || strings.Contains(string(b), "pr merge") {
		t.Fatal(string(b))
	}
}
