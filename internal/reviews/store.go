// Package reviews stores exact-head judgments and their anchored findings.
package reviews

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/oklog/ulid/v2"
)

const DirEnv = "HEV_REVIEWS_DIR"
const MaxBytes = 2 << 20
const maxFindings = 500

// Local filesystem paths in prose are private. Relative anchors and URL
// paths stay intact so reviews can still be searched by package or PR.
var localPathPattern = regexp.MustCompile(`(^|[[:space:]"'(=])((?:/(?:home|Users|root|tmp|var|private|mnt|opt|workspace|etc|run|srv|dev|proc|sys|media|Volumes)/|[A-Za-z]:[\\/]|\\\\)[^[:space:]"')<>]+)`)

var shaPattern = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

type Review struct {
	ID        string    `json:"id"`
	Repo      string    `json:"repo"`
	BaseSHA   string    `json:"base_sha,omitempty"`
	HeadSHA   string    `json:"head_sha"`
	Order     string    `json:"order,omitempty"`
	PR        string    `json:"pr,omitempty"`
	Reviewer  string    `json:"reviewer"`
	Harness   string    `json:"harness"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Verdict   string    `json:"verdict"`
	Summary   string    `json:"summary"`
	Findings  []Finding `json:"findings"`
}

type Finding struct {
	ID             string     `json:"id"`
	File           string     `json:"file"`
	StartLine      int        `json:"start_line,omitempty"`
	EndLine        int        `json:"end_line,omitempty"`
	Severity       string     `json:"severity"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	ResolutionNote string     `json:"resolution_note,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

func Root() string {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".hev")
	dir := strings.TrimSpace(os.Getenv(DirEnv))
	if dir == "" {
		var cfg struct {
			Reviews struct {
				Dir string `toml:"dir"`
			} `toml:"reviews"`
		}
		config := os.Getenv("HEV_CONFIG")
		if config == "" {
			config = filepath.Join(root, "config.toml")
		}
		if info, err := os.Stat(config); err == nil && info.Mode().IsRegular() {
			if _, err := toml.DecodeFile(config, &cfg); err == nil {
				dir = strings.TrimSpace(cfg.Reviews.Dir)
			}
		}
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	if dir == "" {
		return filepath.Join(root, "reviews")
	}
	return dir
}

type Store struct {
	root     string
	scrubber *redact.Scrubber
}

func Open(root string) (*Store, error) {
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, errors.New("could not initialize review redaction")
	}
	scrubber, err := redact.New(salt[:])
	if err != nil {
		return nil, errors.New("could not initialize review redaction")
	}
	return &Store{root: root, scrubber: scrubber}, nil
}

func newID(prefix string) string {
	return prefix + strings.ToLower(ulid.Make().String())
}

func validID(prefix, id string) bool {
	suffix, ok := strings.CutPrefix(id, prefix)
	if !ok || suffix != strings.ToLower(suffix) {
		return false
	}
	parsed, err := ulid.ParseStrict(suffix)
	return err == nil && strings.ToLower(parsed.String()) == suffix
}

func ValidSHA(v string) bool { return shaPattern.MatchString(v) }

func validVerdict(v string) bool {
	return v == "pass" || v == "rework" || v == "comment"
}

func validSeverity(v string) bool {
	return v == "blocker" || v == "major" || v == "minor" || v == "nit"
}

func resolvedStatus(v string) bool { return v == "fixed" || v == "waived" }

func validFile(v string) bool {
	return v != "." && fs.ValidPath(v) && !strings.ContainsAny(v, `\:`) &&
		strings.IndexFunc(v, unicode.IsControl) < 0 && path.Clean(v) == v
}

func (s *Store) scrub(fields ...*string) {
	for _, field := range fields {
		*field, _ = s.scrubber.Text(*field)
		*field = localPathPattern.ReplaceAllString(*field, "${1}[REDACTED:local-path]")
	}
}

func (s *Store) prepareFinding(f *Finding, now time.Time) error {
	if !validFile(f.File) {
		return errors.New("finding file must be a relative repository path")
	}
	if !validSeverity(f.Severity) {
		return errors.New("severity must be blocker, major, minor, or nit")
	}
	if f.StartLine != 0 || f.EndLine != 0 {
		if f.StartLine < 1 || f.EndLine < f.StartLine {
			return errors.New("finding lines must be a positive inclusive range")
		}
	}
	if strings.TrimSpace(f.Body) == "" {
		return errors.New("finding body is required")
	}
	if f.Status == "" {
		f.Status = "open"
	}
	switch {
	case resolvedStatus(f.Status):
		f.ResolvedAt = &now
	case f.Status == "open":
		if f.ResolutionNote != "" || f.ResolvedAt != nil {
			return errors.New("open finding cannot have a resolution")
		}
	default:
		return errors.New("status must be open, fixed, or waived")
	}
	if f.ID != "" {
		return errors.New("finding IDs are assigned by the review store")
	}
	f.ID = newID("fnd_")
	original := f.File
	s.scrub(&f.File, &f.Body, &f.ResolutionNote)
	if f.File != original {
		return errors.New("finding file contains sensitive text")
	}
	return nil
}

func (s *Store) Create(r Review) (Review, error) {
	if r.ID != "" {
		return Review{}, errors.New("review IDs are assigned by the review store")
	}
	if strings.TrimSpace(r.Repo) == "" || !ValidSHA(r.HeadSHA) || (r.BaseSHA != "" && !ValidSHA(r.BaseSHA)) {
		return Review{}, errors.New("repo and a full head SHA are required, and base SHA must be full when supplied")
	}
	r.HeadSHA, r.BaseSHA = strings.ToLower(r.HeadSHA), strings.ToLower(r.BaseSHA)
	if r.Verdict == "" {
		r.Verdict = "comment"
	}
	if !validVerdict(r.Verdict) {
		return Review{}, errors.New("verdict must be pass, rework, or comment")
	}
	if len(r.Findings) > maxFindings {
		return Review{}, errors.New("too many findings")
	}
	now := time.Now().UTC()
	r.ID = newID("rev_")
	r.CreatedAt, r.UpdatedAt = now, now
	r.Findings = append([]Finding{}, r.Findings...)
	for i := range r.Findings {
		if err := s.prepareFinding(&r.Findings[i], now); err != nil {
			return Review{}, err
		}
	}
	s.scrub(&r.Repo, &r.Order, &r.PR, &r.Reviewer, &r.Harness, &r.Model, &r.Summary)
	data, err := encode(r)
	if err != nil {
		return Review{}, err
	}
	if err := platformops.MkdirAll(s.root, 0700); err != nil {
		return Review{}, errors.New("could not create review store")
	}
	dir := filepath.Join(s.root, r.ID)
	if err := os.Mkdir(dir, 0700); err != nil {
		return Review{}, errors.New("could not create review directory")
	}
	if err := platformops.SecureDir(dir); err != nil {
		_ = os.Remove(dir)
		return Review{}, errors.New("could not secure review directory")
	}
	if err := write(dir, data); err != nil {
		_ = os.Remove(dir)
		return Review{}, err
	}
	return r, nil
}

func encode(r Review) ([]byte, error) {
	// Bound inputs before encoding, including strings passed through CLI flags.
	size := len(r.Repo) + len(r.BaseSHA) + len(r.HeadSHA) + len(r.Order) + len(r.PR) +
		len(r.Reviewer) + len(r.Harness) + len(r.Model) + len(r.Summary)
	for _, f := range r.Findings {
		size += len(f.File) + len(f.Body) + len(f.ResolutionNote)
	}
	if size > MaxBytes {
		return nil, errors.New("review exceeds size limit")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil || len(b) >= MaxBytes {
		return nil, errors.New("could not encode review within size limit")
	}
	return append(b, '\n'), nil
}

func write(dir string, data []byte) error {
	if err := platformops.WriteFileAtomic(filepath.Join(dir, "review.json"), data, 0600); err != nil {
		return errors.New("could not save review")
	}
	return nil
}

func (s *Store) Get(id string) (Review, error) {
	if !validID("rev_", id) {
		return Review{}, errors.New("invalid review ID")
	}
	// Use a confined root even if a store entry was manually replaced with a symlink.
	root, err := os.OpenRoot(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return Review{}, fs.ErrNotExist
	}
	if err != nil {
		return Review{}, errors.New("could not open review store")
	}
	defer root.Close()
	info, err := root.Lstat(id)
	if errors.Is(err, fs.ErrNotExist) {
		return Review{}, fs.ErrNotExist
	}
	if err != nil || !info.IsDir() {
		return Review{}, errors.New("could not read review directory")
	}
	name := filepath.Join(id, "review.json")
	info, err = root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return Review{}, fs.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() {
		return Review{}, errors.New("could not read review")
	}
	f, err := root.Open(name)
	if err != nil {
		return Review{}, errors.New("could not read review")
	}
	defer f.Close()
	var r Review
	if err := Decode(f, &r); err != nil || r.ID != id || !ValidSHA(r.HeadSHA) ||
		(r.BaseSHA != "" && !ValidSHA(r.BaseSHA)) || !validVerdict(r.Verdict) || len(r.Findings) > maxFindings {
		return Review{}, errors.New("invalid stored review")
	}
	for _, finding := range r.Findings {
		if !validID("fnd_", finding.ID) || !validFile(finding.File) || !validSeverity(finding.Severity) ||
			(finding.Status != "open" && !resolvedStatus(finding.Status)) {
			return Review{}, errors.New("invalid stored finding")
		}
	}
	return r, nil
}

// Decode reads one bounded JSON value and keeps malformed input out of errors.
func Decode(in io.Reader, dst any) error {
	b, err := io.ReadAll(io.LimitReader(in, MaxBytes+1))
	if err != nil || len(b) > MaxBytes || !utf8.Valid(b) {
		return errors.New("invalid or oversized review JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return errors.New("invalid review JSON")
	}
	return nil
}

func (s *Store) Add(id string, f Finding) (Finding, error) {
	err := s.update(id, func(r *Review, now time.Time) error {
		if len(r.Findings) >= maxFindings {
			return errors.New("too many findings")
		}
		if err := s.prepareFinding(&f, now); err != nil {
			return err
		}
		r.Findings = append(r.Findings, f)
		return nil
	})
	return f, err
}

func (s *Store) Resolve(id, findingID, status, note string) error {
	if !validID("fnd_", findingID) {
		return errors.New("invalid finding ID")
	}
	if !resolvedStatus(status) {
		return errors.New("resolution status must be fixed or waived")
	}
	return s.update(id, func(r *Review, now time.Time) error {
		i := slices.IndexFunc(r.Findings, func(f Finding) bool { return f.ID == findingID })
		if i < 0 {
			return errors.New("finding not found")
		}
		s.scrub(&note)
		f := &r.Findings[i]
		f.Status, f.ResolvedAt, f.ResolutionNote = status, &now, note
		return nil
	})
}

func (s *Store) update(id string, change func(*Review, time.Time) error) error {
	if !validID("rev_", id) {
		return errors.New("invalid review ID")
	}
	dir := filepath.Join(s.root, id)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return fs.ErrNotExist
	}
	if err != nil || !info.IsDir() {
		return errors.New("could not open review directory")
	}
	unlock, err := platformops.LockPath(filepath.Join(dir, ".lock"), true)
	if platformops.IsLockBusy(err) {
		return errors.New("review is being updated, retry the operation")
	}
	if err != nil {
		return errors.New("could not lock review")
	}
	defer unlock()
	r, err := s.Get(id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := change(&r, now); err != nil {
		return err
	}
	r.UpdatedAt = now
	data, err := encode(r)
	if err != nil {
		return err
	}
	return write(dir, data)
}
