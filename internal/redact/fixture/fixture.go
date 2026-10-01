// Package fixture builds synthetic credentials for acceptance tests. Nothing
// here is a valid credential. The seed and pinned policy make runs repeatable.
package fixture

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/redact"
	"github.com/zricethezav/gitleaks/v8/config"
)

type Sample struct{ Rule, Text, Secret string }

// Samples includes every embedded text rule, plus every kit envelope. Rules
// that only inspect file paths have no transcript text and are excluded.
func Samples() ([]Sample, error) {
	var policy struct {
		Rules []struct {
			ID, Regex   string
			SecretGroup int
			Entropy     float64
			Keywords    []string
		}
	}
	if _, err := toml.Decode(config.DefaultConfig, &policy); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewSource(224))
	scrubber, err := redact.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		return nil, err
	}
	var samples []Sample
	for _, rule := range policy.Rules {
		if rule.Regex == "" {
			continue
		}
		tree, err := syntax.Parse(rule.Regex, syntax.Perl)
		if err != nil {
			return nil, err
		}
		re := regexp.MustCompile(rule.Regex)
		found := false
		for attempt := 0; attempt < 1000; attempt++ {
			text := generate(tree, rng)
			// Keyword prefilters operate on the whole payload, independently of the
			// extraction regex. Include context without wrapping the matched value.
			text += "\n" + strings.Join(rule.Keywords, " ")
			m := re.FindStringSubmatch(text)
			if m == nil {
				continue
			}
			group := rule.SecretGroup
			if group == 0 && len(m) > 1 {
				for i := 1; i < len(m); i++ {
					if m[i] != "" {
						group = i
						break
					}
				}
			}
			if group >= len(m) {
				continue
			}
			secret := strings.TrimSpace(m[group])
			if len(secret) < 4 || entropy(secret) < rule.Entropy {
				continue
			}
			clean, counts := scrubber.Text(text)
			if len(counts) == 0 || strings.Contains(clean, secret) {
				continue
			}
			samples = append(samples, Sample{rule.ID, text, secret})
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("cannot synthesize text rule %s", rule.ID)
		}
	}
	extras := []Sample{
		{"pem", "-----BEGIN PRIVATE KEY-----\nAbCdEf1234567\n-----END PRIVATE KEY-----", ""},
		{"pem-certificate", "-----BEGIN CERTIFICATE-----\nAbCdEf1234567\n-----END CERTIFICATE-----", ""},
		{"authorization", "Authorization: Bearer synthetic-authorization-value", "synthetic-authorization-value"},
		{"url-credentials", "https://user:synthetic-password@example.invalid/path", "user:synthetic-password"},
		{"slack-token", "xoxe-" + "1234567890-AbCdEfGhIjKlMnOpQrSt", ""},
		{"sk-token", "sk-" + "ABCDEFGHIJKLMNOP0123456789", ""},
		{"turbopuffer", "tpuf_" + "AbCdEfGhIjKlMnOpQrStUvWx01234567", ""},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.AbCdEfGhIjKlMnOpQrStUvWxYz", ""},
		{"high-entropy-assignment", "UNUSUAL_KEY=AbCdEfGhIjKlMnOpQrStUvWx0123456789", "AbCdEfGhIjKlMnOpQrStUvWx0123456789"},
	}
	for _, s := range extras {
		if s.Secret == "" {
			s.Secret = s.Text
		}
		samples = append(samples, s)
	}
	return samples, nil
}

func entropy(s string) float64 {
	counts := map[rune]int{}
	for _, r := range s {
		counts[r]++
	}
	e := 0.0
	for _, n := range counts {
		p := float64(n) / float64(len([]rune(s)))
		e -= p * math.Log2(p)
	}
	return e
}
func generate(re *syntax.Regexp, r *rand.Rand) string {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune)
	case syntax.OpCharClass:
		var chars []rune
		for i := 0; i < len(re.Rune); i += 2 {
			for c := max(re.Rune[i], 32); c <= min(re.Rune[i+1], 126); c++ {
				chars = append(chars, c)
			}
		}
		if len(chars) == 0 {
			return string(re.Rune[0])
		}
		return string(chars[r.Intn(len(chars))])
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return string(rune('a' + r.Intn(26)))
	case syntax.OpCapture:
		return generate(re.Sub[0], r)
	case syntax.OpConcat:
		var b strings.Builder
		for _, s := range re.Sub {
			b.WriteString(generate(s, r))
		}
		return b.String()
	case syntax.OpAlternate:
		return generate(re.Sub[r.Intn(len(re.Sub))], r)
	case syntax.OpQuest:
		if r.Intn(2) == 0 {
			return ""
		}
		return generate(re.Sub[0], r)
	case syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		lo, hi := 0, 4
		if re.Op == syntax.OpPlus {
			lo = 1
		}
		if re.Op == syntax.OpRepeat {
			lo = re.Min
			hi = re.Max
			if hi < 0 {
				hi = lo + 8
			}
			hi = min(hi, lo+16)
		}
		n := lo + r.Intn(hi-lo+1)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(generate(re.Sub[0], r))
		}
		return b.String()
	default:
		return ""
	}
}
