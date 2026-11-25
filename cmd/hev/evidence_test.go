package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/layerenv"
)

func evidenceHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, ".hev", "config.toml"))
	t.Setenv(evidence.DirEnv, "")
	t.Setenv("HEV_HOOKS", "off")
	t.Setenv("HEV_ORDER", "")
	// A dispatch shell can carry an archive key reference. The local fixture
	// must select its own storage mode independently of that environment.
	t.Setenv("HEV_KIT_SERVICE", "")
	t.Setenv("HEV_API_KEY", "")
	if err := os.Unsetenv("HEV_API_KEY"); err != nil {
		t.Fatal(err)
	}
	for name, aliases := range layerenv.Aliases {
		t.Setenv(name, "")
		for _, alias := range aliases {
			t.Setenv(alias, "")
		}
	}
	return home
}

func runEvidence(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := newEvidenceCommand()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func TestEvidenceCLIRoundTrip(t *testing.T) {
	home := evidenceHome(t)
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 390, 844)))
	shot := filepath.Join(home, "shot.png")
	os.WriteFile(shot, img.Bytes(), 0600)

	out, _, err := runEvidence(t, "", "put", "--title", "Checkout", "--source", "browser-use", "--order", "co-fix",
		"--tag", "Mobile", "--claim", "fits", "--verdict", "pass", "-m", "0 console errors", "--name", "console.log", shot, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if p["schema"] != evidence.Schema || p["state"] != "sealed" || p["order"] != "co-fix" {
		t.Fatalf("put json %s", out)
	}
	items := p["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["name"] != "001-shot.png" || items[1].(map[string]any)["name"] != "002-console.log" {
		t.Fatalf("items %v", items)
	}
	sealedID := p["id"].(string)
	if _, err := os.Stat(filepath.Join(home, ".hev", "evidence", sealedID, "manifest.json")); err != nil {
		t.Fatal(err)
	}

	// Incremental capture.
	out, _, err = runEvidence(t, "", "create", "--title", "Billing UAT", "--source", "computer-use")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out)
	if !evidence.ValidID(id) {
		t.Fatalf("create printed %q", out)
	}
	if _, _, err := runEvidence(t, "", "add", id, shot, "--caption", "before", "--url", "http://localhost:3000/billing", "--meta", "viewport=390x844"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEvidence(t, "POST /api/export 200 OK\n", "add", id, "-", "--name", "export.txt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEvidence(t, "", "add", id, "-m", "x", "--name", "../escape.txt"); err == nil {
		t.Fatal("traversal name accepted")
	}
	if _, _, err := runEvidence(t, "", "claim", id, "CSV export works", "--verdict", "pass"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEvidence(t, "", "seal", id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runEvidence(t, "", "add", id, "-m", "late"); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("add after seal: %v", err)
	}
	if _, _, err := runEvidence(t, "", "claim", id, "late"); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("claim after seal: %v", err)
	}

	// ls and show speak the wire shape.
	out, _, err = runEvidence(t, "", "ls", "--json", "--modality", "image", "--verdict", "pass")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data       []evidence.Summary `json:"data"`
		NextCursor *string            `json:"next_cursor"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list.Data) != 2 || list.Data[0].ID != id || list.NextCursor != nil {
		t.Fatalf("ls %v %s", err, out)
	}
	if strings.Contains(out, `"items"`) {
		t.Fatal("ls carries items; summaries do not")
	}
	out, _, _ = runEvidence(t, "", "ls", "--order", "co-fix")
	if !strings.Contains(out, sealedID) || strings.Contains(out, id) {
		t.Fatalf("ls --order %s", out)
	}
	out, _, err = runEvidence(t, "", "show", id)
	if err != nil || !strings.Contains(out, "001-shot.png") || !strings.Contains(out, "390x844") || !strings.Contains(out, "[pass] CSV export works") {
		t.Fatalf("show %v %s", err, out)
	}

	// Search: notice on stderr, JSON on stdout, Pro-only media refused.
	out, errOut, err := runEvidence(t, "", "search", "export", "--json")
	if err != nil || !strings.Contains(errOut, "semantic: false") {
		t.Fatalf("search %v %s", err, errOut)
	}
	var r evidence.SearchResponse
	if err := json.Unmarshal([]byte(out), &r); err != nil || len(r.Data) != 1 || r.Data[0].Package.ID != id || r.Semantic {
		t.Fatalf("search json %v %s", err, out)
	}
	for _, flag := range []string{"--image", "--video"} {
		_, _, err := runEvidence(t, "", "search", "x", flag, shot)
		if err == nil || !strings.Contains(err.Error(), "needs Pro") {
			t.Fatalf("%s: %v", flag, err)
		}
	}

	// get verifies; a tampered file fails non-zero and exports nothing.
	dest := filepath.Join(home, "out")
	if _, _, err := runEvidence(t, "", "get", id, "--out", dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "001-shot.png")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, ".hev", "evidence", id, "002-export.txt"), []byte("POST /api/export 500 ERR\n"), 0600)
	if _, _, err := runEvidence(t, "", "get", id, "--out", filepath.Join(home, "out2")); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("tampered get: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "out2")); err == nil {
		t.Fatal("tampered get wrote files")
	}
	if _, _, err := runEvidence(t, "", "get", id); err == nil {
		t.Fatal("tampered get without --out passed")
	}

	// Refused media writes nothing.
	svg := filepath.Join(home, "x.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600)
	before, _ := os.ReadDir(filepath.Join(home, ".hev", "evidence"))
	if _, _, err := runEvidence(t, "", "put", "--title", "bad", shot, svg); err == nil || !strings.Contains(err.Error(), "SVG") {
		t.Fatalf("svg put: %v", err)
	}
	after, _ := os.ReadDir(filepath.Join(home, ".hev", "evidence"))
	if len(after) != len(before) {
		t.Fatal("refused put left a package")
	}
}

func TestEvidenceClaimPairing(t *testing.T) {
	if _, err := claimInputs(nil, []string{"pass"}, nil); err == nil {
		t.Fatal("verdict without claim")
	}
	if _, err := claimInputs([]string{"a", "b", "c"}, []string{"pass", "fail"}, nil); err == nil {
		t.Fatal("mismatched verdicts")
	}
	got, err := claimInputs([]string{"a", "b"}, []string{"pass", "fail"}, []string{"crit"})
	if err != nil || got[0].Verdict != "pass" || got[1].Verdict != "fail" || got[1].Criterion != "crit" {
		t.Fatalf("%+v %v", got, err)
	}
}
