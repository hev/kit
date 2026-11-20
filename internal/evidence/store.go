package evidence

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/redact"
	"github.com/oklog/ulid/v2"
)

var (
	idPattern       = regexp.MustCompile(`^ev_[0-9a-z]{26}$`)
	itemIDPattern   = regexp.MustCompile(`^evi_[0-9a-z]{26}$`)
	storedName      = regexp.MustCompile(`^[0-9]{3}-[A-Za-z0-9][A-Za-z0-9._-]*$`)
	unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// ValidID reports whether id is a package id (ev_ + lowercase ULID).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Store is the evidence folder at Root.
type Store struct {
	Root   string
	Limits Limits
	// Scrub redacts secrets from text. Open sets kit's scrubber.
	Scrub func(string) string
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Observe, when set, is told about each created, updated and sealed
	// package after it is on disk.
	Observe func(event string, p *Package, addedItems, addedClaims []string)
}

// Open is the store at root with kit's secret scrubber and default limits.
// Nothing is written until the first package.
func Open(root string) (*Store, error) {
	var salt [32]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, err
	}
	sc, err := redact.New(salt[:])
	if err != nil {
		return nil, fmt.Errorf("evidence needs kit's secret scrubber: %w", err)
	}
	return &Store{Root: root, Limits: DefaultLimits, Scrub: func(s string) string { t, _ := sc.Text(s); return t }}, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC().Truncate(time.Second)
	}
	return time.Now().UTC().Truncate(time.Second)
}

func (s *Store) scrub(v string) string {
	if s.Scrub == nil || v == "" {
		return v
	}
	return s.Scrub(v)
}

// Meta is a package's descriptive fields (EvidencePackageCreate less items
// and claims).
type Meta struct {
	Title, Summary, Source, Order, Session, Gate, Repo, Branch, Commit, Host, Author, Role string
	Tags                                                                                   []string
}

// ItemInput is one item to add. Exactly one of Path or Data is the content.
type ItemInput struct {
	// Path is a file to read; Name defaults to its base name.
	Path string
	// Data is the content when Path is empty.
	Data []byte
	// Name is the caller's file name: a single safe segment.
	Name              string
	Caption, URL, App string
	Step              string
	Metadata          map[string]string
	CapturedAt        time.Time
}

// ClaimInput is one claim to add.
type ClaimInput struct {
	Criterion, Text, Verdict string
	ItemIDs                  []string
}

// Patch edits an open package's descriptive fields; nil leaves a field.
type Patch struct {
	Title, Summary *string
	Tags           *[]string
}

func newID(prefix string) string { return prefix + strings.ToLower(ulid.Make().String()) }

func checkLen(field, v string, max int) error {
	if len(v) > max {
		return invalid("%s is %d bytes, over the %d limit", field, len(v), max)
	}
	if !utf8.ValidString(v) {
		return invalid("%s is not valid UTF-8", field)
	}
	return nil
}

// NormalizeTags lowercases, trims and dedupes tags and applies the bounds.
func NormalizeTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		if err := checkLen("tag", t, MaxTagLen); err != nil {
			return nil, err
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxTags {
		return nil, invalid("%d tags, over the %d limit", len(out), MaxTags)
	}
	return out, nil
}

func (s *Store) applyMeta(p *Package, m Meta) error {
	m.Title = strings.TrimSpace(m.Title)
	if m.Title == "" {
		return invalid("a title is required")
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{{"title", m.Title, 300}, {"summary", m.Summary, 8000}, {"order", m.Order, 200}, {"session", m.Session, 200},
		{"gate", m.Gate, 200}, {"repo", m.Repo, 500}, {"branch", m.Branch, 300}, {"commit", m.Commit, 64},
		{"host", m.Host, 128}, {"author", m.Author, 128}} {
		if err := checkLen(f.name, f.v, f.max); err != nil {
			return err
		}
	}
	if m.Source != "" && !oneOf(m.Source, Sources) {
		return invalid("source %q is not one of %s", m.Source, strings.Join(Sources, ", "))
	}
	if m.Role != "" && !oneOf(m.Role, Roles) {
		return invalid("role %q is not agent or human", m.Role)
	}
	tags, err := NormalizeTags(m.Tags)
	if err != nil {
		return err
	}
	p.Title, p.Summary = s.scrub(m.Title), s.scrub(m.Summary)
	p.Source, p.Order, p.Session, p.Gate, p.Repo = m.Source, m.Order, m.Session, m.Gate, m.Repo
	p.Branch, p.Commit, p.Host, p.Author, p.Role, p.Tags = m.Branch, m.Commit, m.Host, m.Author, m.Role, tags
	return nil
}

// SafeName checks a caller's item name and returns its stored form without
// the sequence prefix. A name with a path separator, a ".." segment, or a
// control character is refused; other characters outside [A-Za-z0-9._-]
// become '-'.
func SafeName(name string) (string, error) {
	if name == "" {
		return "", invalid("an item name is required")
	}
	if len(name) > MaxNameLen {
		return "", invalid("item name is %d bytes, over the %d limit", len(name), MaxNameLen)
	}
	if !utf8.ValidString(name) {
		return "", invalid("item name is not valid UTF-8")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.Contains(name, "..") {
		return "", invalid("item name %q must be a plain file name: no path separators or \"..\"", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", invalid("item name %q has a control character", name)
		}
	}
	if filepath.VolumeName(name) != "" || strings.Contains(name, ":") {
		return "", invalid("item name %q must be a plain file name", name)
	}
	clean := unsafeNameChars.ReplaceAllString(name, "-")
	clean = strings.TrimLeft(clean, ".-_")
	clean = strings.TrimRight(clean, ".")
	if clean == "" {
		clean = "item"
	}
	return clean, nil
}

// prepared is a validated item ready to write.
type prepared struct {
	item Item
	data []byte
}

func readCapped(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrUnsupported, path)
	}
	if info.Size() > max {
		return nil, fmt.Errorf("%w: %s is %d bytes, over every evidence limit", ErrTooLarge, filepath.Base(path), info.Size())
	}
	return ReadCapped(f, max)
}

// ReadCapped reads r to the end, refusing more than max bytes.
func ReadCapped(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%w: the input is over every evidence limit (%d bytes)", ErrTooLarge, max)
	}
	return b, nil
}

// prepare validates one input against seq, reading and sniffing its bytes
// and redacting text and strings. It writes nothing.
func (s *Store) prepare(in ItemInput, seq int) (prepared, error) {
	name := in.Name
	data := in.Data
	if in.Path != "" {
		if name == "" {
			name = filepath.Base(in.Path)
		}
		b, err := readCapped(in.Path, s.Limits.MaxAnyBytes())
		if err != nil {
			return prepared{}, err
		}
		data = b
	}
	clean, err := SafeName(name)
	if err != nil {
		return prepared{}, err
	}
	for _, f := range []struct {
		name, v string
		max     int
	}{{"caption", in.Caption, 2000}, {"url", in.URL, 2048}, {"app", in.App, 200}, {"step", in.Step, 200}} {
		if err := checkLen(f.name, f.v, f.max); err != nil {
			return prepared{}, err
		}
	}
	if len(in.Metadata) > MaxMetadataKeys {
		return prepared{}, invalid("%d metadata keys, over the %d limit", len(in.Metadata), MaxMetadataKeys)
	}
	var meta map[string]string
	for k, v := range in.Metadata {
		if k == "" || len(k) > 64 || !utf8.ValidString(k) {
			return prepared{}, invalid("metadata key %q must be 1-64 bytes of UTF-8", k)
		}
		if err := checkLen("metadata value", v, MaxMetadataValue); err != nil {
			return prepared{}, err
		}
		if meta == nil {
			meta = map[string]string{}
		}
		meta[k] = s.scrub(v)
	}
	m, err := Inspect(data, s.Limits)
	if err != nil {
		return prepared{}, fmt.Errorf("%s: %w", name, err)
	}
	it := Item{ID: newID("evi_"), Seq: seq, Modality: m.Modality, Name: fmt.Sprintf("%03d-%s", seq, clean),
		Caption: s.scrub(in.Caption), MIME: m.MIME, URL: s.scrub(in.URL), App: s.scrub(in.App), Step: s.scrub(in.Step),
		Metadata: meta, Width: m.Width, Height: m.Height, DurationMs: m.DurationMs, CapturedAt: in.CapturedAt.UTC().Truncate(time.Second)}
	if in.CapturedAt.IsZero() {
		it.CapturedAt = s.now()
	}
	if m.Modality == "text" {
		text := s.scrub(string(data))
		data = []byte(text)
		lines := strings.Count(text, "\n")
		if text != "" && !strings.HasSuffix(text, "\n") {
			lines++
		}
		it.LineCount, it.Redacted = &lines, true
		if len(data) <= MaxInlineTextBytes {
			it.Text = &text
		}
	}
	sum := sha256.Sum256(data)
	it.SHA256, it.Size = hex.EncodeToString(sum[:]), int64(len(data))
	return prepared{item: it, data: data}, nil
}

func (s *Store) prepareClaim(p *Package, c ClaimInput, pending []Item) (Claim, error) {
	c.Text = strings.TrimSpace(c.Text)
	if c.Text == "" {
		return Claim{}, invalid("a claim needs text")
	}
	if err := checkLen("claim", c.Text, 4000); err != nil {
		return Claim{}, err
	}
	if err := checkLen("criterion", c.Criterion, 500); err != nil {
		return Claim{}, err
	}
	if c.Verdict != "" && !oneOf(c.Verdict, Verdicts) {
		return Claim{}, invalid("verdict %q is not one of %s", c.Verdict, strings.Join(Verdicts, ", "))
	}
	known := map[string]bool{}
	for _, it := range append(append([]Item(nil), p.Items...), pending...) {
		known[it.ID] = true
	}
	for _, id := range c.ItemIDs {
		if !itemIDPattern.MatchString(id) || !known[id] {
			return Claim{}, invalid("claim item %q is not an item of %s", id, p.ID)
		}
	}
	return Claim{ID: newID("clm_"), Criterion: s.scrub(c.Criterion), Text: s.scrub(c.Text), Verdict: c.Verdict, ItemIDs: c.ItemIDs}, nil
}

// lock takes the store's lock, creating the root.
func (s *Store) lock() (func(), error) {
	if err := platformops.MkdirAll(s.Root, 0700); err != nil {
		return nil, err
	}
	return platformops.LockPath(filepath.Join(s.Root, ".lock"), false)
}

func (s *Store) dir(id string) string { return filepath.Join(s.Root, id) }

func writeManifest(dir string, p *Package) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(filepath.Join(dir, ManifestName), append(b, '\n'), 0600)
}

func (s *Store) notify(event string, p *Package, items, claims []string) {
	if s.Observe != nil {
		s.Observe(event, p, items, claims)
	}
}

// Create starts an open package.
func (s *Store) Create(m Meta) (*Package, error) {
	return s.create(m, nil, nil, false)
}

// Put creates and seals a package with its items and claims in one step.
// Every item is validated before the package appears; a refused item leaves
// nothing on disk.
func (s *Store) Put(m Meta, items []ItemInput, claims []ClaimInput) (*Package, error) {
	return s.create(m, items, claims, true)
}

func (s *Store) create(m Meta, items []ItemInput, claims []ClaimInput, seal bool) (*Package, error) {
	now := s.now()
	p := &Package{Schema: Schema, ID: newID("ev_"), State: "open", Tags: []string{}, Claims: []Claim{}, Items: []Item{}, CreatedAt: now, UpdatedAt: now}
	if err := s.applyMeta(p, m); err != nil {
		return nil, err
	}
	if len(items) > MaxItemsPerPackage {
		return nil, invalid("%d items, over the %d limit", len(items), MaxItemsPerPackage)
	}
	if len(claims) > MaxClaimsPerPackage {
		return nil, invalid("%d claims, over the %d limit", len(claims), MaxClaimsPerPackage)
	}
	// Validate every item before anything is written. Media is re-read when
	// written so a package of large videos is never all in memory at once.
	for i, in := range items {
		pr, err := s.prepare(in, i+1)
		if err != nil {
			return nil, err
		}
		p.Items = append(p.Items, pr.item)
	}
	for _, c := range claims {
		cl, err := s.prepareClaim(p, c, nil)
		if err != nil {
			return nil, err
		}
		p.Claims = append(p.Claims, cl)
	}
	if seal {
		p.State, p.SealedAt = "sealed", &now
	}
	p.recount()
	release, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer release()
	stage, err := os.MkdirTemp(s.Root, ".stage-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if err := platformops.SecureDir(stage); err != nil {
		return nil, err
	}
	for i, in := range items {
		// Read and check again: the file must still be what was validated.
		pr, err := s.prepare(in, i+1)
		if err != nil {
			return nil, err
		}
		if pr.item.SHA256 != p.Items[i].SHA256 {
			return nil, fmt.Errorf("%w: %s changed while it was added", ErrIntegrity, p.Items[i].Name)
		}
		if err := platformops.WriteFileAtomic(filepath.Join(stage, p.Items[i].Name), pr.data, 0600); err != nil {
			return nil, err
		}
	}
	if err := writeManifest(stage, p); err != nil {
		return nil, err
	}
	if err := platformops.Rename(stage, s.dir(p.ID)); err != nil {
		return nil, err
	}
	s.notify("evidence-created", p, itemIDs(p.Items), claimIDs(p.Claims))
	return p, nil
}

func itemIDs(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func claimIDs(cs []Claim) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

// Load reads a package manifest.
func (s *Store) Load(id string) (*Package, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("%w: %q is not an evidence id (ev_ + 26 lowercase characters)", ErrNotFound, id)
	}
	return loadManifest(s.dir(id), id)
}

func loadManifest(dir, id string) (*Package, error) {
	b, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	var p Package
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%w: %s/manifest.json: %v", ErrIntegrity, id, err)
	}
	if p.Schema != Schema || p.ID != id {
		return nil, fmt.Errorf("%w: %s/manifest.json is not package %s", ErrIntegrity, id, id)
	}
	for _, it := range p.Items {
		if !storedName.MatchString(it.Name) {
			return nil, fmt.Errorf("%w: %s item %q has an unsafe name", ErrIntegrity, id, it.Name)
		}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Claims == nil {
		p.Claims = []Claim{}
	}
	if p.Items == nil {
		p.Items = []Item{}
	}
	return &p, nil
}

// mutate loads an open package under the lock, applies fn, and writes the
// manifest back.
func (s *Store) mutate(id string, fn func(p *Package) error) (*Package, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("%w: %q is not an evidence id", ErrNotFound, id)
	}
	if _, err := os.Stat(s.dir(id)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	release, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer release()
	p, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if p.State != "open" {
		return nil, fmt.Errorf("%w: %s was sealed at %s and can no longer change", ErrConflict, id, p.SealedAt.Format(time.RFC3339))
	}
	if err := fn(p); err != nil {
		return nil, err
	}
	p.UpdatedAt = s.now()
	p.recount()
	if err := writeManifest(s.dir(id), p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddItem appends one item to an open package.
func (s *Store) AddItem(id string, in ItemInput) (*Package, *Item, error) {
	var added Item
	p, err := s.mutate(id, func(p *Package) error {
		if len(p.Items) >= MaxItemsPerPackage {
			return invalid("%s already has %d items, the limit", id, MaxItemsPerPackage)
		}
		seq := 1
		for _, it := range p.Items {
			seq = max(seq, it.Seq+1)
		}
		pr, err := s.prepare(in, seq)
		if err != nil {
			return err
		}
		path := filepath.Join(s.dir(id), pr.item.Name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%w: %s already exists", ErrIntegrity, pr.item.Name)
		}
		if err := platformops.WriteFileAtomic(path, pr.data, 0600); err != nil {
			return err
		}
		p.Items = append(p.Items, pr.item)
		added = pr.item
		return nil
	})
	if err != nil {
		if added.Name != "" {
			os.Remove(filepath.Join(s.dir(id), added.Name))
		}
		return nil, nil, err
	}
	s.notify("evidence-updated", p, []string{added.ID}, nil)
	return p, &added, nil
}

// AddClaim appends one claim to an open package.
func (s *Store) AddClaim(id string, c ClaimInput) (*Package, *Claim, error) {
	var added Claim
	p, err := s.mutate(id, func(p *Package) error {
		if len(p.Claims) >= MaxClaimsPerPackage {
			return invalid("%s already has %d claims, the limit", id, MaxClaimsPerPackage)
		}
		cl, err := s.prepareClaim(p, c, nil)
		if err != nil {
			return err
		}
		p.Claims = append(p.Claims, cl)
		added = cl
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.notify("evidence-updated", p, nil, []string{added.ID})
	return p, &added, nil
}

// Edit changes an open package's title, summary or tags.
func (s *Store) Edit(id string, patch Patch) (*Package, error) {
	p, err := s.mutate(id, func(p *Package) error {
		m := Meta{Title: p.Title, Summary: p.Summary, Source: p.Source, Order: p.Order, Session: p.Session, Gate: p.Gate,
			Repo: p.Repo, Branch: p.Branch, Commit: p.Commit, Host: p.Host, Author: p.Author, Role: p.Role, Tags: p.Tags}
		if patch.Title != nil {
			m.Title = *patch.Title
		}
		if patch.Summary != nil {
			m.Summary = *patch.Summary
		}
		if patch.Tags != nil {
			m.Tags = *patch.Tags
		}
		return s.applyMeta(p, m)
	})
	if err != nil {
		return nil, err
	}
	s.notify("evidence-updated", p, nil, nil)
	return p, nil
}

// Seal finalizes an open package. Sealing a sealed package is a conflict.
func (s *Store) Seal(id string) (*Package, error) {
	p, err := s.mutate(id, func(p *Package) error {
		now := s.now()
		p.State, p.SealedAt = "sealed", &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notify("evidence-sealed", p, nil, nil)
	return p, nil
}

// Path is the package's folder.
func (s *Store) Path(id string) string { return s.dir(id) }

// Verify checks every item file against the manifest's sha256 and size.
func (s *Store) Verify(id string) (*Package, error) {
	p, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	for _, it := range p.Items {
		if err := verifyFile(filepath.Join(s.dir(id), it.Name), it); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func verifyFile(path string, it Item) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrIntegrity, it.Name, err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrIntegrity, it.Name, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != it.SHA256 || n != it.Size {
		return fmt.Errorf("%w: %s does not match its manifest sha256 (the file was changed or damaged)", ErrIntegrity, it.Name)
	}
	return nil
}

// Export verifies a package and copies its manifest and items into out.
// Nothing is copied unless every item matches its sha256; a file that
// changes during the copy is removed and the export fails.
func (s *Store) Export(id, out string) (*Package, error) {
	p, err := s.Verify(id)
	if err != nil {
		return nil, err
	}
	if err := platformops.MkdirAll(out, 0700); err != nil {
		return nil, err
	}
	for _, it := range p.Items {
		b, err := os.ReadFile(filepath.Join(s.dir(id), it.Name))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrIntegrity, it.Name, err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != it.SHA256 {
			return nil, fmt.Errorf("%w: %s changed during export", ErrIntegrity, it.Name)
		}
		if err := platformops.WriteFileAtomic(filepath.Join(out, it.Name), b, 0600); err != nil {
			return nil, err
		}
	}
	m, err := os.ReadFile(filepath.Join(s.dir(id), ManifestName))
	if err != nil {
		return nil, err
	}
	if err := platformops.WriteFileAtomic(filepath.Join(out, ManifestName), m, 0600); err != nil {
		return nil, err
	}
	return p, nil
}

// Filter narrows List and Search (the /v1/evidence query parameters).
type Filter struct {
	Order, Session, Repo, Gate, Author, State string
	Sources, Modalities, Verdicts, Tags       []string
	Since, Until                              time.Time
}

// Match reports whether p passes the filter.
func (f Filter) Match(p *Package) bool {
	if f.Order != "" && p.Order != f.Order || f.Session != "" && p.Session != f.Session ||
		f.Repo != "" && p.Repo != f.Repo || f.Gate != "" && p.Gate != f.Gate ||
		f.Author != "" && p.Author != f.Author || f.State != "" && p.State != f.State {
		return false
	}
	if len(f.Sources) > 0 && !oneOf(p.Source, f.Sources) {
		return false
	}
	if !f.Since.IsZero() && p.CreatedAt.Before(f.Since) || !f.Until.IsZero() && p.CreatedAt.After(f.Until) {
		return false
	}
	if len(f.Modalities) > 0 {
		ok := false
		for _, it := range p.Items {
			if oneOf(it.Modality, f.Modalities) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Verdicts) > 0 {
		ok := false
		for _, c := range p.Claims {
			if oneOf(c.Verdict, f.Verdicts) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, t := range f.Tags {
		if !oneOf(strings.ToLower(t), p.Tags) {
			return false
		}
	}
	return true
}

// Validate checks the filter's enum values.
func (f Filter) Validate() error {
	for _, v := range f.Sources {
		if !oneOf(v, Sources) {
			return invalid("source %q is not one of %s", v, strings.Join(Sources, ", "))
		}
	}
	for _, v := range f.Modalities {
		if !oneOf(v, Modalities) {
			return invalid("modality %q is not one of %s", v, strings.Join(Modalities, ", "))
		}
	}
	for _, v := range f.Verdicts {
		if !oneOf(v, Verdicts) {
			return invalid("verdict %q is not one of %s", v, strings.Join(Verdicts, ", "))
		}
	}
	if f.State != "" && f.State != "open" && f.State != "sealed" {
		return invalid("state %q is not open or sealed", f.State)
	}
	return nil
}

// all loads every readable package, newest first. Folders that are not
// packages, and manifests that do not parse, are skipped.
func (s *Store) all() ([]*Package, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Package
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		p, err := loadManifest(s.dir(e.Name()), e.Name())
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// List returns the summaries of packages that pass f, newest first.
func (s *Store) List(f Filter) ([]Summary, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	all, err := s.all()
	if err != nil {
		return nil, err
	}
	out := []Summary{}
	for _, p := range all {
		if f.Match(p) {
			out = append(out, p.Summarize())
		}
	}
	return out, nil
}

// Size is the bytes under the root, for disk usage.
func Size(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
