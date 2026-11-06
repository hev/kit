package intake

import (
	"fmt"
	"net/url"
)

// Watch binds source-owned guards to deterministic clauses. Normalization keeps
// the original poller as the shared execution engine for both configuration forms.
type Watch struct {
	Name   string
	Source string
	Email  EmailConfig
	When   []When
}
type When struct {
	Event    string
	Statuses []string
	Rule
}

func normalizeWatches(c *Config) error {
	if len(c.Watches) == 0 {
		return nil
	}
	if len(c.Status) > 0 || len(c.Mention) > 0 || c.Email.URL != "" {
		return fmt.Errorf("use watch clauses or legacy source rules, not both")
	}
	for _, w := range c.Watches {
		if w.Name == "" || len(w.When) == 0 {
			return fmt.Errorf("watch requires name and when clauses")
		}
		for _, clause := range w.When {
			r := clause.Rule
			if r.Effect == "" {
				r.Effect = "dispatch"
			}
			if r.Effect != "dispatch" && r.Effect != "comment" && r.Effect != "notify" {
				return fmt.Errorf("unsupported watch effect")
			}
			if r.Effect != "dispatch" {
				if r.Batch {
					return fmt.Errorf("comment/notify batching is not supported")
				}
				r.Concurrency = "read"
				if r.Template == "" {
					return fmt.Errorf("watch effect requires a template")
				}
				if r.Effect == "notify" {
					u, e := url.Parse(r.NotifyURL)
					if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
						return fmt.Errorf("notify requires HTTPS notify_url")
					}
				}
			}
			switch w.Source {
			case "linear":
				switch clause.Event {
				case "status-becomes":
					if len(clause.Statuses) == 0 {
						return fmt.Errorf("status clause requires statuses")
					}
					for _, status := range clause.Statuses {
						rule := r
						rule.Name = status
						c.Status = append(c.Status, rule)
					}
				case "mentioned":
					if r.Effect != "dispatch" {
						return fmt.Errorf("mentioned currently supports dispatch")
					}
					c.Mention = append(c.Mention, r)
				default:
					return fmt.Errorf("unsupported Linear when event")
				}
			case "email":
				if clause.Event != "email-arrives" || r.Effect != "dispatch" || c.Email.URL != "" {
					return fmt.Errorf("email supports one triage dispatch clause")
				}
				c.Email = w.Email
				c.Email.TriageTemplate = r.Template
				c.Email.Host = r.Host
				c.Email.Mode = r.Mode
			default:
				return fmt.Errorf("unsupported watch source %q", w.Source)
			}
		}
	}
	return nil
}
func legacyWatches(c Config) []Watch {
	var watches []Watch
	if len(c.Status)+len(c.Mention) > 0 {
		w := Watch{Name: "legacy-linear", Source: "linear"}
		for _, r := range c.Status {
			r.Effect = "dispatch"
			w.When = append(w.When, When{Event: "status-becomes", Statuses: []string{r.Name}, Rule: r})
		}
		for _, r := range c.Mention {
			r.Effect = "dispatch"
			w.When = append(w.When, When{Event: "mentioned", Rule: r})
		}
		watches = append(watches, w)
	}
	if c.Email.URL != "" {
		watches = append(watches, Watch{Name: "legacy-email", Source: "email", Email: c.Email, When: []When{{Event: "email-arrives", Rule: Rule{Effect: "dispatch", Action: "triage", Template: c.Email.TriageTemplate, Host: c.Email.Host, Mode: c.Email.Mode, Concurrency: "read"}}}})
	}
	return watches
}

func validateEffect(r Rule) error {
	if r.Effect == "" || r.Effect == "dispatch" {
		return nil
	}
	if r.Effect != "comment" && r.Effect != "notify" {
		return fmt.Errorf("unsupported watch effect")
	}
	if r.Batch || r.Concurrency != "read" || r.Template == "" {
		return fmt.Errorf("comment/notify require unbatched read concurrency and a template")
	}
	if r.Effect == "notify" {
		u, e := url.Parse(r.NotifyURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("notify requires HTTPS notify_url")
		}
	}
	return nil
}
