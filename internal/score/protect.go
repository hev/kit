package score

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	gitDiffTimeout  = 30 * time.Second
	maxGitDiffBytes = 16 << 20
)

// ValidateProtectPattern validates one protected repository path or glob pattern.
// Patterns are slash-separated repository-relative paths supporting `*`, `?`,
// `[...]` within a path component and `**` as a full path component matching
// zero or more directory levels.
func ValidateProtectPattern(pattern string) error {
	norm, dirOnly, err := normalizeProtectPattern(pattern)
	if err != nil {
		return err
	}
	_ = dirOnly
	for _, seg := range strings.Split(norm, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid path segment %q", seg)
		}
		if seg == "**" {
			continue
		}
		if strings.Contains(seg, "**") {
			return fmt.Errorf("** must be a complete path component")
		}
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("invalid glob syntax: %w", err)
		}
	}
	return nil
}

func normalizeProtectPattern(raw string) (norm string, trailingSlash bool, err error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsRune(s, 0) {
		return "", false, fmt.Errorf("pattern must be non-empty")
	}
	s = strings.ReplaceAll(s, `\`, "/")
	for strings.HasPrefix(s, "./") {
		s = strings.TrimPrefix(s, "./")
	}
	s = strings.TrimPrefix(s, "/")
	trailingSlash = strings.HasSuffix(s, "/")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "", false, fmt.Errorf("pattern must name a repository path")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return "", false, fmt.Errorf("parent directory traversal (..) is not allowed")
		}
	}
	return s, trailingSlash, nil
}

// MatchProtect reports whether a slash-separated repository-relative path
// matches a protected path or glob pattern.
func MatchProtect(pattern, relPath string) (bool, error) {
	if err := ValidateProtectPattern(pattern); err != nil {
		return false, err
	}
	normPat, trailingSlash, _ := normalizeProtectPattern(pattern)
	cleanRel := strings.Trim(strings.ReplaceAll(relPath, `\`, "/"), "/")
	if cleanRel == "" {
		return false, nil
	}
	parts := strings.Split(cleanRel, "/")

	// A trailing-slash pattern ("testdata/recall/") protects everything inside that directory.
	if trailingSlash {
		return matchSegments(append(strings.Split(normPat, "/"), "**"), parts), nil
	}

	patSegs := strings.Split(normPat, "/")
	if matchSegments(patSegs, parts) {
		return true, nil
	}

	// A literal path without glob metacharacters ("testdata/recall") also protects
	// files beneath that directory ("testdata/recall/queries.jsonl") without matching
	// sibling names ("testdata/recall_test.go").
	if !strings.ContainsAny(normPat, "*?[") {
		if matchSegments(append(patSegs, "**"), parts) {
			return true, nil
		}
	}
	return false, nil
}

func matchSegments(patterns, parts []string) bool {
	reachable := make([]bool, len(parts)+1)
	reachable[0] = true
	for _, p := range patterns {
		next := make([]bool, len(parts)+1)
		for j := 0; j <= len(parts); j++ {
			if p == "**" {
				next[j] = reachable[j] || (j > 0 && next[j-1])
			} else if j > 0 && reachable[j-1] {
				matched, _ := path.Match(p, parts[j-1])
				next[j] = matched
			}
		}
		reachable = next
	}
	return reachable[len(parts)]
}

// CheckProtectedPaths returns all unique sorted paths from changedPaths that
// match at least one pattern in patterns.
func CheckProtectedPaths(changedPaths, patterns []string) ([]string, error) {
	for i, pat := range patterns {
		if err := ValidateProtectPattern(pat); err != nil {
			return nil, fmt.Errorf("protect[%d] (%q): %w", i, pat, err)
		}
	}
	if len(changedPaths) == 0 || len(patterns) == 0 {
		return nil, nil
	}
	var matched []string
	seen := map[string]bool{}
	for _, p := range changedPaths {
		norm := strings.Trim(strings.ReplaceAll(p, `\`, "/"), "/")
		if norm == "" || seen[norm] {
			continue
		}
		for _, pat := range patterns {
			ok, err := MatchProtect(pat, norm)
			if err != nil {
				return nil, err
			}
			if ok {
				seen[norm] = true
				matched = append(matched, norm)
				break
			}
		}
	}
	sort.Strings(matched)
	return matched, nil
}

// DiffPaths returns all repository-relative paths changed between parent...commit,
// including both the old path and the new path when a file is renamed or copied.
func DiffPaths(ctx context.Context, repo, parent, commit string) ([]string, error) {
	parentSHA, err := resolveCommit(ctx, repo, parent)
	if err != nil {
		return nil, fmt.Errorf("resolve parent %q: %w", parent, err)
	}
	commitSHA, err := resolveCommit(ctx, repo, commit)
	if err != nil {
		return nil, fmt.Errorf("resolve commit %q: %w", commit, err)
	}
	// Use --name-status -M -z so git emits raw NUL-delimited paths (no C-quoting)
	// and explicitly includes both old and new paths on R* (rename) and C* (copy)
	// entries, whereas --name-only -M alone omits the old path of a rename.
	out, err := runGit(ctx, repo, gitDiffTimeout, maxGitDiffBytes,
		"diff", "--no-ext-diff", "--name-status", "-M", "-z", parentSHA+"..."+commitSHA, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff %s...%s: %w", shortHex(parentSHA), shortHex(commitSHA), err)
	}
	return parseNameStatusZ(out)
}

func parseNameStatusZ(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	tokens := bytes.Split(raw, []byte{0})
	// Trailing NUL produces a final empty token.
	if len(tokens) > 0 && len(tokens[len(tokens)-1]) == 0 {
		tokens = tokens[:len(tokens)-1]
	}
	seen := map[string]bool{}
	var paths []string
	add := func(b []byte) {
		p := strings.Trim(strings.ReplaceAll(string(b), `\`, "/"), "/")
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	for i := 0; i < len(tokens); {
		status := string(tokens[i])
		i++
		if status == "" {
			continue
		}
		if i >= len(tokens) {
			return nil, fmt.Errorf("truncated git diff --name-status output after %q", status)
		}
		add(tokens[i])
		i++
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i >= len(tokens) {
				return nil, fmt.Errorf("truncated rename/copy entry %q in git diff output", status)
			}
			add(tokens[i])
			i++
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func resolveCommit(ctx context.Context, repo, rev string) (string, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" || strings.HasPrefix(rev, "-") || strings.ContainsRune(rev, 0) {
		return "", fmt.Errorf("invalid revision")
	}
	out, err := runGit(ctx, repo, gitDiffTimeout, 4096, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if len(sha) < 40 {
		return "", fmt.Errorf("unexpected commit hash %q", sha)
	}
	return sha, nil
}

func runGit(parentCtx context.Context, dir string, timeout time.Duration, maxBytes int, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	configureSubprocess(cmd)
	cmd.Env = cleanGitEnv()

	var stdout, stderr limitedBuffer
	stdout.max = maxBytes
	stderr.max = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if stdout.overflow {
		return nil, fmt.Errorf("git output exceeded %d bytes", maxBytes)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("git timed out after %s", timeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

func cleanGitEnv() []string {
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
	}
	for _, k := range []string{"PATH", "TMPDIR", "TMP", "TEMP", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}
