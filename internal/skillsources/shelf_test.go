package skillsources

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mitText = `MIT License

Copyright (c) 2026 Example

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction.

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
`

const apacheText = `
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/
`

func TestParseShorthand(t *testing.T) {
	for _, c := range []struct{ in, owner, repo, sub, ref string }{
		{"mattpocock/skills", "mattpocock", "skills", "", ""},
		{"cursor/plugins//pstack", "cursor", "plugins", "pstack", ""},
		{"cursor/plugins//pstack@main", "cursor", "plugins", "pstack", "main"},
		{"modem-dev/hunk//packages/hunk/skills@v1.2.0", "modem-dev", "hunk", "packages/hunk/skills", "v1.2.0"},
		{"anthropics/skills/skills", "anthropics", "skills", "skills", ""},
		{"obra/superpowers@refs/heads/main", "obra", "superpowers", "", "refs/heads/main"},
		{"github:badlogic/pi-skills", "badlogic", "pi-skills", "", ""},
		{"github.com/vercel-labs/agent-browser", "vercel-labs", "agent-browser", "", ""},
		{"https://github.com/vercel-labs/agent-browser.git", "vercel-labs", "agent-browser", "", ""},
		{"https://github.com/cursor/plugins/tree/main/pstack/skills", "cursor", "plugins", "pstack/skills", "main"},
		{"git@github.com:obra/superpowers.git", "obra", "superpowers", "", ""},
		{"obra/superpowers@0123456789abcdef0123456789abcdef01234567", "obra", "superpowers", "", "0123456789abcdef0123456789abcdef01234567"},
	} {
		s, err := ParseShorthand(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if s.Owner != c.owner || s.Repo != c.repo || s.Subdir != c.sub || s.Ref != c.ref {
			t.Errorf("%s: got %+v", c.in, s)
		}
		if s.URL != "https://github.com/"+c.owner+"/"+c.repo+".git" {
			t.Errorf("%s: url %s", c.in, s.URL)
		}
	}
	for _, bad := range []string{"", "skills", "gitlab.com/a/b", "https://example.com/a/b", "a/b//../x", "a/b//x@", "a/b@-x", "a/b/x//y", "a/b/tree/main/x@v1", "a/b@x..y"} {
		if _, err := ParseShorthand(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	names := map[string]string{"mattpocock/skills//skills": "mattpocock", "cursor/plugins//pstack": "pstack", "anthropics/skills//skills": "anthropics",
		"badlogic/pi-skills": "pi-skills", "modem-dev/hunk//packages/hunk/skills": "hunk", "vercel-labs/agent-browser//skills": "agent-browser", "obra/superpowers//skills": "superpowers"}
	for in, want := range names {
		s, _ := ParseShorthand(in)
		if got := s.DefaultName(); got != want {
			t.Errorf("%s: name %s, want %s", in, got, want)
		}
	}
}

func TestDetectLicense(t *testing.T) {
	for text, want := range map[string]string{
		mitText:    "MIT",
		apacheText: "Apache-2.0",
		"Redistribution and use in source and binary forms, with or without modification... Neither the name of the copyright holder": "BSD-3-Clause",
		"Redistribution and use in source and binary forms, with or without modification are permitted":                               "BSD-2-Clause",
		"Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted":      "ISC",
		"Mozilla Public License Version 2.0":                                      "MPL-2.0",
		"GNU GENERAL PUBLIC LICENSE\n Version 3, 29 June 2007":                    "GPL-3.0",
		"// SPDX-License-Identifier: BSD-2-Clause-Patent\n":                       "BSD-2-Clause-Patent",
		"This is free and unencumbered software released into the public domain.": "Unlicense",
		"© Anthropic. All rights reserved. Use is subject to your agreement.":     NoAssertion,
	} {
		if got := DetectLicense(text); got != want {
			t.Errorf("%q: %s, want %s", text[:20], got, want)
		}
	}
	for name, want := range map[string]bool{"LICENSE": true, "LICENSE.txt": true, "license.md": true, "COPYING": true, "LICENSE-MIT": true, "LICENSED.md": false, "README.md": false} {
		if licenseFile(name) != want {
			t.Errorf("licenseFile(%s) != %v", name, want)
		}
	}
}

// shelfRepo is an upstream with a nested pack, a root licence, a skill with
// its own licence, and a symlink outside the pack that must not break the
// install.
func shelfRepo(t *testing.T) (string, string) {
	t.Helper()
	r := repository(t)
	put(t, filepath.Join(r, "LICENSE"), mitText)
	put(t, filepath.Join(r, "pack", "skills", "plan", "SKILL.md"), "---\nname: plan\n---\nMake a plan.\n")
	put(t, filepath.Join(r, "pack", "skills", "docs", "SKILL.md"), "---\nname: docs\n---\nWrite docs.\n")
	put(t, filepath.Join(r, "pack", "skills", "docs", "LICENSE.txt"), apacheText)
	put(t, filepath.Join(r, "pack", "nested", "deep", "review", "SKILL.md"), "---\nname: review\n---\nReview carefully.\n")
	put(t, filepath.Join(r, "other", "x", "SKILL.md"), "outside the pack\n")
	if err := os.Symlink("README.md", filepath.Join(r, "AGENTS.md")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	commit(t, r)
	return r, runGit(t, r, "rev-parse", "HEAD")
}

func TestShelfPackInstalledNotActive(t *testing.T) {
	r, sha := shelfRepo(t)
	h := t.TempDir()
	for _, d := range []string{".claude", ".codex"} {
		os.MkdirAll(filepath.Join(h, d), 0755)
	}
	c := Config{Sources: []Source{{Name: "pack", Kind: KindTool, Path: r, Subdir: "pack", Commit: sha}}}
	apply(t, h, c, true)
	packs, err := Packs(h)
	if err != nil {
		t.Fatal(err)
	}
	p := packs["pack"]
	if p.Commit != sha || p.License != "MIT" || p.Kind != KindTool || len(p.Skills) != 3 {
		t.Fatalf("pack = %+v", p)
	}
	got := map[string]string{}
	for _, s := range p.Skills {
		got[s.Name] = s.License + " " + s.Path
	}
	if got["docs"] != "Apache-2.0 pack/skills/docs" || got["plan"] != "MIT pack/skills/plan" || got["review"] != "MIT pack/nested/deep/review" {
		t.Fatalf("skills = %v", got)
	}
	// Nothing outside the pack (and its licence files) is installed, and
	// nothing is linked into a harness root.
	if _, err := os.Stat(filepath.Join(p.Pin, "other")); !os.IsNotExist(err) {
		t.Fatal("installed outside the pack")
	}
	if _, err := os.Stat(filepath.Join(p.Pin, "LICENSE")); err != nil {
		t.Fatal("root licence not installed")
	}
	for _, d := range []string{".claude", ".codex", ".agents"} {
		if entries, _ := os.ReadDir(filepath.Join(h, d, "skills")); len(entries) > 0 {
			t.Fatalf("%s/skills linked %v", d, entries)
		}
	}
	links, _ := Installed(h)
	if len(links) != 0 {
		t.Fatalf("links %v", links)
	}
	// A check with nothing changed is clean.
	apply(t, h, c, false)
}

func TestShelfPinIsExactAndOnRef(t *testing.T) {
	r, first := shelfRepo(t)
	put(t, filepath.Join(r, "pack", "skills", "plan", "SKILL.md"), "---\nname: plan\n---\nPlan v2.\n")
	commit(t, r)
	h := t.TempDir()
	c := Config{Sources: []Source{{Name: "pack", Kind: KindMode, Path: r, Subdir: "pack", Commit: first}}}
	apply(t, h, c, true)
	packs, _ := Packs(h)
	b, err := os.ReadFile(filepath.Join(packs["pack"].Pin, "pack", "skills", "plan", "SKILL.md"))
	if err != nil || !strings.Contains(string(b), "Make a plan") {
		t.Fatalf("pin installed the ref tip, not the pinned commit: %q %v", b, err)
	}
	// A commit that is not on the ref is refused.
	runGit(t, r, "checkout", "-q", "-b", "side", first)
	put(t, filepath.Join(r, "pack", "skills", "plan", "SKILL.md"), "side\n")
	commit(t, r)
	side := runGit(t, r, "rev-parse", "HEAD")
	runGit(t, r, "checkout", "-q", "main")
	c.Sources[0].Ref = "refs/heads/main"
	c.Sources[0].Commit = side
	if err := Run(h, c, true, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not on refs/heads/main") {
		t.Fatalf("off-ref pin: %v", err)
	}
	for _, bad := range []Source{
		{Name: "pack", Kind: "workflow", Path: r, Subdir: "pack", Commit: first},
		{Name: "pack", Kind: KindTool, Path: r, Subdir: "pack", Commit: "main"},
		{Name: "Bad Name", Kind: KindTool, Path: r, Subdir: "pack", Commit: first},
		{Name: "pack", Kind: KindTool, Path: r, Subdir: "missing", Commit: first},
	} {
		if err := Run(t.TempDir(), Config{Sources: []Source{bad}}, true, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestEnableKindsAndOneMode(t *testing.T) {
	r, sha := shelfRepo(t)
	r2 := repository(t)
	put(t, filepath.Join(r2, "skills", "flow", "SKILL.md"), "---\nname: flow\n---\nflow\n")
	put(t, filepath.Join(r2, "skills", "plan", "SKILL.md"), "---\nname: plan\n---\nanother plan\n")
	commit(t, r2)
	sha2 := runGit(t, r2, "rev-parse", "HEAD")
	h := t.TempDir()
	c := Config{Sources: []Source{
		{Name: "alpha", Kind: KindMode, Path: r, Subdir: "pack", Commit: sha},
		{Name: "beta", Kind: KindMode, Path: r2, Subdir: "skills", Commit: sha2},
	}}
	apply(t, h, c, true)
	en, err := Enable(h, c, []string{"alpha/plan", "alpha/docs"})
	if err != nil || len(en) != 2 || en[0].Kind != KindMode || en[0].Commit != sha || en[1].License != "Apache-2.0" {
		t.Fatalf("enable: %+v %v", en, err)
	}
	if _, err := os.Stat(filepath.Join(en[0].Dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Enable(h, c, []string{"alpha/plan", "beta/flow"}); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("two modes: %v", err)
	}
	c.Sources[1].Kind = KindTool
	if _, err := Enable(h, c, []string{"alpha/review", "beta/flow"}); err != nil {
		t.Fatalf("mode + tool: %v", err)
	}
	for _, refs := range [][]string{{"alpha/plan", "beta/plan"}, {"alpha/nope"}, {"gamma/x"}, {"alpha"}, {"alpha/plan", "alpha/plan"}} {
		if _, err := Enable(h, c, refs); err == nil {
			t.Errorf("accepted %v", refs)
		}
	}
	// A config pin that is not installed yet is refused, not silently served
	// from the old pin.
	put(t, filepath.Join(r, "pack", "skills", "plan", "SKILL.md"), "v2\n")
	commit(t, r)
	c.Sources[0].Commit = runGit(t, r, "rev-parse", "HEAD")
	if _, err := Enable(h, c, []string{"alpha/plan"}); err == nil || !strings.Contains(err.Error(), "hev skills sync") {
		t.Fatalf("stale pin: %v", err)
	}
}

func TestConfigTextEditsAndBump(t *testing.T) {
	sha := strings.Repeat("a", 40)
	text := "# my config\n[skills]\nrole = \"host\"\n\n[[skills.sources]]\nurl = \"https://example.com/team.git\"\n"
	next, err := AddSourceText(text, Source{Name: "pstack", Kind: KindMode, URL: "https://github.com/cursor/plugins.git", Ref: "refs/heads/main", Subdir: "pstack", Commit: sha})
	if err != nil || !strings.HasPrefix(next, text) || !strings.Contains(next, "commit = \""+sha+"\"") {
		t.Fatalf("%v\n%s", err, next)
	}
	if _, err := AddSourceText(next, Source{Name: "pstack", Kind: KindTool, URL: "https://github.com/x/y.git", Commit: sha}); err == nil {
		t.Fatal("duplicate name accepted")
	}
	next2, err := AddSourceText(next, Source{Name: "hunk", Kind: KindTool, URL: "https://github.com/modem-dev/hunk.git", Ref: "refs/heads/main", Commit: sha})
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("b", 40)
	bumped, err := SetCommitText(next2, "pstack", other)
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := decodeText(bumped, &c); err != nil {
		t.Fatal(err)
	}
	if c.Sources[1].Commit != other || c.Sources[2].Commit != sha || !strings.HasPrefix(bumped, text) {
		t.Fatalf("bump moved the wrong pin: %+v", c.Sources)
	}
	if _, err := SetCommitText(next2, "pstack", "main"); err == nil {
		t.Fatal("bump to a ref accepted")
	}
	if _, err := SetCommitText(next2, "nope", other); err == nil {
		t.Fatal("bump of unknown pack accepted")
	}
}

func TestResolveBumpRefusesWithoutCommit(t *testing.T) {
	r, sha := shelfRepo(t)
	put(t, filepath.Join(r, "pack", "skills", "plan", "SKILL.md"), "v2\n")
	commit(t, r)
	next := runGit(t, r, "rev-parse", "HEAD")
	h := t.TempDir()
	c := Config{Sources: []Source{{Name: "pack", Kind: KindTool, Path: r, Subdir: "pack", Commit: sha}}}
	apply(t, h, c, true)
	for _, to := range []string{"", "main", "HEAD", "refs/heads/main", "v1"} {
		if _, _, err := ResolveBump(h, c, "pack", to); err == nil {
			t.Errorf("bump to %q accepted", to)
		}
	}
	_, full, err := ResolveBump(h, c, "pack", next[:10])
	if err != nil || full != next {
		t.Fatalf("bump to commit: %s %v", full, err)
	}
	var out bytes.Buffer
	if _, err := Diff(h, c, "pack", "", true, &out); err != nil || !strings.Contains(out.String(), "pack/skills/plan/SKILL.md") || !strings.Contains(out.String(), "+v2") {
		t.Fatalf("diff: %v\n%s", err, &out)
	}
}

func TestChunkKeysCarryAttribution(t *testing.T) {
	r, sha := shelfRepo(t)
	h := t.TempDir()
	c := Config{Sources: []Source{{Name: "pack", Kind: KindDomain, Path: r, Subdir: "pack", Commit: sha}}}
	apply(t, h, c, true)
	packs, _ := Packs(h)
	chunks, err := Chunks(packs["pack"])
	if err != nil || len(chunks) != 3 {
		t.Fatalf("%d chunks %v", len(chunks), err)
	}
	for _, ch := range chunks {
		if ch.ID != ChunkKey(ch.Source, ch.Commit, ch.Path, ch.Part) || ch.Commit != sha || ch.License == "" || ch.Pack != "pack" || ch.Kind != KindDomain || ch.Source == "" {
			t.Fatalf("chunk %+v", ch)
		}
		if strings.HasSuffix(ch.Path, "LICENSE.txt") {
			t.Fatal("licence text indexed as skill content")
		}
	}
	if ChunkKey("s", "c1", "p", 0) == ChunkKey("s", "c2", "p", 0) || ChunkKey("s", "c", "p", 0) == ChunkKey("s", "c", "p", 1) {
		t.Fatal("key ignores commit or part")
	}
	if parts := split(strings.Repeat("word ", 1000), 2000); len(parts) < 2 {
		t.Fatalf("split %d", len(parts))
	}
	pin := filepath.Join("/home/u/.local/opt", "hev-skills-0123456789abcdef-"+sha, "pack", "skills", "plan", "SKILL.md")
	if PinnedCommit(pin) != sha || PinnedCommit("/home/u/.claude/skills/plan/SKILL.md") != "" {
		t.Fatal("PinnedCommit")
	}
}
