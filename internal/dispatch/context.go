package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/weave"
)

// An order may name a context recipe with "context: PATH". At each launch
// (start, rework, retry, an ended-run say, an amend restart or a host move)
// the target builds the recipe's bundle from the run's worktree into the run
// record, before the harness starts, and the supervisor gives the assembled
// context.md to the harness on standard input. A run whose recipe cannot be
// built never launches.

// ContextDelivery names how the assembled context reaches the harness. Both
// claude -p and codex exec read standard input alongside the prompt argument
// (codex appends it as a <stdin> block), on Unix and Windows alike, so the
// prompt argument and its contract are unchanged.
const ContextDelivery = "stdin"

// contextNote is recorded with every bundle so no reader mistakes the bundle
// for everything the worker saw.
const contextNote = "The bundle is the context dispatch gave the worker explicitly on standard input. It is not everything the harness saw: the harness also loads instruction files and skills from the worktree (listed in harness_also_loads), user-level configuration such as ~/.claude and ~/.codex, and its own system prompt. None of that is captured or suppressed here."

const maxRecipeBytes = 1 << 20

// ContextRecord is what one attempt was given from its context recipe. It
// lives in meta.json (and with each attempt in history.json); the full source
// manifest with file contents is the bundle's own bundle.json.
type ContextRecord struct {
	// Digest is the Weave bundle digest: assignment, ordering, source
	// identities, hashes and assessment.
	Digest string `json:"digest"`
	// Recipe is the order's context: path; RecipeSHA256 hashes its bytes as
	// read at RecipeCommit, the run's base commit for Base.
	Recipe       string `json:"recipe"`
	RecipeSHA256 string `json:"recipe_sha256"`
	RecipeCommit string `json:"recipe_commit"`
	Base         string `json:"base"`
	// Bundle is the bundle directory relative to the run record; Manifest is
	// its bundle.json, and Context the context.md given to the harness.
	Bundle   string          `json:"bundle"`
	Manifest string          `json:"manifest"`
	Context  string          `json:"context"`
	Sources  []ContextSource `json:"sources"`
	Delivery string          `json:"delivery"`
	// HarnessAlsoLoads lists instruction and skill files present in the
	// worktree that a harness may load on its own, outside the bundle.
	HarnessAlsoLoads []string  `json:"harness_also_loads"`
	Note             string    `json:"note"`
	Built            time.Time `json:"built"`
}

// ContextSource is one bundle source without its contents.
type ContextSource struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// orderIntent is the order body after its frontmatter: the assignment.
func orderIntent(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[0] == "---" {
		for i, line := range lines[1:] {
			if line == "---" {
				return strings.TrimSpace(strings.Join(lines[i+2:], "\n"))
			}
		}
	}
	return strings.TrimSpace(text)
}

// gitOutput runs git and returns its exact standard output.
func gitOutput(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c := helperCommand(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// recipeCommit is the run's base commit: the fork point of the run's branch
// from origin/<base>. It is pinned with the first bundle for that base, so a
// worker cannot change the recipe its later attempts are given.
func recipeCommit(wt string, m Meta, prev *ContextRecord) (string, error) {
	if prev != nil && prev.Base == m.Base && prev.RecipeCommit != "" {
		return prev.RecipeCommit, nil
	}
	out, err := gitOutput(wt, "merge-base", "HEAD", "origin/"+m.Base)
	if err != nil {
		return "", fmt.Errorf("context recipe %s: cannot find the base commit for origin/%s: %w", m.Context, m.Base, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// readRecipe reads the recipe blob at commit, refusing anything but a regular
// file (a symlink in the tree is never followed).
func readRecipe(wt, commit, path string) ([]byte, error) {
	if err := weave.CheckPath(path); err != nil {
		return nil, fmt.Errorf("invalid context recipe path: %w", err)
	}
	out, err := gitOutput(wt, "ls-tree", "-z", commit, "--", path)
	if err != nil {
		return nil, err
	}
	entry := strings.TrimSuffix(string(out), "\x00")
	short := commit
	if len(short) > 12 {
		short = short[:12]
	}
	if entry == "" {
		return nil, fmt.Errorf("context recipe %s not found at base commit %s", path, short)
	}
	info, name, _ := strings.Cut(entry, "\t")
	fields := strings.Fields(info)
	if name != path || len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
		return nil, fmt.Errorf("context recipe %s is not a regular file at base commit %s", path, short)
	}
	size, err := gitOutput(wt, "cat-file", "-s", fields[2])
	if err != nil {
		return nil, err
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(string(size))); n > maxRecipeBytes {
		return nil, fmt.Errorf("context recipe %s exceeds %d bytes", path, maxRecipeBytes)
	}
	return gitOutput(wt, "cat-file", "blob", fields[2])
}

// nextBundleDir names a new bundle directory under contexts/. Earlier
// attempts' bundles are never moved or replaced.
func nextBundleDir(dir string) (string, error) {
	if err := os.MkdirAll(filepath.Join(dir, "contexts"), 0700); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "contexts"))
	if err != nil {
		return "", err
	}
	n := 0
	for _, e := range entries {
		if v, err := strconv.Atoi(e.Name()); err == nil && v > n {
			n = v
		}
	}
	return "contexts/" + strconv.Itoa(n+1), nil
}

// privateTree applies the run record's permissions to a published bundle.
func privateTree(root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected symlink in bundle: %s", p)
		}
		mode := os.FileMode(0600)
		if d.IsDir() {
			mode = 0700
		}
		return os.Chmod(p, mode)
	})
}

// prepareContext builds this attempt's bundle from the worktree when the
// order names a recipe, and records it on m. It always clears the previous
// attempt's record from m first; history.json keeps that one. Validators get
// no bundle in this slice.
func prepareContext(dir string, m *Meta, orderText string) error {
	prev := m.ContextBundle
	m.ContextBundle, m.ContextDigest = nil, ""
	if m.Parent != "" || m.Context == "" {
		return nil
	}
	wt := filepath.Join(dir, "wt")
	root, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return fmt.Errorf("context recipe %s: worktree: %w", m.Context, err)
	}
	commit, err := recipeCommit(root, *m, prev)
	if err != nil {
		return err
	}
	raw, err := readRecipe(root, commit, m.Context)
	if err != nil {
		return err
	}
	recipe, err := weave.DecodeRecipe(raw)
	if err != nil {
		return fmt.Errorf("invalid context recipe %s: %w", m.Context, err)
	}
	// The order, not the recipe, says what this attempt is for.
	recipe.Task = orderIntent(orderText)
	bundle, err := weave.Build(root, recipe)
	if err != nil {
		return fmt.Errorf("context recipe %s: %w", m.Context, err)
	}
	rel, err := nextBundleDir(dir)
	if err != nil {
		return err
	}
	out := filepath.Join(dir, filepath.FromSlash(rel))
	if err = weave.Publish(out, bundle); err != nil {
		return fmt.Errorf("context bundle: %w", err)
	}
	if err = privateTree(out); err != nil {
		return fmt.Errorf("context bundle: %w", err)
	}
	rec := &ContextRecord{Digest: bundle.Digest, Recipe: m.Context, RecipeSHA256: weave.Hash(raw), RecipeCommit: commit, Base: m.Base,
		Bundle: rel, Manifest: rel + "/bundle.json", Context: rel + "/context.md", Sources: []ContextSource{}, Delivery: ContextDelivery,
		HarnessAlsoLoads: []string{}, Note: contextNote, Built: time.Now().UTC()}
	for _, f := range bundle.Files {
		rec.Sources = append(rec.Sources, ContextSource{Path: f.Path, Target: f.Target, SHA256: f.SHA256, Bytes: f.Bytes})
	}
	// Discovery stops at the first unreadable entry; whatever it found is
	// still worth recording.
	ambient, _ := weave.Discover(root)
	for _, s := range ambient.Sources {
		rec.HarnessAlsoLoads = append(rec.HarnessAlsoLoads, s.Path)
	}
	sort.Strings(rec.HarnessAlsoLoads)
	m.ContextBundle, m.ContextDigest = rec, bundle.Digest
	return nil
}

// contextInput opens the attempt's assembled context for the harness. An
// order with a recipe and no verified bundle never launches.
func contextInput(dir string, m Meta) (*os.File, error) {
	if m.Parent != "" || m.Context == "" {
		return nil, nil
	}
	rec := m.ContextBundle
	if rec == nil || rec.Digest == "" {
		return nil, fmt.Errorf("context recipe %s has no bundle for this attempt; rework to rebuild it", m.Context)
	}
	b, err := weave.LoadBundle(filepath.Join(dir, filepath.FromSlash(rec.Bundle)))
	if err != nil {
		return nil, fmt.Errorf("context bundle %s: %w", rec.Bundle, err)
	}
	if b.Digest != rec.Digest || b.Digest != m.ContextDigest {
		return nil, fmt.Errorf("context bundle %s digest %s does not match the recorded %s", rec.Bundle, b.Digest, rec.Digest)
	}
	path := filepath.Join(dir, filepath.FromSlash(rec.Context))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("context bundle: %w", err)
	}
	if string(data) != b.Context {
		return nil, fmt.Errorf("context bundle %s: context.md does not match its manifest", rec.Bundle)
	}
	return os.Open(path)
}

// failAttempt records an attempt that ended before launch.
func failAttempt(dir string, m Meta, err error) {
	now := time.Now().UTC()
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{ExitCode: 1, Start: m.Start, End: now, Branch: m.Branch, Error: err.Error(), SameHost: m.SameHost, ContextDigest: m.ContextDigest, TraceID: m.TraceID, SpanID: m.SpanID})
}

// ContextBundleDir returns the bundle directory a run on this host was given:
// the current attempt's when attempt is 0, otherwise that attempt's.
func ContextBundleDir(name string, attempt int) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
		return "", fmt.Errorf("run %s not found on this host: %w", name, err)
	}
	rec := m.ContextBundle
	if attempt > 0 {
		h := history(dir, m)
		if attempt > len(h.Attempts) {
			return "", fmt.Errorf("run %s has %d attempts", name, len(h.Attempts))
		}
		rec = h.Attempts[attempt-1].Meta.ContextBundle
	}
	if rec == nil || rec.Bundle == "" {
		return "", fmt.Errorf("run %s has no context bundle for that attempt", name)
	}
	if err := weave.CheckPath(rec.Bundle); err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(rec.Bundle)), nil
}

// ContextAttribution is the context digest a worker session was given. The
// attempt comes from the result that recorded the session; a session no
// result names falls back to the run's current attempt. Paths outside
// dispatch and runs without a recipe return "".
func ContextAttribution(cwd, session string) string {
	name := attributionName(cwd)
	if name == "" {
		return ""
	}
	dir := filepath.Join(Root(), name)
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
		return ""
	}
	if session != "" {
		for _, r := range runResults(dir) {
			if r.Session == session {
				return r.ContextDigest
			}
		}
	}
	return m.ContextDigest
}
