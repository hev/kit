package intake

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func batchKeys(issues []Issue) []string {
	keys := make([]string, len(issues))
	for j, i := range issues {
		keys[j] = i.Identifier
	}
	sort.Strings(keys)
	return keys
}
func batchName(issues []Issue, r Rule, now time.Time) string {
	h := sha256.Sum256([]byte(strings.Join(batchKeys(issues), "\n")))
	return fmt.Sprintf("%s-%s-%x", r.Action, now.UTC().Format("20060102"), h[:3])
}
func (p *Poller) dispatchBatch(ctx context.Context, issues []Issue, r Rule, names map[string]bool, running *int) error {
	keys := batchKeys(issues)
	d := Decision{Status: r.Name, Action: r.Action, Name: batchName(issues, r, p.now()), Keys: keys, Decision: "skipped"}
	for _, run := range p.state.Batches {
		if run.Rule != r.Name {
			continue
		}
		if p.active[run.Name] || p.state.Pending[run.Name] != "" || p.state.NeedsReconcile[run.Name] != "" {
			d.Reason = "batch active or requires reconciliation"
			return p.log(d)
		}
		if run.Keys == strings.Join(keys, "\n") && p.now().Sub(run.At) < r.cooldown() {
			d.Reason = "batch cooldown"
			return p.log(d)
		}
	}
	if names[d.Name] || p.admitted[d.Name] {
		d.Reason = "dispatch record exists"
		return p.log(d)
	}
	switch {
	case r.Mode == "dry-run":
		d.Decision = "would-dispatch"
	case p.day().Dispatches >= p.cfg.DailyCap:
		d.Reason = "daily cap"
	case r.Concurrency == "code" && *running >= p.cfg.Limits.CodeJobs:
		d.Reason = "code-job limit"
	default:
		rendered, err := renderBatch(issues, r, d.Name)
		if err != nil {
			d.Decision = "failed"
			d.Reason = "template validation failed"
			return p.log(d)
		}
		// Persist the rule/name association before reservation, including uncertain starts.
		p.state.Batches = append(p.state.Batches, BatchRun{r.Name, d.Name, strings.Join(keys, "\n"), p.now()})
		if err = p.save(); err != nil {
			return err
		}
		if err = p.trackRun(d.Name, r, issues); err != nil {
			return err
		}
		if err = p.launchRendered(ctx, rendered, r, d.Name); err != nil {
			if p.state.Pending[d.Name] == "" {
				p.state.Batches = p.state.Batches[:len(p.state.Batches)-1]
			}
			if !errors.Is(err, errHostFull) && ctx.Err() == nil {
				for _, i := range issues {
					name := lease(i, r, p.state.Cache[i.ID].Entered)
					failure := p.state.TicketFailures[name]
					failure.Count++
					failure.Skip = 1 << min(failure.Count-1, 6)
					p.state.TicketFailures[name] = failure
				}
			}
			if ctx.Err() != nil && p.state.Pending[d.Name] == "" {
				return ctx.Err()
			}
			d.Decision = "failed"
			d.Reason = "dispatch failed or uncertain; inspect named record"
			if le := p.log(d); le != nil {
				return le
			}
			return nil
		}
		for _, i := range issues {
			delete(p.state.TicketFailures, lease(i, r, p.state.Cache[i.ID].Entered))
		}
		d.Decision = "dispatched"
		p.active[d.Name] = true
		if r.Concurrency == "code" {
			*running++
		}
	}
	return p.log(d)
}

func renderBatch(issues []Issue, r Rule, name string) (string, error) {
	if len(issues) == 0 {
		return "", fmt.Errorf("batch requires tickets")
	}
	sort.Slice(issues, func(a, b int) bool { return issues[a].Identifier < issues[b].Identifier })
	keys, urls := []string{}, []string{}
	for _, i := range issues {
		if err := validateValues(i.Identifier, i.URL, i.Title, i.State.Name); err != nil {
			return "", err
		}
		keys = append(keys, i.Identifier)
		urls = append(urls, i.URL)
	}
	return renderValues(r, name, strings.NewReplacer("{{identifiers}}", strings.Join(keys, "\n"), "{{urls}}", strings.Join(urls, "\n"), "{{count}}", fmt.Sprint(len(issues)), "{{name}}", name, "{{status}}", r.Name))
}
