package main

import (
	"context"
	"encoding/json"
	"errors"
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
then send them to the run as one anchored message with hev review send RUN.`

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
	web, printNotes        bool
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
  hev review fix-ci --with fix-ci-review --web
  hev review fix-ci --print-notes > notes.json
  hev review send fix-ci --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, d(), o, args[0])
		},
	}
	c.PersistentFlags().StringVar(&o.configPath, "config", "", "Dispatch TOML configuration")
	c.PersistentFlags().StringVar(&o.host, "host", "", "Host of the run, when its name is ambiguous")
	c.PersistentFlags().StringVar(&o.repo, "repo", "", "Local checkout to fetch into and review from (default: the current checkout, else a cache clone)")
	c.Flags().StringSliceVar(&o.with, "with", nil, "Review run whose notes to merge (repeatable; default: found by name)")
	c.Flags().BoolVar(&o.web, "web", false, "Open the review in the browser (hunk diff --web)")
	c.Flags().BoolVar(&o.printNotes, "print-notes", false, "Write the merged notes JSON to stdout and exit")
	c.AddCommand(newReviewSendCommand(d, &o))
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

// collectNotes reads the build's and reviews' notes and merges them for head.
// It also returns each review's verdict line, if any.
func collectNotes(ctx context.Context, d reviewDeps, build reviewRun, reviews []reviewRun, head string) (reviewnotes.Notes, []string, []string, error) {
	var parts []reviewnotes.Part
	var warnings, verdicts []string
	for i, r := range append([]reviewRun{build}, reviews...) {
		c, cancel := rpcCtx(ctx)
		resp, err := d.rpc(c, r.H, dispatch.Request{Action: "show", Name: r.Row.Name})
		cancel()
		if err != nil {
			if i == 0 {
				return reviewnotes.Notes{}, nil, nil, err
			}
			warnings = append(warnings, fmt.Sprintf("skipped review %s: %v", r.Row.Name, err))
			continue
		}
		isReview := i > 0
		verdict, verdictHead := reviewnotes.ReportVerdict(resp.Files["report.md"])
		if isReview && verdict != "" {
			verdicts = append(verdicts, fmt.Sprintf("%s: VERDICT: %s%s", r.Row.Name, verdict, headSuffix(verdictHead)))
		}
		raw := resp.Files[reviewnotes.FileName]
		if strings.TrimSpace(raw) == "" {
			warnings = append(warnings, fmt.Sprintf("%s wrote no %s", r.Row.Name, reviewnotes.FileName))
			continue
		}
		n, err := reviewnotes.Parse([]byte(raw))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("ignored %s from %s: %v", reviewnotes.FileName, r.Row.Name, err))
			continue
		}
		p := reviewnotes.Part{Run: r.Row.Name, Review: isReview, Head: n.Head, Notes: n}
		if p.Head == "" && isReview {
			p.Head = verdictHead
		}
		parts = append(parts, p)
	}
	merged, w := reviewnotes.Merge(head, parts)
	return merged, append(warnings, w...), verdicts, nil
}

func headSuffix(h string) string {
	if h == "" {
		return ""
	}
	return " head=" + h
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
	name := strings.Trim(strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(filepath.Base(filepath.Clean(repoName))), ".-")
	if name == "" {
		name = "repo"
	}
	dir := filepath.Join(cache, "hev", "review", "repos", name)
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
	repo, f, err := fetchRun(ctx, d, build, o.repo, !o.printNotes)
	if err != nil {
		return err
	}
	merged, warnings, verdicts, err := collectNotes(ctx, d, build, reviews, f.Head)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
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

// hunkSession is the part of `hunk session list --json` send reads.
type hunkSession struct {
	SessionID string `json:"sessionId"`
	RepoRoot  string `json:"repoRoot"`
	Title     string `json:"title"`
	Review    struct {
		Head string `json:"head"`
	} `json:"review"`
}

// pickHunkSession chooses the live hunk window showing head: an explicit
// session ID, else the one session whose review head is head, else the
// session for repo when one is given.
func pickHunkSession(listJSON []byte, head, explicit, repo string) ([]string, error) {
	if explicit != "" {
		return []string{explicit}, nil
	}
	var doc struct {
		Sessions []hunkSession `json:"sessions"`
	}
	if err := json.Unmarshal(listJSON, &doc); err != nil {
		return nil, fmt.Errorf("hunk session list: %w", err)
	}
	var match []hunkSession
	for _, s := range doc.Sessions {
		if s.Review.Head != "" && reviewnotes.SameHead(s.Review.Head, head) {
			match = append(match, s)
		}
	}
	switch {
	case len(match) == 1:
		return []string{match[0].SessionID}, nil
	case len(match) > 1:
		return nil, fmt.Errorf("%d hunk windows show %s; pass --session", len(match), reviewnotes.Short(head))
	case repo != "":
		return []string{"--repo", repo}, nil
	}
	var open []string
	for _, s := range doc.Sessions {
		open = append(open, fmt.Sprintf("%s (%s)", s.SessionID, s.Title))
	}
	if len(open) == 0 {
		return nil, fmt.Errorf("no live hunk session; open one with hev review RUN first")
	}
	return nil, fmt.Errorf("no hunk window shows %s; pass --session or --repo (open: %s)", reviewnotes.Short(head), strings.Join(open, "; "))
}

func newReviewSendCommand(d func() reviewDeps, o *reviewOptions) *cobra.Command {
	var dryRun, includeLive bool
	var session string
	c := &cobra.Command{
		Use:   "send RUN",
		Short: "Send your hunk review notes to the run as one anchored message",
		Long: `Read the notes you left in the live hunk window for RUN (hunk session
comment list --type all --json): your notes and your replies to agent notes.
Each becomes an anchored item, path:line and text. They are shown, then sent
to the run as one message: hev steer --level urgent for a live run when this
build has hev steer, otherwise hev dispatch say (which reworks an ended run).

--include-live also takes live agent comments, the kind hunk session comment
add creates, for notes added from a shell. --dry-run prints without sending.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewSend(cmd, d(), *o, args[0], session, includeLive, dryRun)
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print the message without sending it")
	c.Flags().BoolVar(&includeLive, "include-live", false, "Also send live agent comments (hunk session comment add)")
	c.Flags().StringVar(&session, "session", "", "hunk session ID (default: the window showing the run's head)")
	return c
}

func runReviewSend(cmd *cobra.Command, d reviewDeps, o reviewOptions, name, session string, includeLive, dryRun bool) error {
	ctx := cmd.Context()
	stdout := cmd.OutOrStdout()
	cfg, err := d.load()
	if err != nil {
		return err
	}
	if o.host != "" {
		if _, ok := cfg.Hosts[o.host]; !ok {
			return fmt.Errorf("unknown host %s", o.host)
		}
	}
	run, err := locateRun(cfg, d.inventory(cfg, o.host), o.host, name)
	if err != nil {
		return err
	}
	head := run.Row.Head
	if _, f, err := fetchRun(ctx, d, run, "", false); err == nil {
		head = f.Head
	} else if head == "" {
		return err
	}
	if _, err := d.lookPath("hunk"); err != nil {
		return errors.New(hunkInstallHint)
	}
	list, err := d.output("", "hunk", "session", "list", "--json")
	if err != nil {
		return err
	}
	sel, err := pickHunkSession(list, head, session, o.repo)
	if err != nil {
		return err
	}
	raw, err := d.output("", "hunk", append(append([]string{"session", "comment", "list"}, sel...), "--type", "all", "--json")...)
	if err != nil {
		return err
	}
	items, err := reviewnotes.ParseHunkComments(raw, includeLive)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(stdout, "No reviewer notes in the hunk session; nothing to send.")
		return nil
	}
	msg := reviewnotes.FormatItems(name, head, items)
	fmt.Fprint(stdout, msg)
	via := sendRoute(d, run.Row.State)
	if dryRun {
		fmt.Fprintf(stdout, "\n(dry run: would send with %s)\n", strings.Join(via, " "))
		return nil
	}
	self, err := d.self()
	if err != nil {
		return err
	}
	args := append(append([]string{}, via[1:]...), name, "-")
	if via[1] == "steer" {
		args = append(args, "--level", "urgent")
	}
	if o.configPath != "" {
		args = append(args, "--config", o.configPath)
	}
	args = append(args, "--host", run.Host)
	fmt.Fprintf(stdout, "\nSending with %s:\n", strings.Join(via, " "))
	return d.run("", strings.NewReader(msg), stdout, cmd.ErrOrStderr(), self, args...)
}

// sendRoute names how send delivers: hev steer for a live run when this
// build has it, else hev dispatch say.
func sendRoute(d reviewDeps, state string) []string {
	if state == "running" && d.hasSteer() {
		return []string{"hev", "steer"}
	}
	return []string{"hev", "dispatch", "say"}
}
