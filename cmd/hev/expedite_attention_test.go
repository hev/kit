package main

import (
	"testing"
	"time"

	"github.com/hev/kit/internal/attention"
)

func TestExpediteAttention(t *testing.T) {
	now := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	n, ok := expediteAttention("oven/kit-x: ready; [needs you] review authority requires your merge", now)
	if !ok || n.Source != attention.SourceExpedite || n.Host != "oven" || n.Name != "kit-x" || n.Text != "review authority requires your merge" {
		t.Fatalf("%v %+v", ok, n)
	}
	again, _ := expediteAttention("oven/kit-x: ready; [needs you] review authority requires your merge", now.Add(time.Hour))
	if again.ID != n.ID {
		t.Fatal("same request the same day is one notice")
	}
	if _, ok := expediteAttention("oven/kit-x: running", now); ok {
		t.Fatal("a plain status line is not a notice")
	}
}
