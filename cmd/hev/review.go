package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/reviewnotes"
	"github.com/spf13/cobra"
)

const hunkInstallHint = "hunk is not on PATH; install it with: curl -fsSL https://hunk.dev/install.sh | sh   (or: npm i -g hunkdiff)"

const reviewLong = `Review a dispatched run's branch in hunk (https://hunk.dev), with the
run's anchored review notes and any review run's findings beside the code.

hev review fetches the run's branch from its host over the dispatch SSH path
(no push needed) into refs/hev/review/RUN of the current repository, or of a
cache clone when the current directory is not a checkout of the run's
repository. It merges the run's ../review-notes.json with the notes of review
runs for the same head: runs named RUN-review*, runs with "review" in their
name whose base is the run's branch, and any --with RUN. Notes written for a
different head are dropped with a warning. Then it opens

  hunk diff BASE...HEAD --agent-context merged.json --agent-notes

Without hunk on PATH it prints the review verdict, git diff --stat and an
install hint; it never installs anything.

Leave your own notes in the hunk window (c on a line, R to reply to a note),
then send them to the run as one anchored message with hev review send RUN.

Agents review without a TUI: hev review RUN --json fetches the branch the
same way and prints the merged agent context for the exact head, with the
run's recorded reviews (hev review record) merged in, the notes dropped for
other heads named, and the review history per head. hev review record
stores findings or a clean review beside the run; hev review send delivers
the unsent findings for the current head as the rework of an ended run, and
refuses a running run as hev dispatch rework does (stop it or wait for it to
end, then send again; the findings stay unsent until then). Nothing
on this path calls gh, so runs on Azure Repos work the same as GitHub.`

// reviewDeps are the outside effects hev review uses; tests replace them.
type reviewDeps struct {
	load      func() (dispatch.Config, error)
	inventory func(dispatch.Config, string) map[string]dispatch.Response
	rpc       func(context.Context, dispatch.Host, dispatch.Request) (dispatch.Response, error)
	lookPath  func(string) (string, error)
	// run executes a command with the terminal attached.
	run func(dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error
	// output captures a command's standard output.
	output   func(dir, name string, args ...string) ([]byte, error)
	cacheDir func() (string, error)
	getwd    func() (string, error)
	// hasSteer reports whether this build has hev steer.
	hasSteer func() bool
	self     func() (string, error)
}

func defaultReviewDeps(configPath *string) reviewDeps {
	return reviewDeps{
		load:      func() (dispatch.Config, error) { return dispatch.Load(dispatch.ConfigPath(*configPath)) },
		inventory: dispatch.Inventory,
		rpc:       dispatch.RPC,
		lookPath:  exec.LookPath,
		run: func(dir string, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
			c := exec.Command(name, args...)
			c.Dir, c.Stdin, c.Stdout, c.Stderr = dir, stdin, stdout, stderr
			return c.Run()
		},
		output: func(dir, name string, args ...string) ([]byte, error) {
			c := exec.Command(name, args...)
			c.Dir = dir
			var errout strings.Builder
			c.Stderr = &errout
			b, err := c.Output()
			if err != nil {
				return b, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(errout.String()))
			}
			return b, nil
		},
		cacheDir: os.UserCacheDir,
		getwd:    os.Getwd,
		hasSteer: func() bool {
			c, _, err := rootCmd.Find([]string{"steer"})
			return err == nil && c != nil && c != rootCmd && c.Name() == "steer"
		},
		self: os.Executable,
	}
}

func init() { rootCmd.AddCommand(newReviewCommand(nil)) }

type reviewOptions struct {
	configPath, host, repo string
	with                   []string
	web, printNotes, json  bool
}

func newReviewCommand(deps *reviewDeps) *cobra.Command {
	var o reviewOptions
	d := func() reviewDeps {
		if deps != nil {
			return *deps
		}
		return defaultReviewDeps(&o.configPath)
	}
	c := &cobra.Command{
		Use:   "review RUN",
		Short: "Review a dispatched run's branch and notes in hunk",
		Long:  reviewLong,
		Example: `  hev review fix-ci
  hev review fix-ci --with fix-ci-review
  hev review fix-ci --print-notes > notes.json
  hev review fix-ci --json
  hev review record fix-ci --head 8c1d2e4f0a1b --file findings.json
  hev review record fix-ci --head 8c1d2e4f0a1b --clean
  hev review send fix-ci --dry-run`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, d(), o, args[0])
		},
	}
	c.PersistentFlags().StringVar(&o.configPath, "config", "", "Dispatch TOML configuration")
	c.PersistentFlags().StringVar(&o.host, "host", "", "Host of the run, when its name is ambiguous")
	c.PersistentFlags().StringVar(&o.repo, "repo", "", "Local checkout to fetch into and review from (default: the current checkout, else a cache clone)")
	c.Flags().StringSliceVar(&o.with, "with", nil, "Review run whose notes to merge (repeatable; default: found by name)")
	c.Flags().BoolVar(&o.web, "web", false, "Open the review in the browser; needs a hunk with a browser mode (hunk 0.25 has none)")
	c.Flags().BoolVar(&o.printNotes, "print-notes", false, "Write the merged notes JSON to stdout and exit")
	c.Flags().BoolVar(&o.json, "json", false, "Fetch the branch at its exact head and print the merged agent context and review history as JSON, without hunk (for agents)")
	c.AddCommand(newReviewSendCommand(d, &o))
	c.AddCommand(newReviewRecordCommand(d, &o))
	return c
}

// reviewRun is a located run.
type reviewRun struct {
	Host string
	H    dispatch.Host
	Row  dispatch.Row
}

func locateRun(cfg dispatch.Config, inv map[string]dispatch.Response, hostFlag, name string) (reviewRun, error) {
	if !dispatch.ValidName(name) {
		return reviewRun{}, fmt.Errorf("invalid run name %q", name)
	}
	var found []reviewRun
	for _, n := range dispatch.SortedHosts(inv) {
		if hostFlag != "" && n != hostFlag {
			continue
		}
		for _, row := range inv[n].Rows {
			if row.Name == name && row.Kind == "run" {
				found = append(found, reviewRun{Host: n, H: cfg.Hosts[n], Row: row})
			}
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		var down []string
		for _, n := range dispatch.SortedHosts(inv) {
			if inv[n].Error != "" {
				down = append(down, n)
			}
		}
		if len(down) > 0 {
			return reviewRun{}, fmt.Errorf("run %s not found (unreachable: %s)", name, strings.Join(down, ", "))
		}
		return reviewRun{}, fmt.Errorf("run %s not found", name)
	default:
		return reviewRun{}, fmt.Errorf("run %s is on several hosts; pass --host", name)
	}
}

// reviewRunsFor finds the review runs for build by convention: a name
// starting with BUILD-review, or "review" in the name and the build's branch
// as base. Explicit names in with are added (and may be on any host).
func reviewRunsFor(cfg dispatch.Config, inv map[string]dispatch.Response, build reviewRun, with []string) ([]reviewRun, error) {
	var out []reviewRun
	seen := map[string]bool{build.Row.Name: true}
	for _, name := range with {
		if seen[name] {
			continue
		}
		r, err := locateRun(cfg, inv, "", name)
		if err != nil {
			return nil, fmt.Errorf("--with: %w", err)
		}
		seen[name] = true
		out = append(out, r)
	}
	if len(with) > 0 {
		return out, nil
	}
	for _, n := range dispatch.SortedHosts(inv) {
		for _, row := range inv[n].Rows {
			if row.Kind != "run" || seen[row.Name] {
				continue
			}
			byName := strings.HasPrefix(row.Name, build.Row.Name+"-review")
			byBase := strings.Contains(row.Name, "review") && build.Row.Branch != "" && (row.Base == build.Row.Branch || row.Base == "origin/"+build.Row.Branch)
			if byName || byBase {
				seen[row.Name] = true
				out = append(out, reviewRun{Host: n, H: cfg.Hosts[n], Row: row})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Row.Name < out[j].Row.Name })
	return out, nil
}

func rpcCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Minute)
}

// collected is what collectNotes gathered for one head.
type collected struct {
	Merged   reviewnotes.Notes
	Dropped  []reviewnotes.Dropped
	Warnings []string
	Verdicts []string
	// Records are the build run's recorded reviews, every head.
	Records []reviewnotes.Record
}

// collectNotes reads the build's and reviews' notes, and the reviews recorded
// beside the build run, and merges them for head. It also returns each review
// run's verdict line, if any.
func collectNotes(ctx context.Context, d reviewDeps, build reviewRun, reviews []reviewRun, head string) (collected, error) {
	var out collected
	var parts []reviewnotes.Part
	for i, r := range append([]reviewRun{build}, reviews...) {
		c, cancel := rpcCtx(ctx)
		resp, err := d.rpc(c, r.H, dispatch.Request{Action: "show", Name: r.Row.Name})
		cancel()
		if err != nil {
			if i == 0 {
				return out, err
			}
			out.Warnings = append(out.Warnings, fmt.Sprintf("skipped review %s: %v", r.Row.Name, err))
			continue
		}
		isReview := i > 0
		if !isReview {
			out.Records = resp.Reviews
		}
		verdict, verdictHead := reviewnotes.ReportVerdict(resp.Files["report.md"])
		if isReview && verdict != "" {
			out.Verdicts = append(out.Verdicts, fmt.Sprintf("%s: VERDICT: %s%s", r.Row.Name, verdict, headSuffix(verdictHead)))
		}
		raw := resp.Files[reviewnotes.FileName]
		if strings.TrimSpace(raw) == "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s wrote no %s", r.Row.Name, reviewnotes.FileName))
			continue
		}
		if len(raw) > reviewnotes.MaxNotesBytes {
			out.Warnings = append(out.Warnings, fmt.Sprintf("ignored %s from %s: %d bytes is over the cap of %d", reviewnotes.FileName, r.Row.Name, len(raw), reviewnotes.MaxNotesBytes))
			continue
		}
		n, err := reviewnotes.Parse([]byte(raw))
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("ignored %s from %s: %v", reviewnotes.FileName, r.Row.Name, err))
			continue
		}
		p := reviewnotes.Part{Run: r.Row.Name, Review: isReview, Head: n.Head, Notes: n}
		if p.Head == "" && isReview {
			p.Head = verdictHead
		}
		parts = append(parts, p)
	}
	// Recorded reviews follow the review runs: they are the latest word.
	parts = append(parts, reviewnotes.RecordParts(out.Records)...)
	var w []string
	out.Merged, out.Dropped, w = reviewnotes.MergeDetail(head, parts)
	out.Warnings = append(out.Warnings, w...)
	return out, nil
}

func headSuffix(h string) string {
	if h == "" {
		return ""
	}
	return " head=" + h
}

// reviewRepoFile, under the run's review cache directory, records the
// repository hev review opened hunk in.
const reviewRepoFile = "repo"

// checkHunkWeb fails when --web is asked of a hunk without a browser mode.
// hunk 0.25.0 has none: its diff command rejects --web. Without hunk on PATH
// the terminal fallback runs as usual.
func checkHunkWeb(d reviewDeps) error {
	if _, err := d.lookPath("hunk"); err != nil {
		return nil
	}
	help, err := d.output("", "hunk", "diff", "--help")
	if err != nil {
		return fmt.Errorf("checking hunk for a browser mode: %w", err)
	}
	if !hunkHasFlag(string(help), "--web") {
		version := "the installed hunk"
		if v, err := d.output("", "hunk", "--version"); err == nil && strings.TrimSpace(string(v)) != "" {
			version = "hunk " + strings.TrimSpace(string(v))
		}
		return fmt.Errorf("%s has no browser mode (hunk diff --web); run hev review without --web, or use --print-notes with another viewer", version)
	}
	return nil
}

// hunkHasFlag reports whether help text lists flag as an option.
func hunkHasFlag(help, flag string) bool {
	for _, f := range strings.Fields(help) {
		if strings.TrimRight(f, ",") == flag {
			return true
		}
	}
	return false
}

// hunkReviewArgs is the hunk invocation for a fetched run.
func hunkReviewArgs(mergeBase, head, notesPath string, web bool) []string {
	args := []string{"diff", mergeBase + "..." + head, "--agent-context", notesPath, "--agent-notes"}
	if web {
		args = append(args, "--web")
	}
	return args
}

func gitIn(d reviewDeps, dir string, args ...string) (string, error) {
	b, err := d.output(dir, "git", args...)
	return strings.TrimSpace(string(b)), err
}

func hasCommit(d reviewDeps, dir, sha string) bool {
	_, err := gitIn(d, dir, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// reviewRepo picks the local repository to fetch into: --repo, else the
// current checkout when it already has the run's merge base, else a cache
// clone for the run's repository.
func reviewRepo(d reviewDeps, flag, repoName, mergeBase string) (string, error) {
	if flag != "" {
		return gitIn(d, flag, "rev-parse", "--show-toplevel")
	}
	if wd, err := d.getwd(); err == nil {
		if top, err := gitIn(d, wd, "rev-parse", "--show-toplevel"); err == nil && hasCommit(d, top, mergeBase) {
			return top, nil
		}
	}
	cache, err := d.cacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "hev", "review", "repos", cacheRepoName(repoName))
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if _, err := gitIn(d, dir, "init", "-q"); err != nil {
		return "", err
	}
	return dir, nil
}

// cacheRepoName is the cache clone directory name for a run's repository.
func cacheRepoName(repoName string) string {
	name := strings.Trim(strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(filepath.Base(filepath.Clean(repoName))), ".-")
	if name == "" {
		name = "repo"
	}
	return name
}

// fetchRun brings the run's branch into a local repository under
// refs/hev/review/RUN and returns that repository and the fetch record.
func fetchRun(ctx context.Context, d reviewDeps, run reviewRun, repoFlag string, needBranch bool) (string, *dispatch.ReviewFetch, error) {
	c, cancel := rpcCtx(ctx)
	resp, err := d.rpc(c, run.H, dispatch.Request{Action: "review-fetch", Name: run.Row.Name})
	cancel()
	if err != nil {
		if strings.Contains(err.Error(), "unknown action") {
			return "", nil, fmt.Errorf("host %s runs a hev without review-fetch; update hev there (hev dispatch hosts update)", run.Host)
		}
		return "", nil, err
	}
	f := resp.Review
	if f == nil || f.Head == "" {
		return "", nil, fmt.Errorf("host %s returned no branch head for %s", run.Host, run.Row.Name)
	}
	if !needBranch {
		return "", f, nil
	}
	repo, err := reviewRepo(d, repoFlag, run.Row.Repo, f.MergeBase)
	if err != nil {
		return "", nil, err
	}
	ref := "refs/hev/review/" + run.Row.Name
	if !hasCommit(d, repo, f.Head) || !hasCommit(d, repo, f.MergeBase) {
		q := dispatch.Request{Action: "review-fetch", Name: run.Row.Name, Bundle: true}
		if hasCommit(d, repo, f.MergeBase) {
			q.Since = f.MergeBase
		}
		c, cancel := rpcCtx(ctx)
		resp, err = d.rpc(c, run.H, q)
		cancel()
		if err != nil {
			return "", nil, err
		}
		if resp.Review == nil || len(resp.Review.Bundle) == 0 {
			return "", nil, fmt.Errorf("host %s returned no bundle for %s", run.Host, run.Row.Name)
		}
		f.Head, f.MergeBase = resp.Review.Head, resp.Review.MergeBase
		tmp, err := os.CreateTemp("", "hev-review-*.bundle")
		if err != nil {
			return "", nil, err
		}
		defer os.Remove(tmp.Name())
		if _, err = tmp.Write(resp.Review.Bundle); err == nil {
			err = tmp.Close()
		}
		if err != nil {
			return "", nil, err
		}
		if _, err = gitIn(d, repo, "fetch", "-q", "--no-tags", tmp.Name(), "+refs/heads/"+f.Branch+":"+ref); err != nil {
			return "", nil, err
		}
	} else if _, err = gitIn(d, repo, "update-ref", ref, f.Head); err != nil {
		return "", nil, err
	}
	if !hasCommit(d, repo, f.MergeBase) {
		// A since-bundle carries the merge base's descendants only; a full
		// bundle carries the base itself. Either way it must be here now.
		return "", nil, fmt.Errorf("merge base %s missing after fetch", reviewnotes.Short(f.MergeBase))
	}
	return repo, f, nil
}

func runReview(cmd *cobra.Command, d reviewDeps, o reviewOptions, name string) error {
	ctx := cmd.Context()
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	cfg, err := d.load()
	if err != nil {
		return err
	}
	if o.host != "" {
		if _, ok := cfg.Hosts[o.host]; !ok {
			return fmt.Errorf("unknown host %s", o.host)
		}
	}
	inv := d.inventory(cfg, "")
	build, err := locateRun(cfg, inv, o.host, name)
	if err != nil {
		return err
	}
	reviews, err := reviewRunsFor(cfg, inv, build, o.with)
	if err != nil {
		return err
	}
	if o.json && (o.printNotes || o.web) {
		return fmt.Errorf("--json cannot be combined with --print-notes or --web")
	}
	if o.web && !o.printNotes {
		if err := checkHunkWeb(d); err != nil {
			return err
		}
	}
	repo, f, err := fetchRun(ctx, d, build, o.repo, !o.printNotes)
	if err != nil {
		return err
	}
	col, err := collectNotes(ctx, d, build, reviews, f.Head)
	if err != nil {
		return err
	}
	merged, verdicts := col.Merged, col.Verdicts
	if o.json {
		return writeAgentContext(stdout, build, repo, f, reviews, col)
	}
	for _, w := range col.Warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	if h := reviewnotes.History(col.Records, f.Head); h != "" {
		fmt.Fprint(stderr, h)
	}
	b, err := reviewnotes.Marshal(merged)
	if err != nil {
		return err
	}
	if o.printNotes {
		_, err = stdout.Write(b)
		return err
	}
	cache, err := d.cacheDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(cache, "hev", "review", name)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	notesPath := filepath.Join(dir, "merged.json")
	if err = os.WriteFile(notesPath, b, 0o600); err != nil {
		return err
	}
	// hev review send reads this to find the hunk window for this repository.
	if err = os.WriteFile(filepath.Join(dir, reviewRepoFile), []byte(repo+"\n"), 0o600); err != nil {
		return err
	}
	var names []string
	for _, r := range reviews {
		names = append(names, r.Row.Name)
	}
	reviewed := "no review runs"
	if len(names) > 0 {
		reviewed = "reviews: " + strings.Join(names, ", ")
	}
	fmt.Fprintf(stderr, "%s on %s: %s...%s in %s; %s; notes %s\n", name, build.Host, reviewnotes.Short(f.MergeBase), reviewnotes.Short(f.Head), repo, reviewed, notesPath)
	if _, err := d.lookPath("hunk"); err != nil {
		if len(verdicts) == 0 {
			fmt.Fprintln(stdout, "No review verdict.")
		}
		for _, v := range verdicts {
			fmt.Fprintln(stdout, v)
		}
		stat, err := d.output(repo, "git", "diff", "--stat", f.MergeBase+"..."+f.Head)
		if err != nil {
			return err
		}
		fmt.Fprint(stdout, string(stat))
		fmt.Fprintln(stdout, hunkInstallHint)
		return nil
	}
	for _, v := range verdicts {
		fmt.Fprintln(stderr, v)
	}
	return d.run(repo, cmd.InOrStdin(), stdout, stderr, "hunk", hunkReviewArgs(f.MergeBase, f.Head, notesPath, o.web)...)
}

// sendRoute names how send delivers: hev steer for a live run when this
// build has it, else hev dispatch say.
func sendRoute(d reviewDeps, state string) []string {
	if state == "running" && d.hasSteer() {
		return []string{"hev", "steer"}
	}
	return []string{"hev", "dispatch", "say"}
}

// agentContext is hev review --json: the merged agent-context document for
// the exact head (hunk loads it unchanged; it ignores the extra keys), where
// the branch was fetched, the notes dropped for other heads, and the run's
// recorded reviews per head.
type agentContext struct {
	reviewnotes.Notes
	Run        string                `json:"run"`
	Host       string                `json:"host"`
	Branch     string                `json:"branch"`
	MergeBase  string                `json:"merge_base"`
	Repo       string                `json:"repo"`
	Ref        string                `json:"ref"`
	ReviewRuns []string              `json:"review_runs"`
	Dropped    []reviewnotes.Dropped `json:"dropped"`
	Verdicts   []string              `json:"verdicts"`
	Warnings   []string              `json:"warnings"`
	Reviews    []reviewnotes.Record  `json:"reviews"`
}

func writeAgentContext(w io.Writer, build reviewRun, repo string, f *dispatch.ReviewFetch, reviews []reviewRun, col collected) error {
	ac := agentContext{Notes: col.Merged, Run: build.Row.Name, Host: build.Host, Branch: f.Branch, MergeBase: f.MergeBase, Repo: repo, Ref: "refs/hev/review/" + build.Row.Name,
		ReviewRuns: []string{}, Dropped: col.Dropped, Verdicts: col.Verdicts, Warnings: col.Warnings, Reviews: col.Records}
	for _, r := range reviews {
		ac.ReviewRuns = append(ac.ReviewRuns, r.Row.Name)
	}
	if ac.Notes.Version == 0 {
		ac.Notes.Version = reviewnotes.Version
	}
	if ac.Notes.Files == nil {
		ac.Notes.Files = []reviewnotes.File{}
	}
	for _, p := range []*[]string{&ac.Verdicts, &ac.Warnings} {
		if *p == nil {
			*p = []string{}
		}
	}
	if ac.Dropped == nil {
		ac.Dropped = []reviewnotes.Dropped{}
	}
	if ac.Reviews == nil {
		ac.Reviews = []reviewnotes.Record{}
	}
	b, err := json.MarshalIndent(ac, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
