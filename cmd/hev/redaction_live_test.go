package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/redact/fixture"
	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/trace"
)

// Opt in with HEV_REDACTION_LIVE_CONFIG pointing to a real gateway config.
// Only endpoint/key/store are borrowed; all writes use a newly generated
// namespace, source roots, HOME, config and migration journal. No launchd calls.
func TestLiveRedactionAcceptance(t *testing.T) {
	target := os.Getenv("HEV_REDACTION_LIVE_CONFIG")
	if target == "" {
		t.Skip("set HEV_REDACTION_LIVE_CONFIG for real-store acceptance")
	}
	t.Setenv("HEV_CONFIG", target)
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerAPIKey == "" {
		cfg.LayerAPIKey, err = daemon.LayerKey()
		if err != nil {
			t.Fatal("target credential lookup failed")
		}
	}
	if cfg.LayerEndpoint == "" || cfg.LayerAPIKey == "" {
		t.Fatal("target must supply endpoint and credential")
	}
	samples, err := fixture.Samples()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	binary := filepath.Join(home, "hev")
	build := exec.Command("go", "build", "-o", binary, ".")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, b)
	}
	t.Setenv("HOME", home)
	for _, k := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_EMBED_MODEL"} {
		t.Setenv(k, "")
	}
	t.Setenv("HEV_HOST", "redaction-acceptance")
	t.Setenv("HEV_NO_HINTS", "1")
	ns := fmt.Sprintf("kit-redact-%d", time.Now().UnixNano())
	configPath := filepath.Join(home, ".hev", "config.toml")
	mustMkdir(t, filepath.Dir(configPath))
	t.Setenv("HEV_CONFIG", configPath)
	salt := strings.Repeat("01", 32)
	writeConfig := func(enabled bool) {
		mustWrite(t, configPath, []byte(fmt.Sprintf("[layer]\nendpoint=%q\napi_key=%q\nnamespace=%q\nstore=%q\n[capture]\nredact=%t\nredact_salt=%q\nscan_interval='1h'\n", cfg.LayerEndpoint, cfg.LayerAPIKey, ns, cfg.LayerStore, enabled, salt)))
	}
	writeConfig(false)
	cl, err := layer.New(cfg.LayerEndpoint, cfg.LayerAPIKey, ns, "").WithStore(layer.StoreKind(cfg.LayerStore))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("lane=%s namespace=%s synthetic_samples=%d", cl.Caps.Store.Kind, ns, len(samples))
	// Delete only the newly generated acceptance namespaces, even on failure.
	t.Cleanup(func() {
		for _, suffix := range []string{"", "-blocks", "-sessions", "-evals"} {
			req, _ := http.NewRequest("DELETE", cl.Endpoint+"/v2/namespaces/"+ns+suffix, nil)
			req.Header.Set("Authorization", "Bearer "+cl.APIKey)
			resp, err := cl.HTTP.Do(req)
			if err != nil {
				t.Errorf("cleanup %s: %v", suffix, err)
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 && resp.StatusCode != 204 && resp.StatusCode != 404 {
				t.Errorf("cleanup %s status=%d", suffix, resp.StatusCode)
			}
		}
	})
	sources := writeSecretSessions(t, home, samples)
	st := &index.State{Units: map[string]string{}}
	rawIDs := map[string]bool{}
	expected := redact.Counts{}
	scrubber, err := redact.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	rawFiles := map[string][]byte{}
	for _, src := range sources {
		units, err := src.Units()
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range units {
			rawFiles[u.Key], err = os.ReadFile(u.Key)
			if err != nil {
				t.Fatal(err)
			}
			turns, err := src.Read(u)
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != len(samples)*3 {
				t.Fatalf("parser lost payloads: %d", len(turns))
			}
			expected.Add(scrubber.Turns(turns))
		}
		if os.Getenv("HEV_REDACTION_LEGACY_BINARY") == "" {
			report, err := index.Run(src, cl, st, index.Options{Tiers: trace.AllTiers[:], RepoURL: func(string) string { return "" }})
			checkReport(t, report, err)
		}
	}
	if legacy := os.Getenv("HEV_REDACTION_LEGACY_BINARY"); legacy != "" {
		cmd := exec.Command(legacy, "index", "--tier", "all")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("legacy seed: %v: %s", err, b)
		} else {
			t.Logf("legacy seed: %s", strings.TrimSpace(string(b)))
		}
		st = index.LoadState()
		for path := range rawFiles {
			if st.Units[path] == "" {
				t.Fatal("legacy seed did not persist source signatures")
			}
		}
	} else if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	legacySessions, err := cl.ListSessionRows(-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacySessions) != 2 {
		t.Fatalf("legacy sessions=%d", len(legacySessions))
	}
	for i := range legacySessions {
		legacySessions[i].Summary = samples[len(samples)-3].Text
	}
	if _, err := cl.PatchSessionSummaries(legacySessions); err != nil {
		t.Fatal(err)
	}
	seeded, err := cl.ListSessionRows(-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range seeded {
		if row.Summary != samples[len(samples)-3].Text {
			t.Fatal("legacy raw summary was not stored")
		}
	}
	for _, id := range []string{"redaction-claude", "redaction-codex"} {
		rows, err := cl.SessionRows(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatal("raw seed missing chunks")
		}
		for _, r := range rows {
			rawIDs[r.ID] = true
		}
	}
	cli := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, args...)
		b, err := cmd.Output()
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				t.Fatalf("hev %s: %s", args[0], e.Stderr)
			}
			t.Fatal(err)
		}
		return b
	}
	rawTrace := cli("trace", "--json", "redaction-claude")
	if !bytes.Contains(rawTrace, []byte(samples[0].Secret)) {
		t.Fatal("opt-out positive control did not expose raw secret")
	}
	// Default options intentionally omit tool results: migration must rebuild
	// ALL legacy tiers despite unchanged source signatures and this restriction.
	writeConfig(true)
	migrated := cli("index")
	t.Logf("first enabled CLI scan: %s", strings.TrimSpace(string(migrated)))
	dashboard := httptest.NewServer(serve.New(cl).WithCacheTTL(0).Handler())
	defer dashboard.Close()
	get := func(path string) []byte {
		t.Helper()
		resp, err := http.Get(dashboard.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("dashboard %s: status=%d", strings.Split(path, "?")[0], resp.StatusCode)
		}
		return b
	}
	assertClean := func(b []byte) {
		t.Helper()
		for _, s := range samples {
			if containsOriginal(b, s.Secret) {
				t.Fatalf("retrievable original for %s", s.Rule)
			}
		}
	}
	markerRE := regexp.MustCompile(`\[REDACTED:[a-zA-Z0-9_-]+#[0-9a-f]{16}\]`)
	fingerprints := map[string]bool{}
	for _, id := range []string{"redaction-claude", "redaction-codex"} {
		rows, err := cl.SessionRows(id)
		if err != nil {
			t.Fatal(err)
		}
		tiers := map[string]bool{}
		for _, r := range rows {
			if rawIDs[r.ID] {
				t.Fatal("legacy raw chunk ID survived")
			}
			tiers[r.Tier] = true
		}
		if len(tiers) != 3 {
			t.Fatalf("upgrade omitted tiers: %v", tiers)
		}
		b := cli("trace", "--json", id)
		assertClean(b)
		for _, m := range markerRE.FindAllString(string(b), -1) {
			fingerprints[m] = true
		}
		b = get("/api/session/" + id)
		assertClean(b)
		if !bytes.Contains(b, []byte("[REDACTED:")) {
			t.Fatal("dashboard detail empty")
		}
	}
	for _, path := range []string{"/", "/api/sessions?window=all", "/api/stats?window=all", "/api/values?window=all&facet=tool"} {
		assertClean(get(path))
	}
	// Exercise every original as a real hybrid search. Semantic hits are allowed;
	// no returned content may contain originals. Query --json prevents truncation.
	for i, s := range samples {
		assertClean(cli("query", "--json", "--top", "10", "--", "credential "+s.Secret))
		assertClean(get("/api/search?window=all&top=10&q=" + url.QueryEscape("credential "+s.Secret)))
		if (i+1)%40 == 0 {
			t.Logf("original retrieval checks %d/%d", i+1, len(samples))
		}
	}
	// Verify every distinct marker is searchable, rather than assuming the
	// archive's tokenizer indexes '#' and punctuation as literal strings.
	nMarkers := 0
	for m := range fingerprints {
		b := cli("query", "--json", "--top", "100", m)
		if !bytes.Contains(b, []byte(m)) {
			t.Fatalf("marker not searchable: %s", m)
		}
		assertClean(b)
		nMarkers++
		if nMarkers%40 == 0 {
			t.Logf("marker retrieval checks %d/%d", nMarkers, len(fingerprints))
		}
	}
	t.Logf("%d distinct keyed markers searchable", len(fingerprints))
	// An unchanged second scan must preserve IDs and keyed markers.
	second := cli("index")
	if !bytes.Contains(second, []byte("0 indexed, 2 unchanged")) {
		t.Fatalf("second scan not skipped: %s", second)
	}
	for path, want := range rawFiles {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("source transcript changed")
		}
	}
	saved, err := redact.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Identity() != scrubber.Identity() {
		t.Fatal("persisted salt changed")
	}
	// Run the actual daemon against the isolated HOME to verify persisted scan
	// counts and their display in hev s. Remove only this fixture's state.
	if err := index.ResetState(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "daemon")
	var log bytes.Buffer
	cmd.Stderr = &log
	cmd.Stdout = &log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var status daemon.Status
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		status, err = daemon.ReadStatus()
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cmd.Process.Signal(os.Interrupt)
	waitErr := cmd.Wait()
	if err != nil || waitErr != nil {
		t.Fatalf("daemon did not complete: status=%v wait=%v", err, waitErr)
	}
	if status.LastError != "" {
		t.Fatalf("daemon: %s", status.LastError)
	}
	if !reflect.DeepEqual(status.Redactions, expected) {
		t.Fatalf("daemon counts got=%v want=%v", status.Redactions, expected)
	}
	output := cli("s")
	for rule, n := range expected {
		if !bytes.Contains(output, []byte(fmt.Sprintf("  %s: %d", rule, n))) {
			t.Fatalf("hev s missing count %s", rule)
		}
	}
	total := 0
	for _, n := range expected {
		total += n
	}
	t.Logf("hev s verified %d replacements across %d winning rules", total, len(expected))
	if dir := os.Getenv("HEV_REDACTION_EVIDENCE_DIR"); dir != "" {
		mustMkdir(t, dir)
		evidence := map[string]any{"lane": cl.Caps.Store.Kind, "namespace": ns, "samples": len(samples), "turns": len(samples) * 6, "searchable_markers": len(fingerprints), "replacement_total": total, "counts": status.Redactions, "policy": redact.Version, "legacy_binary": os.Getenv("HEV_REDACTION_LEGACY_BINARY") != ""}
		data, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, cl.Caps.Store.Kind+"-counts.json"), data)
	}
	if script := os.Getenv("HEV_REDACTION_BROWSER_SCRIPT"); script != "" {
		specimens := filepath.Join(home, "specimens.json")
		data, err := json.Marshal(samples)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, specimens, data)
		browser := exec.Command("node", script, dashboard.URL, specimens, cl.Caps.Store.Kind)
		if b, err := browser.CombinedOutput(); err != nil {
			t.Fatalf("browser: %v: %s", err, b)
		} else {
			t.Logf("browser: %s", strings.TrimSpace(string(b)))
		}
	}
	// Explicit opt-out after migration invalidates completion and allows raw
	// storage; re-enable must remove that content again.
	writeConfig(false)
	cli("index", "--force", "--tier", "all")
	rawTrace = cli("trace", "--json", "redaction-claude")
	if !bytes.Contains(rawTrace, []byte(samples[0].Secret)) {
		t.Fatal("opt-out after upgrade ineffective")
	}
	writeConfig(true)
	cli("index")
	for _, id := range []string{"redaction-claude", "redaction-codex"} {
		assertClean(cli("trace", "--json", id))
		assertClean(get("/api/session/" + id))
	}
	t.Log("opt-out/re-enable, source immutability, unchanged signatures, salt persistence and legacy chunk removal passed")
}

func containsOriginal(b []byte, secret string) bool {
	// Check decoded JSON string values as well as JSON embedded in tool text.
	var v any
	if json.Unmarshal(b, &v) == nil {
		// Dashboard search echoes the submitted query. This is caller input,
		// not retrieved archive data; inspect every returned row and snippet.
		if response, ok := v.(map[string]any); ok {
			delete(response, "q")
		}
		return originalInValue(v, secret)
	}
	return strings.Contains(string(b), secret)
}
func originalInValue(v any, secret string) bool {
	switch v := v.(type) {
	case string:
		encoded, _ := json.Marshal(secret)
		return strings.Contains(v, secret) || strings.Contains(v, string(encoded[1:len(encoded)-1]))
	case []any:
		for _, x := range v {
			if originalInValue(x, secret) {
				return true
			}
		}
	case map[string]any:
		for _, x := range v {
			if originalInValue(x, secret) {
				return true
			}
		}
	}
	return false
}
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
}
func mustWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func checkReport(t *testing.T, r *index.Report, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Errors) > 0 {
		t.Fatal(r.Errors)
	}
}

func writeSecretSessions(t *testing.T, home string, samples []fixture.Sample) []trace.Source {
	t.Helper()
	claudeRoot := filepath.Join(home, ".claude", "projects")
	codexRoot := filepath.Join(home, ".codex", "sessions")
	mustMkdir(t, claudeRoot)
	mustMkdir(t, codexRoot)
	var claude, codex bytes.Buffer
	ce := json.NewEncoder(&claude)
	xe := json.NewEncoder(&codex)
	ts := time.Now().UTC()
	xe.Encode(map[string]any{"type": "session_meta", "timestamp": ts.Format(time.RFC3339), "payload": map[string]any{"id": "redaction-codex", "cwd": home, "model": "test-model"}})
	for i, s := range samples {
		for j := 0; j < 3; j++ {
			stamp := ts.Add(time.Duration(i*3+j) * time.Second).Format(time.RFC3339)
			id := fmt.Sprintf("fixture-%d-%d", i, j)
			role := []string{"user", "assistant", "user"}[j]
			text := s.Text // one rule's specimen in each prompt, command, env result
			var block map[string]any
			switch j {
			case 0:
				block = map[string]any{"type": "text", "text": text}
			case 1:
				block = map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]string{"command": text}}
			case 2:
				block = map[string]any{"type": "tool_result", "tool_use_id": fmt.Sprintf("fixture-%d-1", i), "content": "env output:\n" + text}
			}
			if err := ce.Encode(map[string]any{"type": role, "uuid": id, "sessionId": "redaction-claude", "timestamp": stamp, "cwd": home, "message": map[string]any{"role": role, "content": []any{block}}}); err != nil {
				t.Fatal(err)
			}
			p := map[string]any{"id": id}
			switch j {
			case 0:
				p["type"] = "message"
				p["role"] = "user"
				p["content"] = []any{map[string]any{"type": "input_text", "text": text}}
			case 1:
				p["type"] = "function_call"
				p["name"] = "shell_command"
				p["call_id"] = id
				arguments, _ := json.Marshal(map[string]string{"command": text})
				p["arguments"] = string(arguments)
			case 2:
				p["type"] = "function_call_output"
				p["call_id"] = fmt.Sprintf("fixture-%d-1", i)
				p["output"] = "env output:\n" + text
			}
			if err := xe.Encode(map[string]any{"type": "response_item", "timestamp": stamp, "payload": p}); err != nil {
				t.Fatal(err)
			}
		}
	}
	mustWrite(t, filepath.Join(claudeRoot, "fixture.jsonl"), claude.Bytes())
	mustWrite(t, filepath.Join(codexRoot, "fixture.jsonl"), codex.Bytes())
	return []trace.Source{&trace.ClaudeSource{Root: claudeRoot}, &trace.CodexSource{Root: codexRoot}}
}

func TestRedactionParsedFixtureCoverage(t *testing.T) {
	samples, err := fixture.Samples()
	if err != nil {
		t.Fatal(err)
	}
	sources := writeSecretSessions(t, t.TempDir(), samples)
	s, err := redact.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range sources {
		units, err := src.Units()
		if err != nil {
			t.Fatal(err)
		}
		turns, err := src.Read(units[0])
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != len(samples)*3 {
			t.Fatalf("turns=%d", len(turns))
		}
		for i, sample := range samples {
			for j := 0; j < 3; j++ {
				t.Run(fmt.Sprintf("%s/%s/%d", turns[0].Harness, sample.Rule, j), func(t *testing.T) {
					turn := turns[i*3+j]
					before, _ := json.Marshal(turn)
					if !containsOriginal(before, sample.Secret) {
						t.Fatal("fixture original absent before scrubbing")
					}
					s.Turns([]trace.Turn{turn}) // Blocks share their backing array.
					after, _ := json.Marshal(turn)
					if containsOriginal(after, sample.Secret) {
						t.Fatal("original survives parsed payload scrubbing")
					}
				})
			}
		}
	}
}
