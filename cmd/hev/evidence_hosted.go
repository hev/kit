package main

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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/storagemode"
	"github.com/oklog/ulid/v2"
)

const hostedEvidencePath = "/v1/evidence"
const hostedEvidenceBlobCap = 10 << 20

var hostedEvidenceKey = regexp.MustCompile(`^hk_[A-Za-z0-9_-]{43}$`)
var hostedEvidenceID = regexp.MustCompile(`^ev_[0-9a-hjkmnp-tv-z]{26}$`)
var hostedEvidenceHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var hostedEvidenceLocalPath = regexp.MustCompile(`(?i)(?:/(?:users|home|private|tmp|var|etc|mnt|opt)/|[a-z]:[\\/]|\\\\)[^\s"'<>]+`)
var hostedEvidenceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._ -]{0,199}$`)

type remoteEvidence struct {
	ctx       context.Context
	client    *http.Client
	base, key string
	scrub     func(string) string
}

func hostedEvidence(ctx context.Context) (*remoteEvidence, error) {
	var config struct {
		Kit struct {
			Key     *string `toml:"key"`
			API     string  `toml:"api"`
			Service string  `toml:"service"`
		} `toml:"kit"`
	}
	if _, err := toml.DecodeFile(layerenv.ConfigPath(), &config); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("could not read evidence storage configuration")
	}
	key := config.Kit.Key
	base := defaultString(config.Kit.API, config.Kit.Service)
	if env, ok := os.LookupEnv("HEV_API_KEY"); ok {
		key = &env
		base = defaultString(os.Getenv("HEV_KIT_SERVICE"), base)
	} else if base == "" {
		base = os.Getenv("HEV_KIT_SERVICE")
	}
	if key == nil {
		mode, err := storagemode.Resolve(storagemode.Options{ConfigKey: true, Peek: true})
		reasons, intentErr := storagemode.Intent("")
		if err != nil || intentErr != nil || mode.Mode == storagemode.Layer || len(reasons) != 0 || base != "" {
			return nil, errors.New("hosted evidence requires a signed-in kit machine key")
		}
		return nil, nil
	}
	if !hostedEvidenceKey.MatchString(strings.TrimSpace(*key)) {
		return nil, errors.New("invalid hosted evidence credential")
	}
	base = defaultString(base, "https://kit.hevmind.com")
	u, err := url.Parse(base)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
		return nil, errors.New("invalid hosted evidence API origin")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	token := strings.TrimSpace(*key)
	salt := sha256.Sum256([]byte(token))
	scrubber, err := redact.New(salt[:])
	if err != nil {
		return nil, errors.New("could not initialize hosted evidence redaction")
	}
	scrub := func(text string) string {
		text, _ = scrubber.Text(strings.ReplaceAll(text, token, "[redacted]"))
		return hostedEvidenceLocalPath.ReplaceAllString(text, "[local path]")
	}
	return &remoteEvidence{ctx, client, strings.TrimRight(base, "/"), token, scrub}, nil
}

func (s *remoteEvidence) request(method, path string, input io.Reader, length int64, contentType, match, key string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(s.ctx, method, s.base+path, input)
	if err != nil {
		return nil, errors.New("could not prepare hosted evidence request")
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if match != "" {
		req.Header.Set("If-Match", match)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if length > 0 {
		req.ContentLength = length
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("hosted evidence request failed")
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return res, nil
	}
	res.Body.Close()
	switch res.StatusCode {
	case 404:
		return nil, evidence.ErrNotFound
	case 409:
		return nil, evidence.ErrConflict
	case 413:
		return nil, evidence.ErrTooLarge
	case 415:
		return nil, evidence.ErrUnsupported
	default:
		return nil, fmt.Errorf("hosted evidence request refused (HTTP %d)", res.StatusCode)
	}
}

func (s *remoteEvidence) json(method, path string, input any, match, key string, out any) (string, error) {
	var body io.Reader
	var size int64
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return "", errors.New("invalid hosted evidence request")
		}
		if len(b) > 256<<10 {
			return "", evidence.ErrTooLarge
		}
		body, size = bytes.NewReader(b), int64(len(b))
	}
	res, err := s.request(method, path, body, size, "application/json", match, key)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	d := json.NewDecoder(io.LimitReader(res.Body, (5<<20)+1))
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return "", errors.New("invalid hosted evidence response")
	}
	return res.Header.Get("ETag"), nil
}

type remoteEvidenceItem struct {
	Name     string            `json:"name"`
	Hash     string            `json:"sha256"`
	Modality string            `json:"modality"`
	Caption  string            `json:"caption,omitempty"`
	URL      string            `json:"url,omitempty"`
	App      string            `json:"app,omitempty"`
	Step     string            `json:"step,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (s *remoteEvidence) upload(it evidence.ItemInput) (remoteEvidenceItem, error) {
	in := io.Reader(bytes.NewReader(it.Data))
	size, name := int64(len(it.Data)), it.Name
	if it.Path != "" {
		initial, err := os.Stat(it.Path)
		if err != nil || !initial.Mode().IsRegular() {
			return remoteEvidenceItem{}, errors.New("evidence input must be a regular file")
		}
		f, err := os.Open(it.Path)
		if err != nil {
			return remoteEvidenceItem{}, errors.New("could not open evidence input")
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			return remoteEvidenceItem{}, errors.New("evidence input must be a regular file")
		}
		in, size = f, stat.Size()
		name = defaultString(name, filepath.Base(it.Path))
	}
	if size < 1 || size > hostedEvidenceBlobCap {
		return remoteEvidenceItem{}, evidence.ErrTooLarge
	}
	if !hostedEvidenceName.MatchString(name) || strings.Contains(name, "..") || s.scrub(name) != name {
		return remoteEvidenceItem{}, evidence.ErrInvalid
	}
	reader := bufio.NewReaderSize(io.LimitReader(in, size), 512)
	prefix, _ := reader.Peek(512)
	if strings.HasPrefix(http.DetectContentType(prefix), "text/") {
		if size > evidence.MaxTextBytes {
			return remoteEvidenceItem{}, evidence.ErrTooLarge
		}
		raw, err := io.ReadAll(reader)
		if err != nil {
			return remoteEvidenceItem{}, errors.New("could not read evidence input")
		}
		text := s.scrub(string(raw))
		in, size = strings.NewReader(text), int64(len(text))
	} else {
		in = reader
	}
	res, err := s.request(http.MethodPost, hostedEvidencePath+"/blobs", io.LimitReader(in, size), size, "application/octet-stream", "", "")
	if err != nil {
		return remoteEvidenceItem{}, err
	}
	defer res.Body.Close()
	var b remoteEvidenceItem
	if json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&b) != nil || !hostedEvidenceHash.MatchString(b.Hash) {
		return remoteEvidenceItem{}, errors.New("invalid hosted evidence blob response")
	}
	b.Name, b.Caption, b.URL, b.App, b.Step = name, s.scrub(it.Caption), s.scrub(it.URL), s.scrub(it.App), s.scrub(it.Step)
	if len(it.Metadata) != 0 {
		b.Metadata = make(map[string]string, len(it.Metadata))
		for key, value := range it.Metadata {
			if s.scrub(key) != key {
				return remoteEvidenceItem{}, evidence.ErrInvalid
			}
			b.Metadata[key] = s.scrub(value)
		}
	}
	return b, nil
}

func (s *remoteEvidence) remoteClaim(c evidence.ClaimInput) map[string]any {
	return map[string]any{"criterion": s.scrub(c.Criterion), "text": s.scrub(c.Text), "verdict": c.Verdict, "item_ids": c.ItemIDs}
}

func (s *remoteEvidence) Create(m evidence.Meta) (*evidence.Package, error) {
	return s.create(m, nil, nil, "open")
}

func (s *remoteEvidence) Put(m evidence.Meta, items []evidence.ItemInput, claims []evidence.ClaimInput) (*evidence.Package, error) {
	return s.create(m, items, claims, "sealed")
}

func (s *remoteEvidence) create(m evidence.Meta, items []evidence.ItemInput, claims []evidence.ClaimInput, state string) (*evidence.Package, error) {
	if len(items) > evidence.MaxItemsPerPackage || len(claims) > evidence.MaxClaimsPerPackage {
		return nil, evidence.ErrTooLarge
	}
	for _, claim := range claims {
		if len(claim.ItemIDs) != 0 {
			return nil, errors.New("attach item IDs to hosted claims after creating the package")
		}
	}
	data := make([]remoteEvidenceItem, 0, len(items))
	for _, item := range items {
		blob, err := s.upload(item)
		if err != nil {
			return nil, err
		}
		data = append(data, blob)
	}
	c := make([]map[string]any, 0, len(claims))
	for _, claim := range claims {
		c = append(c, s.remoteClaim(claim))
	}
	tags := make([]string, len(m.Tags))
	for i, tag := range m.Tags {
		tags[i] = s.scrub(tag)
	}
	payload := map[string]any{"title": s.scrub(m.Title), "summary": s.scrub(m.Summary), "source": m.Source, "order": s.scrub(m.Order), "session": s.scrub(m.Session),
		"gate": s.scrub(m.Gate), "repo": s.scrub(m.Repo), "branch": s.scrub(m.Branch), "commit": s.scrub(m.Commit), "role": m.Role, "tags": tags, "items": data, "claims": c, "state": state}
	var p evidence.Package
	_, err := s.json(http.MethodPost, hostedEvidencePath, payload, "", ulid.Make().String(), &p)
	return &p, err
}

func (s *remoteEvidence) load(id string) (*evidence.Package, string, error) {
	if !hostedEvidenceID.MatchString(id) {
		return nil, "", evidence.ErrInvalid
	}
	var p evidence.Package
	etag, err := s.json(http.MethodGet, hostedEvidencePath+"/"+id, nil, "", "", &p)
	if err == nil && (p.ID != id || etag == "") {
		return nil, "", errors.New("invalid hosted evidence package response")
	}
	return &p, etag, err
}

func (s *remoteEvidence) Load(id string) (*evidence.Package, error) {
	p, _, err := s.load(id)
	return p, err
}

func (s *remoteEvidence) patch(id string, payload any) (*evidence.Package, error) {
	p, etag, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if p.State != "open" {
		return nil, evidence.ErrConflict
	}
	var updated evidence.Package
	_, err = s.json(http.MethodPatch, hostedEvidencePath+"/"+id, payload, etag, ulid.Make().String(), &updated)
	return &updated, err
}

func (s *remoteEvidence) AddItem(id string, it evidence.ItemInput) (*evidence.Package, *evidence.Item, error) {
	blob, err := s.upload(it)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.patch(id, map[string]any{"add_items": []remoteEvidenceItem{blob}})
	if err != nil {
		return nil, nil, err
	}
	if len(p.Items) == 0 {
		return nil, nil, errors.New("hosted response omitted added item")
	}
	return p, &p.Items[len(p.Items)-1], nil
}

func (s *remoteEvidence) AddClaim(id string, claim evidence.ClaimInput) (*evidence.Package, *evidence.Claim, error) {
	p, err := s.patch(id, map[string]any{"add_claims": []any{s.remoteClaim(claim)}})
	if err != nil {
		return nil, nil, err
	}
	if len(p.Claims) == 0 {
		return nil, nil, errors.New("hosted response omitted added claim")
	}
	return p, &p.Claims[len(p.Claims)-1], nil
}

func (s *remoteEvidence) Seal(id string) (*evidence.Package, error) {
	return s.patch(id, map[string]string{"state": "sealed"})
}

func (s *remoteEvidence) List(filter evidence.Filter) ([]evidence.Summary, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	var result []evidence.Summary
	cursor := ""
	for range 20 {
		path := hostedEvidencePath + "?limit=20"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		var page struct {
			Data []evidence.Package `json:"data"`
			Next string             `json:"next_cursor"`
		}
		if _, err := s.json(http.MethodGet, path, nil, "", "", &page); err != nil {
			return nil, err
		}
		for _, p := range page.Data {
			for modality, count := range map[string]int{"text": p.ItemCount.Text, "image": p.ItemCount.Image, "video": p.ItemCount.Video} {
				if count > 0 {
					p.Items = append(p.Items, evidence.Item{Modality: modality})
				}
			}
			if filter.Match(&p) {
				result = append(result, p.Summarize())
			}
		}
		if page.Next == "" {
			return result, nil
		}
		if !hostedEvidenceID.MatchString(page.Next) || (cursor != "" && page.Next >= cursor) {
			return nil, errors.New("invalid hosted evidence page cursor")
		}
		cursor = page.Next
	}
	return nil, errors.New("hosted evidence list exceeded the 400-package scan limit")
}

func (s *remoteEvidence) Path(id string) string {
	if id == "" {
		return s.base + hostedEvidencePath
	}
	return s.base + hostedEvidencePath + "/" + id
}

func (s *remoteEvidence) download(item evidence.Item, out io.Writer) error {
	if !hostedEvidenceHash.MatchString(item.SHA256) || item.Size < 1 || item.Size > hostedEvidenceBlobCap {
		return evidence.ErrIntegrity
	}
	res, err := s.request(http.MethodGet, hostedEvidencePath+"/blobs/"+item.SHA256, nil, 0, "", "", "")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(h, out), io.LimitReader(res.Body, item.Size+1))
	if err != nil || n != item.Size || hex.EncodeToString(h.Sum(nil)) != item.SHA256 {
		return evidence.ErrIntegrity
	}
	return nil
}

func (s *remoteEvidence) Verify(id string) (*evidence.Package, error) {
	p, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if len(p.Items) > evidence.MaxItemsPerPackage {
		return nil, evidence.ErrInvalid
	}
	for _, it := range p.Items {
		if err := s.download(it, io.Discard); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (s *remoteEvidence) Export(id, out string) (*evidence.Package, error) {
	p, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if len(p.Items) > evidence.MaxItemsPerPackage {
		return nil, evidence.ErrInvalid
	}
	if _, err := os.Lstat(out); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("evidence export destination must not exist")
	}
	dir, err := os.MkdirTemp(filepath.Dir(out), ".hev-evidence-export-")
	if err != nil {
		return nil, errors.New("cannot create evidence export")
	}
	defer os.RemoveAll(dir)
	for i, it := range p.Items {
		if !hostedEvidenceName.MatchString(it.Name) || strings.Contains(it.Name, "..") {
			return nil, evidence.ErrInvalid
		}
		name := fmt.Sprintf("%03d-%s", i+1, it.Name)
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, errors.New("cannot create evidence export file")
		}
		err = s.download(it, file)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, errors.New("cannot finish evidence export file")
		}
		p.Items[i].Name = name
	}
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if os.WriteFile(filepath.Join(dir, evidence.ManifestName), body, 0600) != nil || os.Rename(dir, out) != nil {
		return nil, errors.New("cannot finish evidence export")
	}
	return p, nil
}

func (s *remoteEvidence) Search(context.Context, evidence.SearchQuery) (*evidence.SearchResponse, error) {
	return nil, errors.New("hosted evidence search is unavailable")
}
