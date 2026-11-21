package score

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

const (
	StatusScored       = "scored"
	StatusDisqualified = "disqualified"

	// MaxStdoutBytes bounds stdout read from a scorer command (1 MiB).
	MaxStdoutBytes = 1 << 20
	// MaxStderrBytes bounds stderr retained from a scorer command (64 KiB).
	MaxStderrBytes = 64 << 10
	// gitCheckoutTimeout bounds temporary checkout setup.
	gitCheckoutTimeout = 60 * time.Second
)

// ErrTimeout is returned when the scorer command exceeds its configured timeout.
var ErrTimeout = errors.New("scorer command timed out")

// RunOptions configures one synchronous commit scoring run.
type RunOptions struct {
	ScorerPath   string
	PinStorePath string
	RepoPath     string
	Commit       string
	Parent       string
	ExtraProtect []string
	Session      string
	Host         string
}

// Result is the structured outcome of scoring a candidate commit.
// When Status == StatusDisqualified, Value, Mark, and Marks are always nil.
type Result struct {
	Scorer         string         `json:"scorer"`
	Status         string         `json:"status"`
	Commit         string         `json:"commit"`
	Parent         string         `json:"parent,omitempty"`
	Pin            string         `json:"pin"`
	Direction      string         `json:"direction"`
	Unit           string         `json:"unit,omitempty"`
	Scale          int            `json:"scale,omitempty"`
	Hidden         bool           `json:"hidden"`
	Value          *float64       `json:"value,omitempty"`
	Mark           *int           `json:"mark,omitempty"`
	Marks          map[string]int `json:"marks,omitempty"`
	N              *int           `json:"n,omitempty"`
	Detail         map[string]any `json:"detail,omitempty"`
	ProtectedPaths []string       `json:"protected_paths,omitempty"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
}

// Eval converts a scored Result into an RFC 0005 trace.Eval row compatible with
// internal/layer and internal/filearchive. Disqualified results cannot produce
// numeric evaluation rows.
func (r Result) Eval(repoName, session, host string, ts time.Time) (trace.Eval, error) {
	if r.Status != StatusScored || r.Mark == nil || r.Value == nil {
		return trace.Eval{}, fmt.Errorf("cannot build eval row for %s result", r.Status)
	}
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		repoName = "repo"
	}
	subject := repoName + "@" + r.Commit
	if strings.TrimSpace(session) == "" {
		session = "commit:" + subject
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	markKey := strings.ReplaceAll(r.Scorer, "-", "_")
	summary := fmt.Sprintf("%s: %g", r.Scorer, *r.Value)
	if r.Unit != "" {
		summary += " " + r.Unit
	}
	evidence := map[string]string{
		"subject":   subject,
		"scorer":    r.Scorer + " " + r.Pin,
		"direction": r.Direction,
		"scale":     strconv.Itoa(r.Scale),
	}
	if r.Parent != "" {
		evidence["parent"] = r.Parent
	}
	return trace.Eval{
		Session:  session,
		TS:       ts.UTC().Format(time.RFC3339Nano),
		Role:     "scorer",
		Instance: r.Scorer,
		Host:     host,
		Marks:    map[string]int{markKey: *r.Mark},
		Poor:     false,
		Summary:  summary,
		Evidence: evidence,
		Findings: []string{},
	}, nil
}

// RunCommit verifies the scorer's host pin set, checks protected paths if a
// parent revision is given, runs the pinned host command in an isolated
// temporary checkout at Commit under the configured timeout, re-verifies the
// pin set after execution, and parses and scales the numeric mark.
func RunCommit(ctx context.Context, opts RunOptions) (Result, error) {
	scrubber, err := newEphemeralScrubber()
	if err != nil {
		return Result{}, err
	}
	hostHome, _ := os.UserHomeDir()
	envSecrets := collectSensitiveEnvValues()

	sanitize := func(s string, extraPaths ...string) string {
		return redactOutput(scrubber, s, envSecrets, append(extraPaths, hostHome))
	}

	if strings.TrimSpace(opts.Commit) == "" {
		return Result{}, fmt.Errorf("commit revision is required")
	}

	// 1. Verify host pin set before touching git or running any command.
	spec, pinSet, err := Verify(opts.PinStorePath, opts.ScorerPath)
	if err != nil {
		return Result{}, err
	}

	repoPath := strings.TrimSpace(opts.RepoPath)
	if repoPath == "" {
		repoPath = "."
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return Result{}, fmt.Errorf("invalid repository path")
	}
	if _, err := runGit(ctx, absRepo, gitDiffTimeout, 4096, "rev-parse", "--git-dir"); err != nil {
		return Result{}, fmt.Errorf("invalid git repository: %s", sanitize(err.Error(), absRepo))
	}

	commitSHA, err := resolveCommit(ctx, absRepo, opts.Commit)
	if err != nil {
		return Result{}, fmt.Errorf("resolve commit: %s", sanitize(err.Error(), absRepo))
	}

	var parentSHA string
	allProtect := append(append([]string(nil), spec.Protect...), opts.ExtraProtect...)
	if strings.TrimSpace(opts.Parent) != "" {
		parentSHA, err = resolveCommit(ctx, absRepo, opts.Parent)
		if err != nil {
			return Result{}, fmt.Errorf("resolve parent: %s", sanitize(err.Error(), absRepo))
		}
		if len(allProtect) > 0 {
			changed, err := DiffPaths(ctx, absRepo, parentSHA, commitSHA)
			if err != nil {
				return Result{}, fmt.Errorf("%s", sanitize(err.Error(), absRepo))
			}
			matched, err := CheckProtectedPaths(changed, allProtect)
			if err != nil {
				return Result{}, err
			}
			if len(matched) > 0 {
				return Result{
					Scorer:         spec.Name,
					Status:         StatusDisqualified,
					Commit:         commitSHA,
					Parent:         parentSHA,
					Pin:            pinSet.Pin,
					Direction:      spec.Direction,
					Unit:           spec.Unit,
					Hidden:         spec.Hidden,
					ProtectedPaths: matched,
				}, nil
			}
		}
	}

	// 2. Prepare isolated scratch root, detached checkout, scratch HOME and TMPDIR.
	scratchRoot, err := os.MkdirTemp("", "hev-score-*")
	if err != nil {
		return Result{}, fmt.Errorf("create scoring workspace: %w", err)
	}
	defer os.RemoveAll(scratchRoot)
	_ = platformops.SecureDir(scratchRoot)

	checkoutDir := filepath.Join(scratchRoot, "wt")
	scratchHome := filepath.Join(scratchRoot, "home")
	scratchTmp := filepath.Join(scratchRoot, "tmp")
	for _, d := range []string{scratchHome, scratchTmp} {
		if err := platformops.MkdirAll(d, 0700); err != nil {
			return Result{}, fmt.Errorf("prepare scoring workspace: %w", err)
		}
	}
	localPaths := []string{checkoutDir, scratchHome, scratchTmp, scratchRoot, absRepo, filepath.Dir(pinSet.SpecPath)}

	if err := prepareDetachedCheckout(ctx, absRepo, checkoutDir, commitSHA); err != nil {
		return Result{}, fmt.Errorf("checkout candidate %s: %s", shortHex(commitSHA), sanitize(err.Error(), localPaths...))
	}

	// 3. Execute the pinned host command copy in the detached checkout with bounded timeout & minimal env.
	runCtx, cancel := context.WithTimeout(ctx, spec.TimeoutDuration())
	defer cancel()

	cmd := exec.CommandContext(runCtx, pinSet.Command.Path)
	configureSubprocess(cmd)
	cmd.Dir = checkoutDir
	cmd.Env = scorerMinimalEnv(spec, pinSet, commitSHA, checkoutDir, scratchHome, scratchTmp)

	var stdout, stderr limitedBuffer
	stdout.max = MaxStdoutBytes
	stderr.max = MaxStderrBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	// 4. Re-verify pin set immediately after execution so any mutation during the run fails closed.
	if _, _, postErr := Verify(opts.PinStorePath, opts.ScorerPath); postErr != nil {
		return Result{}, fmt.Errorf("post-run pin verification failed: %w", postErr)
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return Result{}, fmt.Errorf("%w after %s (%s)", ErrTimeout, spec.TimeoutDuration(), spec.Name)
	}
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			if stderr.overflow {
				detail += " [truncated]"
			}
			return Result{}, fmt.Errorf("scorer %s command failed: %s", spec.Name, sanitize(detail, localPaths...))
		}
		return Result{}, fmt.Errorf("scorer %s command failed: %s", spec.Name, sanitize(runErr.Error(), localPaths...))
	}
	if stdout.overflow {
		return Result{}, fmt.Errorf("scorer %s stdout exceeded %d bytes", spec.Name, MaxStdoutBytes)
	}

	// 5. Parse numeric score and scale to an integer mark.
	parsed, err := parseMetricOutput(stdout.Bytes())
	if err != nil {
		return Result{}, fmt.Errorf("scorer %s invalid output: %s", spec.Name, sanitize(err.Error(), localPaths...))
	}
	mark, err := scaleToMark(parsed.Value, spec.Scale)
	if err != nil {
		return Result{}, fmt.Errorf("scorer %s scale mark: %w", spec.Name, err)
	}

	var scrubbedDetail map[string]any
	if len(parsed.Detail) > 0 {
		if m, ok := scrubValue(parsed.Detail, func(s string) string { return sanitize(s, localPaths...) }).(map[string]any); ok {
			scrubbedDetail = m
		}
	}

	val := parsed.Value
	markCopy := mark
	return Result{
		Scorer:     spec.Name,
		Status:     StatusScored,
		Commit:     commitSHA,
		Parent:     parentSHA,
		Pin:        pinSet.Pin,
		Direction:  spec.Direction,
		Unit:       spec.Unit,
		Scale:      spec.Scale,
		Hidden:     spec.Hidden,
		Value:      &val,
		Mark:       &markCopy,
		Marks:      map[string]int{spec.MarkName(): markCopy},
		N:          parsed.N,
		Detail:     scrubbedDetail,
		DurationMS: elapsed.Milliseconds(),
	}, nil
}

func prepareDetachedCheckout(ctx context.Context, srcRepo, dstDir, commitSHA string) error {
	cctx, cancel := context.WithTimeout(ctx, gitCheckoutTimeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "git", "-c", "protocol.file.allow=always", "clone", "--quiet", "--local", "--no-checkout", srcRepo, dstDir)
	configureSubprocess(cmd)
	cmd.Env = cleanGitEnv()
	var stderr limitedBuffer
	stderr.max = 16 << 10
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("checkout clone timed out")
		}
		return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	// Strip the origin remote so the checkout carries no reference back to srcRepo.
	_, _ = runGit(cctx, dstDir, gitDiffTimeout, 4096, "remote", "remove", "origin")
	if _, err := runGit(cctx, dstDir, gitCheckoutTimeout, 4096, "checkout", "--quiet", "--detach", commitSHA); err != nil {
		// Unreferenced commits in srcRepo may not be copied by default clone refs; fetch directly by SHA if needed.
		if _, ferr := runGit(cctx, dstDir, gitCheckoutTimeout, 4096, "-c", "protocol.file.allow=always", "fetch", "--quiet", "--no-tags", srcRepo, commitSHA); ferr != nil {
			return err
		}
		if _, cerr := runGit(cctx, dstDir, gitCheckoutTimeout, 4096, "checkout", "--quiet", "--detach", "FETCH_HEAD"); cerr != nil {
			return cerr
		}
	}
	return nil
}

func scorerMinimalEnv(spec Spec, pin PinSet, commitSHA, checkoutDir, scratchHome, scratchTmp string) []string {
	dataPaths := make([]string, len(pin.Data))
	for i, d := range pin.Data {
		dataPaths[i] = d.Path
	}
	env := []string{
		"HOME=" + scratchHome,
		"USERPROFILE=" + scratchHome,
		"TMPDIR=" + scratchTmp,
		"TMP=" + scratchTmp,
		"TEMP=" + scratchTmp,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"HEV_SCORE_SCORER=" + spec.Name,
		"HEV_SCORE_COMMIT=" + commitSHA,
		"HEV_SCORE_WORKTREE=" + checkoutDir,
		"HEV_SCORE_DATA_DIR=" + filepath.Dir(pin.SpecPath),
		"HEV_SCORE_DATA=" + strings.Join(dataPaths, string(os.PathListSeparator)),
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	for _, k := range []string{"LANG", "LC_ALL"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT"} {
			if v := os.Getenv(k); v != "" {
				env = append(env, k+"="+v)
			}
		}
	}
	for i, p := range dataPaths {
		env = append(env, fmt.Sprintf("HEV_SCORE_DATA_%d=%s", i, p))
	}
	return env
}

type metricOutput struct {
	Value  float64
	N      *int
	Detail map[string]any
}

func parseMetricOutput(raw []byte) (metricOutput, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return metricOutput{}, fmt.Errorf("empty stdout")
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Value  *float64       `json:"value"`
			N      *int           `json:"n,omitempty"`
			Detail map[string]any `json:"detail,omitempty"`
		}
		dec := json.NewDecoder(strings.NewReader(trimmed))
		if err := dec.Decode(&payload); err != nil {
			return metricOutput{}, fmt.Errorf("decode JSON output: %w", err)
		}
		var trailing any
		if err := dec.Decode(&trailing); err != nil && !errors.Is(err, os.ErrClosed) && err.Error() != "EOF" {
			return metricOutput{}, fmt.Errorf("unexpected trailing content after JSON object")
		}
		if payload.Value == nil {
			return metricOutput{}, fmt.Errorf("JSON output missing numeric \"value\" field")
		}
		if math.IsNaN(*payload.Value) || math.IsInf(*payload.Value, 0) {
			return metricOutput{}, fmt.Errorf("metric value must be finite")
		}
		return metricOutput{Value: *payload.Value, N: payload.N, Detail: payload.Detail}, nil
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return metricOutput{}, fmt.Errorf("expected a number or JSON object with \"value\"")
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return metricOutput{}, fmt.Errorf("expected a finite number or JSON object with \"value\"")
	}
	return metricOutput{Value: v}, nil
}

func scaleToMark(value float64, scale int) (int, error) {
	scaled := value * float64(scale)
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || math.Abs(scaled) > 1e15 {
		return 0, fmt.Errorf("scaled mark out of range")
	}
	return int(math.Round(scaled)), nil
}

// limitedBuffer caps retained bytes while discarding overflow so child writers do not deadlock on pipes.
type limitedBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	origLen := len(p)
	if b.max <= 0 {
		b.overflow = true
		return origLen, nil
	}
	rem := b.max - b.buf.Len()
	if rem <= 0 {
		b.overflow = true
		return origLen, nil
	}
	if len(p) > rem {
		_, _ = b.buf.Write(p[:rem])
		b.overflow = true
		return origLen, nil
	}
	_, _ = b.buf.Write(p)
	return origLen, nil
}

func (b *limitedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *limitedBuffer) String() string { return b.buf.String() }

func newEphemeralScrubber() (*redact.Scrubber, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return redact.New(salt)
}

func collectSensitiveEnvValues() []string {
	sensitiveTokens := []string{
		"SECRET", "TOKEN", "KEY", "PASS", "CRED", "AUTH",
		"PRIVATE", "BEARER", "COOKIE", "SESSION", "SIGN", "DSN", "WEBHOOK",
	}
	seen := map[string]bool{}
	var values []string
	for _, entry := range os.Environ() {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) < 4 {
			continue
		}
		upper := strings.ToUpper(k)
		isSensitive := strings.HasPrefix(v, "op://")
		if !isSensitive {
			for _, tok := range sensitiveTokens {
				if strings.Contains(upper, tok) {
					isSensitive = true
					break
				}
			}
		}
		if isSensitive && !seen[v] {
			seen[v] = true
			values = append(values, v)
		}
	}
	// Replace longer secrets first so partial prefix matches never leave suffixes behind.
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

func redactOutput(scrubber *redact.Scrubber, text string, envSecrets []string, localPaths []string) string {
	if text == "" {
		return ""
	}
	for _, secret := range envSecrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED:env]")
		}
	}
	clean, _ := scrubber.Text(text)
	// Replace longer paths first so nested directories match before parents.
	paths := make([]string, 0, len(localPaths))
	for _, p := range localPaths {
		p = strings.TrimSpace(p)
		if p != "" && p != "/" && p != "." {
			paths = append(paths, p)
			slash := filepath.ToSlash(p)
			if slash != p {
				paths = append(paths, slash)
			}
		}
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		clean = strings.ReplaceAll(clean, p, "<redacted-path>")
	}
	return clean
}

func scrubValue(v any, scrub func(string) string) any {
	switch x := v.(type) {
	case string:
		return scrub(x)
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = scrubValue(x[i], scrub)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = scrubValue(val, scrub)
		}
		return out
	default:
		return v
	}
}
