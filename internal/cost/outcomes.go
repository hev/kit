package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type PR struct {
	URL      string `json:"url"`
	Repo     string `json:"repo"`
	MergedAt int64  `json:"merged_at"`
}
type Deploy struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	URL      string `json:"url"`
	LandedAt int64  `json:"landed_at"`
}
type Outcome struct {
	Issue   string   `json:"issue"`
	Team    string   `json:"team"`
	State   string   `json:"state"`
	DoneAt  int64    `json:"done_at"`
	PRs     []PR     `json:"prs"`
	Deploys []Deploy `json:"deploys"`
}
type Outcomes struct {
	PRs         []PR      `json:"prs,omitempty"`
	Team        string    `json:"team"`
	RefreshedAt int64     `json:"refreshed_at"`
	Issues      []Outcome `json:"issues"`
	Deploys     []Deploy  `json:"deploys,omitempty"`
}

// ReadOutcomes consumes the gaffer's cache. Scope is validated before exposing
// any data; a worker never obtains account-global notifications or calls Linear.
func ReadOutcomes(path, team string, repos []string, now time.Time) (Outcomes, error) {
	var out Outcomes
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	if team == "" || out.Team != team {
		return Outcomes{}, fmt.Errorf("outcome cache team does not match configured team")
	}
	if out.RefreshedAt <= 0 || out.RefreshedAt > now.UnixMilli() {
		return Outcomes{}, fmt.Errorf("invalid outcome refresh time")
	}
	for _, p := range out.PRs {
		if !contains(repos, p.Repo) || p.URL == "" || p.MergedAt < 0 {
			return Outcomes{}, fmt.Errorf("invalid or out-of-scope global PR")
		}
	}
	for _, d := range out.Deploys {
		if !contains(repos, d.Repo) || d.ID == "" || d.LandedAt < 0 {
			return Outcomes{}, fmt.Errorf("invalid or out-of-scope global deployment")
		}
	}
	seen := map[string]bool{}
	for _, i := range out.Issues {
		if i.Team != team || i.Issue == "" || seen[i.Issue] {
			return Outcomes{}, fmt.Errorf("invalid outcome issue scope or duplicate")
		}
		seen[i.Issue] = true
		for _, p := range i.PRs {
			if !contains(repos, p.Repo) {
				return Outcomes{}, fmt.Errorf("PR outside configured repo scope")
			}
		}
		for _, d := range i.Deploys {
			if !contains(repos, d.Repo) {
				return Outcomes{}, fmt.Errorf("deploy outside configured repo scope")
			}
		}
	}
	return out, nil
}
