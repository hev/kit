package preview

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
)

func TestBindGuardConfigValidation(t *testing.T) {
	accepted := []string{
		"127.0.0.1",
		"127.0.0.2",
		"::1",
		"100.64.0.1",
		"100.104.233.125",
		"100.127.255.254",
	}
	for _, bind := range accepted {
		t.Run("accept_"+bind, func(t *testing.T) {
			cfgPath := filepath.Join(t.TempDir(), "config.toml")
			tomlText := fmt.Sprintf(`
[[preview]]
name = "site"
bind = %q
port = 4370
root = "/tmp/static"
`, bind)
			if err := os.WriteFile(cfgPath, []byte(tomlText), 0600); err != nil {
				t.Fatal(err)
			}
			cfgs, err := Load(cfgPath)
			if err != nil {
				t.Fatalf("expected bind %q to be accepted, got: %v", bind, err)
			}
			if len(cfgs) != 1 || cfgs[0].Bind != bind {
				t.Fatalf("unexpected loaded config: %+v", cfgs)
			}
		})
	}

	refused := []string{
		"0.0.0.0",
		"::",
		"192.168.1.10",
		"10.0.0.5",
		"172.16.0.1",
		"5.78.208.86",
		"100.63.255.255",
		"100.128.0.1",
		"169.254.1.1",
		"fe80::1",
		"fd7a:115c:a1e0::1",
		"::ffff:127.0.0.1",
		"::ffff:100.104.233.125",
		"localhost",
		"",
		"127.0.0.1:4370",
	}
	for _, bind := range refused {
		t.Run("refuse_"+bind, func(t *testing.T) {
			cfgPath := filepath.Join(t.TempDir(), "config.toml")
			tomlText := fmt.Sprintf(`
[[preview]]
name = "site"
bind = %q
port = 4370
root = "/tmp/static"
`, bind)
			if err := os.WriteFile(cfgPath, []byte(tomlText), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(cfgPath); err == nil {
				t.Fatalf("expected bind %q to be refused in config validation", bind)
			}
		})
	}
}

func TestBindGuardServeDefenseInDepth(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, bind := range []string{"0.0.0.0", "::", "192.168.1.10", "5.78.208.86"} {
		t.Run("serve_refuse_"+bind, func(t *testing.T) {
			cfg := Config{
				Name: "site",
				Bind: bind,
				Port: 18080,
				Root: staticDir,
			}
			if _, err := Handler(cfg, t.TempDir()); err == nil {
				t.Fatalf("Handler must refuse bind %q", bind)
			}
			if _, _, err := Listen(cfg); err == nil {
				t.Fatalf("Listen must refuse bind %q", bind)
			}
			if err := Serve(t.Context(), cfg, t.TempDir(), nil); err == nil {
				t.Fatalf("Serve must refuse bind %q", bind)
			}
		})
	}

	// Tailscale CGNAT IP passes the bind guard in Handler and ValidateBind.
	tsCfg := Config{Name: "site", Bind: "100.104.233.125", Port: 18080, Root: staticDir}
	if _, err := Handler(tsCfg, t.TempDir()); err != nil {
		t.Fatalf("Handler must accept Tailscale CGNAT IP, got: %v", err)
	}
	if _, err := ValidateBind("100.104.233.125"); err != nil {
		t.Fatalf("ValidateBind must accept 100.104.233.125: %v", err)
	}
}

func TestConfigValidationRules(t *testing.T) {
	// Missing file returns nil, nil.
	cfgs, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil || cfgs != nil {
		t.Fatalf("missing config should return nil, nil; got %v, %v", cfgs, err)
	}

	validCases := []struct {
		name  string
		toml  string
		check func(t *testing.T, c Config)
	}{
		{
			name: "repo_defaults",
			toml: `
[[preview]]
name = "kit-docs"
bind = "127.0.0.1"
port = 4370
repo = "https://github.com/hev/kit.git"
`,
			check: func(t *testing.T, c Config) {
				if c.Ref != DefaultRef {
					t.Fatalf("default ref = %q", c.Ref)
				}
				if c.OutDir != DefaultOutDir {
					t.Fatalf("default out_dir = %q", c.OutDir)
				}
				if c.EffectiveKeepBuilds() != DefaultKeepBuilds {
					t.Fatalf("default keep_builds = %d", c.EffectiveKeepBuilds())
				}
				if c.EffectiveTimeout() != DefaultTimeout {
					t.Fatalf("default timeout = %s", c.EffectiveTimeout())
				}
			},
		},
		{
			name: "bounds_min_max",
			toml: `
[[preview]]
name = "maxed"
bind = "100.104.233.125"
port = 65535
repo = "https://github.com/hev/kit.git"
ref = "origin/v0.7"
subdir = "site"
out_dir = "."
keep_builds = 5
timeout = "30m"
build_cmd = ["echo", "ok"]
env = { PUBLIC_KIT_APP_ORIGIN = "https://app.example.ts.net" }
`,
			check: func(t *testing.T, c Config) {
				if c.EffectiveKeepBuilds() != 5 || c.EffectiveTimeout() != 30*time.Minute {
					t.Fatalf("unexpected bounds: keep=%d timeout=%s", c.EffectiveKeepBuilds(), c.EffectiveTimeout())
				}
			},
		},
		{
			name: "port_min_keep_min",
			toml: `
[[preview]]
name = "minned"
bind = "::1"
port = 1024
root = "~/static"
keep_builds = 1
timeout = "1s"
`,
			check: func(t *testing.T, c Config) {
				if c.URL() != "http://[::1]:1024" {
					t.Fatalf("unexpected IPv6 URL: %q", c.URL())
				}
			},
		},
	}

	for _, tc := range validCases {
		t.Run("valid_"+tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(p, []byte(tc.toml), 0600); err != nil {
				t.Fatal(err)
			}
			cfgs, err := Load(p)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfgs) != 1 {
				t.Fatalf("expected 1 config, got %d", len(cfgs))
			}
			tc.check(t, cfgs[0])
		})
	}

	invalidCases := []struct {
		name string
		toml string
	}{
		{"unknown_field", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
bogus = true`},
		{"bad_name_upper", `[[preview]]
name = "Bad"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"`},
		{"bad_name_dash", `[[preview]]
name = "-bad"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"`},
		{"duplicate_name", `[[preview]]
name = "dup"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
[[preview]]
name = "dup"
bind = "127.0.0.1"
port = 4371
root = "/tmp/b"`},
		{"port_too_low", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 1023
root = "/tmp/a"`},
		{"port_too_high", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 65536
root = "/tmp/a"`},
		{"neither_repo_nor_root", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370`},
		{"both_repo_and_root", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
repo = "https://example.com/r.git"
root = "/tmp/a"`},
		{"root_with_ref", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
ref = "origin/main"`},
		{"subdir_traversal", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
repo = "https://example.com/r.git"
subdir = "../escape"`},
		{"out_dir_abs", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
repo = "https://example.com/r.git"
out_dir = "/etc"`},
		{"keep_builds_zero", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
keep_builds = 0`},
		{"keep_builds_too_high", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
keep_builds = 6`},
		{"timeout_too_high", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
timeout = "31m"`},
		{"timeout_zero", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
timeout = "0s"`},
		{"env_bad_key", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
env = { "1BAD" = "x" }`},
		{"env_case_dup", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
root = "/tmp/a"
env = { foo = "1", FOO = "2" }`},
		{"repo_flag_injection", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
repo = "--upload-pack=evil"`},
		{"ref_flag_injection", `[[preview]]
name = "a"
bind = "127.0.0.1"
port = 4370
repo = "https://example.com/r.git"
ref = "-main"`},
	}

	for _, tc := range invalidCases {
		t.Run("invalid_"+tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(p, []byte(tc.toml), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil {
				t.Fatalf("expected invalid config %q to be refused", tc.name)
			}
		})
	}
}

type bareTestRepo struct {
	bare string
	work string
}

func newBareTestRepo(t *testing.T) *bareTestRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	bare := filepath.Join(base, "origin.git")
	work := filepath.Join(base, "work")
	runGit(t, base, "init", "--bare", "-q", "-b", "v0.7", bare)
	runGit(t, base, "clone", "-q", bare, work)
	return &bareTestRepo{bare: bare, work: work}
}

func (r *bareTestRepo) commitAndPush(t *testing.T, branch string, files map[string]string, msg string, force bool) string {
	t.Helper()
	// Ensure we are on branch.
	if err := tryGit(r.work, "checkout", "-q", branch); err != nil {
		runGit(t, r.work, "checkout", "-q", "-b", branch)
	}
	for rel, body := range files {
		full := filepath.Join(r.work, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, r.work, "add", "-A")
	runGit(t, r.work, "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg)
	args := []string{"push", "-q", "origin", "HEAD:" + branch}
	if force {
		args = []string{"push", "-q", "--force", "origin", "HEAD:" + branch}
	}
	runGit(t, r.work, args...)
	return strings.TrimSpace(runGit(t, r.work, "rev-parse", "HEAD"))
}

func gitTestEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=tester",
		"GIT_AUTHOR_EMAIL=tester@example.test",
		"GIT_COMMITTER_NAME=tester",
		"GIT_COMMITTER_EMAIL=tester@example.test",
	)
}

func tryGit(dir string, args ...string) error {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = gitTestEnv()
	return c.Run()
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = gitTestEnv()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func shellBuildCmd(script string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/c", script}
	}
	return []string{"/bin/sh", "-c", script}
}

func TestSyncAtomicBoundedSkipPruneAndServe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test script uses POSIX sh")
	}
	repo := newBareTestRepo(t)
	sha1 := repo.commitAndPush(t, "v0.7", map[string]string{
		"site/page.txt": "version-1",
	}, "commit 1", false)

	home := t.TempDir()
	counterFile := filepath.Join(t.TempDir(), "builds.count")
	buildScript := fmt.Sprintf(`
echo x >> %q
mkdir -p dist/docs dist/empty
printf 'root-%%s-%%s' "$(cat page.txt)" "$PUBLIC_KIT_APP_ORIGIN" > dist/index.html
printf 'docs-%%s' "$(cat page.txt)" > dist/docs/index.html
`, counterFile)

	keep := 2
	cfg := Config{
		Name:       "kit-docs",
		Bind:       "127.0.0.1",
		Port:       freePort(t),
		Repo:       repo.bare,
		Ref:        "origin/v0.7",
		Subdir:     "site",
		BuildCmd:   shellBuildCmd(buildScript),
		OutDir:     "dist",
		KeepBuilds: &keep,
		Timeout:    "2m",
		Env:        map[string]string{"PUBLIC_KIT_APP_ORIGIN": "https://app1.ts.net"},
	}

	runner := Runner{Home: home}

	// Sync 1: builds commit 1.
	res1, err := runner.Sync(t.Context(), cfg, SyncOptions{})
	if err != nil {
		t.Fatalf("sync 1 failed: %v", err)
	}
	if res1.Skipped || res1.Status != "synced" || res1.SHA != sha1 {
		t.Fatalf("unexpected sync 1 result: %+v", res1)
	}
	if res1.Repo != "local:origin.git" {
		t.Fatalf("expected sanitized local repo label, got %q", res1.Repo)
	}
	if strings.Contains(res1.Repo, repo.bare) {
		t.Fatalf("preview metadata leaked local repo path: %q", res1.Repo)
	}

	// Verify preview.json schema and contents under current/.
	currentDir := filepath.Join(PreviewDir(home, cfg.Name), "current")
	rawMeta, err := os.ReadFile(filepath.Join(currentDir, "preview.json"))
	if err != nil {
		t.Fatalf("read preview.json: %v", err)
	}
	if strings.Contains(string(rawMeta), home) || strings.Contains(string(rawMeta), repo.bare) || strings.Contains(string(rawMeta), "app1.ts.net") {
		t.Fatalf("preview.json leaked local path or raw env value: %s", rawMeta)
	}
	var meta Metadata
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		t.Fatalf("parse preview.json: %v", err)
	}
	if meta.Schema != SchemaVersion || meta.Site != "kit-docs" || meta.Ref != "origin/v0.7" || meta.SHA != sha1 || meta.EnvFingerprint != EnvFingerprint(cfg.Env) || meta.BuiltAt == "" {
		t.Fatalf("unexpected preview.json metadata: %+v", meta)
	}
	if countLines(t, counterFile) != 1 {
		t.Fatalf("expected 1 build invocation, got %d", countLines(t, counterFile))
	}

	// Sync 2: unchanged SHA and env must skip rebuild.
	res2, err := runner.Sync(t.Context(), cfg, SyncOptions{})
	if err != nil {
		t.Fatalf("sync 2 failed: %v", err)
	}
	if !res2.Skipped || res2.Status != "up_to_date" || res2.SHA != sha1 {
		t.Fatalf("expected skip on unchanged SHA, got: %+v", res2)
	}
	if countLines(t, counterFile) != 1 {
		t.Fatalf("expected unchanged sync to skip build command, count=%d", countLines(t, counterFile))
	}

	// Start Serve on 127.0.0.1:<port> and test headers, MagicDNS, traversal, symlink escape, and live swap.
	srvCtx, cancelSrv := context.WithCancel(t.Context())
	defer cancelSrv()
	ready := make(chan string, 1)
	srvDone := make(chan error, 1)
	go func() {
		srvDone <- Serve(srvCtx, cfg, home, func(addr string) { ready <- addr })
	}()
	addr := <-ready

	// Normal requests and MagicDNS Host header.
	for _, host := range []string{"preview-host.example-tailnet.ts.net", fmt.Sprintf("100.104.233.125:%d", cfg.Port), addr} {
		resp, body := httpGetWithHost(t, addr, "/docs/", host)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("host %q: expected 200, got %d", host, resp.StatusCode)
		}
		if body != "docs-version-1" {
			t.Fatalf("host %q: unexpected body %q", host, body)
		}
		assertSecurityHeaders(t, resp)
	}

	// Root index.html and /docs redirect.
	resp, body := httpGetWithHost(t, addr, "/", addr)
	if resp.StatusCode != http.StatusOK || body != "root-version-1-https://app1.ts.net" {
		t.Fatalf("root response: status=%d body=%q", resp.StatusCode, body)
	}
	assertSecurityHeaders(t, resp)

	respNoRedirect, _ := httpGetNoRedirect(t, addr, "/docs")
	if respNoRedirect.StatusCode != http.StatusMovedPermanently || respNoRedirect.Header.Get("Location") != "/docs/" {
		t.Fatalf("expected /docs -> /docs/ redirect, got %d %q", respNoRedirect.StatusCode, respNoRedirect.Header.Get("Location"))
	}
	assertSecurityHeaders(t, respNoRedirect)

	// Directory without index.html must return 404 (no directory listing).
	respEmpty, _ := httpGetWithHost(t, addr, "/empty/", addr)
	if respEmpty.StatusCode != http.StatusNotFound {
		t.Fatalf("empty dir without index.html must return 404, got %d", respEmpty.StatusCode)
	}
	assertSecurityHeaders(t, respEmpty)

	// POST must be refused with 405 and security headers.
	postReq, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/", nil)
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", postResp.StatusCode)
	}
	assertSecurityHeaders(t, postResp)

	// Place a secret file outside the build root to prove traversal and symlink escape cannot read it.
	outsideSecret := filepath.Join(PreviewDir(home, cfg.Name), "outside-secret.txt")
	if err := os.WriteFile(outsideSecret, []byte("TOP-SECRET-OUTSIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	// Raw TCP path traversal requests (so Go's HTTP client doesn't normalize ../).
	traversalPaths := []string{
		"/../outside-secret.txt",
		"/../../outside-secret.txt",
		"/docs/../../outside-secret.txt",
		"/%2e%2e/outside-secret.txt",
		"/..%2foutside-secret.txt",
		"/..\\outside-secret.txt",
		"/docs/..%5c..%5coutside-secret.txt",
	}
	for _, rawPath := range traversalPaths {
		status, headers, rawBody := rawHTTPGet(t, addr, rawPath)
		if status != http.StatusForbidden {
			t.Fatalf("traversal %q: expected 403, got %d (body=%q)", rawPath, status, rawBody)
		}
		if strings.Contains(rawBody, "TOP-SECRET-OUTSIDE") {
			t.Fatalf("traversal %q leaked outside secret!", rawPath)
		}
		if headers.Get("X-Robots-Tag") != HeaderRobotsTag || headers.Get("X-Content-Type-Options") != HeaderContentTypeOptions {
			t.Fatalf("traversal %q missing security headers: %v", rawPath, headers)
		}
	}

	// Symlink escape inside active build must be refused with 403.
	realCurrent, err := filepath.EvalSymlinks(currentDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(realCurrent, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/outside/path", filepath.Join(realCurrent, "dangling-escape.txt")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/escape.txt", "/dangling-escape.txt"} {
		resp, body := httpGetWithHost(t, addr, p, addr)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("symlink escape %q: expected 403, got %d (body=%q)", p, resp.StatusCode, body)
		}
		if strings.Contains(body, "TOP-SECRET-OUTSIDE") {
			t.Fatalf("symlink escape %q leaked secret", p)
		}
		assertSecurityHeaders(t, resp)
	}

	// Sync 3: env change with same SHA triggers rebuild and live server picks it up without restart!
	cfg.Env = map[string]string{"PUBLIC_KIT_APP_ORIGIN": "https://app2.ts.net"}
	res3, err := runner.Sync(t.Context(), cfg, SyncOptions{})
	if err != nil {
		t.Fatalf("sync 3 (env change) failed: %v", err)
	}
	if res3.Skipped || res3.EnvFingerprint != EnvFingerprint(cfg.Env) {
		t.Fatalf("expected rebuild on env change, got %+v", res3)
	}
	_, body = httpGetWithHost(t, addr, "/", addr)
	if body != "root-version-1-https://app2.ts.net" {
		t.Fatalf("live server did not pick up atomic swap on env rebuild, got %q", body)
	}

	// Sync 4 & Sync 5: two more commits -> builds pruned to <= keep_builds (2).
	sha2 := repo.commitAndPush(t, "v0.7", map[string]string{"site/page.txt": "version-2"}, "commit 2", false)
	if _, err := runner.Sync(t.Context(), cfg, SyncOptions{}); err != nil {
		t.Fatalf("sync 4 failed: %v", err)
	}
	sha3 := repo.commitAndPush(t, "v0.7", map[string]string{"site/page.txt": "version-3"}, "commit 3", false)
	res5, err := runner.Sync(t.Context(), cfg, SyncOptions{})
	if err != nil {
		t.Fatalf("sync 5 failed: %v", err)
	}
	if res5.SHA != sha3 || sha3 == sha2 {
		t.Fatalf("expected sha3 %s, got %+v", sha3, res5)
	}
	buildsCount := countBuilds(filepath.Join(PreviewDir(home, cfg.Name), "builds"))
	if buildsCount > keep {
		t.Fatalf("expected builds <= %d after pruning, got %d", keep, buildsCount)
	}
	_, body = httpGetWithHost(t, addr, "/docs/", addr)
	if body != "docs-version-3" {
		t.Fatalf("live server did not reflect latest atomic swap, got %q", body)
	}

	// Failed build must leave current serving version-3 intact and leave no staging/tmp clutter.
	badCfg := cfg
	badCfg.BuildCmd = shellBuildCmd("echo boom >&2; exit 1")
	if _, err := runner.Sync(t.Context(), badCfg, SyncOptions{Force: true}); err == nil {
		t.Fatal("expected failing build command to return error")
	}
	_, body = httpGetWithHost(t, addr, "/docs/", addr)
	if body != "docs-version-3" {
		t.Fatalf("failed build corrupted active preview, got %q", body)
	}
	if _, err := os.Stat(filepath.Join(PreviewDir(home, cfg.Name), "staging")); !os.IsNotExist(err) {
		t.Fatalf("staging directory should be cleaned after failed build")
	}

	cancelSrv()
	if err := <-srvDone; err != nil {
		t.Fatalf("Serve returned error on shutdown: %v", err)
	}
}

func TestSyncDefaultPnpmCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub pnpm uses POSIX sh")
	}
	repo := newBareTestRepo(t)
	sha := repo.commitAndPush(t, "v0.7", map[string]string{
		"site/package.json": `{"name":"stub"}`,
		"site/content.txt":  "hello-pnpm",
	}, "initial", false)

	binDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "pnpm.log")
	stubPnpm := fmt.Sprintf(`#!/bin/sh
echo "$*" >> %q
case "$1" in
  install) exit 0 ;;
  build) mkdir -p dist && cp content.txt dist/index.html ;;
  *) exit 1 ;;
esac
`, logFile)
	pnpmPath := filepath.Join(binDir, "pnpm")
	if err := os.WriteFile(pnpmPath, []byte(stubPnpm), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	home := t.TempDir()
	cfg := Config{
		Name:   "pnpm-site",
		Bind:   "127.0.0.1",
		Port:   4371,
		Repo:   repo.bare,
		Ref:    "origin/v0.7",
		Subdir: "site",
	}
	res, err := (Runner{Home: home}).Sync(t.Context(), cfg, SyncOptions{})
	if err != nil {
		t.Fatalf("default pnpm sync failed: %v", err)
	}
	if res.SHA != sha || res.Status != "synced" {
		t.Fatalf("unexpected result: %+v", res)
	}
	calls, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "install --frozen-lockfile\nbuild\n"
	if string(calls) != want {
		t.Fatalf("pnpm calls = %q, want %q", string(calls), want)
	}
}

func TestSyncLockBusyAndTimeoutAndRedaction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test commands use POSIX sh")
	}
	repo := newBareTestRepo(t)
	repo.commitAndPush(t, "v0.7", map[string]string{"index.html": "ok"}, "init", false)

	home := t.TempDir()
	cfg := Config{
		Name:     "locked-site",
		Bind:     "127.0.0.1",
		Port:     4372,
		Repo:     repo.bare,
		Ref:      "origin/v0.7",
		BuildCmd: shellBuildCmd("mkdir -p dist && cp index.html dist/index.html"),
		Timeout:  "1m",
	}

	// Lock busy: hold sync.lock and verify Sync fails with ErrLockBusy.
	pDir := PreviewDir(home, cfg.Name)
	if err := platformops.MkdirAll(pDir, 0700); err != nil {
		t.Fatal(err)
	}
	release, err := platformops.LockPath(filepath.Join(pDir, "sync.lock"), true)
	if err != nil {
		t.Fatal(err)
	}
	_, lockErr := (Runner{Home: home}).Sync(t.Context(), cfg, SyncOptions{})
	release()
	if !errors.Is(lockErr, ErrLockBusy) {
		t.Fatalf("expected ErrLockBusy, got %v", lockErr)
	}

	// Timeout: build sleeps past timeout and must be terminated promptly.
	slowCfg := cfg
	slowCfg.Timeout = "300ms"
	slowCfg.BuildCmd = shellBuildCmd("sleep 10")
	start := time.Now()
	_, timeoutErr := (Runner{Home: home}).Sync(t.Context(), slowCfg, SyncOptions{})
	if timeoutErr == nil || !strings.Contains(timeoutErr.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", timeoutErr)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}

	// Redaction: secret token and local paths in build error must be scrubbed.
	secretToken := "sk-live-supersecret-token-9876543210abcdef"
	badCfg := cfg
	badCfg.BuildCmd = shellBuildCmd(fmt.Sprintf(`echo "failed at %s with %s and https://bot:topsecretpass@example.com/repo.git" >&2; exit 1`, home, secretToken))
	_, err = (Runner{Home: home}).Sync(t.Context(), badCfg, SyncOptions{})
	if err == nil {
		t.Fatal("expected build failure")
	}
	msg := err.Error()
	for _, leaked := range []string{secretToken, "topsecretpass", home, pDir} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("error message leaked sensitive data %q: %s", leaked, msg)
		}
	}
	if !strings.Contains(msg, "[REDACTED") {
		t.Fatalf("expected redaction marker in error, got: %s", msg)
	}
}

func TestSanitizeRepo(t *testing.T) {
	cases := map[string]string{
		"https://github.com/hev/kit.git":            "https://github.com/hev/kit.git",
		"https://bot:secret@github.com/hev/kit.git": "https://github.com/hev/kit.git",
		"git@github.com:hev/kit.git":                "git@github.com:hev/kit.git",
		"ssh://git@github.com/hev/kit.git":          "ssh://git@github.com/hev/kit.git",
		"ssh://git:secret@github.com/hev/kit.git":   "ssh://github.com/hev/kit.git",
		"/Users/operator/workspace/kit":             "local:kit",
		"file:///tmp/test-dir/origin.git":           "local:origin.git",
		`C:\Users\operator\workspace\kit`:           "local:kit",
	}
	for in, want := range cases {
		if got := SanitizeRepo(in); got != want {
			t.Errorf("SanitizeRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStaticRootModeAndUnbuiltRepo503(t *testing.T) {
	home := t.TempDir()
	staticRoot := filepath.Join(home, "my-site")
	if err := os.MkdirAll(staticRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("static-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	staticCfg := Config{
		Name: "static-site",
		Bind: "127.0.0.1",
		Port: 4380,
		Root: "~/my-site",
	}
	if _, err := (Runner{Home: home}).Sync(t.Context(), staticCfg, SyncOptions{}); err == nil {
		t.Fatal("Sync must refuse static root preview")
	}
	h, err := Handler(staticCfg, home)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "static-ok" {
		t.Fatalf("static serve got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Robots-Tag") != HeaderRobotsTag {
		t.Fatalf("missing robots header")
	}

	// Unbuilt repo preview returns 503 with security headers.
	unbuiltCfg := Config{
		Name: "unbuilt",
		Bind: "127.0.0.1",
		Port: 4381,
		Repo: "https://github.com/hev/kit.git",
	}
	uh, err := Handler(unbuiltCfg, home)
	if err != nil {
		t.Fatal(err)
	}
	urec := httptest.NewRecorder()
	uh.ServeHTTP(urec, httptest.NewRequest(http.MethodGet, "/", nil))
	if urec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unbuilt preview, got %d", urec.Code)
	}
	if urec.Header().Get("X-Robots-Tag") != HeaderRobotsTag || urec.Header().Get("X-Content-Type-Options") != HeaderContentTypeOptions {
		t.Fatalf("503 response missing security headers")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Split(strings.TrimSpace(string(b)), "\n"))
}

func assertSecurityHeaders(t *testing.T, resp *http.Response) {
	t.Helper()
	if got := resp.Header.Get("X-Content-Type-Options"); got != HeaderContentTypeOptions {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, HeaderContentTypeOptions)
	}
	if got := resp.Header.Get("X-Robots-Tag"); got != HeaderRobotsTag {
		t.Fatalf("X-Robots-Tag = %q, want %q", got, HeaderRobotsTag)
	}
}

func httpGetWithHost(t *testing.T, addr, reqPath, host string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+reqPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func httpGetNoRedirect(t *testing.T, addr, reqPath string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get("http://" + addr + reqPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func rawHTTPGet(t *testing.T, addr, rawPath string) (int, http.Header, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", rawPath, addr); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, string(b)
}
