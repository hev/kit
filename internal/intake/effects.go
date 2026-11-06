package intake

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// applyEffect runs only after the source's immutable human guard has succeeded.
// Persist before external side effects: uncertain outcomes require reconciliation.
func (p *Poller) applyEffect(ctx context.Context, i Issue, r Rule, name string) (string, string, error) {
	if p.state.Effects == nil {
		p.state.Effects = map[string]string{}
	}
	if phase := p.state.Effects[name]; phase != "" {
		return "skipped", "watch effect already " + phase, nil
	}
	if r.Mode == "dry-run" {
		return "would-" + r.Effect, "", nil
	}
	if p.day().Dispatches >= p.cfg.DailyCap {
		return "skipped", "daily cap", nil
	}
	if ctx.Err() != nil || (r.Effect == "comment" && (p.state.SkipNext || p.now().Before(p.state.BackoffUntil))) {
		return "skipped", "effect deferred before send", nil
	}
	body, err := renderOrder(i, r, name)
	if err != nil {
		return "failed", "effect template invalid", nil
	}
	p.state.Effects[name] = "pending"
	p.day().Dispatches++
	if err = p.save(); err != nil {
		return "", "", err
	}
	switch r.Effect {
	case "comment":
		var result struct{ CommentCreate struct{ Success bool } }
		err = p.gql(ctx, stoppedMutation, map[string]any{"input": map[string]any{"issueId": i.ID, "body": "hev-watch:v1 run=" + name + "\n" + body}}, &result)
		if err == nil && !result.CommentCreate.Success {
			err = fmt.Errorf("comment rejected")
		}
	case "notify":
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, r.NotifyURL, strings.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
			req.Header.Set("Idempotency-Key", name)
			client := &http.Client{Timeout: p.client.Timeout, Transport: p.client.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			var res *http.Response
			res, err = client.Do(req)
			if err == nil {
				res.Body.Close()
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					err = fmt.Errorf("notification rejected")
				}
			}
		}
	}
	if err != nil {
		if errors.Is(err, errNotSent) {
			delete(p.state.Effects, name)
			p.day().Dispatches--
			return "skipped", "effect deferred before send", p.save()
		}
		p.state.Effects[name] = "needs-reconcile"
		p.state.NeedsReconcile[name] = "read"
		if e := p.save(); e != nil {
			return "", "", e
		}
		return "failed", "watch effect uncertain; needs reconciliation", nil
	}
	p.state.Effects[name] = "completed"
	decision := "commented"
	if r.Effect == "notify" {
		decision = "notified"
	}
	return decision, "", p.save()
}
