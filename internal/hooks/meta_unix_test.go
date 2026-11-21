//go:build !windows

package hooks

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMetaEnvelopeAndObserveOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("CAPTURE", filepath.Join(home, "capture"))
	script := "#!/bin/sh\n/bin/cat >> \"$CAPTURE\"\nprintf '\\n' >> \"$CAPTURE\"\nprintf '%s' '{\"decision\":\"block\",\"reason\":\"fixture denial\"}'\n"
	if err := os.WriteFile(filepath.Join(home, "hev-hook-meta"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	orders := map[string]any{"name": "run", "repo": "kit", "worker_harness": "codex"}
	steers := map[string]any{"steer_id": "st-1", "target": "dispatch:run", "level": "urgent", "delivery": "tool-boundary"}
	admission := map[string]any{"name": "run", "repo": "kit", "action": "start"}
	cluster := map[string]any{"name": "member", "kind": "member"}
	peer := map[string]any{"peer": "mini/desk", "agent": "codex"}
	fixtures := map[string]map[string]any{
		"order-queued": orders, "order-started": orders,
		"order-rework": {"name": "run", "repo": "kit", "worker_harness": "codex", "action": "say"},
		"steer-sent":   steers, "steer-acked": steers, "steer-escalated": steers,
		"admission-check": admission, "admission-denied": admission,
		"cluster-join": cluster, "cluster-invite": cluster, "cluster-remove": cluster,
		"peer-online": peer, "peer-offline": peer,
		"board-reply": {"thread_id": 42, "entry_id": "p:42:2", "board": "floor", "author": "agent", "body_bytes": 10},
		"board-pin":   {"thread_id": 42},
	}
	if len(fixtures) != len(MetaEvents) {
		t.Fatal("fixture coverage drift")
	}
	for _, name := range MetaEvents {
		t.Run(name, func(t *testing.T) {
			capture := os.Getenv("CAPTURE")
			log := filepath.Join(home, ".hev", "hooks", "events.jsonl")
			for _, path := range []string{capture, log} {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			data, ok := fixtures[name]
			if !ok {
				t.Fatal("missing event fixture")
			}
			input := maps.Clone(data)
			input["path"] = "/private/project"
			input["token"] = "ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
			r := Meta(context.Background(), name, input, "")
			want := Continue
			if name == "admission-check" {
				want = Block
			}
			if r.Decision != want {
				t.Fatalf("decision = %q, want %q", r.Decision, want)
			}
			for _, path := range []string{capture, log} {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var ev Event
				if err := json.Unmarshal(raw, &ev); err != nil {
					t.Fatalf("expected exactly one event in %s: %v", filepath.Base(path), err)
				}
				if ev.Event != name || ev.Harness != "kit" || ev.Schema != Schema || ev.Host == "" ||
					ev.Data["path"] != "[redacted]" || strings.Contains(string(raw), "/private/project") || strings.Contains(string(raw), "AbCdEfGh") {
					t.Fatalf("unexpected event: %s", raw)
				}
				delete(ev.Data, "path")
				delete(ev.Data, "token")
				wantJSON, _ := json.Marshal(data)
				var wantData map[string]any
				if err := json.Unmarshal(wantJSON, &wantData); err != nil || !reflect.DeepEqual(ev.Data, wantData) {
					t.Fatalf("data = %v, want %v", ev.Data, wantData)
				}
			}
		})
	}
}
