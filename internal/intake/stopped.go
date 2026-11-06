package intake

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CommentRetry struct {
	Attempts  int
	NextRetry time.Time
}

// CommentRun is saved before dispatch so even a lost dispatch response can
// later be reconciled from inventory. Outcomes and retries are per ticket.
type CommentRun struct {
	Retries  map[string]CommentRetry
	Rule     Rule
	At       time.Time
	Tickets  []Issue
	Outcomes map[string]string
	Terminal bool
	Exit     *int
}

func (p *Poller) trackRun(name string, r Rule, tickets []Issue) error {
	if r.MarkerPrefix == "" {
		return nil
	}
	if p.state.CommentRuns == nil {
		p.state.CommentRuns = map[string]*CommentRun{}
	}
	if p.state.CommentRuns[name] == nil {
		p.state.CommentRuns[name] = &CommentRun{Rule: r, At: p.now(), Tickets: tickets, Outcomes: map[string]string{}}
	}
	return p.save()
}

const stoppedQuery = `query StoppedTicket($id: String!, $after: String) {
 viewer { id }
 issue(id: $id) { assignee { id } comments(first: 250, after: $after) {
 nodes { body createdAt user { id } } pageInfo { hasNextPage endCursor }
 } }
}`
const stoppedMutation = `mutation StoppedComment($input: CommentCreateInput!) { commentCreate(input: $input) { success } }`

func (p *Poller) stoppedOutcome(ctx context.Context, bot, name string, run *CommentRun, i Issue) (string, error) {
	var after *string
	for {
		var d struct {
			Viewer struct{ ID string }
			Issue  *struct {
				Assignee *struct{ ID string }
				Comments struct {
					Nodes []struct {
						Body      string
						CreatedAt time.Time
						User      *struct{ ID string }
					}
					PageInfo Page
				}
			}
		}
		if err := p.gqlRequest(ctx, stoppedQuery, map[string]any{"id": i.ID, "after": after}, &d, true); err != nil {
			return "", err
		}
		if d.Issue == nil || d.Issue.Assignee == nil || d.Issue.Assignee.ID != bot {
			return "reassigned", nil
		}
		if d.Viewer.ID != bot {
			return "", fmt.Errorf("Linear viewer differs from bot")
		}
		for _, c := range d.Issue.Comments.Nodes {
			if c.User == nil || c.User.ID != bot {
				continue
			}
			for _, line := range strings.Split(c.Body, "\n") {
				if line == "hev-intake:v1 phase=stopped run="+name {
					return "already-commented", nil
				}
			}
			if c.CreatedAt.After(run.At) && strings.Contains(c.Body, run.Rule.MarkerPrefix) {
				return "marker-found", nil
			}
		}
		page := d.Issue.Comments.PageInfo
		if !page.HasNextPage {
			break
		}
		if page.EndCursor == "" || (after != nil && *after == page.EndCursor) {
			return "", fmt.Errorf("invalid comment pagination")
		}
		cursor := page.EndCursor
		after = &cursor
	}
	mode := run.Rule.Mode
	for _, rule := range p.cfg.Status {
		if rule.Name == run.Rule.Name {
			mode = rule.Mode
			break
		}
	}
	if mode == "off" {
		return "off", nil
	}
	if mode == "dry-run" {
		return "would-comment", nil
	}
	exit := "unknown"
	if run.Exit != nil {
		exit = fmt.Sprint(*run.Exit)
	}
	body := fmt.Sprintf("hev-intake:v1 phase=stopped run=%s\nhevbot's %s run stopped without updating this ticket (exit %s).\nNothing was changed. To retry, move the ticket out of %s and back.", name, run.Rule.Action, exit, run.Rule.Name)
	var d struct{ CommentCreate struct{ Success bool } }
	if err := p.gqlRequest(ctx, stoppedMutation, map[string]any{"input": map[string]string{"issueId": i.ID, "body": body}}, &d, true); err != nil {
		return "", err
	}
	if !d.CommentCreate.Success {
		return "", fmt.Errorf("comment rejected")
	}
	return "commented", nil
}

func (p *Poller) checkStopped(ctx context.Context, bot string) {
	for name, run := range p.state.CommentRuns {
		if !run.Terminal {
			continue
		}
		for _, i := range run.Tickets {
			if outcome := run.Outcomes[i.ID]; outcome != "" && outcome != "failed" {
				continue
			}
			if ctx.Err() != nil || p.now().Before(p.state.BackoffUntil) || p.state.SkipNext {
				return
			}
			if p.now().Before(run.Retries[i.ID].NextRetry) {
				continue
			}
			if run.Outcomes == nil {
				run.Outcomes = map[string]string{}
			}
			if run.Retries == nil {
				run.Retries = map[string]CommentRetry{}
			}
			outcome, err := p.stoppedOutcome(ctx, bot, name, run, i)
			if err != nil {
				outcome = "failed"
				retry := run.Retries[i.ID]
				retry.Attempts++
				retry.NextRetry = p.now().Add(min(5*time.Minute*time.Duration(1<<min(retry.Attempts-1, 6)), time.Hour))
				run.Retries[i.ID] = retry
			} else {
				delete(run.Retries, i.ID)
			}
			run.Outcomes[i.ID] = outcome
			// Errors and worker output deliberately never enter ticket comments or logs.
			_ = p.log(Decision{Name: name, Identifier: i.Identifier, Action: run.Rule.Action, Decision: outcome})
		}
	}
}
