// Package redact scrubs transcript strings before archive rows or model inputs
// are built. Rules and fingerprints are shared by ingestion and archive migration.
package redact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/trace"
	"github.com/zricethezav/gitleaks/v8/config"
)

// Version identifies the scrubbing policy for upgrade migration bookkeeping.
const Version = "gitleaks-8.24.3-kit-1"

type Counts map[string]int

func (c Counts) Add(other Counts) {
	for rule, n := range other {
		c[rule] += n
	}
}

type rule struct {
	id       string
	re       *regexp.Regexp
	group    int
	entropy  float64
	keywords []string
}
type Scrubber struct {
	salt  []byte
	rules []rule
}

var rulesOnce sync.Once
var knownRules []rule
var rulesErr error

// New creates an immutable scrubber, safe to share across read-side workers.
func New(salt []byte) (*Scrubber, error) {
	if len(salt) < 32 {
		return nil, fmt.Errorf("redaction salt must contain at least 32 bytes")
	}
	rulesOnce.Do(func() {
		var raw struct {
			Rules []struct {
				ID, Regex   string
				SecretGroup int
				Entropy     float64
				Keywords    []string
			}
		}
		if _, rulesErr = toml.Decode(config.DefaultConfig, &raw); rulesErr != nil {
			return
		}
		// Transcript content has no trusted allowlist: examples and comments can
		// contain real pasted credentials too. Path-only rules have no text to scrub.
		for _, r := range raw.Rules {
			if r.Regex == "" {
				continue
			}
			re, err := regexp.Compile(r.Regex)
			if err != nil {
				rulesErr = err
				return
			}
			group := r.SecretGroup
			if group == 0 && re.NumSubexp() > 0 {
				group = -1
			}
			knownRules = append(knownRules, rule{r.ID, re, group, r.Entropy, r.Keywords})
		}
	})
	if rulesErr != nil {
		return nil, rulesErr
	}
	extra := []rule{
		{id: "pem", re: regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z0-9 ]*PRIVATE KEY|CERTIFICATE)-----.*?-----END (?:[A-Z0-9 ]*PRIVATE KEY|CERTIFICATE)-----`)},
		{id: "authorization", re: regexp.MustCompile(`(?i)\bauthorization["']?\s*:\s*["']?(?:[A-Za-z][A-Za-z0-9_-]*[ \t]+)?([^\r\n"'\x60]+)`), group: 1},
		{id: "url-credentials", re: regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://([^\s/@]+:[^\s/@]+)@`), group: 1},
		{id: "slack-token", re: regexp.MustCompile(`\bxox[a-z]-[A-Za-z0-9-]{10,}`)},
		{id: "sk-token", re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`)},
		{id: "turbopuffer", re: regexp.MustCompile(`\btpuf_[A-Za-z0-9_-]{16,}`)},
		{id: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)},
		{id: "high-entropy-assignment", re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\s*=\s*["']?([A-Za-z0-9_+/=.-]{20,})`), group: 1, entropy: 3.5},
	}
	// Specific outer envelopes win ties; gitleaks provider rules precede the
	// generic entropy rule. Sorted interval selection below handles overlap.
	rules := append(extra[:len(extra)-1:len(extra)-1], knownRules...)
	rules = append(rules, extra[len(extra)-1])
	return &Scrubber{append([]byte(nil), salt...), rules}, nil
}

type hit struct {
	start, end, priority int
	rule                 string
}

var marker = regexp.MustCompile(`\[REDACTED:[a-zA-Z0-9_-]+#[0-9a-f]{16}\]`)

// Text returns scrubbed text and counts of replacements (not overlapping detections).
// Fingerprints are HMAC-SHA256 truncated to 64 bits, independent of rule name.
func (s *Scrubber) Text(text string) (string, Counts) {
	counts := Counts{}
	if s == nil || text == "" {
		return text, counts
	}
	protected := marker.FindAllStringIndex(text, -1)
	var hits []hit
	lower := strings.ToLower(text)
	for priority, r := range s.rules {
		if len(r.keywords) > 0 {
			found := false
			for _, k := range r.keywords {
				if strings.Contains(lower, strings.ToLower(k)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			group := r.group * 2
			if r.group == -1 {
				group = 0
				for g := 2; g+1 < len(m); g += 2 {
					if m[g] >= 0 && m[g+1] > m[g] {
						group = g
						break
					}
				}
			}
			if group+1 >= len(m) || m[group] < 0 {
				continue
			}
			start, end := m[group], m[group+1]
			if start == end {
				continue
			}
			if r.entropy > 0 && entropy(text[start:end]) < r.entropy {
				continue
			}
			// Preserve only the marker itself. A marker embedded beside a new
			// credential must not exempt the entire Authorization/PEM match.
			pieces := [][2]int{{start, end}}
			for _, p := range protected {
				var remaining [][2]int
				for _, part := range pieces {
					if part[0] >= p[1] || part[1] <= p[0] {
						remaining = append(remaining, part)
						continue
					}
					if part[0] < p[0] {
						remaining = append(remaining, [2]int{part[0], p[0]})
					}
					if part[1] > p[1] {
						remaining = append(remaining, [2]int{p[1], part[1]})
					}
				}
				pieces = remaining
			}
			for _, part := range pieces {
				// Header whitespace is not part of the credential identity.
				value := text[part[0]:part[1]]
				left := len(value) - len(strings.TrimLeft(value, " \t\r\n"))
				right := len(strings.TrimRight(value, " \t\r\n"))
				if left < right {
					hits = append(hits, hit{part[0] + left, part[0] + right, priority, r.id})
				}
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].start != hits[j].start {
			return hits[i].start < hits[j].start
		}
		if hits[i].end != hits[j].end {
			return hits[i].end > hits[j].end
		}
		return hits[i].priority < hits[j].priority
	})
	// Union partially overlapping hits as well as contained ones: no suffix of
	// a credential can survive because another rule matched its prefix first.
	var out strings.Builder
	cursor := 0
	for i := 0; i < len(hits); {
		h := hits[i]
		i++
		for i < len(hits) && hits[i].start < h.end {
			if hits[i].end > h.end {
				h.end = hits[i].end
			}
			i++
		}
		out.WriteString(text[cursor:h.start])
		mac := hmac.New(sha256.New, s.salt)
		mac.Write([]byte(text[h.start:h.end]))
		out.WriteString("[REDACTED:" + h.rule + "#" + hex.EncodeToString(mac.Sum(nil)[:8]) + "]")
		counts[h.rule]++
		cursor = h.end
	}
	if cursor == 0 {
		return text, counts
	}
	out.WriteString(text[cursor:])
	return out.String(), counts
}
func entropy(v string) float64 {
	counts := map[rune]int{}
	for _, r := range v {
		counts[r]++
	}
	n := float64(len([]rune(v)))
	e := 0.0
	for _, c := range counts {
		p := float64(c) / n
		e -= p * math.Log2(p)
	}
	return e
}

// Turns mutates every string field, including harness titles and metadata, before
// any derived rows are made. Reflection covers new string fields by default.
func (s *Scrubber) Turns(turns []trace.Turn) Counts {
	counts := Counts{}
	var scrub func(reflect.Value)
	scrub = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.String:
			text, c := s.Text(v.String())
			v.SetString(text)
			counts.Add(c)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				scrub(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				scrub(v.Index(i))
			}
		}
	}
	scrub(reflect.ValueOf(turns))
	return counts
}
