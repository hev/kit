// Package notify posts code-composed messages to chat apps through named,
// node-local notifiers (RFC 0017). It never reads agent prose, never accepts a
// literal credential, and never puts a credential in an error.
package notify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Kinds built in this version. RFC 0017 phases 3 and 4 add discord, teams and
// command; until then the decoder refuses them by name.
const KindSlack = "slack"

const (
	AudienceInternal = "internal"
	AudienceExternal = "external"
)

// Notifier is one named destination. Token is a secret reference, never a
// value; Channels maps node-local labels to Slack channel IDs.
type Notifier struct {
	Name     string
	Kind     string
	Token    string
	Audience string
	Repos    []string
	Channels map[string]string
}

// External reports whether agent-written fields must be withheld.
func (n Notifier) External() bool { return n.Audience == AudienceExternal }

// AllowsRepo reports whether a run in repo may be posted. A notifier without
// repos allows every repo (RFC 0017 open question 7: recommended, not required).
func (n Notifier) AllowsRepo(repo string) bool {
	return len(n.Repos) == 0 || slices.Contains(n.Repos, repo)
}

// Config is a decoded notify.toml.
type Config struct {
	Default   string
	Notifiers map[string]Notifier
}

// Names returns notifier names in order.
func (c Config) Names() []string {
	out := make([]string, 0, len(c.Notifiers))
	for n := range c.Notifiers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Lookup resolves a notifier name, filling an empty name from default.
func (c Config) Lookup(name string) (Notifier, error) {
	if name == "" {
		name = c.Default
	}
	if name == "" {
		return Notifier{}, errors.New("no notifier named and no default in notify.toml")
	}
	n, ok := c.Notifiers[name]
	if !ok {
		return Notifier{}, fmt.Errorf("notifier %s not defined on this node", name)
	}
	return n, nil
}

// Path is notify.toml beside the target's own default dispatch config: the
// directory of HEV_DISPATCH_CONFIG when set, else ~/.config/hev. Like
// admission policy it is never chosen by a sender's --config or config_path.
func Path() string {
	if p := os.Getenv("HEV_DISPATCH_CONFIG"); p != "" {
		return filepath.Join(filepath.Dir(expand(p)), "notify.toml")
	}
	return expand("~/.config/hev/notify.toml")
}

func expand(p string) string {
	h, _ := os.UserHomeDir()
	if p == "~" {
		return h
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(h, p[2:])
	}
	return p
}

// ErrNoConfig means the node has no notify.toml, so it has no notifiers.
var ErrNoConfig = errors.New("no notify.toml on this node")

// Load reads and strictly decodes this node's notify.toml.
func Load() (Config, error) { return LoadFile(Path()) }

// LoadFile reads one file. A missing file returns ErrNoConfig.
func LoadFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, ErrNoConfig
	}
	if err != nil {
		return Config{}, fmt.Errorf("notify.toml: %w", err)
	}
	return Decode(b)
}

var (
	labelPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	slackChannelID   = regexp.MustCompile(`^[CGD][A-Z0-9]{6,20}$`)
	opReferenceShape = regexp.MustCompile(`^op://[^/\s{}]+/[^/\s{}]+/[^\s{}]+$`)
)

// ValidLabel matches dispatch's label rule (internal/dispatch/order.go).
func ValidLabel(s string) bool { return labelPattern.MatchString(s) }

// ValidReference accepts the credential forms phase 1 resolves: op://
// references through hev secrets. env: is RFC 0017 phase 4.
func ValidReference(s string) bool { return opReferenceShape.MatchString(s) }

// Decode is strict: unknown keys, unsupported kinds and anything that is not
// a secret reference in a credential field are errors. Errors name keys and
// notifiers, never a value from the file.
func Decode(b []byte) (Config, error) {
	var raw struct {
		Default   string `toml:"default"`
		Notifiers map[string]struct {
			Kind     string            `toml:"kind"`
			Token    string            `toml:"token"`
			Audience string            `toml:"audience"`
			Repos    []string          `toml:"repos"`
			Channels map[string]string `toml:"channels"`
		} `toml:"notifiers"`
	}
	md, err := toml.Decode(string(b), &raw)
	if err != nil {
		// Parser messages can quote the offending text, which may be a
		// credential pasted by mistake. Report only the position.
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Config{}, fmt.Errorf("notify.toml: syntax error on line %d", pe.Position.Line)
		}
		return Config{}, errors.New("notify.toml: invalid TOML or wrong value type")
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("notify.toml: unknown key %s", keys[0])
	}
	c := Config{Default: raw.Default, Notifiers: map[string]Notifier{}}
	names := make([]string, 0, len(raw.Notifiers))
	for name := range raw.Notifiers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r := raw.Notifiers[name]
		if !ValidLabel(name) {
			return Config{}, fmt.Errorf("notify.toml: notifier name %q must be lowercase letters, digits and -", name)
		}
		n := Notifier{Name: name, Kind: r.Kind, Token: r.Token, Audience: r.Audience, Repos: r.Repos, Channels: map[string]string{}}
		if n.Audience == "" {
			n.Audience = AudienceInternal
		}
		if n.Audience != AudienceInternal && n.Audience != AudienceExternal {
			return Config{}, fmt.Errorf("notify.toml: notifier %s: audience must be internal or external", name)
		}
		switch n.Kind {
		case KindSlack:
		case "":
			return Config{}, fmt.Errorf("notify.toml: notifier %s: kind is required", name)
		case "discord", "teams", "command":
			return Config{}, fmt.Errorf("notify.toml: notifier %s: kind %s is not supported by this version", name, n.Kind)
		default:
			return Config{}, fmt.Errorf("notify.toml: notifier %s: unknown kind", name)
		}
		if n.Token == "" {
			return Config{}, fmt.Errorf("notify.toml: notifier %s: token is required (a secret reference such as op://VAULT/ITEM/FIELD)", name)
		}
		if !ValidReference(n.Token) {
			// Never echo the value: it may be a pasted token.
			return Config{}, fmt.Errorf("notify.toml: notifier %s: token must be a secret reference (op://VAULT/ITEM/FIELD), not a value", name)
		}
		for _, repo := range n.Repos {
			if strings.TrimSpace(repo) == "" {
				return Config{}, fmt.Errorf("notify.toml: notifier %s: empty repo", name)
			}
		}
		if len(r.Channels) == 0 {
			return Config{}, fmt.Errorf("notify.toml: notifier %s: channels is required", name)
		}
		for label, id := range r.Channels {
			if !ValidLabel(label) {
				return Config{}, fmt.Errorf("notify.toml: notifier %s: channel label %q must be lowercase letters, digits and -", name, label)
			}
			// Slack advises channel IDs over names, and IDs survive renames
			// (https://docs.slack.dev/reference/methods/chat.postMessage).
			if !slackChannelID.MatchString(id) {
				return Config{}, fmt.Errorf("notify.toml: notifier %s: channel %s must be a Slack channel ID", name, label)
			}
			n.Channels[label] = id
		}
		c.Notifiers[name] = n
	}
	if c.Default != "" {
		if _, ok := c.Notifiers[c.Default]; !ok {
			return Config{}, fmt.Errorf("notify.toml: default names undefined notifier %s", c.Default)
		}
	}
	return c, nil
}
