//go:build !windows

package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hev/kit/internal/hooks"
	"github.com/hev/kit/internal/testenv"
)

// Rule: evidence-created, evidence-updated and evidence-sealed reach
// hev-hook-NAME handlers and events.jsonl with redacted metadata, and
// never carry item bytes or text.
func TestHooksFireWithMetadataOnly(t *testing.T) {
	home, bin, out := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv("HEV_HOOKS", "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	exe := filepath.Join(bin, "hev-hook-record")
	script := "#!/bin/sh\n[ \"$1\" = " + testenv.PrimeArg + " ] && exit 0\n" +
		`n=$(ls "` + out + `" | wc -l | tr -d ' '); cat > "` + out + `/$n-$1.json"` + "\n"
	if err := os.WriteFile(exe, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	testenv.PrimeExec(t, nil, exe)

	s := testStore(t)
	s.Observe = HookObserver(context.Background())
	secret := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	img := pngOf(t, 32, 32)
	textBody := "unique-text-body-marker " + secret
	aws := "AKIA" + "ABCDEFGHIJKLMNOP"
	p, err := s.Create(Meta{Title: "hooked " + secret, Source: "browser-use", Order: "order-" + aws, Branch: secret, Author: aws})
	if err != nil {
		t.Fatal(err)
	}
	// The validator's case: a secret-pattern item name reached
	// evidence-created's metadata. Names, metadata keys and values too.
	_, item, err := s.AddItem(p.ID, ItemInput{Data: img, Name: "shot-" + secret + ".png", Caption: "cap " + secret,
		Metadata: map[string]string{"key-" + secret: aws, "viewport": "390x844"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddItem(p.ID, ItemInput{Data: []byte(textBody), Name: "log.txt"}); err != nil {
		t.Fatal(err)
	}
	_, claim, err := s.AddClaim(p.ID, ClaimInput{Text: "claim " + secret, Verdict: "pass", ItemIDs: []string{item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Seal(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(Meta{Title: "one-shot", Session: secret}, []ItemInput{{Data: img, Name: secret + ".png", Metadata: map[string]string{aws: secret}}}, nil); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool { return seqOf(names[i]) < seqOf(names[j]) })
	var events []string
	var payloads [][]byte
	for _, n := range names {
		b, _ := os.ReadFile(filepath.Join(out, n))
		payloads = append(payloads, b)
		events = append(events, strings.SplitN(strings.TrimSuffix(n, ".json"), "-", 2)[1])
	}
	want := []string{"evidence-created", "evidence-updated", "evidence-updated", "evidence-updated", "evidence-sealed", "evidence-created"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events %v, want %v", events, want)
	}
	log, _ := os.ReadFile(filepath.Join(home, ".hev", "hooks", "events.jsonl"))
	if n := bytes.Count(log, []byte("\n")); n != len(want) {
		t.Fatalf("events.jsonl has %d lines:\n%s", n, log)
	}
	forbidden := [][]byte{[]byte(secret), []byte(aws), img[:16], []byte(base64.StdEncoding.EncodeToString(img)[:24]), []byte("unique-text-body-marker")}
	for i, b := range append(payloads, log) {
		for _, f := range forbidden {
			if bytes.Contains(b, f) {
				t.Fatalf("payload %d carries %q:\n%s", i, f, b)
			}
		}
	}
	// The item is still described, under its redacted name.
	if !bytes.Contains(payloads[1], []byte(`"name":"001-shot--REDACTED-`)) {
		t.Fatalf("evidence-updated lost the redacted item name:\n%s", payloads[1])
	}
	var sealed struct {
		Schema, Event, Harness string
		Package                Summary
		Items                  []map[string]any
		AddedItemIDs           []string `json:"added_item_ids"`
	}
	if err := json.Unmarshal(payloads[4], &sealed); err != nil {
		t.Fatal(err)
	}
	if sealed.Schema != "hev.evidence.event/v1" || sealed.Harness != "kit" || sealed.Package.State != "sealed" ||
		sealed.Package.ItemCount.Total != 2 || len(sealed.Items) != 2 || len(sealed.Package.Claims) != 1 ||
		sealed.Package.Claims[0].ID != claim.ID || !strings.Contains(sealed.Package.Claims[0].Text, "[REDACTED:") {
		t.Fatalf("sealed payload %s", payloads[4])
	}
	for _, it := range sealed.Items {
		if _, ok := it["text"]; ok {
			t.Fatalf("item carries text: %v", it)
		}
	}
	var updated struct {
		AddedItemIDs []string `json:"added_item_ids"`
	}
	json.Unmarshal(payloads[1], &updated)
	if !reflect.DeepEqual(updated.AddedItemIDs, []string{item.ID}) {
		t.Fatalf("updated payload %s", payloads[1])
	}
	// The payload matches the published schema's properties.
	checkSchemaFields(t, payloads[4])

	// The family pattern names every evidence event and nothing else.
	for _, e := range hooks.EvidenceEvents {
		if !hooks.MatchEvent("evidence-*", e) {
			t.Errorf("evidence-* misses %s", e)
		}
	}
	for _, e := range []string{"board-post", "evidence", "evidence-", "evidencex-created", "needs-you"} {
		if hooks.MatchEvent("evidence-*", e) {
			t.Errorf("evidence-* matches %s", e)
		}
	}
	if !hooks.MatchEvent("board-post", "board-post") || hooks.MatchEvent("board-post", "board-posts") {
		t.Error("exact names")
	}

	// HEV_HOOKS=off raises nothing.
	t.Setenv("HEV_HOOKS", "off")
	before := len(names)
	s.Put(Meta{Title: "quiet"}, nil, nil)
	if after, _ := os.ReadDir(out); len(after) != before {
		t.Fatal("HEV_HOOKS=off still ran handlers")
	}
}

func seqOf(name string) int {
	n := 0
	for _, c := range name {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func checkSchemaFields(t *testing.T, payload []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "evidence_event.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Required   []string
		Properties map[string]json.RawMessage
	}
	json.Unmarshal(raw, &spec)
	var fields map[string]any
	json.Unmarshal(payload, &fields)
	for _, k := range spec.Required {
		if _, ok := fields[k]; !ok {
			t.Errorf("payload lacks required %q", k)
		}
	}
	for k := range fields {
		if _, ok := spec.Properties[k]; !ok {
			t.Errorf("payload field %q is not in the schema", k)
		}
	}
}
