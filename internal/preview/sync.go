package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

const metadataLimit = 64 << 10

// ErrLockBusy is returned when another process holds the preview's sync lock.
var ErrLockBusy = errors.New("preview sync already in progress")

// Metadata is the preview.json schema written into every completed build.
type Metadata struct {
	Schema         string `json:"schema"`
	Site           string `json:"site"`
	Repo           string `json:"repo"`
	Ref            string `json:"ref"`
	SHA            string `json:"sha"`
	EnvFingerprint string `json:"env_fingerprint"`
	BuiltAt        string `json:"built_at"`
}

// SyncOptions controls Sync behavior.
type SyncOptions struct {
	Force bool
}

// SyncResult describes the outcome of a Sync call.
type SyncResult struct {
	Site           string `json:"site"`
	Status         string `json:"status"`
	Repo           string `json:"repo"`
	Ref            string `json:"ref"`
	SHA            string `json:"sha"`
	EnvFingerprint string `json:"env_fingerprint"`
	BuiltAt        string `json:"built_at"`
	BuildID        string `json:"build_id,omitempty"`
	Skipped        bool   `json:"skipped"`
	Pruned         int    `json:"pruned"`
}

// Runner executes sync and inspect operations under Home.
type Runner struct {
	Home string
	Now  func() time.Time
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Sync fetches repo/ref, builds when (sha, env_fingerprint) changed, writes
// preview.json, atomically swaps current, and prunes old builds to <= keep_builds.
func (r Runner) Sync(ctx context.Context, cfg Config, opts SyncOptions) (SyncResult, error) {
	if err := cfg.Validate(); err != nil {
		return SyncResult{}, err
	}
	if cfg.Mode() == "static" {
		return SyncResult{}, fmt.Errorf("preview %q uses static root and does not sync from git", cfg.Name)
	}
	if r.Home == "" {
		return SyncResult{}, errors.New("preview home directory unavailable")
	}

	previewDir := PreviewDir(r.Home, cfg.Name)
	if err := platformops.MkdirAll(previewDir, 0700); err != nil {
		return SyncResult{}, errors.New("could not prepare preview directory")
	}

	release, err := platformops.LockPath(filepath.Join(previewDir, "sync.lock"), true)
	if err != nil {
		if platformops.IsLockBusy(err) {
			return SyncResult{}, ErrLockBusy
		}
		return SyncResult{}, errors.New("could not acquire preview sync lock")
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, cfg.EffectiveTimeout())
	defer cancel()

	sourceGit := filepath.Join(previewDir, "source.git")
	buildsDir := filepath.Join(previewDir, "builds")
	stagingBase := filepath.Join(previewDir, "staging")

	redactTerms := []string{r.Home, previewDir, sourceGit, buildsDir, stagingBase, cfg.Repo}

	if err := ensureBareMirror(ctx, sourceGit, cfg.Repo, redactTerms); err != nil {
		return SyncResult{}, err
	}

	sha, err := resolveRefSHA(ctx, sourceGit, cfg.Ref, redactTerms)
	if err != nil {
		return SyncResult{}, err
	}
	envFP := EnvFingerprint(cfg.Env)
	sanitizedRepo := SanitizeRepo(cfg.Repo)

	if !opts.Force {
		if curDir, curID, err := resolveCurrent(previewDir); err == nil {
			if curMeta, err := readMetadata(filepath.Join(curDir, "preview.json")); err == nil {
				if curMeta.Schema == SchemaVersion &&
					curMeta.Site == cfg.Name &&
					curMeta.SHA == sha &&
					curMeta.EnvFingerprint == envFP {
					pruned, _ := pruneBuilds(buildsDir, curID, cfg.EffectiveKeepBuilds())
					return SyncResult{
						Site:           cfg.Name,
						Status:         "up_to_date",
						Repo:           curMeta.Repo,
						Ref:            cfg.Ref,
						SHA:            sha,
						EnvFingerprint: envFP,
						BuiltAt:        curMeta.BuiltAt,
						BuildID:        curID,
						Skipped:        true,
						Pruned:         pruned,
					}, nil
				}
			}
		}
	}

	if err := platformops.MkdirAll(buildsDir, 0700); err != nil {
		return SyncResult{}, errors.New("could not create preview builds directory")
	}
	// Clean up any abandoned staging directories from prior interrupted runs.
	_ = os.RemoveAll(stagingBase)
	if err := platformops.MkdirAll(stagingBase, 0700); err != nil {
		return SyncResult{}, errors.New("could not create preview staging directory")
	}
	defer os.RemoveAll(stagingBase)

	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	stageDir := filepath.Join(stagingBase, "run-"+hex.EncodeToString(nonce[:]))
	srcDir := filepath.Join(stageDir, "src")
	if err := platformops.MkdirAll(srcDir, 0700); err != nil {
		return SyncResult{}, errors.New("could not create preview source directory")
	}
	redactTerms = append(redactTerms, stageDir, srcDir)

	// Extract commit tree into srcDir using a temporary index file so source.git stays clean.
	indexFile := filepath.Join(stageDir, "index")
	if _, err := runCommandTree(ctx, stageDir, gitEnv("GIT_INDEX_FILE="+indexFile), redactTerms,
		"git", "--git-dir", sourceGit, "--work-tree", srcDir, "checkout", "--force", sha, "--", "."); err != nil {
		return SyncResult{}, fmt.Errorf("checkout %s: %w", shortHex(sha, 12), err)
	}

	workDir := srcDir
	if cfg.Subdir != "" {
		workDir = filepath.Join(srcDir, filepath.FromSlash(cfg.Subdir))
	}
	if err := verifyDirWithin(srcDir, workDir); err != nil {
		return SyncResult{}, fmt.Errorf("subdir %q is not a valid directory in commit %s", cfg.Subdir, shortHex(sha, 12))
	}
	redactTerms = append(redactTerms, workDir)

	buildEnv := mergeEnv(os.Environ(), cfg.Env)
	if len(cfg.BuildCmd) == 0 {
		if _, err := runCommandTree(ctx, workDir, buildEnv, redactTerms, "pnpm", "install", "--frozen-lockfile"); err != nil {
			return SyncResult{}, fmt.Errorf("build install: %w", err)
		}
		if _, err := runCommandTree(ctx, workDir, buildEnv, redactTerms, "pnpm", "build"); err != nil {
			return SyncResult{}, fmt.Errorf("build: %w", err)
		}
	} else {
		if _, err := runCommandTree(ctx, workDir, buildEnv, redactTerms, cfg.BuildCmd[0], cfg.BuildCmd[1:]...); err != nil {
			return SyncResult{}, fmt.Errorf("build: %w", err)
		}
	}

	outDir := filepath.Join(workDir, filepath.FromSlash(cfg.EffectiveOutDir()))
	if err := verifyDirWithin(srcDir, outDir); err != nil {
		return SyncResult{}, fmt.Errorf("out_dir %q was not produced inside the build workspace", cfg.EffectiveOutDir())
	}

	builtAt := r.now()
	buildID := nextBuildID(buildsDir, builtAt, sha, envFP)
	tmpBuild := filepath.Join(buildsDir, buildID+".tmp")
	finalBuild := filepath.Join(buildsDir, buildID)
	_ = os.RemoveAll(tmpBuild)
	defer os.RemoveAll(tmpBuild)

	if err := copyDirContained(outDir, tmpBuild); err != nil {
		return SyncResult{}, fmt.Errorf("stage build output: %w", err)
	}

	meta := Metadata{
		Schema:         SchemaVersion,
		Site:           cfg.Name,
		Repo:           sanitizedRepo,
		Ref:            cfg.Ref,
		SHA:            sha,
		EnvFingerprint: envFP,
		BuiltAt:        builtAt.Format(time.RFC3339),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return SyncResult{}, errors.New("could not encode preview.json")
	}
	metaBytes = append(metaBytes, '\n')
	if err := platformops.WriteFileAtomic(filepath.Join(tmpBuild, "preview.json"), metaBytes, 0600); err != nil {
		return SyncResult{}, errors.New("could not write preview.json")
	}

	if err := os.Rename(tmpBuild, finalBuild); err != nil {
		return SyncResult{}, errors.New("could not finalize build directory")
	}
	if err := swapCurrent(previewDir, buildID); err != nil {
		return SyncResult{}, errors.New("could not atomically swap current build")
	}

	pruned, err := pruneBuilds(buildsDir, buildID, cfg.EffectiveKeepBuilds())
	if err != nil {
		return SyncResult{}, errors.New("could not prune old builds")
	}

	return SyncResult{
		Site:           cfg.Name,
		Status:         "synced",
		Repo:           meta.Repo,
		Ref:            meta.Ref,
		SHA:            meta.SHA,
		EnvFingerprint: meta.EnvFingerprint,
		BuiltAt:        meta.BuiltAt,
		BuildID:        buildID,
		Skipped:        false,
		Pruned:         pruned,
	}, nil
}

func ensureBareMirror(ctx context.Context, sourceGit, repo string, redactTerms []string) error {
	env := gitEnv()
	if _, err := os.Stat(filepath.Join(sourceGit, "HEAD")); os.IsNotExist(err) {
		if err := platformops.MkdirAll(sourceGit, 0700); err != nil {
			return errors.New("could not create source.git directory")
		}
		if _, err := runCommandTree(ctx, sourceGit, env, redactTerms, "git", "init", "--bare", "--quiet"); err != nil {
			return fmt.Errorf("init source mirror: %w", err)
		}
		if _, err := runCommandTree(ctx, sourceGit, env, redactTerms, "git", "remote", "add", "origin", repo); err != nil {
			return fmt.Errorf("configure remote: %w", err)
		}
	} else if err != nil {
		return errors.New("could not inspect source.git")
	} else {
		if _, err := runCommandTree(ctx, sourceGit, env, redactTerms, "git", "remote", "set-url", "origin", repo); err != nil {
			return fmt.Errorf("update remote: %w", err)
		}
	}
	if _, err := runCommandTree(ctx, sourceGit, env, redactTerms,
		"git", "fetch", "--quiet", "--prune", "--tags", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return fmt.Errorf("fetch ref: %w", err)
	}
	return nil
}

func resolveRefSHA(ctx context.Context, sourceGit, ref string, redactTerms []string) (string, error) {
	env := gitEnv()
	candidates := []string{ref + "^{commit}"}
	if !strings.HasPrefix(ref, "origin/") && !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates, "origin/"+ref+"^{commit}")
	}
	var lastErr error
	for _, spec := range candidates {
		out, err := runCommandTree(ctx, sourceGit, env, redactTerms, "git", "rev-parse", "--verify", "--quiet", spec)
		if err == nil {
			sha := strings.TrimSpace(out)
			if len(sha) >= 40 && isHex(sha) {
				return sha, nil
			}
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, lastErr)
	}
	return "", fmt.Errorf("ref %q did not resolve to a commit", ref)
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func shortHex(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// nextBuildID generates a strictly monotonic lexicographically sortable build ID
// so pruning order never depends on coarse filesystem mtime granularity.
func nextBuildID(buildsDir string, now time.Time, sha, envFP string) string {
	seq := now.UnixNano()
	if seq < 1 {
		seq = 1
	}
	if entries, err := os.ReadDir(buildsDir); err == nil {
		for _, e := range entries {
			prefix, _, ok := strings.Cut(e.Name(), "-")
			if !ok {
				continue
			}
			if n, err := strconv.ParseInt(prefix, 10, 64); err == nil && n >= seq {
				seq = n + 1
			}
		}
	}
	return fmt.Sprintf("%020d-%s-%s", seq, shortHex(sha, 12), shortHex(envFP, 8))
}

func readMetadata(path string) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, metadataLimit+1))
	if err != nil || len(data) > metadataLimit {
		return Metadata{}, errors.New("preview.json exceeds limit")
	}
	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return Metadata{}, err
	}
	return m, nil
}

func verifyDirWithin(baseDir, targetDir string) error {
	realBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return err
	}
	realTarget, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return err
	}
	if !isWithin(realBase, realTarget) {
		return errors.New("path escapes workspace")
	}
	st, err := os.Stat(realTarget)
	if err != nil || !st.IsDir() {
		return errors.New("not a directory")
	}
	return nil
}

func isWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// copyDirContained copies src into dst while refusing symlinks that escape src.
func copyDirContained(src, dst string) error {
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return errors.New("source output directory unavailable")
	}
	if err := platformops.MkdirAll(dst, 0700); err != nil {
		return errors.New("could not create staging output directory")
	}
	return filepath.WalkDir(realSrc, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(realSrc, p)
		if err != nil || !isWithin(realSrc, p) {
			return errors.New("output path escapes build directory")
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		typ := d.Type()
		if typ&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil || !isWithin(realSrc, resolved) {
				return errors.New("build output contains symlink escaping out_dir")
			}
			linkTarget, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if filepath.IsAbs(linkTarget) {
				// Rewrite internal absolute symlink as a safe relative symlink.
				if relLink, err := filepath.Rel(filepath.Dir(p), resolved); err == nil {
					linkTarget = relLink
				} else {
					return errors.New("build output contains absolute symlink")
				}
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				// On platforms without symlink privileges, materialize regular files safely.
				st, statErr := os.Stat(resolved)
				if statErr != nil || !st.Mode().IsRegular() {
					return errors.New("could not copy symlink in build output")
				}
				return copyRegularFile(resolved, target, st.Mode().Perm())
			}
			return nil
		}
		if d.IsDir() {
			return platformops.MkdirAll(target, 0700)
		}
		if !typ.IsRegular() {
			return errors.New("build output contains unsupported non-regular file")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyRegularFile(p, target, info.Mode().Perm())
	})
}

func copyRegularFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	mode := perm & 0755
	if mode&0400 == 0 {
		mode |= 0600
	}
	out, err := platformops.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// pruneBuilds removes leftover .tmp dirs and oldest builds beyond keepBuilds,
// never deleting activeID.
func pruneBuilds(buildsDir, activeID string, keepBuilds int) (int, error) {
	entries, err := os.ReadDir(buildsDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var dirs []string
	pruned := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			if err := os.RemoveAll(filepath.Join(buildsDir, name)); err == nil {
				pruned++
			}
			continue
		}
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, name)
	}
	// Newest first (lexicographic descending on our zero-padded sequence prefix).
	sort.Slice(dirs, func(i, j int) bool { return dirs[i] > dirs[j] })

	kept := make(map[string]bool, keepBuilds)
	if activeID != "" {
		kept[activeID] = true
	}
	for _, name := range dirs {
		if len(kept) >= keepBuilds {
			break
		}
		kept[name] = true
	}
	for _, name := range dirs {
		if kept[name] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(buildsDir, name)); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}
