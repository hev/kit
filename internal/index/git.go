package index

import (
	"context"
	"encoding/json"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/trace"
)

// GitEnricher captures evidence without making ingestion depend on git or gh.
// Enrich caps each session at eight seconds; shorter caller deadlines win.
// Run is injectable for offline backfills/tests.
// PR is a decimal string, consistent with the existing chunk attribution field;
// empty means unknown. Commits contains only full object IDs, sorted and unique.
type GitEnricher struct {
	Run func(context.Context, string, string, ...string) ([]byte, error)
}

func NewGitEnricher() *GitEnricher {
	return &GitEnricher{Run: func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		c := exec.CommandContext(ctx, name, args...)
		c.Dir = dir
		c.Env = append(c.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1")
		return c.Output()
	}}
}

var fullSHA = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var commitLine = regexp.MustCompile(`(?m)^\[[^\]\r\n]+ ([0-9a-f]{7,64})\] `)
var commitCommand = regexp.MustCompile(`\bgit\s+(?:-C\s+\S+\s+)?commit\b`)
var failedExit = regexp.MustCompile(`(?i)(?:exit_code[" ]*:\s*[1-9]|exited with code [1-9]|exit code:? [1-9])`)

// Enrich merges local evidence into row. Missing/deleted branches never fall
// back to HEAD or --all, which could attribute another session's commits.
// A successful commit result can recover a full SHA after its worktree is gone;
// abbreviations are accepted only after git uniquely resolves a commit object.
func (g *GitEnricher) Enrich(ctx context.Context, row *trace.SessionRow, turns []trace.Turn) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	seen := map[string]bool{}
	for _, sha := range row.Commits {
		if fullSHA.MatchString(sha) {
			seen[sha] = true
		}
	}
	add := func(sha string) {
		if fullSHA.MatchString(sha) {
			seen[sha] = true
		}
	}
	if row.Workdir != "" && row.Branch != "" && !strings.HasPrefix(row.Branch, "-") && row.Start > 0 && row.End >= row.Start {
		out, err := g.Run(ctx, row.Workdir, "git", "log", "--format=%H %ct", "--since=@"+strconv.FormatInt(row.Start/1000, 10), "--until=@"+strconv.FormatInt(row.End/1000, 10), "refs/heads/"+row.Branch, "--")
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				f := strings.Fields(line)
				if len(f) != 2 {
					continue
				}
				sec, e := strconv.ParseInt(f[1], 10, 64)
				if e == nil && sec >= row.Start/1000 && sec <= row.End/1000 {
					add(f[0])
				}
			}
		}
	}
	uses := map[string]bool{}
	for _, t := range turns {
		if t.SessionID != row.SessionID {
			continue
		}
		for _, b := range t.Blocks {
			if b.Type == "tool_use" {
				uses[b.ToolUseID] = commitCommand.MatchString(b.Text)
				continue
			}
			if b.Type != "tool_result" || b.IsError || !uses[b.ToolUseID] || failedExit.MatchString(b.Text) {
				continue
			}
			for _, m := range commitLine.FindAllStringSubmatch(b.Text, -1) {
				sha := m[1]
				if fullSHA.MatchString(sha) {
					add(sha)
					continue
				}
				if row.Workdir == "" {
					continue
				}
				out, err := g.Run(ctx, row.Workdir, "git", "rev-parse", "--verify", sha+"^{commit}")
				if err == nil {
					resolved := strings.TrimSpace(string(out))
					if strings.HasPrefix(resolved, sha) {
						add(resolved)
					}
				}
			}
		}
	}
	row.Commits = trace.StringList{}
	for sha := range seen {
		row.Commits = append(row.Commits, sha)
	}
	sort.Strings(row.Commits)
	if row.PR != "" || row.RepoURL == "" || row.Branch == "" {
		return
	}
	// Explicit repo/head and all states also find PRs for deleted or merged heads.
	// Never guess when a reused branch has multiple PRs: require linked commits.
	out, err := g.Run(ctx, "", "gh", "pr", "list", "--repo", githubRepo(row.RepoURL), "--head", row.Branch, "--state", "all", "--limit", "20", "--json", "number,commits")
	if err != nil {
		return
	}
	var prs []struct {
		Number  int `json:"number"`
		Commits []struct {
			OID string `json:"oid"`
		} `json:"commits"`
	}
	if json.Unmarshal(out, &prs) != nil {
		return
	}
	var matches []int
	for _, pr := range prs {
		if pr.Number <= 0 {
			continue
		}
		for _, c := range pr.Commits {
			if seen[c.OID] {
				matches = append(matches, pr.Number)
				break
			}
		}
	}
	if len(matches) == 1 {
		row.PR = strconv.Itoa(matches[0])
	}
}

// githubRepo normalizes git origins without forwarding embedded credentials.
func githubRepo(remote string) string {
	if strings.HasPrefix(remote, "git@") {
		remote = "https://" + strings.Replace(strings.TrimPrefix(remote, "git@"), ":", "/", 1)
	}
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		return u.Host + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	}
	return strings.TrimSuffix(remote, ".git")
}
