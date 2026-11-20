package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeFile(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// tree lists every path under root, relative, with hidden lock files.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && p != root {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestRootResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(DirEnv, "")
	cfg := filepath.Join(home, ".hev", "config.toml")
	t.Setenv("HEV_CONFIG", cfg)
	if got := Root(); got != filepath.Join(home, ".hev", "evidence") {
		t.Fatalf("default root %s", got)
	}
	os.MkdirAll(filepath.Dir(cfg), 0700)
	os.WriteFile(cfg, []byte("[evidence]\ndir = \"~/proof\"\n"), 0600)
	if got := Root(); got != filepath.Join(home, "proof") {
		t.Fatalf("configured root %s", got)
	}
	t.Setenv(DirEnv, filepath.Join(home, "env"))
	if got := Root(); got != filepath.Join(home, "env") {
		t.Fatalf("env root %s", got)
	}
	// A broken config means the default, never a failure.
	t.Setenv(DirEnv, "")
	os.WriteFile(cfg, []byte("[evidence\n"), 0600)
	if got := Root(); got != filepath.Join(home, ".hev", "evidence") {
		t.Fatalf("broken config root %s", got)
	}
}

func TestPutLayoutAndModes(t *testing.T) {
	s := testStore(t)
	in := t.TempDir()
	shot := writeFile(t, in, "checkout 390px.png", pngOf(t, 390, 844))
	clip := writeFile(t, in, "repro.mp4", mp4Of("isom", 14200, 390, 844))
	log := writeFile(t, in, "console.log", []byte("POST /api/checkout 200 OK\n0 console errors\n"))
	p, err := s.Put(Meta{Title: "Checkout mobile", Source: "browser-use", Order: "checkout-fix", Tags: []string{"Mobile", "mobile", " validation "}},
		[]ItemInput{{Path: shot, Caption: "before submit"}, {Path: clip}, {Path: log}},
		[]ClaimInput{{Text: "Submit stays in viewport", Verdict: "pass", Criterion: "no horizontal scroll"}})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(p.ID) || p.State != "sealed" || p.SealedAt == nil || p.Schema != Schema {
		t.Fatalf("package %+v", p)
	}
	if strings.Join(p.Tags, ",") != "mobile,validation" {
		t.Fatalf("tags %v", p.Tags)
	}
	if p.ItemCount != (Counts{Total: 3, Text: 1, Image: 1, Video: 1}) {
		t.Fatalf("counts %+v", p.ItemCount)
	}
	want := []string{".lock", p.ID, p.ID + "/001-checkout-390px.png", p.ID + "/002-repro.mp4", p.ID + "/003-console.log", p.ID + "/manifest.json"}
	if got := tree(t, s.Root); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("tree %v\nwant %v", got, want)
	}
	img, vid, txt := p.Items[0], p.Items[1], p.Items[2]
	if img.MIME != "image/png" || img.Width != 390 || img.Height != 844 || img.Caption != "before submit" {
		t.Fatalf("image %+v", img)
	}
	if vid.MIME != "video/mp4" || vid.DurationMs == nil || *vid.DurationMs != 14200 {
		t.Fatalf("video %+v", vid)
	}
	if txt.Text == nil || *txt.Text != "POST /api/checkout 200 OK\n0 console errors\n" || *txt.LineCount != 2 || !txt.Redacted {
		t.Fatalf("text %+v", txt)
	}
	if runtime.GOOS != "windows" {
		for _, rel := range want[1:] {
			info, _ := os.Stat(filepath.Join(s.Root, rel))
			mode := info.Mode().Perm()
			if info.IsDir() && mode != 0700 || !info.IsDir() && mode != 0600 {
				t.Errorf("%s mode %o", rel, mode)
			}
		}
		if info, _ := os.Stat(s.Root); info.Mode().Perm() != 0700 {
			t.Errorf("root mode %o", info.Mode().Perm())
		}
	}
	// The manifest on disk is the wire shape.
	raw, _ := os.ReadFile(filepath.Join(s.Root, p.ID, ManifestName))
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema", "id", "state", "title", "item_counts", "items", "claims", "tags", "created_at", "updated_at", "sealed_at"} {
		if _, ok := wire[k]; !ok {
			t.Errorf("manifest lacks %q", k)
		}
	}
	got, err := s.Load(p.ID)
	if err != nil || got.Items[1].SHA256 != vid.SHA256 {
		t.Fatalf("load %v", err)
	}
}

// Rule: a refused item writes nothing: no package folder, no partial
// file, and (for one-shot put) no package at all.
func TestRefusedItemsWriteNothing(t *testing.T) {
	s := testStore(t)
	in := t.TempDir()
	good := writeFile(t, in, "ok.png", pngOf(t, 4, 4))
	for name, b := range map[string][]byte{
		"x.svg":    []byte(`<svg onload="alert(1)"/>`),
		"long.mp4": mp4Of("isom", 61000, 8, 8),
		"cut.webm": webmOf("webm", 1000, 8, 8)[:30],
		"blob.bin": {0xff, 0xfe, 0x00, 0x01},
	} {
		bad := writeFile(t, in, name, b)
		if _, err := s.Put(Meta{Title: "t"}, []ItemInput{{Path: good}, {Path: bad}}, nil); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if got := tree(t, s.Root); len(got) > 1 || len(got) == 1 && got[0] != ".lock" {
		t.Fatalf("refused puts left %v", got)
	}
	// Traversal in a caller's name is refused before anything is read.
	if _, err := s.Put(Meta{Title: "t"}, []ItemInput{{Data: []byte("x"), Name: "../escape.txt"}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("traversal %v", err)
	}
	p, err := s.Create(Meta{Title: "open"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../../escape.txt", "sub/escape.txt", `..\escape.txt`} {
		if _, _, err := s.AddItem(p.ID, ItemInput{Data: []byte("hi"), Name: name}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("AddItem %q: %v", name, err)
		}
	}
	if _, _, err := s.AddItem(p.ID, ItemInput{Path: writeFile(t, in, "y.svg", []byte("<svg/>"))}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("svg add %v", err)
	}
	if got := tree(t, filepath.Join(s.Root, p.ID)); strings.Join(got, " ") != "manifest.json" {
		t.Fatalf("refused adds left %v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.Root), "escape.txt")); err == nil {
		t.Fatal("escaped the package")
	}
}

// Rule: an open package accepts items, claims and edits; a sealed one
// refuses every mutation with ErrConflict and stays unchanged.
func TestLifecycle(t *testing.T) {
	s := testStore(t)
	p, err := s.Create(Meta{Title: "Admin billing UAT", Source: "computer-use"})
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "open" || p.SealedAt != nil {
		t.Fatalf("created %+v", p)
	}
	_, item, err := s.AddItem(p.ID, ItemInput{Data: pngOf(t, 2, 2), Name: "before.png", URL: "http://localhost:3000/billing"})
	if err != nil || item.Seq != 1 || item.Name != "001-before.png" {
		t.Fatalf("add %+v %v", item, err)
	}
	_, item2, err := s.AddItem(p.ID, ItemInput{Data: []byte("POST /api/export 200 OK"), Name: "export-status.txt"})
	if err != nil || item2.Seq != 2 || item2.Name != "002-export-status.txt" {
		t.Fatalf("add text %+v %v", item2, err)
	}
	_, claim, err := s.AddClaim(p.ID, ClaimInput{Text: "CSV export downloads", Verdict: "pass", ItemIDs: []string{item.ID}})
	if err != nil || claim.Verdict != "pass" {
		t.Fatalf("claim %v", err)
	}
	if _, _, err := s.AddClaim(p.ID, ClaimInput{Text: "x", ItemIDs: []string{"evi_00000000000000000000000000"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("claim on unknown item %v", err)
	}
	if _, _, err := s.AddClaim(p.ID, ClaimInput{Text: "x", Verdict: "maybe"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad verdict %v", err)
	}
	title := "Admin billing UAT (retry)"
	if _, err := s.Edit(p.ID, Patch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal(p.ID)
	if err != nil || sealed.State != "sealed" || sealed.SealedAt == nil || sealed.Title != title {
		t.Fatalf("seal %+v %v", sealed, err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Root, p.ID, ManifestName))
	for name, mutate := range map[string]func() error{
		"add": func() error {
			_, _, err := s.AddItem(p.ID, ItemInput{Data: []byte("late"), Name: "late.txt"})
			return err
		},
		"claim": func() error { _, _, err := s.AddClaim(p.ID, ClaimInput{Text: "late"}); return err },
		"edit":  func() error { _, err := s.Edit(p.ID, Patch{Title: &title}); return err },
		"seal":  func() error { _, err := s.Seal(p.ID); return err },
	} {
		if err := mutate(); !errors.Is(err, ErrConflict) {
			t.Errorf("%s on sealed: %v", name, err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, p.ID, ManifestName))
	if string(before) != string(after) {
		t.Fatal("a refused mutation changed the manifest")
	}
	if _, err := os.Stat(filepath.Join(s.Root, p.ID, "003-late.txt")); err == nil {
		t.Fatal("a refused add wrote its file")
	}
	if _, err := s.Load("ev_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id %v", err)
	}
	if _, err := s.Load("../" + p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal id %v", err)
	}
}

// Rule: package bounds hold at the limit and refuse one past it.
func TestBounds(t *testing.T) {
	s := testStore(t)
	tags := func(n int) []string {
		var out []string
		for i := range n {
			out = append(out, fmt.Sprintf("t%d", i))
		}
		return out
	}
	if _, err := s.Create(Meta{Title: "x", Tags: tags(MaxTags)}); err != nil {
		t.Fatalf("20 tags: %v", err)
	}
	if _, err := s.Create(Meta{Title: "x", Tags: tags(MaxTags + 1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("21 tags: %v", err)
	}
	if _, err := s.Create(Meta{Title: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no title: %v", err)
	}
	if _, err := s.Create(Meta{Title: "x", Source: "camera"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad source: %v", err)
	}
	meta := func(n, size int) map[string]string {
		m := map[string]string{}
		for i := range n {
			m[fmt.Sprintf("k%d", i)] = strings.Repeat("v", size)
		}
		return m
	}
	p, _ := s.Create(Meta{Title: "x"})
	if _, _, err := s.AddItem(p.ID, ItemInput{Data: []byte("a"), Name: "a.txt", Metadata: meta(MaxMetadataKeys, MaxMetadataValue)}); err != nil {
		t.Fatalf("16 keys of 256B: %v", err)
	}
	if _, _, err := s.AddItem(p.ID, ItemInput{Data: []byte("a"), Name: "a.txt", Metadata: meta(MaxMetadataKeys+1, 1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("17 keys: %v", err)
	}
	if _, _, err := s.AddItem(p.ID, ItemInput{Data: []byte("a"), Name: "a.txt", Metadata: meta(1, MaxMetadataValue+1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("257B value: %v", err)
	}
	var items []ItemInput
	for i := range MaxItemsPerPackage + 1 {
		items = append(items, ItemInput{Data: []byte("x"), Name: fmt.Sprintf("%d.txt", i)})
	}
	if _, err := s.Put(Meta{Title: "x"}, items, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("65 items: %v", err)
	}
	full, err := s.Put(Meta{Title: "x"}, items[:MaxItemsPerPackage], nil)
	if err != nil || len(full.Items) != MaxItemsPerPackage {
		t.Fatalf("64 items: %v", err)
	}
	open, _ := s.Create(Meta{Title: "x"})
	for i := range MaxClaimsPerPackage {
		if _, _, err := s.AddClaim(open.ID, ClaimInput{Text: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.AddClaim(open.ID, ClaimInput{Text: "one too many"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("33 claims: %v", err)
	}
}

// Rule: secrets in text items and in every manifest string are scrubbed
// before they reach the disk; ordinary text is kept.
func TestRedactionBeforeDisk(t *testing.T) {
	s := testStore(t)
	secret := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	aws := "AKIA" + "ABCDEFGHIJKLMNOP"
	in := t.TempDir()
	log := writeFile(t, in, "env.log", []byte("GITHUB_TOKEN="+secret+"\nstatus ok\n"))
	big := writeFile(t, in, "big.log", []byte(strings.Repeat("filler line\n", 7000)+"token "+secret+"\n"))
	p, err := s.Put(Meta{Title: "leak " + secret, Summary: "aws " + aws},
		[]ItemInput{{Path: log, Caption: "caption " + secret, URL: "https://x.test/?token=" + secret, Metadata: map[string]string{"k": aws}}, {Path: big}},
		[]ClaimInput{{Text: "claim " + secret, Criterion: "criterion " + aws, Verdict: "fail"}})
	if err != nil {
		t.Fatal(err)
	}
	open, _ := s.Create(Meta{Title: "open"})
	s.AddItem(open.ID, ItemInput{Data: []byte("key=" + aws), Name: "stdin.txt", Caption: secret})
	s.AddClaim(open.ID, ClaimInput{Text: secret, Criterion: aws})
	filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), secret) || strings.Contains(string(b), aws) {
			t.Errorf("%s holds a secret:\n%s", path, b)
		}
		return nil
	})
	manifest, _ := os.ReadFile(filepath.Join(s.Root, p.ID, ManifestName))
	for _, keep := range []string{"leak [REDACTED:", "status ok", "caption [REDACTED:", "claim [REDACTED:"} {
		if !strings.Contains(string(manifest), keep) {
			t.Errorf("manifest lost %q", keep)
		}
	}
	text, _ := os.ReadFile(filepath.Join(s.Root, p.ID, p.Items[0].Name))
	if !strings.Contains(string(text), "status ok") || !strings.Contains(string(text), "[REDACTED:") {
		t.Fatalf("text item %q", text)
	}
	// Over 64 KiB: kept in the file, not inlined, still redacted.
	if p.Items[1].Text != nil || *p.Items[1].LineCount != 7001 {
		t.Fatalf("big item %+v", p.Items[1])
	}
	// The manifest's hash is of the stored (redacted) bytes.
	if _, err := s.Verify(p.ID); err != nil {
		t.Fatal(err)
	}
}

// Rule: get --out copies a package only when every file matches its
// sha256; a changed or missing file fails and copies nothing.
func TestExportVerifiesIntegrity(t *testing.T) {
	s := testStore(t)
	p, err := s.Put(Meta{Title: "x"}, []ItemInput{{Data: pngOf(t, 3, 3), Name: "a.png"}, {Data: []byte("log"), Name: "b.log"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	if _, err := s.Export(p.ID, out); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, out); strings.Join(got, " ") != "001-a.png 002-b.log manifest.json" {
		t.Fatalf("export %v", got)
	}
	// Tamper: same size, different bytes.
	target := filepath.Join(s.Root, p.ID, "002-b.log")
	os.WriteFile(target, []byte("LOG"), 0600)
	out2 := filepath.Join(t.TempDir(), "out2")
	if _, err := s.Export(p.ID, out2); !errors.Is(err, ErrIntegrity) || !strings.Contains(err.Error(), "002-b.log") {
		t.Fatalf("tampered export %v", err)
	}
	if _, err := os.Stat(out2); err == nil {
		t.Fatal("a failed export wrote files")
	}
	os.Remove(target)
	if _, err := s.Verify(p.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("missing file %v", err)
	}
	// A manifest edited to point outside the package is refused.
	m, _ := os.ReadFile(filepath.Join(s.Root, p.ID, ManifestName))
	os.WriteFile(filepath.Join(s.Root, p.ID, ManifestName), []byte(strings.Replace(string(m), `"001-a.png"`, `"../../x.png"`, 1)), 0600)
	if _, err := s.Load(p.ID); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("unsafe manifest name %v", err)
	}
}

func TestListFilters(t *testing.T) {
	s := testStore(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	a, _ := s.Put(Meta{Title: "a", Order: "o1", Source: "validator", Tags: []string{"ui"}}, []ItemInput{{Data: pngOf(t, 1, 1), Name: "a.png"}}, []ClaimInput{{Text: "ok", Verdict: "pass"}})
	now = now.Add(time.Hour)
	b, _ := s.Put(Meta{Title: "b", Order: "o2", Session: "s2", Source: "spike"}, []ItemInput{{Data: []byte("t"), Name: "b.txt"}}, []ClaimInput{{Text: "bad", Verdict: "fail"}})
	ids := func(f Filter) string {
		got, err := s.List(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range got {
			out = append(out, x.ID)
		}
		return strings.Join(out, ",")
	}
	for name, tt := range map[string]struct {
		f    Filter
		want string
	}{
		"all newest first": {Filter{}, b.ID + "," + a.ID},
		"order":            {Filter{Order: "o1"}, a.ID},
		"session":          {Filter{Session: "s2"}, b.ID},
		"modality":         {Filter{Modalities: []string{"image"}}, a.ID},
		"verdict":          {Filter{Verdicts: []string{"fail"}}, b.ID},
		"tag":              {Filter{Tags: []string{"UI"}}, a.ID},
		"source":           {Filter{Sources: []string{"spike"}}, b.ID},
		"since":            {Filter{Since: now.Add(-time.Minute)}, b.ID},
		"until":            {Filter{Until: now.Add(-time.Minute)}, a.ID},
	} {
		if got := ids(tt.f); got != tt.want {
			t.Errorf("%s: %s, want %s", name, got, tt.want)
		}
	}
	if _, err := s.List(Filter{Modalities: []string{"audio"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad modality %v", err)
	}
}
