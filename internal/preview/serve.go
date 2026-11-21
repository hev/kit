package preview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderContentTypeOptions = "nosniff"
	HeaderRobotsTag          = "noindex, nofollow"
)

var errPathEscape = errors.New("path escapes preview root")

// Listen validates bind and port (refusing non-loopback, non-Tailscale IPs) and
// opens a TCP listener restricted to that IP family.
func Listen(cfg Config) (net.Listener, netip.Addr, error) {
	addr, err := ValidateBind(cfg.Bind)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	if err := ValidatePort(cfg.Port); err != nil {
		return nil, netip.Addr{}, err
	}
	network := "tcp4"
	if addr.Is6() {
		network = "tcp6"
	}
	ln, err := net.Listen(network, net.JoinHostPort(addr.String(), strconv.Itoa(cfg.Port)))
	if err != nil {
		return nil, netip.Addr{}, err
	}
	return ln, addr, nil
}

// Handler returns an HTTP handler serving cfg's current build (or static root).
// It validates bind and port upfront so callers cannot bypass the bind guard.
func Handler(cfg Config, home string) (http.Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	resolveRoot := func() (string, error) {
		if cfg.Mode() == "static" {
			root := cfg.ExpandedRoot(home)
			if st, err := os.Stat(root); err != nil || !st.IsDir() {
				return "", os.ErrNotExist
			}
			return root, nil
		}
		if home == "" {
			return "", os.ErrNotExist
		}
		dir, _, err := resolveCurrent(PreviewDir(home, cfg.Name))
		if err != nil {
			return "", err
		}
		return dir, nil
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", HeaderContentTypeOptions)
		w.Header().Set("X-Robots-Tag", HeaderRobotsTag)

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if hasTraversal(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		activeRoot, err := resolveRoot()
		if err != nil {
			http.Error(w, "preview not ready", http.StatusServiceUnavailable)
			return
		}
		realRoot, err := filepath.EvalSymlinks(activeRoot)
		if err != nil {
			http.Error(w, "preview not ready", http.StatusServiceUnavailable)
			return
		}
		rootFS, err := os.OpenRoot(realRoot)
		if err != nil {
			http.Error(w, "preview not ready", http.StatusServiceUnavailable)
			return
		}
		defer rootFS.Close()

		reqPath := r.URL.Path
		if reqPath == "" {
			reqPath = "/"
		}
		cleaned := path.Clean("/" + reqPath)
		rel := strings.TrimPrefix(cleaned, "/")

		// Check containment and symlink escapes before opening.
		if rel != "" {
			if err := checkContained(realRoot, rel); err != nil {
				if errors.Is(err, errPathEscape) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if errors.Is(err, os.ErrNotExist) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		serveTarget(w, r, realRoot, rootFS, rel, strings.HasSuffix(reqPath, "/"))
	}), nil
}

func serveTarget(w http.ResponseWriter, r *http.Request, realRoot string, rootFS *os.Root, rel string, trailingSlash bool) {
	// Root directory request.
	if rel == "" {
		serveIndex(w, r, realRoot, rootFS, "")
		return
	}

	f, err := rootFS.Open(rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		_ = f.Close()
		if !trailingSlash {
			// Only redirect when an index.html is actually servable inside the dir.
			idxRel := path.Join(rel, "index.html")
			if err := checkContained(realRoot, idxRel); err != nil {
				if errors.Is(err, errPathEscape) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				http.NotFound(w, r)
				return
			}
			idx, err := rootFS.Open(idxRel)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			idxInfo, statErr := idx.Stat()
			_ = idx.Close()
			if statErr != nil || idxInfo.IsDir() {
				http.NotFound(w, r)
				return
			}
			loc := "/" + rel + "/"
			if r.URL.RawQuery != "" {
				loc += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, loc, http.StatusMovedPermanently)
			return
		}
		serveIndex(w, r, realRoot, rootFS, rel)
		return
	}
	defer f.Close()
	if !info.Mode().IsRegular() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func serveIndex(w http.ResponseWriter, r *http.Request, realRoot string, rootFS *os.Root, dirRel string) {
	idxRel := "index.html"
	if dirRel != "" {
		idxRel = path.Join(dirRel, "index.html")
	}
	if err := checkContained(realRoot, idxRel); err != nil {
		if errors.Is(err, errPathEscape) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
		return
	}
	f, err := rootFS.Open(idxRel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, "index.html", info.ModTime(), f)
}

// hasTraversal rejects raw or encoded '..', backslashes, or NUL bytes in the
// request path before any normalization occurs.
func hasTraversal(r *http.Request) bool {
	candidates := []string{r.URL.Path}
	if r.URL.RawPath != "" {
		candidates = append(candidates, r.URL.RawPath)
	}
	if r.RequestURI != "" {
		reqPath := r.RequestURI
		if q := strings.IndexByte(reqPath, '?'); q >= 0 {
			reqPath = reqPath[:q]
		}
		candidates = append(candidates, reqPath)
	}
	for _, raw := range candidates {
		if pathIsSuspicious(raw) {
			return true
		}
		if decoded, err := url.PathUnescape(raw); err == nil && decoded != raw {
			if pathIsSuspicious(decoded) {
				return true
			}
		}
	}
	return false
}

func pathIsSuspicious(p string) bool {
	if strings.ContainsAny(p, "\x00\\") {
		return true
	}
	lower := strings.ToLower(p)
	if strings.Contains(lower, "%00") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%2f..") || strings.Contains(lower, "..%2f") {
		return true
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
		if decoded, err := url.PathUnescape(seg); err == nil && (decoded == ".." || strings.ContainsAny(decoded, "\x00\\/")) {
			return true
		}
	}
	return false
}

// checkContained walks every path component under realRoot and refuses any
// symlink that points outside realRoot (even if its target does not exist).
func checkContained(realRoot, rel string) error {
	cur := realRoot
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." || strings.ContainsAny(seg, "\x00\\") {
			return errPathEscape
		}
		next := filepath.Join(cur, seg)
		info, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				return os.ErrNotExist
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(next)
			if err != nil {
				return errPathEscape
			}
			var resolved string
			if filepath.IsAbs(linkTarget) {
				resolved = filepath.Clean(linkTarget)
			} else {
				resolved = filepath.Clean(filepath.Join(cur, linkTarget))
			}
			if !isWithin(realRoot, resolved) {
				return errPathEscape
			}
			// Also evaluate chained symlinks if target exists.
			if evaled, err := filepath.EvalSymlinks(next); err == nil {
				if !isWithin(realRoot, evaled) {
					return errPathEscape
				}
				cur = evaled
				continue
			} else if os.IsNotExist(err) {
				return os.ErrNotExist
			} else {
				return errPathEscape
			}
		}
		cur = next
	}
	return nil
}

// Serve binds strictly on <bind>:<port> and serves cfg until ctx is canceled.
// If onReady is non-nil, it is called with the bound host:port once listening.
func Serve(ctx context.Context, cfg Config, home string, onReady func(addr string)) error {
	handler, err := Handler(cfg, home)
	if err != nil {
		return err
	}
	ln, _, err := Listen(cfg)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.Port)), err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	if onReady != nil {
		onReady(ln.Addr().String())
	}

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
