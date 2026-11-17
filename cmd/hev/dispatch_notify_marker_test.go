package main

import (
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

// A failed completion notification shows in ls's ERROR column and in show.
func TestNotifyFailureMarkers(t *testing.T) {
	failed := "ssh: Could not resolve hostname adams-macbook-air\nmore"
	for _, c := range []struct {
		row  dispatch.Row
		want string
	}{
		{dispatch.Row{NotifyError: failed}, "notify failed: ssh: Could not resolve hostname adams-macbook-air"},
		{dispatch.Row{Error: "exit 1\ntrace", NotifyError: failed}, "exit 1; notify failed: ssh: Could not resolve hostname adams-macbook-air"},
		{dispatch.Row{Error: "exit 1"}, "exit 1"},
		{dispatch.Row{}, ""},
	} {
		if got := rowError(c.row); got != c.want {
			t.Fatalf("rowError(%+v) = %q, want %q", c.row, got, c.want)
		}
	}
	if got := notifyFailure(`{"exit_code":0,"notify_error":"ssh: Could not resolve hostname adams-macbook-air"}`); got != "notify failed: ssh: Could not resolve hostname adams-macbook-air" {
		t.Fatalf("show marker = %q", got)
	}
	if got := notifyFailure(`{"exit_code":0}`); got != "" {
		t.Fatalf("clean show marker = %q", got)
	}
}
