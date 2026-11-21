// Package evidence is kit's sixth data surface on a machine with no Pro
// account (RFC 0022): a folder of self-contained proof packages.
//
//	<root>/.lock
//	<root>/ev_<ulid>/manifest.json
//	<root>/ev_<ulid>/001-checkout-390px.png
//	<root>/ev_<ulid>/002-repro.mp4
//	<root>/ev_<ulid>/003-console.log
//
// The root is $HEV_EVIDENCE_DIR, else [evidence] dir in ~/.hev/config.toml,
// else ~/.hev/evidence. Every file's modality and media type are decided by
// its bytes, never by a file name or a caller's MIME. Text and every manifest
// string pass through kit's secret scrubber before they reach the disk.
// Directories are 0700, files 0600, and every write is atomic.
//
// The package depends on no external binary: video containers are parsed in
// Go. Search uses ripgrep to narrow the files to read when it is on PATH and
// a Go walk otherwise; the decision is made in Go either way, so both return
// the same hits.
package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Schema names the package manifest version, on disk and over /v1/evidence.
const Schema = "hev.evidence.package/v1"

// DirEnv overrides the evidence root.
const DirEnv = "HEV_EVIDENCE_DIR"

// ManifestName is the package manifest's file name.
const ManifestName = "manifest.json"

// Default caps and bounds (RFC 0022, EvidenceBlobLimits).
const (
	MaxImageBytes       int64 = 10 << 20
	MaxVideoBytes       int64 = 100 << 20
	MaxVideoDurationMs  int64 = 60_000
	MaxTextBytes        int64 = 5 << 20
	MaxInlineTextBytes        = 64 << 10
	MaxItemsPerPackage        = 64
	MaxClaimsPerPackage       = 32
	MaxTags                   = 20
	MaxTagLen                 = 64
	MaxMetadataKeys           = 16
	MaxMetadataValue          = 256
	MaxNameLen                = 200
)

// Modalities, states, verdicts and sources the wire schema allows.
var (
	Modalities = []string{"text", "image", "video"}
	Verdicts   = []string{"pass", "fail", "inconclusive", "info"}
	Sources    = []string{"validator", "browser-use", "computer-use", "spike", "manual", "api"}
	Roles      = []string{"agent", "human"}

	TextTypes  = []string{"text/plain", "text/markdown", "application/json", "text/x-diff", "text/csv", "text/html"}
	ImageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}
	VideoTypes = []string{"video/mp4", "video/webm", "video/quicktime"}
)

// Errors callers can test with errors.Is. Each wraps a human message.
var (
	ErrNotFound    = errors.New("no such evidence package")
	ErrConflict    = errors.New("evidence package is sealed")
	ErrTooLarge    = errors.New("evidence item is too large")
	ErrUnsupported = errors.New("unsupported evidence item")
	ErrInvalid     = errors.New("invalid evidence")
	ErrIntegrity   = errors.New("evidence integrity check failed")
)

// Claim is one assertion about a package.
type Claim struct {
	ID        string   `json:"id"`
	Criterion string   `json:"criterion,omitempty"`
	Text      string   `json:"text"`
	Verdict   string   `json:"verdict,omitempty"`
	ItemIDs   []string `json:"item_ids,omitempty"`
}

// Item is one typed artifact in a package. Media fields come from the bytes.
type Item struct {
	ID           string            `json:"id"`
	Seq          int               `json:"seq"`
	Modality     string            `json:"modality"`
	Name         string            `json:"name"`
	Caption      string            `json:"caption,omitempty"`
	MIME         string            `json:"mime"`
	SHA256       string            `json:"sha256"`
	Size         int64             `json:"size"`
	URL          string            `json:"url,omitempty"`
	App          string            `json:"app,omitempty"`
	Step         string            `json:"step,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Width        int               `json:"width,omitempty"`
	Height       int               `json:"height,omitempty"`
	DurationMs   *int64            `json:"duration_ms,omitempty"`
	PosterSHA256 string            `json:"poster_sha256,omitempty"`
	LineCount    *int              `json:"line_count,omitempty"`
	Redacted     bool              `json:"redacted,omitempty"`
	Text         *string           `json:"text,omitempty"`
	CapturedAt   time.Time         `json:"captured_at"`
}

// Counts is EvidencePackageSummary.item_counts.
type Counts struct {
	Total int `json:"total"`
	Text  int `json:"text"`
	Image int `json:"image"`
	Video int `json:"video"`
}

// Package is the manifest (EvidencePackage).
type Package struct {
	Schema    string     `json:"schema"`
	ID        string     `json:"id"`
	State     string     `json:"state"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary,omitempty"`
	Source    string     `json:"source,omitempty"`
	Order     string     `json:"order,omitempty"`
	Session   string     `json:"session,omitempty"`
	Gate      string     `json:"gate,omitempty"`
	Repo      string     `json:"repo,omitempty"`
	Branch    string     `json:"branch,omitempty"`
	Commit    string     `json:"commit,omitempty"`
	Host      string     `json:"host,omitempty"`
	Author    string     `json:"author,omitempty"`
	Role      string     `json:"role,omitempty"`
	Tags      []string   `json:"tags"`
	Claims    []Claim    `json:"claims"`
	ItemCount Counts     `json:"item_counts"`
	Items     []Item     `json:"items"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
}

// Summary is EvidencePackageSummary: a package without its items.
type Summary struct {
	Schema    string     `json:"schema"`
	ID        string     `json:"id"`
	State     string     `json:"state"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary,omitempty"`
	Source    string     `json:"source,omitempty"`
	Order     string     `json:"order,omitempty"`
	Session   string     `json:"session,omitempty"`
	Gate      string     `json:"gate,omitempty"`
	Repo      string     `json:"repo,omitempty"`
	Branch    string     `json:"branch,omitempty"`
	Commit    string     `json:"commit,omitempty"`
	Host      string     `json:"host,omitempty"`
	Author    string     `json:"author,omitempty"`
	Role      string     `json:"role,omitempty"`
	Tags      []string   `json:"tags"`
	Claims    []Claim    `json:"claims"`
	ItemCount Counts     `json:"item_counts"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
}

// Summarize drops the items.
func (p *Package) Summarize() Summary {
	return Summary{Schema: p.Schema, ID: p.ID, State: p.State, Title: p.Title, Summary: p.Summary, Source: p.Source,
		Order: p.Order, Session: p.Session, Gate: p.Gate, Repo: p.Repo, Branch: p.Branch, Commit: p.Commit, Host: p.Host,
		Author: p.Author, Role: p.Role, Tags: p.Tags, Claims: p.Claims, ItemCount: p.ItemCount,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, SealedAt: p.SealedAt}
}

func (p *Package) recount() {
	c := Counts{}
	for _, it := range p.Items {
		c.Total++
		switch it.Modality {
		case "text":
			c.Text++
		case "image":
			c.Image++
		case "video":
			c.Video++
		}
	}
	p.ItemCount = c
}

// ConfigPath is kit's config file: $HEV_CONFIG or ~/.hev/config.toml.
func ConfigPath() string {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".hev", "config.toml")
	}
	return filepath.Join(home, ".hev", "config.toml")
}

// DefaultRoot is ~/.hev/evidence.
func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".hev", "evidence")
	}
	return filepath.Join(home, ".hev", "evidence")
}

// Root resolves the evidence directory: $HEV_EVIDENCE_DIR, then [evidence]
// dir in the config file, then ~/.hev/evidence. A leading ~/ is the home
// directory. A config file that is missing or unreadable means the default.
func Root() string {
	if v := strings.TrimSpace(os.Getenv(DirEnv)); v != "" {
		return expand(v)
	}
	if dir := configuredDir(ConfigPath()); dir != "" {
		return expand(dir)
	}
	return DefaultRoot()
}

func configuredDir(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	var cfg struct {
		Evidence struct {
			Dir string `toml:"dir"`
		} `toml:"evidence"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Evidence.Dir)
}

func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}
