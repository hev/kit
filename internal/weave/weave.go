// Package weave composes explicit, reproducible context artifacts. It is the
// engine behind hev context and dispatch's context recipes, ported from the
// standalone Weave prototype (github.com/hev/weave, Apache-2.0). Bundle bytes
// and digests are unchanged from Weave; see NOTICE.
package weave

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Source struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	// Keep and Drop narrow a Markdown source by heading (see narrow.go).
	// Both empty means the whole file, exactly as before.
	Keep []string `json:"keep,omitempty"`
	Drop []string `json:"drop,omitempty"`
}
type Recipe struct {
	Version int      `json:"version"`
	Task    string   `json:"task"`
	Sources []Source `json:"sources"`
}
type File struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Data   []byte `json:"data"`
	// Selection is set only for a narrowed source: Data, SHA256 and Bytes
	// are then the narrowed bytes, and Selection names the source file's own
	// hash and size and the sections kept and dropped.
	Selection *Selection `json:"selection,omitempty"`
}
type Finding struct {
	Rule   string `json:"rule"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Detail string `json:"detail"`
}
type Assessment struct {
	Method   string    `json:"method"`
	Findings []Finding `json:"findings"`
	Improved Recipe    `json:"improved_recipe"`
}
type Bundle struct {
	Version    int        `json:"version"`
	Task       string     `json:"task"`
	Digest     string     `json:"digest"`
	Files      []File     `json:"files"`
	Context    string     `json:"context"`
	Assessment Assessment `json:"assessment"`
}
type Change struct {
	Target string `json:"target"`
	Before string `json:"before"`
	After  string `json:"after"`
	Status string `json:"status"`
}
type Difference struct {
	Before  string   `json:"before_digest"`
	After   string   `json:"after_digest"`
	Changes []Change `json:"changes"`
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encode(v any) []byte  { b, _ := json.MarshalIndent(v, "", "  "); return append(b, '\n') }
func clean(p string) error {
	if p == "" || strings.Contains(p, "\\") || filepath.IsAbs(p) || filepath.ToSlash(filepath.Clean(p)) != p || p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("unsafe relative path %q", p)
	}
	return nil
}

// Reject symlinks at every component, including the root. Recipes cannot escape.
func safe(root, p string) (string, error) {
	if err := clean(p); err != nil {
		return "", err
	}
	a, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// Walk from the volume root. Weave started at "/", which on Windows turns
	// C:\... into \C:\... and refuses every source; VolumeName is "" on Unix.
	full := filepath.Join(a, filepath.FromSlash(p))
	vol := filepath.VolumeName(full)
	cur := vol + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(full[len(vol):], string(filepath.Separator)), string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, e := os.Lstat(cur)
		if e != nil {
			return "", fmt.Errorf("cannot access source %s", p)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink refused in source path: %s", p)
		}
	}
	return cur, nil
}
func read(root, p string) ([]byte, error) {
	path, e := safe(root, p)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", p)
	}
	if st.Size() > 16<<20 {
		return nil, fmt.Errorf("source exceeds 16 MiB: %s", p)
	}
	return os.ReadFile(path)
}
func load(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func Discover(root string) (Recipe, error) {
	r := Recipe{Version: 1, Task: "Describe the assignment; remove sources that do not apply.", Sources: []Source{}}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		selected := d.Name() == "AGENTS.md" || d.Name() == "CLAUDE.md" || strings.HasPrefix(rel, ".claude/agents/") || strings.HasPrefix(rel, ".claude/skills/") || strings.HasPrefix(rel, ".agents/skills/")
		if selected {
			if _, e := read(root, rel); e != nil {
				return e
			}
			r.Sources = append(r.Sources, Source{Path: rel, Target: rel})
		}
		return nil
	})
	sort.Slice(r.Sources, func(i, j int) bool { return r.Sources[i].Path < r.Sources[j].Path })
	return r, err
}

var linkRE = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

func Assess(root string, r Recipe) Assessment {
	a := Assessment{Method: "Deterministic lexical checks, not semantic or hidden-injection analysis.", Findings: []Finding{}, Improved: Recipe{Version: 1, Task: r.Task, Sources: []Source{}}}
	seen := map[string]string{}
	lines := map[string]string{}
	polarity := map[string]struct {
		path     string
		line     int
		negative bool
	}{}
	add := func(rule, p string, l int, d string) { a.Findings = append(a.Findings, Finding{rule, p, l, d}) }
	for _, s := range r.Sources {
		b, e := read(root, s.Path)
		if e != nil {
			add("missing-or-unsafe", s.Path, 1, e.Error())
			continue
		}
		h := hash(b)
		duplicate := false
		if prev, ok := seen[h]; ok && strings.HasSuffix(s.Path, ".md") && (filepath.Base(s.Path) == "AGENTS.md" || filepath.Base(s.Path) == "CLAUDE.md") {
			add("duplicate-file", s.Path, 1, "Identical to "+prev+"; omitted from proposed recipe.")
			duplicate = true
		}
		seen[h] = s.Path
		if !duplicate {
			a.Improved.Sources = append(a.Improved.Sources, s)
		}
		if !strings.HasSuffix(s.Path, ".md") {
			continue
		}
		if len(b) > 8000 {
			add("oversized", s.Path, 1, fmt.Sprintf("%d bytes; consider splitting by task.", len(b)))
		}
		if (filepath.Base(s.Path) == "AGENTS.md" || filepath.Base(s.Path) == "CLAUDE.md") && !strings.Contains(string(b), "Scope:") {
			add("unscoped", s.Path, 1, "No explicit Scope: marker; review applicability to nested projects and tasks.")
		}
		for i, line := range strings.Split(string(b), "\n") {
			t := strings.TrimSpace(line)
			if len(t) > 35 {
				if prev, ok := lines[t]; ok {
					add("repeated-line", s.Path, i+1, "Repeated instruction from "+prev)
				} else {
					lines[t] = fmt.Sprintf("%s:%d", s.Path, i+1)
				}
			}
			lower := strings.ToLower(t)
			neg := strings.Contains(lower, "never ") || strings.Contains(lower, "must not ")
			key := strings.TrimSpace(strings.TrimSuffix(lower, "."))
			for _, prefix := range []string{"always ", "never ", "must not ", "must "} {
				key = strings.TrimPrefix(key, prefix)
			}
			if key != lower && len(key) > 3 {
				if prev, ok := polarity[key]; ok && prev.negative != neg {
					add("possible-conflict", s.Path, i+1, fmt.Sprintf("Opposite lexical directives at %s:%d and here: %s", prev.path, prev.line, t))
				}
				polarity[key] = struct {
					path     string
					line     int
					negative bool
				}{s.Path, i + 1, neg}
			}
			for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
				ref := strings.Split(m[1], "#")[0]
				if ref == "" || strings.Contains(ref, "://") || strings.HasPrefix(ref, "mailto:") {
					continue
				}
				p := filepath.ToSlash(filepath.Join(filepath.Dir(s.Path), ref))
				if _, e := read(root, p); e != nil {
					add("missing-reference", s.Path, i+1, "Cannot resolve "+m[1]+": "+e.Error())
				}
			}
		}
	}
	return a
}
func Build(root string, r Recipe) (Bundle, error) {
	b := Bundle{Version: 1, Task: r.Task, Files: []File{}}
	if r.Version != 1 || len(r.Sources) == 0 || strings.TrimSpace(r.Task) == "" {
		return b, errors.New("recipe requires version 1, a task and nonempty sources")
	}
	targets := map[string]bool{}
	for _, s := range r.Sources {
		if e := clean(s.Target); e != nil {
			return b, e
		}
		if targets[s.Target] {
			return b, fmt.Errorf("duplicate target %s", s.Target)
		}
		for t := range targets {
			if strings.HasPrefix(t, s.Target+"/") || strings.HasPrefix(s.Target, t+"/") {
				return b, fmt.Errorf("overlapping targets %s and %s", t, s.Target)
			}
		}
		targets[s.Target] = true
		data, e := read(root, s.Path)
		if e != nil {
			return b, fmt.Errorf("source %s: %w", s.Path, e)
		}
		var sel *Selection
		if len(s.Keep) > 0 || len(s.Drop) > 0 {
			if !strings.HasSuffix(s.Path, ".md") {
				return b, fmt.Errorf("source %s: sections can only be selected in a Markdown (.md) file", s.Path)
			}
			if data, sel, e = Narrow(data, s.Keep, s.Drop); e != nil {
				return b, fmt.Errorf("source %s: %w", s.Path, e)
			}
		}
		b.Files = append(b.Files, File{s.Path, s.Target, hash(data), len(data), data, sel})
	}
	b.Context = "# Assignment\n\n" + r.Task + "\n"
	for _, f := range b.Files {
		if strings.HasSuffix(f.Target, ".md") {
			label := f.Path + " → " + f.Target
			if f.Selection != nil {
				label += fmt.Sprintf(" (narrowed: %d of %d bytes)", f.Bytes, f.Selection.SourceBytes)
			}
			b.Context += "\n---\n\n# Source: " + label + "\n\n" + string(f.Data) + "\n"
		}
	}
	b.Assessment = Assess(root, r)
	b.Digest = hash(encode(b))
	return b, nil
}

// Write a new directory only. Stage completely, then rename; never replace output.
func publish(out string, files map[string][]byte) error {
	abs, e := filepath.Abs(out)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(abs); !os.IsNotExist(e) {
		return fmt.Errorf("output already exists or cannot be checked: %s", out)
	}
	parent := filepath.Dir(abs)
	resolved, e := filepath.EvalSymlinks(parent)
	if e != nil {
		return e
	}
	parent = resolved
	abs = filepath.Join(parent, filepath.Base(abs))
	tmp, e := os.MkdirTemp(parent, ".weave-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	for p, data := range files {
		if e := clean(p); e != nil {
			return e
		}
		dest := filepath.Join(tmp, filepath.FromSlash(p))
		if e = os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			return e
		}
		if e = os.WriteFile(dest, data, 0644); e != nil {
			return e
		}
	} // mkdir claims the destination so concurrent writers cannot replace it.
	if e = os.Mkdir(abs, 0755); e != nil {
		return e
	}
	entries, e := os.ReadDir(tmp)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if e = os.Rename(filepath.Join(tmp, entry.Name()), filepath.Join(abs, entry.Name())); e != nil {
			return e
		}
	}
	return nil
}
func bundleFiles(b Bundle) map[string][]byte {
	m := map[string][]byte{"bundle.json": encode(b), "context.md": []byte(b.Context), "index.html": render(b, nil)}
	for _, f := range b.Files {
		m["materialized/"+f.Target] = f.Data
	}
	return m
}
func Diff(a, b Bundle) Difference {
	d := Difference{Before: a.Digest, After: b.Digest, Changes: []Change{}}
	am := map[string]File{}
	bm := map[string]File{}
	keys := map[string]bool{}
	for _, f := range a.Files {
		am[f.Target] = f
		keys[f.Target] = true
	}
	for _, f := range b.Files {
		bm[f.Target] = f
		keys[f.Target] = true
	}
	if a.Task != b.Task {
		d.Changes = append(d.Changes, Change{"Assignment", a.Task, b.Task, "changed"})
	}
	beforeOrder, afterOrder := []string{}, []string{}
	for _, f := range a.Files {
		beforeOrder = append(beforeOrder, f.Target)
	}
	for _, f := range b.Files {
		afterOrder = append(afterOrder, f.Target)
	}
	if strings.Join(beforeOrder, "\n") != strings.Join(afterOrder, "\n") {
		d.Changes = append(d.Changes, Change{"Source ordering", strings.Join(beforeOrder, "\n"), strings.Join(afterOrder, "\n"), "changed"})
	}
	order := []string{}
	for k := range keys {
		order = append(order, k)
	}
	sort.Strings(order)
	for _, k := range order {
		x, xok := am[k]
		y, yok := bm[k]
		if xok && yok && x.SHA256 == y.SHA256 && x.Path == y.Path {
			continue
		}
		status := "changed"
		if !xok {
			status = "added"
		}
		if !yok {
			status = "removed"
		}
		d.Changes = append(d.Changes, Change{k, string(x.Data), string(y.Data), status})
	}
	return d
}
func loadBundle(dir string) (Bundle, error) {
	var b Bundle
	e := load(filepath.Join(dir, "bundle.json"), &b)
	if e != nil {
		return b, e
	}
	digest := b.Digest
	b.Digest = ""
	if b.Version != 1 || hash(encode(b)) != digest {
		return b, errors.New("bundle digest mismatch")
	}
	b.Digest = digest
	for _, f := range b.Files {
		if clean(f.Target) != nil || hash(f.Data) != f.SHA256 || len(f.Data) != f.Bytes {
			return b, errors.New("invalid bundle file")
		}
	}
	return b, nil
}
func Run(args []string, w io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(w, "hev context — explicit context, woven for a task\n\nCommands: assess, extract, build, diff, review\nUse hev context COMMAND --help for flags. Recipes are JSON; outputs must not exist.")
		return nil
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(w)
	root := f.String("root", ".", "project or extracted source directory")
	out := f.String("out", "", "new output directory (required except review)")
	recipe := f.String("recipe", "", "JSON recipe (required for build)")
	before := f.String("before", "", "before bundle directory")
	after := f.String("after", "", "after bundle directory")
	bundle := f.String("bundle", "", "bundle directory to serve")
	addr := f.String("addr", "127.0.0.1:0", "loopback listen address; port 0 chooses a free port")
	if e := f.Parse(args[1:]); e != nil {
		if e == flag.ErrHelp {
			return nil
		}
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch args[0] {
	case "assess", "extract":
		if *out == "" {
			return errors.New("--out is required")
		}
		r, e := Discover(*root)
		if e != nil {
			return e
		}
		if len(r.Sources) == 0 {
			return errors.New("no context sources discovered")
		}
		a := Assess(*root, r)
		m := map[string][]byte{"assessment.json": encode(a), "recipe.json": encode(r), "improved.recipe.json": encode(a.Improved)}
		if args[0] == "extract" {
			for _, s := range r.Sources {
				b, e := read(*root, s.Path)
				if e != nil {
					return e
				}
				m["sources/"+s.Path] = b
			}
		}
		if e = publish(*out, m); e != nil {
			return e
		}
		fmt.Fprintf(w, "%s: %d sources, %d findings → %s\n", args[0], len(r.Sources), len(a.Findings), *out)
	case "build":
		if *recipe == "" || *out == "" {
			return errors.New("--recipe and --out are required")
		}
		var r Recipe
		if e := load(*recipe, &r); e != nil {
			return e
		}
		b, e := Build(*root, r)
		if e != nil {
			return e
		}
		if e = publish(*out, bundleFiles(b)); e != nil {
			return e
		}
		fmt.Fprintf(w, "Built %s (%d files) → %s\n", b.Digest, len(b.Files), *out)
		for _, f := range b.Files {
			if f.Selection != nil {
				fmt.Fprintf(w, "  %s: %d of %d bytes (kept %d sections, dropped %d)\n", f.Path, f.Bytes, f.Selection.SourceBytes, len(f.Selection.Kept), len(f.Selection.Dropped))
			}
		}
	case "diff":
		if *before == "" || *after == "" || *out == "" {
			return errors.New("--before, --after and --out are required")
		}
		a, e := loadBundle(*before)
		if e != nil {
			return e
		}
		b, e := loadBundle(*after)
		if e != nil {
			return e
		}
		d := Diff(a, b)
		if e = publish(*out, map[string][]byte{"diff.json": encode(d), "index.html": render(b, &d)}); e != nil {
			return e
		}
		fmt.Fprintf(w, "%d changes → %s\n", len(d.Changes), *out)
	case "review":
		if *bundle == "" {
			return errors.New("--bundle is required")
		}
		host, _, e := net.SplitHostPort(*addr)
		if e != nil {
			return e
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("review address must be a loopback IP")
		}
		data, e := os.ReadFile(filepath.Join(*bundle, "index.html"))
		if e != nil {
			return e
		}
		ln, e := net.Listen("tcp", *addr)
		if e != nil {
			return e
		}
		defer ln.Close()
		fmt.Fprintf(w, "Review http://%s (Ctrl-C to stop; no browser launched)\n", ln.Addr())
		srv := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" && r.URL.Path != "/index.html" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Write(data)
		})}
		return srv.Serve(ln)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return nil
}

// DecodeRecipe reads one recipe with the same strictness as build --recipe:
// unknown fields and trailing values are refused.
func DecodeRecipe(b []byte) (Recipe, error) {
	var r Recipe
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return r, e
	}
	if d.Decode(new(any)) != io.EOF {
		return r, errors.New("expected one JSON object")
	}
	return r, nil
}

// Hash is the hex SHA-256 used for every source and digest.
func Hash(b []byte) string { return hash(b) }

// Publish writes a built bundle to a new directory, exactly as build does.
func Publish(out string, b Bundle) error { return publish(out, bundleFiles(b)) }

// LoadBundle reads a published bundle and verifies its digest and file hashes.
func LoadBundle(dir string) (Bundle, error) { return loadBundle(dir) }

// CheckPath applies the recipe path rules (relative, slash-separated, clean,
// no traversal) without touching the filesystem.
func CheckPath(p string) error { return clean(p) }
