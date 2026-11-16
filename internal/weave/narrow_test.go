package weave

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const guide = "# Guide\nIntro line.\n\n## Build\nmake\n### Windows\nuse wsl\n## Deploy ##\nship it\n```sh\n# not a heading\n## also not\n```\n## Testing\nrun go test\n"

func TestNarrowKeepAndDrop(t *testing.T) {
	out, sel, err := Narrow([]byte(guide), []string{"## Build"}, []string{"### Windows"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "# Guide\n## Build\nmake\n" {
		t.Fatalf("keep/drop %q", out)
	}
	// "# Guide" survives as the kept section's ancestor, its body ("Intro
	// line.") goes: there is no preamble here, the file starts with a heading.
	if sel.SourceSHA256 != hash([]byte(guide)) || sel.SourceBytes != len(guide) {
		t.Fatalf("source identity %+v", sel)
	}
	names := func(ss []Section) string {
		var n []string
		for _, s := range ss {
			n = append(n, s.Heading)
		}
		return strings.Join(n, ",")
	}
	if names(sel.Kept) != "# Guide,## Build" || names(sel.Dropped) != "# Guide (body),### Windows,## Deploy,## Testing" {
		t.Fatalf("kept %s dropped %s", names(sel.Kept), names(sel.Dropped))
	}
	if !sel.Kept[0].HeadingOnly || sel.Kept[1].Bytes != len("## Build\nmake\n") {
		t.Fatalf("kept %+v", sel.Kept)
	}
	// Every kept byte is a source byte in source order; kept + dropped = all.
	total := 0
	for _, s := range append(sel.Kept, sel.Dropped...) {
		total += s.Bytes
	}
	if total != len(guide) {
		t.Fatalf("sections account for %d of %d bytes", total, len(guide))
	}

	// Drop only; a closing # run is decoration and fenced "#" lines are text.
	out, _, err = Narrow([]byte(guide), nil, []string{"##   Deploy", "## Build"})
	if err != nil || string(out) != "# Guide\nIntro line.\n\n## Testing\nrun go test\n" {
		t.Fatalf("drop %q %v", out, err)
	}
	// Selecting a fenced pseudo-heading matches nothing.
	if _, _, err = Narrow([]byte(guide), nil, []string{"## also not"}); err == nil || !strings.Contains(err.Error(), "matches no heading") {
		t.Fatalf("fenced heading selected: %v", err)
	}
}

func TestNarrowPreambleAndErrors(t *testing.T) {
	doc := "Read me first.\n# A\na\n# B\nb\n"
	out, sel, err := Narrow([]byte(doc), []string{"# B"}, nil)
	if err != nil || string(out) != "Read me first.\n# B\nb\n" || sel.Kept[0].Heading != Preamble {
		t.Fatalf("keep keeps the preamble: %q %+v %v", out, sel, err)
	}
	if out, _, err = Narrow([]byte(doc), []string{"# B"}, []string{Preamble}); err != nil || string(out) != "# B\nb\n" {
		t.Fatalf("drop preamble %q %v", out, err)
	}
	for _, c := range []struct {
		keep, drop []string
		want       string
	}{
		{[]string{"# Missing"}, nil, `"# Missing" matches no heading`},
		{nil, []string{"## A"}, "matches no heading"}, // levels are part of the selector
		{[]string{"A"}, nil, "invalid section selector"},
		{[]string{"#"}, nil, "invalid section selector"},
		{nil, []string{Preamble, "# A", "# B"}, "leaves nothing"},
		{nil, nil, "needs keep or drop"},
	} {
		if _, _, err := Narrow([]byte(doc), c.keep, c.drop); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v/%v: %v", c.keep, c.drop, err)
		}
	}
	if _, _, err := Narrow([]byte("# A\na\n"), nil, []string{Preamble}); err == nil || !strings.Contains(err.Error(), "matches nothing") {
		t.Fatalf("absent preamble: %v", err)
	}
	// CRLF files narrow without rewriting line endings.
	crlf := "# A\r\na\r\n# B\r\nb\r\n"
	if out, _, err = Narrow([]byte(crlf), nil, []string{"# A"}); err != nil || string(out) != "# B\r\nb\r\n" {
		t.Fatalf("crlf %q %v", out, err)
	}
}

func TestSectionsOutline(t *testing.T) {
	o := Sections([]byte("pre\n# A\na\n## A1\nx\n# B\n"))
	if len(o) != 4 || o[0].Heading != Preamble || o[1].Heading != "# A" || o[1].Bytes != len("# A\na\n## A1\nx\n") || o[2].Level != 2 || o[3].Line != 6 {
		t.Fatalf("%+v", o)
	}
}

func TestBuildNarrowed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(guide), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("# x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	whole := Recipe{1, "t", []Source{{Path: "AGENTS.md", Target: "AGENTS.md"}}}
	wb, err := Build(root, whole)
	if err != nil || wb.Files[0].Selection != nil || bytes.Contains(encode(wb), []byte("selection")) {
		t.Fatalf("whole-file bundle carries a selection: %v", err)
	}
	narrow := Recipe{1, "t", []Source{{Path: "AGENTS.md", Target: "AGENTS.md", Drop: []string{"## Deploy"}}}}
	nb, err := Build(root, narrow)
	if err != nil {
		t.Fatal(err)
	}
	f := nb.Files[0]
	if f.Selection == nil || f.Selection.SourceSHA256 != hash([]byte(guide)) || f.SHA256 != hash(f.Data) || bytes.Contains(f.Data, []byte("ship it")) || nb.Digest == wb.Digest {
		t.Fatalf("narrowed file %+v", f)
	}
	if !strings.Contains(nb.Context, "# Source: AGENTS.md → AGENTS.md (narrowed: ") || strings.Contains(nb.Context, "ship it") {
		t.Fatalf("context %q", nb.Context)
	}
	// A narrowed bundle publishes, reloads and verifies like any other.
	out := filepath.Join(t.TempDir(), "b")
	if err := Publish(out, nb); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadBundle(out); err != nil || got.Digest != nb.Digest || got.Files[0].Selection.Dropped[0].Heading != "## Deploy" {
		t.Fatalf("reload %v", err)
	}
	if page, _ := os.ReadFile(filepath.Join(out, "index.html")); !bytes.Contains(page, []byte("Dropped")) {
		t.Fatal("reviewer page does not show dropped sections")
	}
	for _, bad := range []Recipe{
		{1, "t", []Source{{Path: "notes.txt", Target: "notes.txt", Keep: []string{"# x"}}}},
		{1, "t", []Source{{Path: "AGENTS.md", Target: "AGENTS.md", Keep: []string{"## Nope"}}}},
	} {
		if _, err := Build(root, bad); err == nil {
			t.Errorf("accepted %+v", bad.Sources)
		}
	}
	// Same inputs, same bytes.
	again, _ := Build(root, narrow)
	if again.Digest != nb.Digest {
		t.Fatal("narrowing is not deterministic")
	}
}
