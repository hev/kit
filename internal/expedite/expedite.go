// Package expedite implements the bounded follow-through policy for dispatch runs.
package expedite

import (
	"context"
	"fmt"
	"sort"

	"github.com/hev/kit/internal/dispatch"
)

type Run struct {
	Host  string `json:"host"`
	Name  string `json:"name"`
	Repo  string `json:"repo"`
	Model string `json:"model"`
}
type Plan struct {
	Name      string `json:"name"`
	Reply     string `json:"reply"`
	Authority string `json:"authority"`
	Runs      []Run  `json:"runs"`
}
type Evidence struct {
	Abandoned bool   `json:"abandoned,omitempty"`
	Head      string `json:"head"`
	Reviewed  bool   `json:"reviewed"`
	Verified  bool   `json:"verified"`
	Findings  string `json:"findings,omitempty"`
	Amend     string `json:"amend,omitempty"`
}
type PR struct {
	Head   string `json:"headRefOid"`
	State  string `json:"state"`
	Checks []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	} `json:"statusCheckRollup"`
}

func (p PR) Green() bool {
	if p.Head == "" || len(p.Checks) == 0 {
		return false
	}
	for _, c := range p.Checks {
		if !((c.Status == "COMPLETED" && c.Conclusion == "SUCCESS") || c.State == "SUCCESS") {
			return false
		}
	}
	return true
}
func Collect(inv map[string]dispatch.Response, reply string) ([]Run, error) {
	var runs []Run
	for host, r := range inv {
		if r.Error != "" {
			return nil, fmt.Errorf("cannot snapshot %s: %s", host, r.Error)
		}
		for _, row := range r.Rows {
			if row.Reply == reply && row.State != "merged" && row.State != "abandoned" && row.Until != "all followed runs merged or abandoned" {
				runs = append(runs, Run{host, row.Name, row.Repo, row.Model})
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Host+"/"+runs[i].Name < runs[j].Host+"/"+runs[j].Name })
	return runs, nil
}

// Driver keeps policy independent of transports and makes round ordering testable.
type Driver interface {
	Inbox(context.Context) error
	Inspect(context.Context, Run) (dispatch.Row, PR, error)
	Amend(context.Context, Run, string) error
	Merge(context.Context, Run, string, string) error
	Report(context.Context, string) error
}

func Round(ctx context.Context, p Plan, evidence map[string]Evidence, d Driver) (bool, error) {
	if p.Authority != "review" && p.Authority != "merge-on-green" {
		return false, fmt.Errorf("invalid authority")
	}
	if err := d.Inbox(ctx); err != nil {
		return false, err
	}
	all := true
	for _, r := range p.Runs {
		ev := evidence[r.Host+"/"+r.Name]
		if ev.Abandoned {
			if err := d.Report(ctx, r.Host+"/"+r.Name+": closed (explicitly abandoned)"); err != nil {
				return false, err
			}
			continue
		}
		row, pr, err := d.Inspect(ctx, r)
		line := r.Host + "/" + r.Name + ": "
		if err != nil {
			all = false
			line += "[needs you] " + err.Error()
		} else if pr.State == "MERGED" || row.State == "abandoned" {
			line += "closed"
		} else {
			all = false
			switch {
			case row.State == "running":
				line += "running"
			case ev.Amend != "":
				if err = d.Amend(ctx, r, ev.Amend); err != nil {
					line += "[needs you] amend failed: " + err.Error()
				} else {
					line += "rework requested via amend"
				}
			case row.PR == "":
				line += "[needs you] no PR"
			case pr.State != "OPEN":
				line += "[needs you] PR is not open"
			case ev.Head != pr.Head || !ev.Reviewed || !ev.Verified:
				line += "awaiting review and independent verification of head " + pr.Head
			case !pr.Green():
				line += "waiting for green CI on head " + pr.Head
			case p.Authority == "merge-on-green":
				if err = d.Merge(ctx, r, row.PR, pr.Head); err != nil {
					line += "[needs you] merge outcome requires inspection: " + err.Error()
				} else {
					line += "merged verified green head"
				}
			default:
				line += "ready; [needs you] review authority requires your merge"
			}
		}
		if err = d.Report(ctx, line); err != nil {
			return false, err
		}
	}
	return all, nil
}
