package intake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type EmailConfig struct {
	Mode, URL, Require, Team, Status, Label, Host string
	TokenEnv                                      string `toml:"token_env"`
	MaxAttachmentBytes                            int64  `toml:"max_attachment_bytes"`
	Allow                                         []string
	TriageTemplate                                string `toml:"triage_template"`
}
type SourceCounts struct{ Admitted, Skipped, NeedsReconcile int }
type MailRecord struct {
	Phase, Authorizer string
	AttachmentPolls   int
	Issue             Issue
}
type SourceState struct {
	MentionSince               time.Time
	MentionCursor              time.Time
	EmailCursor                string
	EmailSince                 time.Time
	Mentions                   map[string]bool
	Mail                       map[string]*MailRecord
	EmailIssues                map[string]bool
	Suppressed                 map[string]bool
	MentionCounts, EmailCounts SourceCounts
}
type MentionComment struct {
	ID, URL   string
	CreatedAt time.Time
	User      *Actor
	BotActor  *struct{ ID string }
	Issue     Issue
}

const commentFields = `id url createdAt user { id } botActor { id } issue { id identifier url title branchName team { key } state { id name } }`
const mentionsQuery = `query IntakeMentions($after: String, $filter: NotificationFilter!) {
 viewer { id }
 notifications(first: 50, after: $after, filter: $filter) { nodes { type ... on IssueNotification { comment { ` + commentFields + ` } } } pageInfo { hasNextPage endCursor } }
}`
const mentionFallbackQuery = `query IntakeMentionFallback($after: String, $filter: CommentFilter!) { comments(first: 50, after: $after, filter: $filter) { nodes { ` + commentFields + ` } pageInfo { hasNextPage endCursor } } }`
const mentionRenderQuery = `query IntakeRenderComment($id: String!) { comment(id: $id) { ` + commentFields + ` } }`

func sourceLease(i Issue, action, id string) string {
	h := sha256.Sum256([]byte(id))
	return strings.ToLower(i.Identifier + "-" + action + "-" + hex.EncodeToString(h[:])[:6])
}
func (p *Poller) allRules() []Rule {
	return append(append([]Rule{}, p.cfg.Status...), p.cfg.Mention...)
}
func (p *Poller) initSources() {
	s := &p.state.Sources
	if s.MentionSince.IsZero() && len(p.cfg.Mention) > 0 {
		s.MentionSince = p.now()
		s.MentionCursor = p.now()
	}
	if s.EmailSince.IsZero() && p.cfg.Email.URL != "" {
		s.EmailSince = p.now()
	}
	if s.Mentions == nil {
		s.Mentions = map[string]bool{}
	}
	if s.Mail == nil {
		s.Mail = map[string]*MailRecord{}
	}
	if s.EmailIssues == nil {
		s.EmailIssues = map[string]bool{}
	}
	if s.Suppressed == nil {
		s.Suppressed = map[string]bool{}
	}
}
func renderMention(c MentionComment, r Rule) (string, error) {
	template, err := os.ReadFile(r.Template)
	if err != nil {
		return "", fmt.Errorf("read mention template: %w", err)
	}
	allowed := map[string]bool{"name": true, "identifier": true, "url": true, "title": true, "comment_id": true, "comment_url": true, "author": true, "branch": true}
	for _, match := range regexp.MustCompile(`\{\{([a-z_]+)\}\}`).FindAllStringSubmatch(string(template), -1) {
		if !allowed[match[1]] {
			return "", fmt.Errorf("unsupported mention placeholder %s", match[1])
		}
	}
	name := sourceLease(c.Issue, r.Action, c.ID)
	author := ""
	if c.User != nil {
		author = c.User.ID
	}
	if err := validateValues(c.Issue.Identifier, c.Issue.URL, c.Issue.Title, c.ID, c.URL, author, c.Issue.BranchName); err != nil {
		return "", err
	}
	return renderValues(r, name, strings.NewReplacer("{{name}}", name, "{{identifier}}", c.Issue.Identifier, "{{url}}", c.Issue.URL, "{{title}}", c.Issue.Title, "{{comment_id}}", c.ID, "{{comment_url}}", c.URL, "{{author}}", author, "{{branch}}", c.Issue.BranchName))
}
func (p *Poller) mentionComments(ctx context.Context, bot string) ([]MentionComment, error) {
	var result []MentionComment
	var after *string
	fallback := false
	profileURL := ""
	for {
		var page Page
		if !fallback {
			var d struct {
				Viewer        struct{ ID string }
				Notifications struct {
					Nodes []struct {
						Type    string
						Comment *MentionComment
					}
					PageInfo Page
				}
			}
			err := p.gql(ctx, mentionsQuery, map[string]any{"after": after, "filter": map[string]any{"type": map[string]string{"eq": "issueCommentMention"}, "createdAt": map[string]string{"gte": p.state.Sources.MentionCursor.UTC().Format(time.RFC3339Nano)}}}, &d)
			if err != nil {
				if errors.Is(err, errGraphQL) && after == nil {
					var viewer struct{ Viewer struct{ ID, URL string } }
					if e := p.gql(ctx, `query IntakeMentionViewer { viewer { id url } }`, nil, &viewer); e != nil {
						return nil, e
					}
					if viewer.Viewer.ID != bot || viewer.Viewer.URL == "" {
						return nil, fmt.Errorf("mention fallback requires bot profile URL")
					}
					profileURL = viewer.Viewer.URL
					fallback = true
					continue
				}
				return nil, err
			}
			if d.Viewer.ID != bot {
				return nil, fmt.Errorf("mention key must belong to configured bot")
			}
			for _, n := range d.Notifications.Nodes {
				if n.Type == "issueCommentMention" && n.Comment != nil {
					result = append(result, *n.Comment)
				}
			}
			page = d.Notifications.PageInfo
		} else {
			var d struct {
				Comments struct {
					Nodes    []MentionComment
					PageInfo Page
				}
			}
			// Filtering happens at Linear; no comment text enters the poller.
			err := p.gql(ctx, mentionFallbackQuery, map[string]any{"after": after, "filter": map[string]any{"issue": map[string]any{"team": map[string]any{"key": map[string]string{"eq": p.cfg.Linear.Team}}}, "createdAt": map[string]string{"gte": p.state.Sources.MentionCursor.UTC().Format(time.RFC3339Nano)}, "or": profileMentionFilters(profileURL)}}, &d)
			if err != nil {
				return nil, err
			}
			result = append(result, d.Comments.Nodes...)
			page = d.Comments.PageInfo
		}
		if !page.HasNextPage {
			return result, nil
		}
		if page.EndCursor == "" || after != nil && *after == page.EndCursor {
			return nil, fmt.Errorf("invalid mention pagination")
		}
		c := page.EndCursor
		after = &c
	}
}

// Match a complete canonical profile URL, including common Markdown/plain URL
// terminators. A prefix match could admit mentions of a similarly named user.
func profileMentionFilters(profile string) []any {
	filters := []any{map[string]any{"body": map[string]string{"endsWith": profile}}}
	for _, suffix := range []string{")", ">", " ", "\n", "\t", ",", ".", "!", "?", ":", ";"} {
		filters = append(filters, map[string]any{"body": map[string]string{"contains": profile + suffix}})
	}
	return filters
}

func (p *Poller) pollSources(ctx context.Context, bot string, names map[string]bool, running *int) error {
	for _, r := range p.cfg.Mention {
		if r.Mode == "off" {
			continue
		}
		until := p.now()
		comments, err := p.mentionSource(bot).Fetch(ctx)
		if err != nil {
			return err
		}
		blocked := false
	mentionLoop:
		for _, c := range comments {
			s := &p.state.Sources
			if !c.CreatedAt.After(s.MentionSince) || s.Mentions[c.ID] {
				continue
			}
			if c.CreatedAt.After(until) {
				continue
			}
			d := Decision{Identifier: c.Issue.Identifier, Action: r.Action, Name: sourceLease(c.Issue, r.Action, c.ID), Decision: "skipped"}
			var member *Actor
			if c.User != nil {
				member, err = p.member(ctx, c.User.ID)
				if err != nil {
					return err
				}
			}
			switch {
			case c.ID == "" || c.Issue.Team.Key != p.cfg.Linear.Team:
				d.Reason = "mention outside configured team"
			case c.BotActor != nil || c.User == nil || nonHuman(c.User.ID, bot, member) || member == nil || member.ID != c.User.ID || !p.allowed(History{Actor: member}, bot):
				d.Reason = "mention has no allowed human authorizer"
			case p.admitted[d.Name] || names[d.Name]:
				d.Reason = "dispatch record exists"
			case p.state.Pending[d.Name] != "" || p.state.NeedsReconcile[d.Name] != "":
				if err := p.sourceLog(d, "mention", "dispatch outcome requires reconciliation"); err != nil {
					return err
				}
				blocked = true
				continue mentionLoop
			case p.state.TicketFailures[d.Name].Skip > 0:
				f := p.state.TicketFailures[d.Name]
				f.Skip--
				p.state.TicketFailures[d.Name] = f
				if err := p.sourceLog(d, "mention", "ticket failure backoff"); err != nil {
					return err
				}
				blocked = true
				continue mentionLoop
			case p.day().Dispatches >= p.cfg.DailyCap:
				return p.sourceLog(d, "mention", "daily cap")
			case *running >= p.cfg.Limits.CodeJobs:
				if err := p.sourceLog(d, "mention", "code-job limit"); err != nil {
					return err
				}
				blocked = true
				continue mentionLoop
			case r.Mode == "dry-run":
				d.Decision = "would-dispatch"
			default:
				rendered, e := renderMention(c, r)
				if e != nil {
					return fmt.Errorf("mention template validation failed")
				}
				if e = p.launchRendered(ctx, rendered, r, d.Name); e != nil {
					d.Reason = "dispatch failed; needs reconciliation"
					if p.state.Pending[d.Name] != "" {
						if e = p.reservation(d.Name, r.Concurrency, "needs-reconcile"); e != nil {
							return e
						}
						*running += 1 // A lost response may have started a code job.
					} else {
						failure := p.state.TicketFailures[d.Name]
						failure.Count++
						failure.Skip = 1 << min(failure.Count-1, 6)
						p.state.TicketFailures[d.Name] = failure
						if err := p.sourceLog(d, "mention", "dispatch rejected or local preparation failed; retry after backoff"); err != nil {
							return err
						}
						blocked = true
						continue mentionLoop
					}
					s.MentionCounts.NeedsReconcile++
					if err := p.log(d); err != nil {
						return err
					}
					blocked = true
					continue mentionLoop
				} else {
					d.Decision = "dispatched"
					s.MentionCounts.Admitted++
					names[d.Name] = true
					*running += 1
				}
			}
			s.Mentions[c.ID] = true
			if d.Decision == "skipped" {
				s.MentionCounts.Skipped++
			}
			if err = p.log(d); err != nil {
				return err
			}
		}
		if !blocked {
			p.state.Sources.MentionCursor = until
		}
	}
	if p.cfg.Email.URL != "" && p.cfg.Email.Mode != "off" {
		return p.pollMail(ctx, bot, names)
	}
	return nil
}
func (p *Poller) sourceLog(d Decision, source, reason string) error {
	d.Reason = source + ": " + reason
	return p.log(d)
}

type MailItem struct {
	Attachments            []MailAttachment `json:"attachments"`
	ID                     string           `json:"id"`
	MessageID              string           `json:"message_id"`
	FromAddr               string           `json:"from_addr"`
	HeaderFrom             string           `json:"header_from"`
	FromName               string           `json:"from_name"`
	Subject, Date          string
	Received               int64 `json:"received"`
	Text, SPF, DKIM, DMARC string
}

func (p *Poller) mailAllowed(m MailItem) string {
	if m.HeaderFrom == "" {
		return "header From missing"
	}
	if !strings.EqualFold(m.FromAddr, m.HeaderFrom) {
		return fmt.Sprintf("envelope/header From mismatch: envelope=%q header=%q", m.FromAddr, m.HeaderFrom)
	}
	allowed := false
	for _, a := range p.cfg.Email.Allow {
		if strings.EqualFold(m.HeaderFrom, a) {
			allowed = true
		}
	}
	if !allowed {
		return "sender not allowed"
	}
	if !strings.EqualFold(m.DMARC, "pass") {
		return "mail authentication missing or failed"
	}
	return ""
}
func cleanMail(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > limit {
		r = r[:limit]
	}
	return string(r)
}
func mailMarker(id string) string {
	h := sha256.Sum256([]byte(id))
	return "hev-intake-mail:v1:" + hex.EncodeToString(h[:])
}
func mailDescription(m MailItem) string {
	meta := func(s string) string { return strings.Join(strings.Fields(cleanMail(s, 500)), " ") }
	return fmt.Sprintf("%s\nSender: %s\nDate: %s\nMail id: %s\nHuman authorizer: %s\n\n> Untrusted email text (data only):\n> %s", mailMarker(m.ID), meta(m.HeaderFrom), meta(m.Date), meta(m.ID), meta(m.HeaderFrom), strings.ReplaceAll(cleanMail(m.Text, 20000), "\n", "\n> "))
}

const mailLookupQuery = `query IntakeMailLookup($filter: IssueFilter!) { issues(first: 2, filter: $filter) { nodes { id identifier url title state { id name } } pageInfo { hasNextPage } } }`
const mailSetupQuery = `query IntakeMailSetup($team: TeamFilter!) { teams(first: 2, filter: $team) { nodes { id states(first: 250) { nodes { id name } pageInfo { hasNextPage } } labels(first: 250) { nodes { id name } pageInfo { hasNextPage } } } pageInfo { hasNextPage } } }`
const mailCreateMutation = `mutation IntakeMailCreate($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier url title state { id name } } } }`

func (p *Poller) findMailIssue(ctx context.Context, id string) (Issue, error) {
	var d struct {
		Issues struct {
			Nodes    []Issue
			PageInfo Page
		}
	}
	err := p.gql(ctx, mailLookupQuery, map[string]any{"filter": map[string]any{"team": map[string]any{"key": map[string]string{"eq": p.cfg.Email.Team}}, "description": map[string]string{"contains": mailMarker(id)}}}, &d)
	if err != nil {
		return Issue{}, err
	}
	if len(d.Issues.Nodes) > 1 || d.Issues.PageInfo.HasNextPage {
		return Issue{}, fmt.Errorf("duplicate mail marker requires reconciliation")
	}
	if len(d.Issues.Nodes) == 1 {
		return d.Issues.Nodes[0], nil
	}
	return Issue{}, nil
}
func (p *Poller) createMailIssue(ctx context.Context, m MailItem, bot string, attachments string) (Issue, error) {
	var d struct {
		Teams struct {
			Nodes []struct {
				ID             string
				States, Labels struct {
					Nodes    []StateRef
					PageInfo Page
				}
			}
			PageInfo Page
		}
	}
	if err := p.gql(ctx, mailSetupQuery, map[string]any{"team": map[string]any{"key": map[string]string{"eq": p.cfg.Email.Team}}}, &d); err != nil {
		return Issue{}, err
	}
	if len(d.Teams.Nodes) != 1 || d.Teams.PageInfo.HasNextPage {
		return Issue{}, fmt.Errorf("email team must resolve uniquely")
	}
	team := d.Teams.Nodes[0]
	state := ""
	labels := []string{}
	if team.States.PageInfo.HasNextPage || team.Labels.PageInfo.HasNextPage {
		return Issue{}, fmt.Errorf("email setup exceeds pagination limit")
	}
	for _, s := range team.States.Nodes {
		if s.Name == p.cfg.Email.Status {
			state = s.ID
		}
	}
	for _, l := range team.Labels.Nodes {
		if l.Name == p.cfg.Email.Label {
			labels = append(labels, l.ID)
		}
	}
	if state == "" {
		return Issue{}, fmt.Errorf("email Triage status missing")
	}
	title := cleanMail(strings.Join(strings.Fields(m.Subject), " "), 200)
	if title == "" {
		title = "Email intake"
	}
	input := map[string]any{"teamId": team.ID, "stateId": state, "assigneeId": bot, "title": title, "description": mailDescription(m) + attachments}
	if len(labels) > 0 {
		input["labelIds"] = labels
	}
	var created struct {
		IssueCreate struct {
			Success bool
			Issue   Issue
		}
	}
	err := p.gql(ctx, mailCreateMutation, map[string]any{"input": input}, &created)
	if err != nil {
		return Issue{}, err
	}
	if !created.IssueCreate.Success || created.IssueCreate.Issue.ID == "" {
		return Issue{}, fmt.Errorf("mail create result uncertain")
	}
	return created.IssueCreate.Issue, nil
}
func (p *Poller) pollMail(ctx context.Context, bot string, names map[string]bool) error {
	s := &p.state.Sources
	batch, err := p.mailSource().Fetch(ctx)
	if err != nil {
		return err
	}
	for _, m := range batch.Items {
		if m.ID == "" {
			return fmt.Errorf("mailbox item missing id")
		}
		record := s.Mail[m.ID]
		if record != nil && (record.Phase == "admitted" || record.Phase == "skipped" || record.Phase == "needs-reconcile" || record.Phase == "dry-run") {
			continue
		}
		d := Decision{Action: "triage", Decision: "skipped"}
		reason := p.mailAllowed(m)
		if m.Received <= s.EmailSince.UnixMilli() {
			reason = "mail predates intake start"
		}
		if reason != "" {
			s.Mail[m.ID] = &MailRecord{Phase: "skipped"}
			s.EmailCounts.Skipped++
			if err = p.sourceLog(d, "email", reason); err != nil {
				return err
			}
			continue
		}
		if p.day().Dispatches >= p.cfg.DailyCap {
			return p.sourceLog(d, "email", "daily cap")
		}
		if p.cfg.Email.Mode == "dry-run" {
			s.Mail[m.ID] = &MailRecord{Phase: "dry-run", Authorizer: m.HeaderFrom}
			d.Decision = "would-create-and-dispatch"
			if err = p.log(d); err != nil {
				return err
			}
			continue
		}
		fresh := record == nil
		if fresh {
			record = &MailRecord{Phase: "preparing", Authorizer: m.HeaderFrom}
			s.Mail[m.ID] = record
			if err = p.save(); err != nil {
				return err
			}
		}
		if record.Issue.ID == "" {
			// A persisted reservation means a previous create may have succeeded. Search before any retry.
			if !fresh && record.Phase != "preparing" {
				record.Issue, err = p.findMailIssue(ctx, m.ID)
				if err != nil {
					return err
				}
				if record.Issue.ID == "" {
					record.Phase = "needs-reconcile"
					s.EmailCounts.NeedsReconcile++
					if err = p.sourceLog(d, "email", "uncertain create has no visible marker; needs-reconcile"); err != nil {
						return err
					}
					continue
				}
			} else {
				record.AttachmentPolls++
				description, assets, retry, e := p.prepareMailAttachments(ctx, m)
				if e != nil {
					return e
				}
				if retry && record.AttachmentPolls < 3 {
					p.failureBackoff()
					if e = p.save(); e != nil {
						return e
					}
					return fmt.Errorf("email attachments deferred")
				}
				record.Phase = "reserved"
				if e = p.save(); e != nil {
					return e
				}
				record.Issue, err = p.createMailIssue(ctx, m, bot, description)
				if err != nil {
					// Leave the reservation for a later marker lookup, including when the shared budget is exhausted.
					_ = p.save()
					return fmt.Errorf("email create uncertain; reserved for marker reconciliation")
				}
				if e = p.save(); e != nil {
					return e
				}
				p.attachMailAssets(ctx, record.Issue.ID, assets)
			}
		}
		s.EmailIssues[record.Issue.ID] = true
		record.Phase = "created"
		if err = p.save(); err != nil {
			return err
		}
		r := p.emailRule()
		name := sourceLease(record.Issue, "triage", m.ID)
		d.Name = name
		d.Identifier = record.Issue.Identifier
		if names[name] || p.admitted[name] {
			record.Phase = "admitted"
			continue
		}
		if p.state.Pending[name] != "" || p.state.NeedsReconcile[name] != "" {
			record.Phase = "needs-reconcile"
			s.EmailCounts.NeedsReconcile++
			continue
		}
		if err = p.launch(ctx, record.Issue, r, name); err != nil {
			record.Phase = "needs-reconcile"
			s.EmailCounts.NeedsReconcile++
			d.Reason = "email dispatch failed; needs-reconcile"
			if err = p.reservation(name, r.Concurrency, "needs-reconcile"); err != nil {
				return err
			}
		} else {
			record.Phase = "admitted"
			s.EmailCounts.Admitted++
			d.Decision = "dispatched"
			names[name] = true
		}
		if err = p.log(d); err != nil {
			return err
		}
	}
	if batch.Next != "" {
		s.EmailCursor = batch.Next
	}
	return nil
}
func (p *Poller) emailRule() Rule {
	return Rule{Name: p.cfg.Email.Status, Action: "triage", Mode: p.cfg.Email.Mode, Template: p.cfg.Email.TriageTemplate, Host: p.cfg.Email.Host, Concurrency: "read"}
}

type mailBatch struct {
	Items []MailItem
	Next  string
}

func (p *Poller) mailSource() Source[mailBatch] { return sourceFunc[mailBatch](p.fetchMail) }
func (p *Poller) fetchMail(ctx context.Context) (batch mailBatch, err error) {
	u, err := url.Parse(p.cfg.Email.URL)
	if err != nil {
		return batch, fmt.Errorf("invalid mailbox URL")
	}
	q := u.Query()
	if p.state.Sources.EmailCursor != "" {
		q.Set("after", p.state.Sources.EmailCursor)
	} else {
		q.Del("after")
	}
	q.Set("limit", "50")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return batch, fmt.Errorf("invalid mailbox request")
	}
	token := os.Getenv(p.cfg.Email.TokenEnv)
	if token == "" {
		return batch, fmt.Errorf("mailbox token environment variable empty")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := p.attachmentHTTP(req)
	if err != nil {
		p.failureBackoff()
		return batch, fmt.Errorf("mailbox transport failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		p.failureBackoff()
		return batch, fmt.Errorf("mailbox HTTP %d", res.StatusCode)
	}

	if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&batch); err != nil {
		p.failureBackoff()
		return batch, fmt.Errorf("invalid mailbox response")
	}
	if len(batch.Items) > 50 {
		return batch, fmt.Errorf("mailbox page exceeds limit")
	}

	return batch, nil
}
