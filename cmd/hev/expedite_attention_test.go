package main

import (
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

func TestExpediteAttention(t *testing.T) {
	now := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	// A merge to make is not a decision: a done-kind record, one per line a day.
	n, ok := expediteAttention("oven/kit-x: ready; [needs you] review authority requires your merge", now)
	if !ok || n.Event != attention.KindDone || n.Source != attention.SourceExpedite || n.Host != "oven" || n.Name != "kit-x" || n.Text != "review authority requires your merge" {
		t.Fatalf("%v %+v", ok, n)
	}
	again, _ := expediteAttention("oven/kit-x: ready; [needs you] review authority requires your merge", now.Add(time.Hour))
	if again.ID != n.ID {
		t.Fatal("same line the same day is one record")
	}
	for _, line := range []string{"oven/kit-x: [needs you] no PR", "oven/kit-x: [needs you] amend failed: exit 1"} {
		if n, ok := expediteAttention(line, now); !ok || n.Event != attention.KindDone {
			t.Fatalf("%q: %v %+v", line, ok, n)
		}
	}
	// An open decision raises needs-you, keyed by run and decision: the same
	// question on another day or round is the same notice.
	d, ok := expediteAttention("oven/kit-x: [needs you] needs-decision merge: squash or rebase?", now)
	if !ok || d.Event != attention.KindNeedsYou || d.Text != "needs-decision merge: squash or rebase?" {
		t.Fatalf("%v %+v", ok, d)
	}
	if later, _ := expediteAttention("oven/kit-x: waiting; [needs you] needs-decision merge: squash  or rebase?", now.Add(72*time.Hour)); later.ID != d.ID {
		t.Fatal("an unchanged decision is a new notice")
	}
	if changed, _ := expediteAttention("oven/kit-x: [needs you] needs-decision merge: squash, rebase or merge?", now); changed.ID == d.ID {
		t.Fatal("a changed question kept the ID")
	}
	if _, ok := expediteAttention("oven/kit-x: running", now); ok {
		t.Fatal("a plain status line is not a notice")
	}
}
