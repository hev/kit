package cost

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"

	kit "github.com/hev/kit"
)

var (
	defaultOnce  sync.Once
	defaultRates []Rate
	defaultErr   error
)

// DefaultRates is the validated rate table embedded from prices.toml.
func DefaultRates() ([]Rate, error) {
	defaultOnce.Do(func() {
		defaultRates, defaultErr = ParseRates(kit.PricesTOML)
	})
	return defaultRates, defaultErr
}

// ParseRates decodes and validates a prices.toml document.
func ParseRates(data []byte) ([]Rate, error) {
	var c Config
	if _, err := toml.Decode(string(data), &c); err != nil {
		return nil, err
	}
	return c.Rates, validateRates(c.Rates)
}

func validateRates(rates []Rate) error {
	seen := map[string]bool{}
	for _, r := range rates {
		_, err := time.Parse("2006-01-02", r.Effective)
		key := r.Model + "/" + r.Effective
		if err != nil || r.Model == "" || seen[key] || r.Input < 0 || r.Cached < 0 || r.Creation < 0 || r.Output < 0 || invalidNumber(r.Input, r.Cached, r.Creation, r.Output) {
			return fmt.Errorf("invalid or duplicate rate %q", key)
		}
		if (r.Creation1h != nil && (*r.Creation1h < 0 || invalidNumber(*r.Creation1h))) || r.LongContext < 0 || (r.LongContext > 0 && (r.LongInput <= 0 || r.LongOutput <= 0 || invalidNumber(r.LongInput, r.LongOutput))) {
			return fmt.Errorf("invalid extended rate %q", key)
		}
		seen[key] = true
	}
	return nil
}

var (
	dateSuffix   = regexp.MustCompile(`-\d{8}$`)
	effortSuffix = regexp.MustCompile(`-(none|minimal|low|medium|high|xhigh|max)$`)
)

// NormalizeModel reduces a transcript model ID to the rate-table family it
// bills as: no provider prefix, context tag, snapshot date or effort level.
// "anthropic/claude-haiku-4-5-20251001" and "gemini-4-neon-medium" become
// "claude-haiku-4-5" and "gemini-4-neon". Version digits are kept, so
// "claude-opus-5-5" never matches "claude-opus-5".
func NormalizeModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	if i := strings.IndexAny(m, "[@"); i > 0 {
		m = m[:i]
	}
	for {
		next := effortSuffix.ReplaceAllString(dateSuffix.ReplaceAllString(m, ""), "")
		if next == m {
			return m
		}
		m = next
	}
}

// LookupRate finds the rate in effect for model on date (YYYY-MM-DD). An exact
// model match wins over a normalized one. dated is false when the rate is only
// an approximation: date is empty (the latest rate is used) or precedes the
// model's earliest row (the earliest is used). It returns nil for a model no
// row prices.
func LookupRate(rates []Rate, model, date string) (rate *Rate, dated bool) {
	var candidates []*Rate
	for i := range rates {
		if rates[i].Model == model {
			candidates = append(candidates, &rates[i])
		}
	}
	if len(candidates) == 0 {
		want := NormalizeModel(model)
		for i := range rates {
			if want != "" && NormalizeModel(rates[i].Model) == want {
				candidates = append(candidates, &rates[i])
			}
		}
	}
	var best, earliest, latest *Rate
	for _, r := range candidates {
		if date != "" && r.Effective <= date && (best == nil || r.Effective > best.Effective) {
			best = r
		}
		if earliest == nil || r.Effective < earliest.Effective {
			earliest = r
		}
		if latest == nil || r.Effective > latest.Effective {
			latest = r
		}
	}
	switch {
	case best != nil:
		return best, true
	case date == "":
		return latest, false
	default:
		return earliest, false
	}
}

// USD prices one usage record. perRequest says the usage is a single API
// request, so the long-context tier can be judged from its own input; for an
// aggregate the tier is unknowable. ok is false when a category is unpriced.
func (r Rate) USD(u Usage, perRequest bool) (usd float64, ok bool) {
	if u.Creation1h > 0 && r.Creation1h == nil {
		return 0, false
	}
	inputMultiplier, outputMultiplier := 1.0, 1.0
	if r.LongContext > 0 && u.Input+u.Cached+u.Creation+u.Creation1h > r.LongContext {
		if !perRequest {
			return 0, false
		}
		inputMultiplier, outputMultiplier = r.LongInput, r.LongOutput
	}
	creation1h := 0.0
	if r.Creation1h != nil {
		creation1h = float64(u.Creation1h) * *r.Creation1h
	}
	return ((float64(u.Input)*r.Input+float64(u.Cached)*r.Cached+float64(u.Creation)*r.Creation+creation1h)*inputMultiplier + float64(u.Output)*r.Output*outputMultiplier) / 1e6, true
}
