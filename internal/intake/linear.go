package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const endpoint = "https://api.linear.app/graphql"

type StateRef struct{ ID, Name string }
type Issue struct {
	ID, Identifier, URL, Title, UpdatedAt, BranchName string
	Team                                              struct{ Key string }
	State                                             StateRef
}
type Page struct {
	HasNextPage bool
	EndCursor   string
}
type History struct {
	CreatedAt                                            time.Time
	ToStateID, FromStateID, ToAssigneeID, FromAssigneeID string
	BotActor                                             *struct{ ID string }
	Actor                                                *Actor
}

type Actor struct {
	ID      string
	Active  bool
	IsGuest bool `json:"guest"`
	IsApp   bool `json:"app"`
	Teams   struct {
		Nodes    []struct{ Key string }
		PageInfo Page
	}
}

const candidateQuery = `query Candidates($filter: IssueFilter!, $users: UserFilter!) {
 issues(first: 250, filter: $filter) { nodes { id identifier url title state { id name } updatedAt } pageInfo { hasNextPage endCursor } }
 users(first: 2, filter: $users) { nodes { id } pageInfo { hasNextPage } }
}`
const historyQuery = `query History($id: String!, $after: String, $team: TeamFilter!) {
 issue(id: $id) { createdAt botActor { id } creator { id guest app active teams(first: 1, filter: $team) { nodes { key } pageInfo { hasNextPage } } } history(first: 250, after: $after) { nodes {
 createdAt toStateId fromStateId toAssigneeId fromAssigneeId botActor { id }
 actor { id guest app active teams(first: 1, filter: $team) { nodes { key } pageInfo { hasNextPage } } }
 } pageInfo { hasNextPage endCursor } } }
}`

func (p *Poller) gql(ctx context.Context, q string, v any, out any) error {
	return p.gqlRequest(ctx, q, v, out, false)
}

// Comment retries isolate transport and server failures; quota limits stay shared.
func (p *Poller) gqlRequest(ctx context.Context, q string, v any, out any, comment bool) error {
	if p.now().Before(p.state.BackoffUntil) {
		return fmt.Errorf("%w: Linear backoff active", errNotSent)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w: context canceled", errNotSent)
	}
	body, marshalErr := json.Marshal(map[string]any{"query": q, "variables": v})
	if marshalErr != nil {
		return fmt.Errorf("%w: invalid request", errNotSent)
	}
	req, e := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if e != nil {
		return fmt.Errorf("%w: invalid request", errNotSent)
	}
	req.Header.Set("Authorization", p.key)
	req.Header.Set("Content-Type", "application/json")
	p.day().Requests++
	res, e := p.attachmentHTTP(req)
	if e != nil {
		if !comment {
			p.failureBackoff()
		}
		return fmt.Errorf("Linear transport failed")
	}
	defer res.Body.Close()
	p.headers(res.Header)
	raw, readErr := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	var envelope struct {
		Data   json.RawMessage
		Errors []struct{ Extensions struct{ Code string } }
	}
	_ = json.Unmarshal(raw, &envelope)
	limited := res.StatusCode == 429
	for _, err := range envelope.Errors {
		limited = limited || err.Extensions.Code == "RATELIMITED"
	}
	if limited {
		until := p.state.BackoffUntil
		for _, kind := range []string{"Requests", "Complexity"} {
			if n, err := strconv.ParseInt(res.Header.Get("X-RateLimit-"+kind+"-Reset"), 10, 64); err == nil {
				if t := time.UnixMilli(n); t.After(until) {
					until = t
				}
			}
		}
		if !until.After(p.now()) {
			until = p.now().Add(time.Hour)
		}
		if s := res.Header.Get("Retry-After"); s != "" {
			if n, e := strconv.Atoi(s); e == nil && n > 0 {
				until = p.now().Add(time.Duration(n) * time.Second)
			} else if t, e := http.ParseTime(s); e == nil && t.After(p.now()) {
				until = t
			}
		}
		if p.state.BackoffUntil.Before(until) {
			p.state.BackoffUntil = until
		}
		return fmt.Errorf("Linear rate limited; calls suspended until %s", p.state.BackoffUntil.Format(time.RFC3339))
	}
	if res.StatusCode >= 500 && !comment {
		p.failureBackoff()
	}
	if res.StatusCode != 200 {
		return linearHTTPError(res.StatusCode)
	}
	if readErr != nil {
		return fmt.Errorf("invalid Linear response")
	}
	if e = json.Unmarshal(raw, &envelope); e != nil {
		return fmt.Errorf("invalid Linear response")
	}
	if p.state.SkipNext && !strings.HasPrefix(strings.TrimSpace(q), "mutation") {
		return errQuota
	}
	if len(envelope.Errors) > 0 || len(envelope.Data) == 0 {
		return errGraphQL
	}
	return json.Unmarshal(envelope.Data, out)
}
func (p *Poller) headers(h http.Header) {
	for _, kind := range []string{"Requests", "Complexity"} {
		prefix := "X-RateLimit-" + kind + "-"
		remaining, e1 := strconv.ParseFloat(h.Get(prefix+"Remaining"), 64)
		limit, e2 := strconv.ParseFloat(h.Get(prefix+"Limit"), 64)
		if e1 == nil && e2 == nil && limit > 0 && 100*remaining/limit < p.cfg.Limits.MinRemaining {
			p.state.SkipNext = true
		}
		if remaining == 0 && e1 == nil {
			if n, e := strconv.ParseInt(h.Get(prefix+"Reset"), 10, 64); e == nil {
				t := time.UnixMilli(n)
				if t.After(p.state.BackoffUntil) {
					p.state.BackoffUntil = t
				}
			}
		}
	}
}
func userFilter(s string) map[string]any {
	if strings.Count(s, "-") == 4 {
		return map[string]any{"id": map[string]string{"eq": s}}
	}
	return map[string]any{"or": []any{map[string]any{"name": map[string]string{"eq": s}}, map[string]any{"displayName": map[string]string{"eq": s}}}}
}
func (p *Poller) candidates(ctx context.Context) ([]Issue, string, error) {
	names := []string{}
	for _, r := range p.cfg.Status {
		names = append(names, r.Name)
	}
	filter := map[string]any{"team": map[string]any{"key": map[string]string{"eq": p.cfg.Linear.Team}}, "assignee": userFilter(p.cfg.Linear.Assignee), "state": map[string]any{"name": map[string]any{"in": names}}}
	var data struct {
		Issues struct {
			Nodes    []Issue
			PageInfo Page
		}
		Users struct {
			Nodes    []struct{ ID string }
			PageInfo Page
		}
	}
	e := p.gql(ctx, candidateQuery, map[string]any{"filter": filter, "users": userFilter(p.cfg.Linear.Assignee)}, &data)
	if e != nil {
		return nil, "", e
	}
	if data.Issues.PageInfo.HasNextPage {
		return nil, "", fmt.Errorf("candidate query exceeds 250 issues; narrow configuration")
	}
	if len(data.Users.Nodes) != 1 || data.Users.PageInfo.HasNextPage {
		return nil, "", fmt.Errorf("assignee must resolve uniquely; configure its id")
	}
	return data.Issues.Nodes, data.Users.Nodes[0].ID, nil
}
func (p *Poller) history(ctx context.Context, i Issue, bot string) (Cached, error) {
	var all []History
	var createdAt time.Time
	var creator *Actor
	var creatorBot *struct{ ID string }
	var after *string
	for {
		var d struct {
			Issue *struct {
				CreatedAt time.Time
				Creator   *Actor
				BotActor  *struct{ ID string }
				History   struct {
					Nodes    []History
					PageInfo Page
				}
			}
		}
		e := p.gql(ctx, historyQuery, map[string]any{"id": i.ID, "after": after, "team": map[string]any{"key": map[string]string{"eq": p.cfg.Linear.AllowedActorTeam}}}, &d)
		if e != nil {
			return Cached{}, e
		}
		if d.Issue == nil {
			return Cached{}, fmt.Errorf("issue no longer available")
		}
		createdAt, creator, creatorBot = d.Issue.CreatedAt, d.Issue.Creator, d.Issue.BotActor
		all = append(all, d.Issue.History.Nodes...)
		page := d.Issue.History.PageInfo
		if !page.HasNextPage {
			break
		}
		if page.EndCursor == "" || (after != nil && page.EndCursor == *after) {
			return Cached{}, fmt.Errorf("invalid history pagination")
		}
		cursor := page.EndCursor
		after = &cursor
	}
	var entered, assigned time.Time
	allowed := false
	transitionAllowed := false
	var assignmentActor, transitionActor string
	var assignmentUser, transitionUser *Actor
	var assignmentNonHuman, transitionNonHuman bool
	for _, h := range all {
		if h.ToStateID == i.State.ID && (h.CreatedAt.After(entered) || (h.CreatedAt.Equal(entered) && !p.allowed(h, bot))) {
			entered = h.CreatedAt
			transitionUser = h.Actor
			transitionNonHuman = h.BotActor != nil
			transitionAllowed = p.allowed(h, bot)
			if h.Actor != nil {
				transitionActor = h.Actor.ID
			} else {
				transitionActor = ""
			}
		}
		if h.ToAssigneeID == bot && (h.CreatedAt.After(assigned) || (h.CreatedAt.Equal(assigned) && !p.allowed(h, bot))) {
			assigned = h.CreatedAt
			assignmentUser = h.Actor
			assignmentNonHuman = h.BotActor != nil
			if h.Actor != nil {
				assignmentActor = h.Actor.ID
			} else {
				assignmentActor = ""
			}
			allowed = p.allowed(h, bot)
		}
	}
	if entered.IsZero() && creator != nil {
		entered = createdAt
		transitionActor = creator.ID
		transitionUser = creator
		transitionNonHuman = creatorBot != nil
		transitionAllowed = p.allowed(History{Actor: creator, BotActor: creatorBot}, bot)
	}
	if assigned.IsZero() && creator != nil {
		assigned = createdAt
		assignmentActor = creator.ID
		assignmentUser = creator
		assignmentNonHuman = creatorBot != nil
		allowed = p.allowed(History{Actor: creator, BotActor: creatorBot}, bot)
	}
	c := Cached{Updated: i.UpdatedAt, State: i.State.ID, Entered: entered, Allowed: allowed, AssignmentActor: assignmentActor, TransitionActor: transitionActor, Checked: p.now(), AuthorizerBot: bot, HumanVersion: 3}
	if entered.IsZero() {
		c.Reason = "status entry missing"
	} else if assigned.IsZero() {
		c.Reason = "assignment history missing"
	} else if !allowed {
		c.Reason = "assigning actor outside allowed team"
	}
	if c.Reason == "" && !transitionAllowed {
		c.Reason = "transition actor outside allowed team"
		c.Allowed = false
	}
	if assignmentNonHuman || transitionNonHuman || nonHuman(assignmentActor, bot, assignmentUser) || nonHuman(transitionActor, bot, transitionUser) {
		c.NonHumanEvent = true
		c.Reason = "no human authorizer"
		c.Allowed = false
	}
	return c, nil
}

var errQuota = fmt.Errorf("Linear quota threshold reached")

func (p *Poller) allowed(h History, bot string) bool {
	if h.BotActor != nil || h.Actor == nil || !h.Actor.Active || h.Actor.IsGuest || nonHuman(h.Actor.ID, bot, h.Actor) {
		return false
	}
	for _, t := range h.Actor.Teams.Nodes {
		if t.Key == p.cfg.Linear.AllowedActorTeam {
			return true
		}
	}
	return false
}
func (p *Poller) failureBackoff() {
	p.state.Failures++
	n := min(p.state.Failures-1, 6)
	until := p.now().Add(min(5*time.Minute*time.Duration(1<<n), time.Hour))
	if until.After(p.state.BackoffUntil) {
		p.state.BackoffUntil = until
	}
}

const membershipQuery = `query Membership($id: String!, $team: TeamFilter!) { user(id: $id) { id guest app active teams(first: 1, filter: $team) { nodes { key } } } }`

func (p *Poller) member(ctx context.Context, id string) (*Actor, error) {
	if id == "" {
		return nil, nil
	}
	var d struct{ User *Actor }
	e := p.gql(ctx, membershipQuery, map[string]any{"id": id, "team": map[string]any{"key": map[string]string{"eq": p.cfg.Linear.AllowedActorTeam}}}, &d)
	return d.User, e
}

func nonHuman(id, bot string, a *Actor) bool {
	return id == bot || (a != nil && a.IsApp)
}

var errGraphQL = fmt.Errorf("Linear GraphQL error")

type linearHTTPError int

func (e linearHTTPError) Error() string { return fmt.Sprintf("Linear HTTP %d", int(e)) }

var errNotSent = fmt.Errorf("request not sent")
