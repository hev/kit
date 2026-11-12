// Package dispatch sends bounded agent runs to configured machines.
package dispatch

import (
	"encoding/base64"
	"fmt"
	"github.com/hev/kit/internal/weave"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Order struct {
	Every       string `json:"every,omitempty"`
	Until       string `json:"until,omitempty"`
	MaxLifetime string `json:"max_lifetime,omitempty"`
	Preview     string `json:"preview,omitempty"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Repo        string `json:"repo"`
	Base        string `json:"base"`
	Harness     string `json:"harness"`
	Model       string `json:"model,omitempty"`
	Validation  string `json:"validation"`
	Reply       string `json:"reply,omitempty"`
	Issue       string `json:"issue,omitempty"`
	Class       string `json:"class,omitempty"`
	// Context names a context recipe: a JSON file in the order's repository,
	// read at the run's base commit. Empty means no recipe (see context.go).
	Context string `json:"context,omitempty"`
	// ContextRecipe is a recipe carried in the order for this dispatch only:
	// standard base64 of its JSON (hev dispatch --context-file writes it).
	// order.md carries it; meta.json records its hash instead.
	ContextRecipe string `json:"-"`
	Timeout       int    `json:"timeout"`
}

// decodeContextRecipe checks a carried recipe and returns its bytes.
func decodeContextRecipe(v string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("invalid context_recipe: not base64: %w", err)
	}
	if len(raw) > maxRecipeBytes {
		return nil, fmt.Errorf("invalid context_recipe: exceeds %d bytes", maxRecipeBytes)
	}
	if _, err := weave.DecodeRecipe(raw); err != nil {
		return nil, fmt.Errorf("invalid context_recipe: %w", err)
	}
	return raw, nil
}

// EncodeContextRecipe checks a local recipe file's bytes and returns the
// context_recipe value that carries them in an order.
func EncodeContextRecipe(raw []byte) (string, error) {
	if len(raw) > maxRecipeBytes {
		return "", fmt.Errorf("context recipe exceeds %d bytes", maxRecipeBytes)
	}
	r, err := weave.DecodeRecipe(raw)
	if err != nil {
		return "", fmt.Errorf("invalid context recipe: %w", err)
	}
	for _, s := range r.Sources {
		if err := weave.CheckPath(s.Path); err != nil {
			return "", fmt.Errorf("invalid context recipe: %w", err)
		}
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// WithContextRecipe adds a context_recipe line to an order's frontmatter. An
// order that already names a recipe is refused.
func WithContextRecipe(text string, raw []byte) (string, error) {
	o, err := Parse(text)
	if err != nil {
		return "", err
	}
	if o.Context != "" || o.ContextRecipe != "" {
		return "", fmt.Errorf("the order already names a context recipe; use either context: or --context-file")
	}
	v, err := EncodeContextRecipe(raw)
	if err != nil {
		return "", err
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return "---\ncontext_recipe: " + v + "\n" + strings.TrimPrefix(text, "---\n"), nil
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidName accepts run and orchestrator names. The o- prefix is reserved for
// orchestrator row addresses, so a run can never shadow one.
func ValidName(s string) bool { return namePattern.MatchString(s) && !strings.HasPrefix(s, "o-") }

// ValidLabel accepts host labels and other identifiers outside run routing.
func ValidLabel(s string) bool { return namePattern.MatchString(s) }

// ValidAddress accepts a run name or an o-NAME orchestrator row address.
func ValidAddress(s string) bool {
	if n, ok := strings.CutPrefix(s, "o-"); ok {
		return ValidName(n)
	}
	return ValidName(s)
}
func Parse(text string) (Order, error) {
	o := Order{Base: "main", Harness: "codex", Validation: "none", Timeout: 120}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return o, fmt.Errorf("order must start with ---")
	}
	fields := map[string]string{}
	end := 0
	for i, line := range lines[1:] {
		if line == "---" {
			end = i + 1
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return o, fmt.Errorf("expected flat key: value")
		}
		v = strings.TrimSpace(v)
		switch k {
		case "name", "host", "repo", "base", "harness", "model", "validation", "reply", "issue", "linear", "timeout", "preview", "every", "until", "max_lifetime", "class", "context", "context_recipe", "trace":
		default:
			return o, fmt.Errorf("unknown key: %s", k)
		}
		if k == "linear" {
			k = "issue"
		}
		if _, ok := fields[k]; ok {
			return o, fmt.Errorf("duplicate key: %s", k)
		}
		if strings.HasPrefix(v, "\"") || strings.HasPrefix(v, "'") {
			q := v[:1]
			j := strings.Index(v[1:], q)
			if j < 0 {
				return o, fmt.Errorf("unclosed quote")
			}
			rest := strings.TrimSpace(v[j+2:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return o, fmt.Errorf("invalid scalar")
			}
			v = v[1 : j+1]
		} else {
			v = strings.TrimSpace(strings.SplitN(v, "#", 2)[0])
			scalar := v
			if k == "preview" {
				scalar = strings.ReplaceAll(scalar, "{port}", "")
			}
			if strings.ContainsAny(scalar, "[]{}\t") {
				return o, fmt.Errorf("expected scalar")
			}
		}
		if v == "" {
			return o, fmt.Errorf("empty field: %s", k)
		}
		fields[k] = v
	}
	if end == 0 {
		return o, fmt.Errorf("unclosed frontmatter")
	}
	accept := false
	for _, l := range lines[end+1:] {
		if strings.TrimSpace(l) == "## Acceptance" {
			accept = true
		}
	}
	if !accept {
		return o, fmt.Errorf("missing ## Acceptance section")
	}
	for k, v := range fields {
		switch k {
		case "every":
			o.Every = v
		case "until":
			o.Until = v
		case "max_lifetime":
			o.MaxLifetime = v
		case "preview":
			o.Preview = v
		case "name":
			o.Name = v
		case "host":
			o.Host = v
		case "repo":
			o.Repo = v
		case "base":
			o.Base = v
		case "harness":
			o.Harness = v
		case "model":
			o.Model = v
		case "validation":
			o.Validation = v
		case "reply":
			o.Reply = v
		case "issue":
			o.Issue = v
		case "class":
			o.Class = v
		case "context":
			o.Context = v
		case "context_recipe":
			o.ContextRecipe = v
		case "trace":
			// Sender-side only: hev dispatch strips it before sending and
			// records it as the run's trace context, never on the Order.
			if _, _, err := ParseTraceRef(v); err != nil {
				return o, err
			}
		case "timeout":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 525600 {
				return o, fmt.Errorf("invalid timeout")
			}
			o.Timeout = n
		}
	}
	if o.Every != "" {
		if o.MaxLifetime == "" {
			o.MaxLifetime = "24h"
		}
		for _, v := range []string{o.Every, o.MaxLifetime} {
			if d, err := time.ParseDuration(v); err != nil || d <= 0 {
				return o, fmt.Errorf("invalid repeating duration: %s", v)
			}
		}
	} else if o.Until != "" || o.MaxLifetime != "" {
		return o, fmt.Errorf("until and max_lifetime require every")
	}
	if !ValidName(o.Name) {
		return o, fmt.Errorf("invalid name")
	}
	if o.Class != "" && !ValidLabel(o.Class) {
		return o, fmt.Errorf("invalid class")
	}
	if o.Repo == "" {
		return o, fmt.Errorf("repo required")
	}
	if o.Context != "" {
		if err := weave.CheckPath(o.Context); err != nil {
			return o, fmt.Errorf("invalid context recipe path: %w", err)
		}
	}
	if o.ContextRecipe != "" {
		if o.Context != "" {
			return o, fmt.Errorf("context and context_recipe are exclusive")
		}
		if _, err := decodeContextRecipe(o.ContextRecipe); err != nil {
			return o, err
		}
	}
	if o.Harness != "codex" && o.Harness != "claude" {
		return o, fmt.Errorf("invalid harness")
	}
	if o.Validation != "none" && o.Validation != "browser" {
		return o, fmt.Errorf("invalid validation")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9/_.-]*$`).MatchString(o.Base) || strings.Contains(o.Base, "..") {
		return o, fmt.Errorf("invalid base")
	}
	return o, nil
}
func CheckCap(n, cap int) error {
	if n >= cap {
		return fmt.Errorf("host full (%d/%d)", n, cap)
	}
	return nil
}
