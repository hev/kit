package index

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// OutcomeEnricher bounds each observation to 20 seconds and ten pages per API.
// Run uses gh's existing authentication and is replaceable for offline checks.
type OutcomeEnricher struct {
	Run func(context.Context, string, string, ...string) ([]byte, error)
	Now func() time.Time
}

func NewOutcomeEnricher() *OutcomeEnricher {
	return &OutcomeEnricher{Run: NewGitEnricher().Run, Now: time.Now}
}
func (g *OutcomeEnricher) api(ctx context.Context, repo, path string, out any) error {
	parts := strings.Split(repo, "/")
	host := "github.com"
	if len(parts) == 3 {
		host = parts[0]
		parts = parts[1:]
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repository")
	}
	b, e := g.Run(ctx, "", "gh", "api", "--hostname", host, "repos/"+parts[0]+"/"+parts[1]+"/"+path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func pages[T any](ctx context.Context, g *OutcomeEnricher, repo, path string, decode func(json.RawMessage) ([]T, error)) ([]T, error) {
	var all []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; page <= 10; page++ {
		var raw json.RawMessage
		if e := g.api(ctx, repo, path+sep+"per_page=100&page="+strconv.Itoa(page), &raw); e != nil {
			return nil, e
		}
		rows, e := decode(raw)
		if e != nil {
			return nil, e
		}
		all = append(all, rows...)
		if len(rows) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("pagination limit reached")
}
func array[T any](raw json.RawMessage) ([]T, error) {
	var v []T
	e := json.Unmarshal(raw, &v)
	return v, e
}

type workflowRun struct {
	ID         int64  `json:"id"`
	WorkflowID int64  `json:"workflow_id"`
	Attempt    int    `json:"run_attempt"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"html_url"`
	SHA        string `json:"head_sha"`
}
type historyCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message   string `json:"message"`
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

var revertMessage = regexp.MustCompile(`(?im)^This reverts commit ([0-9a-f]{40}|[0-9a-f]{64})(?:\.\s*$|, reversing\b)`)

// Enrich never converts transport/truncation failures to false or success.
func (g *OutcomeEnricher) Enrich(ctx context.Context, row *trace.SessionRow) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	now := g.Now()
	o := trace.SessionOutcomes{PRState: "unknown", PRMerged: "unknown", PRClosed: "unknown", CIState: "unknown", Reverted: "unknown", RevertState: "unknown", OutcomeChecked: now.UnixMilli(), CIConclusions: trace.StringList{}, CIRuns: trace.StringList{}, RevertCommits: trace.StringList{}, WorkflowAttributes: map[string]string{}}
	for k, v := range row.WorkflowAttributes {
		o.WorkflowAttributes[k] = v
		if strings.HasSuffix(k, "_conclusion") || strings.HasSuffix(k, "_reverted") {
			o.WorkflowAttributes[k] = "unknown"
		}
	}
	defer func() { row.SessionOutcomes = o }()
	if row.PR == "" {
		(&GitEnricher{Run: g.Run}).Enrich(ctx, row, nil)
	}
	for _, sha := range row.Commits {
		if fullSHA.MatchString(sha) {
			o.WorkflowAttributes["commit_outcome_"+sha+"_reverted"] = "unknown"
		}
	}
	n, e := strconv.Atoi(row.PR)
	if e != nil || n <= 0 || row.RepoURL == "" {
		return
	}
	repo := githubRepo(row.RepoURL)
	var pr struct {
		State    string     `json:"state"`
		Merged   *bool      `json:"merged"`
		MergedAt *time.Time `json:"merged_at"`
		URL      string     `json:"html_url"`
		MergeSHA string     `json:"merge_commit_sha"`
		Head     struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if g.api(ctx, repo, "pulls/"+row.PR, &pr) != nil {
		return
	}
	if pr.Merged == nil || (pr.State != "open" && pr.State != "closed") {
		return
	}
	o.PRState = pr.State
	o.PRMerged = strconv.FormatBool(*pr.Merged)
	o.PRClosed = strconv.FormatBool(pr.State == "closed")
	o.PRURL = pr.URL
	if *pr.Merged {
		o.PRState = "merged"
	}
	if pr.Head.SHA != "" {
		shas := []string{pr.Head.SHA}
		if *pr.Merged && fullSHA.MatchString(pr.MergeSHA) && pr.MergeSHA != pr.Head.SHA {
			shas = append(shas, pr.MergeSHA)
		}
		var runs []workflowRun
		unavailable := false
		for _, sha := range shas {
			found, err := pages(ctx, g, repo, "actions/runs?head_sha="+url.QueryEscape(sha), func(raw json.RawMessage) ([]workflowRun, error) {
				var v struct {
					Runs []workflowRun `json:"workflow_runs"`
				}
				e := json.Unmarshal(raw, &v)
				return v.Runs, e
			})
			if err != nil {
				unavailable = true
				continue
			}
			for _, r := range found {
				if r.SHA == sha {
					runs = append(runs, r)
				}
			}
		}

		if len(runs) > 0 {
			latest := map[int64]workflowRun{}
			for _, r := range runs {
				if r.Name == "" || r.ID <= 0 || r.WorkflowID <= 0 {
					continue
				}
				old := latest[r.WorkflowID]
				if r.ID > old.ID || r.ID == old.ID && r.Attempt > old.Attempt {
					latest[r.WorkflowID] = r
				}
			}
			o.CIState = "complete"
			if unavailable {
				o.CIState = "unknown"
			}
			if len(latest) == 0 {
				o.CIState = "unknown"
			}
			for _, r := range latest {
				conclusion := r.Conclusion
				if r.Status != "completed" {
					conclusion = "pending"
					o.CIState = "pending"
				} else if conclusion == "" {
					conclusion = "unknown"
					if o.CIState != "pending" {
						o.CIState = "unknown"
					}
				}
				key := WorkflowAttribute(r.Name, r.WorkflowID)
				o.WorkflowAttributes[key+"_name"] = r.Name
				o.WorkflowAttributes[key+"_conclusion"] = conclusion
				o.WorkflowAttributes[key+"_run"] = strconv.FormatInt(r.ID, 10)
				o.WorkflowAttributes[key+"_attempt"] = strconv.Itoa(r.Attempt)
				o.WorkflowAttributes[key+"_url"] = r.URL
				o.WorkflowAttributes[key+"_sha"] = r.SHA
				o.CIConclusions = append(o.CIConclusions, r.Name+"="+conclusion)
				o.CIRuns = append(o.CIRuns, fmt.Sprintf("%s=%s|%d|%d|%s", r.Name, conclusion, r.ID, r.Attempt, r.URL))
			}
			sort.Strings(o.CIConclusions)
			sort.Strings(o.CIRuns)
		}
	}
	if !*pr.Merged || pr.MergedAt == nil {
		return
	}
	until := pr.MergedAt.Add(14 * 24 * time.Hour)
	o.RevertUntil = until.UnixMilli()
	end := now
	if end.After(until) {
		end = until
	}
	targets := map[string]bool{}
	for _, sha := range row.Commits {
		targets[sha] = true
	}
	if fullSHA.MatchString(pr.MergeSHA) {
		targets[pr.MergeSHA] = true
	}
	for sha := range targets {
		if fullSHA.MatchString(sha) {
			o.WorkflowAttributes["commit_outcome_"+sha+"_reverted"] = "unknown"
		}
	}
	if len(targets) == 0 || pr.Base.Ref == "" {
		return
	}
	commits, err := pages(ctx, g, repo, "commits?sha="+url.QueryEscape(pr.Base.Ref)+"&since="+url.QueryEscape(pr.MergedAt.Format(time.RFC3339))+"&until="+url.QueryEscape(end.Format(time.RFC3339)), array[historyCommit])
	if err != nil {
		return
	}
	for _, c := range commits {
		d := c.Commit.Committer.Date
		if d.Before(*pr.MergedAt) || d.After(end) {
			continue
		}
		for _, m := range revertMessage.FindAllStringSubmatch(c.Commit.Message, -1) {
			if targets[m[1]] {
				o.RevertCommits = append(o.RevertCommits, c.SHA)
				o.WorkflowAttributes["commit_outcome_"+m[1]+"_reverted"] = "true"
				o.WorkflowAttributes["commit_outcome_"+m[1]+"_revert"] = c.SHA
			}
		}
	}
	if !now.Before(until) {
		for sha := range targets {
			key := "commit_outcome_" + sha + "_reverted"
			if o.WorkflowAttributes[key] != "true" {
				o.WorkflowAttributes[key] = "false"
			}
		}
	}
	sort.Strings(o.RevertCommits)
	if len(o.RevertCommits) > 0 {
		o.Reverted = "true"
		o.RevertState = "observed"
	} else if now.Before(until) {
		o.RevertState = "observing"
	} else {
		o.Reverted = "false"
		o.RevertState = "complete"
	}
}

// SweepOutcomes returns an ID cursor. Reuse it until empty, then start again.
// Rows are revisited even after completed observations so new CI retries appear.
func SweepOutcomes(ctx context.Context, cl *layer.Client, g *OutcomeEnricher, after string, since int64, limit int) (string, int, error) {
	rows, e := cl.OutcomePage(ctx, after, since, limit)
	if e != nil {
		return after, 0, e
	}
	if len(rows) == 0 {
		return "", 0, nil
	}
	count := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			return after, count, ctx.Err()
		}
		if row.ID <= after {
			return after, count, fmt.Errorf("outcome cursor did not advance")
		}
		old := row.SessionOutcomes
		pr := row.PR
		commitsBefore := append(trace.StringList{}, row.Commits...)
		g.Enrich(ctx, &row)
		// Observation timestamp alone is not a semantic change.
		old.OutcomeChecked = row.OutcomeChecked
		// Wire storage metadata and indexed canonical tokens are not new observations.
		old.OutcomeDetails = ""
		old.FlattenedWorkflowAttributes = nil
		row.FlattenedWorkflowAttributes = nil
		if old.CIConclusions != nil {
			names := trace.StringList{}
			for _, entry := range old.CIConclusions {
				key, _, ok := strings.Cut(entry, "=")
				_, token := old.WorkflowAttributes[key]
				if ok && token && strings.HasSuffix(key, "_conclusion") {
					continue
				}
				names = append(names, entry)
			}
			old.CIConclusions = names
		}

		if !reflect.DeepEqual(old, row.SessionOutcomes) || pr != row.PR || !reflect.DeepEqual(commitsBefore, row.Commits) {
			if e = cl.PatchOutcomes(ctx, row); e != nil {
				return after, count, e
			}
			count++
		}
		after = row.ID
	}
	if len(rows) < limit {
		after = ""
	}
	return after, count, nil
}

// WorkflowAttribute is stable across runs/retries and disambiguates equal names.
func WorkflowAttribute(name string, id int64) string {
	return fmt.Sprintf("ci_workflow_%x_%d", sha256.Sum256([]byte(name)), id)
}
