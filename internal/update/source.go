package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Repository is the only place builds are fetched from.
const Repository = "hev/kit"

// Source is where published builds are read. Official is the only Source
// the binary uses; tests substitute a local fake. Nothing a caller sends
// (a dispatch request, a flag) can choose another.
type Source struct {
	// API is the GitHub REST base for the repository.
	API string
	// Download is the base of release asset downloads: <Download>/<tag>/<name>.
	Download string
	Client   *http.Client
}

// Official is the official repository's published builds.
var Official = Source{
	API:      "https://api.github.com/repos/" + Repository,
	Download: "https://github.com/" + Repository + "/releases/download",
}

const (
	manifestName  = "manifest.json"
	checksumsName = "checksums.txt"
	maxSmallFile  = 1 << 20
	maxArchive    = 512 << 20
)

// Manifest is the machine-readable description an edge build (and, from the
// workflow that adds it, a release) publishes beside its archives.
type Manifest struct {
	Schema   int     `json:"schema"`
	Version  string  `json:"version"`
	Channel  string  `json:"channel"`
	Train    string  `json:"train,omitempty"`
	Commit   string  `json:"commit,omitempty"`
	Sequence int     `json:"sequence,omitempty"`
	Built    string  `json:"built,omitempty"`
	Assets   []Asset `json:"assets"`
}

type Asset struct {
	Name   string `json:"name"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

// Build is one published build, read and checked from its source.
type Build struct {
	Manifest
	Tag string
	// FromChecksums is set for a release published before manifests: its
	// digests come from checksums.txt alone.
	FromChecksums bool
}

// Asset returns the build's archive for a platform.
func (b Build) Asset(goos, goarch string) (Asset, error) {
	want := AssetName(b.Version, goos, goarch)
	for _, a := range b.Assets {
		if a.Name == want {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%s publishes no build for %s/%s", b.Version, goos, goarch)
}

var (
	hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
	edgeTag   = regexp.MustCompile(`^edge-(\d+\.\d+\.\d+-edge\.[0-9a-f]{7,40})$`)
	assetRe   = regexp.MustCompile(`^kit_(.+)_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)$`)
)

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (s Source) get(ctx context.Context, url string, limit int64, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "hev-update")
	if strings.HasPrefix(url, s.API) {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("GET %s: response larger than %d bytes", url, limit)
	}
	return nil
}

var errNotFound = errors.New("not found")

func (s Source) small(ctx context.Context, url string) ([]byte, error) {
	var b bytes.Buffer
	err := s.get(ctx, url, maxSmallFile, &b)
	return b.Bytes(), err
}

// Find reads a named build: its manifest, or for a release published before
// manifests its checksums.txt. Only the version is caller-chosen; the URL is
// built from it and the official repository.
func (s Source) Find(ctx context.Context, version string) (Build, error) {
	v, err := NormalizeVersion(version)
	if err != nil {
		return Build{}, err
	}
	tag := Tag(v)
	b := Build{Tag: tag}
	raw, err := s.small(ctx, s.Download+"/"+tag+"/"+manifestName)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &b.Manifest); err != nil {
			return Build{}, fmt.Errorf("%s: manifest: %w", tag, err)
		}
	case errors.Is(err, errNotFound) && VersionChannel(v) == ChannelRelease:
		b.Manifest = Manifest{Schema: 1, Version: v, Channel: ChannelRelease}
		b.FromChecksums = true
	case errors.Is(err, errNotFound):
		return Build{}, fmt.Errorf("no published build %s", v)
	default:
		return Build{}, fmt.Errorf("%s: manifest: %w", tag, err)
	}
	sums, err := s.small(ctx, s.Download+"/"+tag+"/"+checksumsName)
	if err != nil && !(errors.Is(err, errNotFound) && !b.FromChecksums) {
		if errors.Is(err, errNotFound) {
			return Build{}, fmt.Errorf("no published build %s", v)
		}
		return Build{}, fmt.Errorf("%s: checksums: %w", tag, err)
	}
	if err := b.check(v, sums); err != nil {
		return Build{}, fmt.Errorf("%s: %w", tag, err)
	}
	return b, nil
}

// check holds a manifest to the version it was fetched for and to
// checksums.txt, when there is one. The two come from the same release, so
// agreeing does not prove the build is good; it proves the release is
// consistent and catches a half-uploaded or hand-edited one.
func (b *Build) check(v string, sums []byte) error {
	m := &b.Manifest
	if m.Version != v {
		return fmt.Errorf("manifest names version %q, not %s", m.Version, v)
	}
	if m.Channel != VersionChannel(v) {
		return fmt.Errorf("manifest names channel %q for %s", m.Channel, v)
	}
	listed := map[string]string{}
	if sums != nil {
		sc := bufio.NewScanner(bytes.NewReader(sums))
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) != 2 {
				continue
			}
			listed[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	if b.FromChecksums {
		for name, sum := range listed {
			mm := assetRe.FindStringSubmatch(name)
			if mm == nil || mm[1] != v {
				continue
			}
			m.Assets = append(m.Assets, Asset{Name: name, OS: mm[2], Arch: mm[3], SHA256: sum})
		}
		sort.Slice(m.Assets, func(i, j int) bool { return m.Assets[i].Name < m.Assets[j].Name })
	}
	if len(m.Assets) == 0 {
		return fmt.Errorf("no assets listed")
	}
	for _, a := range m.Assets {
		if a.Name != AssetName(v, a.OS, a.Arch) {
			return fmt.Errorf("asset %q does not match its version and platform", a.Name)
		}
		if !hexDigest.MatchString(a.SHA256) {
			return fmt.Errorf("asset %s has no sha256 digest", a.Name)
		}
		if sum, ok := listed[a.Name]; ok && sum != a.SHA256 {
			return fmt.Errorf("asset %s: manifest and checksums.txt disagree", a.Name)
		}
	}
	return nil
}

type apiRelease struct {
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// Latest reads the newest build on a channel: the repository's latest
// release, or the newest published edge build (highest X.Y.Z, then most
// recently published).
func (s Source) Latest(ctx context.Context, channel string) (Build, error) {
	switch channel {
	case ChannelRelease:
		raw, err := s.small(ctx, s.API+"/releases/latest")
		if err != nil {
			return Build{}, fmt.Errorf("latest release: %w", err)
		}
		var r apiRelease
		if err := json.Unmarshal(raw, &r); err != nil {
			return Build{}, fmt.Errorf("latest release: %w", err)
		}
		v := strings.TrimPrefix(r.TagName, "v")
		if r.Draft || r.Prerelease || !releaseVersion.MatchString(v) || r.TagName != "v"+v {
			return Build{}, fmt.Errorf("latest release %q is not a vX.Y.Z release", r.TagName)
		}
		return s.Find(ctx, v)
	case ChannelEdge:
		raw, err := s.small(ctx, s.API+"/releases?per_page=100")
		if err != nil {
			return Build{}, fmt.Errorf("edge builds: %w", err)
		}
		var rs []apiRelease
		if err := json.Unmarshal(raw, &rs); err != nil {
			return Build{}, fmt.Errorf("edge builds: %w", err)
		}
		var best *apiRelease
		var bestV string
		for i := range rs {
			r := &rs[i]
			m := edgeTag.FindStringSubmatch(r.TagName)
			if m == nil || r.Draft || !r.Prerelease || !hasAsset(r, manifestName) {
				continue
			}
			if best != nil {
				c, ok := Compare(m[1], bestV)
				if (ok && c < 0) || (!ok && !r.PublishedAt.After(best.PublishedAt)) {
					continue
				}
			}
			best, bestV = r, m[1]
		}
		if best == nil {
			return Build{}, fmt.Errorf("no edge build is published")
		}
		return s.Find(ctx, bestV)
	}
	return Build{}, fmt.Errorf("unknown channel %q (want release or edge)", channel)
}

func hasAsset(r *apiRelease, name string) bool {
	for _, a := range r.Assets {
		if a.Name == name {
			return true
		}
	}
	return false
}

// Fetch downloads a build's archive for a platform to path and verifies its
// digest. A mismatch removes the file and is an error; nothing else has been
// touched.
func (s Source) Fetch(ctx context.Context, b Build, goos, goarch, path string) (Asset, error) {
	a, err := b.Asset(goos, goarch)
	if err != nil {
		return a, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return a, err
	}
	h := sha256.New()
	err = s.get(ctx, s.Download+"/"+b.Tag+"/"+a.Name, maxArchive, io.MultiWriter(f, h))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
			err = fmt.Errorf("digest mismatch for %s: manifest says %s, download is %s; nothing was replaced", a.Name, a.SHA256, got)
		}
	}
	if err != nil {
		os.Remove(path)
		return a, err
	}
	return a, nil
}
