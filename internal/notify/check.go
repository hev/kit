package notify

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Resolver returns a notifier's credential value. fresh asks the secrets
// provider to bypass its cache. Implementations must not put a reference or
// value in an error.
type Resolver func(ctx context.Context, n Notifier, fresh bool) (string, error)

// ChannelCheck is one label's membership result. It never carries the ID.
type ChannelCheck struct {
	Label  string `json:"label"`
	Status string `json:"status"` // member, not a member, membership unknown, channel not found, error
	Detail string `json:"detail,omitempty"`
}

// CheckResult is one notifier's health. It carries labels, the workspace
// name and classified errors, never a token, reference or channel ID.
type CheckResult struct {
	Notifier  string         `json:"notifier"`
	Kind      string         `json:"kind"`
	Audience  string         `json:"audience"`
	Auth      string         `json:"auth"` // ok, failed, unresolved
	Error     string         `json:"error,omitempty"`
	Workspace string         `json:"workspace,omitempty"`
	Channels  []ChannelCheck `json:"channels,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// OK reports whether the notifier can post to every label.
func (r CheckResult) OK() bool {
	if r.Auth != "ok" {
		return false
	}
	for _, c := range r.Channels {
		if c.Status != "member" && c.Status != "membership unknown" {
			return false
		}
	}
	return true
}

// String is the one-line form, for example
// "team: slack, auth ok (workspace Example), builds: member".
func (r CheckResult) String() string {
	var b strings.Builder
	switch r.Auth {
	case "ok":
		fmt.Fprintf(&b, "%s: %s, auth ok", r.Notifier, r.Kind)
		if r.Workspace != "" {
			fmt.Fprintf(&b, " (workspace %s)", r.Workspace)
		}
		for _, c := range r.Channels {
			fmt.Fprintf(&b, ", %s: %s", c.Label, c.Status)
			if c.Detail != "" {
				fmt.Fprintf(&b, " (%s)", c.Detail)
			}
		}
	case "unresolved":
		fmt.Fprintf(&b, "%s: credential not resolved (%s)", r.Notifier, r.Error)
	default:
		fmt.Fprintf(&b, "%s: auth failed (%s)", r.Notifier, r.Error)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "; warning: %s", w)
	}
	return b.String()
}

// Check probes each notifier without posting: resolve the credential, call
// auth.test, then conversations.info per label. only limits it to one name.
func Check(ctx context.Context, cfg Config, only string, resolve Resolver, slack func(token string) Slack) ([]CheckResult, error) {
	names := cfg.Names()
	if only != "" {
		if _, ok := cfg.Notifiers[only]; !ok {
			return nil, fmt.Errorf("notifier %s not defined on this node", only)
		}
		names = []string{only}
	}
	out := []CheckResult{}
	for _, name := range names {
		n := cfg.Notifiers[name]
		r := CheckResult{Notifier: name, Kind: n.Kind, Audience: n.Audience}
		if n.External() && len(n.Repos) == 0 {
			r.Warnings = append(r.Warnings, "external notifier has no repos list")
		}
		token, err := resolve(ctx, n, false)
		if err != nil {
			r.Auth, r.Error = "unresolved", err.Error()
			out = append(out, r)
			continue
		}
		client := slack(token)
		team, err := client.AuthTest(ctx)
		if err != nil {
			r.Auth, r.Error = "failed", codeOf(err)
			out = append(out, r)
			continue
		}
		r.Auth, r.Workspace = "ok", Clean(team)
		labels := make([]string, 0, len(n.Channels))
		for l := range n.Channels {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		for _, label := range labels {
			c := ChannelCheck{Label: label}
			member, err := client.Membership(ctx, n.Channels[label])
			switch {
			case err == nil && member:
				c.Status = "member"
			case err == nil:
				c.Status, c.Detail = "not a member", "invite the app to the channel"
			case IsCode(err, "missing_scope"):
				c.Status, c.Detail = "membership unknown", "add channels:read and groups:read to check"
			case IsCode(err, "channel_not_found"):
				c.Status = "channel not found"
			default:
				c.Status, c.Detail = "error", codeOf(err)
			}
			r.Channels = append(r.Channels, c)
		}
		out = append(out, r)
	}
	return out, nil
}

func codeOf(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return "error"
}
