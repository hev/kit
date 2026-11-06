package intake

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// renderOrder is the single preparation path for previews and live dispatch.
func renderOrder(i Issue, r Rule, name string) (string, error) {
	if r.Batch {
		return renderBatch([]Issue{i}, r, name)
	}
	if err := validateValues(i.Identifier, i.URL, i.Title, i.State.Name); err != nil {
		return "", err
	}
	return renderValues(r, name, strings.NewReplacer("{{identifier}}", i.Identifier, "{{url}}", i.URL, "{{title}}", i.Title, "{{name}}", name, "{{status}}", i.State.Name))
}
func validateValues(values ...string) error {
	for _, s := range values {
		if strings.ContainsAny(s, "\r\n\x00") {
			return fmt.Errorf("template value contains control characters")
		}
	}
	return nil
}
func renderValues(r Rule, name string, replacer *strings.Replacer) (string, error) {
	b, err := os.ReadFile(r.Template)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}
	forbidden := []string{"identifiers", "urls", "count"}
	if r.Batch {
		forbidden = []string{"identifier", "url", "title"}
	}
	for _, key := range forbidden {
		if strings.Contains(string(b), "{{"+key+"}}") {
			return "", fmt.Errorf("placeholder %s incompatible with batch=%v", key, r.Batch)
		}
	}
	rendered := replacer.Replace(strings.ReplaceAll(string(b), "\r\n", "\n"))
	if err := validateOrder(rendered, name); err != nil {
		return "", fmt.Errorf("template validation failed: %w", err)
	}
	return rendered, nil
}

const renderQuery = "query RenderIssue($id: String!) { issue(id: $id) { identifier url title state { name } } }"

// Deliberately do not call newPoller: preview owns only in-memory request accounting.
func previewClient(c Config, key string) *Poller {
	return &Poller{cfg: c, key: key, endpoint: endpoint, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now, state: State{Days: map[string]*Daily{}}}
}
func (p *Poller) renderIssue(ctx context.Context, ticket string) (Issue, error) {
	var d struct{ Issue *Issue }
	if err := p.gql(ctx, renderQuery, map[string]string{"id": ticket}, &d); err != nil {
		return Issue{}, err
	}
	if d.Issue == nil {
		return Issue{}, fmt.Errorf("Linear ticket not found")
	}
	return *d.Issue, nil
}

func runTemplate(args []string, out, diagnostics io.Writer) error {
	action := args[0]
	f := flag.NewFlagSet(action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	path := f.String("config", "", "TOML configuration")
	mention := f.Bool("mention", false, "render mention feedback")
	email := f.Bool("email", false, "render email triage")
	comment := f.String("comment", "", "Linear comment id")
	commentURL := f.String("comment-url", "", "offline comment URL")
	author := f.String("author", "", "offline comment author id")
	branch := f.String("branch", "", "offline issue branch")
	mailID := f.String("mail-id", "", "offline mail id for stable triage lease")
	var status, title *string
	var tickets, identifiers, urls stringFlags
	if action == "render" {
		status = f.String("status", "", "rule and offline ticket status")
		f.Var(&tickets, "ticket", "Linear ticket key (repeatable)")
		f.Var(&identifiers, "identifier", "offline identifier (repeatable)")
		f.Var(&urls, "url", "offline URL (repeatable)")
		title = f.String("title", "", "offline ticket title")
	}
	if err := f.Parse(args[1:]); err != nil {
		return usageError(err)
	}
	if *path == "" || f.NArg() != 0 {
		return usageError(fmt.Errorf("--config PATH required; unexpected positional arguments are not allowed"))
	}
	c, err := loadConfig(*path)
	if err != nil {
		return usageError(err)
	}
	if action == "check" {
		var failures []string
		checked := 0
		for _, r := range c.Status {
			if r.Mode == "off" || r.Template == "" {
				continue
			}
			checked++
			i := Issue{Identifier: "TEST-1", URL: "https://example.invalid/TEST-1", Title: "Synthetic template check", State: StateRef{Name: r.Name}}
			if _, err := renderOrder(i, r, previewName(i, r)); err != nil {
				failures = append(failures, fmt.Sprintf("rule %q: %v", r.Name, err))
			}
		}
		for _, r := range c.Mention {
			if r.Mode == "off" || r.Template == "" {
				continue
			}
			checked++
			sample := MentionComment{ID: "comment-preview", URL: "https://example.invalid/comment", User: &Actor{ID: "human"}, Issue: Issue{Identifier: "TEST-1", URL: "https://example.invalid/TEST-1", Title: "Synthetic check", BranchName: "existing-branch"}}
			if _, e := renderMention(sample, r); e != nil {
				failures = append(failures, fmt.Sprintf("mention: %v", e))
			}
		}
		if c.Email.URL != "" && c.Email.Mode != "off" {
			checked++
			p := previewClient(c, "")
			i := Issue{Identifier: "TEST-1", URL: "https://example.invalid/TEST-1", Title: "Synthetic email triage", State: StateRef{Name: c.Email.Status}}
			if _, e := renderOrder(i, p.emailRule(), sourceLease(i, "triage", "mail-preview")); e != nil {
				failures = append(failures, fmt.Sprintf("email: %v", e))
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("%s", strings.Join(failures, "\n"))
		}
		if checked == 0 {
			_, err := fmt.Fprintln(out, "No templated rules to check")
			return err
		}
		_, err := fmt.Fprintln(out, "Templates valid")
		return err
	}
	if *mention || *email {
		if *mention && *email {
			return usageError(fmt.Errorf("choose --mention or --email"))
		}
		if len(tickets) > 0 || len(identifiers) > 1 || len(urls) > 1 {
			return usageError(fmt.Errorf("source preview requires one offline issue or an online --comment"))
		}
		var rendered string
		if *mention {
			if len(c.Mention) != 1 || *comment == "" {
				return usageError(fmt.Errorf("mention rule and --comment ID required"))
			}
			var cm MentionComment
			if len(identifiers) == 0 && len(urls) == 0 && *title == "" && *commentURL == "" && *author == "" && *branch == "" {
				key := os.Getenv(c.Linear.APIKeyEnv)
				if key == "" {
					return usageError(fmt.Errorf("API key environment variable is empty"))
				}
				var d struct{ Comment *MentionComment }
				if err := previewClient(c, key).gql(context.Background(), mentionRenderQuery, map[string]string{"id": *comment}, &d); err != nil {
					return err
				}
				if d.Comment == nil {
					return fmt.Errorf("Linear comment not found")
				}
				cm = *d.Comment
			} else {
				if len(identifiers) != 1 || len(urls) != 1 || *commentURL == "" || *author == "" || *branch == "" {
					return usageError(fmt.Errorf("offline mention requires --identifier --url --title --comment-url --author --branch"))
				}
				cm = MentionComment{ID: *comment, URL: *commentURL, User: &Actor{ID: *author}, Issue: Issue{Identifier: identifiers[0], URL: urls[0], Title: *title, BranchName: *branch}}
			}
			rendered, err = renderMention(cm, c.Mention[0])
		} else {
			if c.Email.URL == "" || *mailID == "" || len(identifiers) != 1 || len(urls) != 1 {
				return usageError(fmt.Errorf("email preview requires email configuration and --mail-id --identifier --url --title"))
			}
			i := Issue{Identifier: identifiers[0], URL: urls[0], Title: *title, State: StateRef{Name: c.Email.Status}}
			rendered, err = renderOrder(i, previewClient(c, "").emailRule(), sourceLease(i, "triage", *mailID))
		}
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, rendered)
		return err
	}
	if *status == "" {
		return usageError(fmt.Errorf("--status NAME required"))
	}
	var rule *Rule
	for j := range c.Status {
		if c.Status[j].Name == *status {
			rule = &c.Status[j]
			break
		}
	}
	if rule == nil {
		return usageError(fmt.Errorf("no rule for status %q", *status))
	}
	seen := map[string]bool{}
	f.Visit(func(v *flag.Flag) { seen[v.Name] = true })
	offline := seen["identifier"] || seen["url"] || seen["title"]
	if (len(tickets) > 0 && offline) || (len(tickets) == 0 && (len(identifiers) == 0 || len(identifiers) != len(urls) || (!rule.Batch && !seen["title"]))) {
		return usageError(fmt.Errorf("provide --ticket KEY or all of --identifier/--url/--title/--status"))
	}
	var issues []Issue
	if len(tickets) > 0 {
		key := os.Getenv(c.Linear.APIKeyEnv)
		if key == "" {
			return usageError(fmt.Errorf("API key environment variable is empty"))
		}
		client := previewClient(c, key)
		for _, ticket := range tickets {
			i, e := client.renderIssue(context.Background(), ticket)
			if e != nil {
				return usageError(e)
			}
			if i.State.Name != *status {
				fmt.Fprintf(diagnostics, "warning: selected rule %q differs from ticket status %q\n", *status, i.State.Name)
			}
			issues = append(issues, i)
		}
	} else {
		for j, id := range identifiers {
			issues = append(issues, Issue{Identifier: id, URL: urls[j], Title: *title, State: StateRef{Name: *status}})
		}
	}
	if !rule.Batch && len(issues) != 1 {
		return usageError(fmt.Errorf("single-ticket rule requires one ticket"))
	}
	var rendered string
	if rule.Batch {
		rendered, err = renderBatch(issues, *rule, batchName(issues, *rule, time.Time{}))
	} else {
		rendered, err = renderOrder(issues[0], *rule, previewName(issues[0], *rule))
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, rendered)
	return err
}

// A stable synthetic lease makes previews reproducible without reserving a live lease.
func previewName(i Issue, r Rule) string {
	return lease(i, r, time.Time{})
}

type stringFlags []string

func (s *stringFlags) String() string     { return strings.Join(*s, ",") }
func (s *stringFlags) Set(v string) error { *s = append(*s, v); return nil }
