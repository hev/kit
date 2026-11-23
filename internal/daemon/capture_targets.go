package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/storagemode"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/workspace"
)

type targetConfig struct {
	Name       string   `toml:"name"`
	Endpoint   string   `toml:"endpoint"`
	ArchiveDir string   `toml:"archive_dir"`
	Namespace  string   `toml:"namespace"`
	KeyRef     string   `toml:"api_key_ref"`
	Mode       string   `toml:"mode"`
	Workspaces []string `toml:"workspaces"`
	Dirs       []string `toml:"dirs"`
}

type captureTarget struct {
	targetConfig
	owners workspace.Config
	err    error
}

const primaryTarget = "primary"

func captureTargets(entries []targetConfig, workspaces map[string]workspace.Workspace, host string) []captureTarget {
	if workspaces == nil {
		workspaces = make(map[string]workspace.Workspace)
	}
	if w := workspaces[workspace.Default]; w.Dir == "" {
		w.Dir = "~/workspace"
		workspaces[workspace.Default] = w
	}
	owners := workspace.Config{Workspaces: workspaces}
	targets := make([]captureTarget, 0, len(entries))
	for _, entry := range entries {
		targets = append(targets, captureTarget{targetConfig: entry, owners: owners})
	}

	names := make([]string, 0, len(workspaces))
	for name := range workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w := workspaces[name]
		s := workspace.Selection{Name: name, Workspace: w}
		t := w.TraceSettings()
		if w.Archived || !s.AllowsHost(host) || (w.TraceCapture == (workspace.TraceCapture{}) && t.Endpoint == "" && t.KeyRef == "") {
			continue
		}
		namespace, _ := s.Namespaces()
		entry := targetConfig{Name: name, Endpoint: t.Endpoint, Namespace: namespace, ArchiveDir: t.ArchiveDir, KeyRef: t.KeyRef, Mode: t.Mode}
		if entry.Mode != "mirror" {
			entry.Workspaces = []string{name}
		}
		if entry.ArchiveDir == "" && entry.Endpoint == "" && entry.KeyRef == "" {
			entry.ArchiveDir = filepath.Join(s.Root(), "archive")
		}
		targets = append(targets, captureTarget{targetConfig: entry, owners: owners})
	}
	counts := map[string]int{primaryTarget: 1}
	for _, t := range targets {
		counts[strings.ToLower(t.Name)]++
	}
	for i := range targets {
		t := &targets[i]
		switch {
		case !workspace.ValidName(t.Name) || !filepath.IsLocal(t.Name) || len(t.Name) > 64:
			t.Name = fmt.Sprintf("(invalid target %d)", i+1)
			t.err = errors.New("invalid target name")
		case counts[strings.ToLower(t.Name)] > 1:
			t.err = errors.New("duplicate or reserved target name")
		default:
			t.err = t.validate()
		}
	}
	return targets
}

func (t *captureTarget) validate() error {
	if !workspace.ValidName(t.Namespace) {
		return errors.New("invalid or missing namespace")
	}
	filtered := len(t.Workspaces)+len(t.Dirs) > 0
	if t.Mode == "" {
		t.Mode = "mirror"
		if filtered {
			t.Mode = "workspace"
		}
	}
	if (t.Mode != "mirror" || filtered) && (t.Mode != "workspace" || !filtered) {
		return errors.New("workspace mode requires filters; mirror mode requires an empty filter")
	}
	for _, name := range t.Workspaces {
		w, ok := t.owners.Workspaces[name]
		if !ok || w.Archived || w.Dir == "" || !filepath.IsAbs(expandPath(w.Dir)) {
			return errors.New("workspace filter requires an active workspace with an absolute directory")
		}
	}
	for _, dir := range t.Dirs {
		if !filepath.IsAbs(expandPath(dir)) {
			return errors.New("directory filter requires an absolute directory")
		}
	}
	if t.ArchiveDir != "" {
		if t.Endpoint != "" || t.KeyRef != "" {
			return errors.New("archive_dir cannot be combined with a Layer endpoint or key reference")
		}
		if !filepath.IsAbs(expandPath(t.ArchiveDir)) {
			return errors.New("archive_dir must be absolute")
		}
		return nil
	}
	if t.Endpoint == "" {
		t.Endpoint = layer.DefaultEndpoint
	}
	if CaptureEndpoint(t.Endpoint) == "" {
		return errors.New("invalid Layer endpoint")
	}
	if t.KeyRef == "" {
		return errors.New("Layer target requires an API key reference")
	}
	return nil
}

// Archives that share a namespace family also share derived rows such as
// sessions and blocks. Keep a scoped target away from another writer's rows.
func checkCaptureDestinations(targets []captureTarget, primary targetConfig) []captureTarget {
	specs := slices.Clone(targets)
	for i := range specs {
		if specs[i].err != nil {
			continue
		}
		if overlapsCaptureArchive(specs[i].targetConfig, primary) {
			specs[i].err = errors.New("archive overlaps the primary target")
			continue
		}
		for j := range specs[:i] {
			if specs[j].err == nil && overlapsCaptureArchive(specs[i].targetConfig, specs[j].targetConfig) {
				specs[i].err = errors.New("archive overlaps another capture target")
				break
			}
		}
	}
	return specs
}

func overlapsCaptureArchive(a, b targetConfig) bool {
	if a.ArchiveDir != "" && b.ArchiveDir != "" {
		left := filepath.Join(expandPath(a.ArchiveDir), a.Namespace)
		right := filepath.Join(expandPath(b.ArchiveDir), b.Namespace)
		return workspace.Contains(left, right) || workspace.Contains(right, left)
	}
	if a.ArchiveDir != "" || b.ArchiveDir != "" || !strings.EqualFold(strings.TrimRight(a.Endpoint, "/"), strings.TrimRight(b.Endpoint, "/")) {
		return false
	}
	if a.Namespace == b.Namespace {
		return true
	}
	for _, suffix := range []string{"-blocks", "-sessions", "-spans", "-instructions", "-evals"} {
		if a.Namespace+suffix == b.Namespace || b.Namespace+suffix == a.Namespace {
			return true
		}
	}
	return false
}

// CaptureEndpoint returns a status-safe origin without URL credentials, paths or queries.
func CaptureEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" ||
		u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

type captureKeyResolver func(context.Context, string, []secrets.Ref) (map[string]string, error)

func resolveCaptureKey(ctx context.Context, ref string, resolve captureKeyResolver) (string, error) {
	var key string
	if env, ok := strings.CutPrefix(ref, "env://"); ok {
		key = os.Getenv(env)
	} else {
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		values, err := resolve(ctx, "hevd capture", []secrets.Ref{{Name: "HEVLAYER_API_KEY", URI: ref, TTL: layerenv.KeyTTL}})
		if err != nil {
			if secrets.IsRateLimit(err) {
				return "", errors.New("API key provider rate limited")
			}
			return "", errors.New("could not resolve API key reference")
		}
		key = values["HEVLAYER_API_KEY"]
	}
	if key = strings.TrimSpace(key); key == "" || key == layerenv.PlaceholderKey {
		return "", errors.New("API key reference returned no usable key")
	}
	return key, nil
}

func keySource(ref string) string {
	switch scheme, _, _ := strings.Cut(ref, "://"); scheme {
	case "env":
		return "environment"
	case "op":
		return "1Password"
	case "azkv":
		return "Azure Key Vault"
	case "secret":
		return "secret manager"
	default:
		return "configured reference"
	}
}

func (t captureTarget) open(ctx context.Context, cfg *Config, resolve captureKeyResolver, current Target) (Target, error) {
	if t.err != nil {
		return Target{}, t.err
	}
	if t.ArchiveDir != "" && current.Files != nil {
		return current, nil
	}
	if t.ArchiveDir != "" {
		dir := expandPath(t.ArchiveDir)
		store, err := filearchive.Open(dir, t.Namespace)
		if err != nil {
			return Target{}, errors.New("could not open file archive")
		}
		return Target{Mode: storagemode.Filesystem, Files: store, Images: imageCaptureAt(dir, t.Namespace, !cfg.CaptureImages)}, nil
	}

	key, err := resolveCaptureKey(ctx, t.KeyRef, resolve)
	if err != nil {
		return Target{}, err
	}
	if current.Layer != nil && key == current.Layer.APIKey {
		return current, nil
	}
	client := layer.New(t.Endpoint, key, t.Namespace, layerenv.Get("HEVLAYER_EMBED_MODEL"))
	images, _ := ImageCapture(cfg, t.Namespace)
	return Target{Mode: storagemode.Layer, Layer: client, Images: images, KeySource: keySource(t.KeyRef)}, nil
}

func (t captureTarget) matches(dir string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	for _, root := range t.Dirs {
		if workspace.Contains(expandPath(root), dir) {
			return true
		}
	}
	return len(t.Workspaces) > 0 && slices.Contains(t.Workspaces, t.owners.DirectoryOwner(dir, true))
}

// filterTurns keeps whole sessions, including turns with no directory after a
// matching turn. A session with a known directory outside the filter is refused.
func (t captureTarget) filterTurns(turns []trace.Turn) []trace.Turn {
	matched, refused := map[string]bool{}, map[string]bool{}
	dirs := map[string]bool{}
	for _, turn := range turns {
		if turn.Workdir == "" {
			continue
		}
		ok, known := dirs[turn.Workdir]
		if !known {
			ok = t.matches(turn.Workdir)
			dirs[turn.Workdir] = ok
		}
		if ok {
			matched[turn.SessionID] = true
		} else {
			refused[turn.SessionID] = true
		}
	}
	return slices.DeleteFunc(turns, func(turn trace.Turn) bool {
		return turn.SessionID == "" || !matched[turn.SessionID] || refused[turn.SessionID]
	})
}

func (t captureTarget) stateScope() string {
	if t.Mode != "workspace" {
		return ""
	}
	roots := make(map[string]string, len(t.owners.Workspaces))
	if len(t.Workspaces) > 0 {
		for name, w := range t.owners.Workspaces {
			roots[name] = expandPath(w.Dir)
		}
	}
	raw, _ := json.Marshal([]any{"workspace-filter-v1", expandPathList(t.Dirs), t.Workspaces, roots})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// Capture diagnostics may come from providers, transport URLs, archive
// responses or local files. Only emit facts we classify ourselves.
func captureError(operation string, err error) string {
	var response *layer.HTTPError
	var network net.Error
	switch {
	case errors.As(err, &response):
		return fmt.Sprintf("%s: HTTP %d", operation, response.Status)
	case errors.Is(err, context.DeadlineExceeded):
		return operation + ": timed out"
	case errors.Is(err, context.Canceled):
		return operation + ": canceled"
	case errors.As(err, &network):
		return operation + ": network error"
	default:
		return operation + ": failed"
	}
}
