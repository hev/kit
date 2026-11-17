package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hev/kit/internal/reviewnotes"
	"github.com/spf13/cobra"
)

// hunkSession is the part of `hunk session list --json` send reads.
type hunkSession struct {
	SessionID string `json:"sessionId"`
	RepoRoot  string `json:"repoRoot"`
	Title     string `json:"title"`
	Review    struct {
		Head string `json:"head"`
	} `json:"review"`
}

// samePath compares two directories after resolving symlinks where they exist.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	norm := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return filepath.Clean(p)
	}
	return norm(a) == norm(b)
}

// pickHunkSession chooses the live hunk window to read for a run whose head
// is head in the repository repo. The window must show both that repository
// and that head: two clones can hold the same commit, and a window for the
// wrong one would send another review's notes to this run. An explicit
// session must exist and show head; it must also be in repo when strictRepo
// (repo came from --repo).
func pickHunkSession(listJSON []byte, head, explicit, repo string, strictRepo bool) (string, error) {
	var doc struct {
		Sessions []hunkSession `json:"sessions"`
	}
	if err := json.Unmarshal(listJSON, &doc); err != nil {
		return "", fmt.Errorf("hunk session list: %w", err)
	}
	describe := func(s hunkSession) string {
		return fmt.Sprintf("%s (%s at %s)", s.SessionID, s.RepoRoot, reviewnotes.Short(s.Review.Head))
	}
	if explicit != "" {
		for _, s := range doc.Sessions {
			if s.SessionID != explicit {
				continue
			}
			if !reviewnotes.SameHead(s.Review.Head, head) {
				return "", fmt.Errorf("hunk session %s shows %s, not the run's head %s", explicit, reviewnotes.Short(s.Review.Head), reviewnotes.Short(head))
			}
			if strictRepo && !samePath(s.RepoRoot, repo) {
				return "", fmt.Errorf("hunk session %s is in %s, not --repo %s", explicit, s.RepoRoot, repo)
			}
			return s.SessionID, nil
		}
		return "", fmt.Errorf("no live hunk session %s", explicit)
	}
	if len(doc.Sessions) == 0 {
		return "", errors.New("no live hunk session; open one with hev review RUN first")
	}
	var match, elsewhere []hunkSession
	for _, s := range doc.Sessions {
		if !reviewnotes.SameHead(s.Review.Head, head) {
			continue
		}
		if samePath(s.RepoRoot, repo) {
			match = append(match, s)
		} else {
			elsewhere = append(elsewhere, s)
		}
	}
	switch {
	case len(match) == 1:
		return match[0].SessionID, nil
	case len(match) > 1:
		return "", fmt.Errorf("%d hunk windows show %s in %s; pass --session", len(match), reviewnotes.Short(head), repo)
	case len(elsewhere) > 0:
		var w []string
		for _, s := range elsewhere {
			w = append(w, describe(s))
		}
		return "", fmt.Errorf("no hunk window shows %s in %s; windows in other repositories show that commit and were not read: %s (pass --repo or --session to choose one)", reviewnotes.Short(head), repo, strings.Join(w, "; "))
	}
	var open []string
	for _, s := range doc.Sessions {
		open = append(open, describe(s))
	}
	return "", fmt.Errorf("no hunk window shows %s in %s; open one with hev review RUN (open: %s)", reviewnotes.Short(head), repo, strings.Join(open, "; "))
}

// sendRepo is the repository whose hunk window send reads: --repo, else the
// one hev review last opened for the run, else the one hev review would pick
// now. It never creates anything.
func sendRepo(d reviewDeps, flag, name, repoName, mergeBase string) (repo string, strict bool, err error) {
	if flag != "" {
		top, err := gitIn(d, flag, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", true, fmt.Errorf("--repo %s: %w", flag, err)
		}
		return top, true, nil
	}
	cache, err := d.cacheDir()
	if err != nil {
		return "", false, err
	}
	if b, err := os.ReadFile(filepath.Join(cache, "hev", "review", name, reviewRepoFile)); err == nil {
		if r := strings.TrimSpace(string(b)); r != "" {
			return r, false, nil
		}
	}
	if wd, err := d.getwd(); err == nil {
		if top, err := gitIn(d, wd, "rev-parse", "--show-toplevel"); err == nil && (mergeBase == "" || hasCommit(d, top, mergeBase)) {
			return top, false, nil
		}
	}
	return filepath.Join(cache, "hev", "review", "repos", cacheRepoName(repoName)), false, nil
}

// sendLedger records which hunk notes reached which run, so a repeated send
// delivers only new or edited notes. Keys: target (host/run), hunk session,
// note ID; value: a digest of the note's anchor and text.
type sendLedger struct {
	Version    int                                     `json:"version"`
	Deliveries map[string]map[string]map[string]string `json:"deliveries"`
}

func ledgerPath(d reviewDeps) (string, error) {
	cache, err := d.cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "hev", "review", "sent.json"), nil
}

func loadLedger(path string) (sendLedger, error) {
	l := sendLedger{Version: 1, Deliveries: map[string]map[string]map[string]string{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	if err := json.Unmarshal(b, &l); err != nil {
		return l, fmt.Errorf("%s: %w", path, err)
	}
	if l.Deliveries == nil {
		l.Deliveries = map[string]map[string]map[string]string{}
	}
	return l, nil
}

func (l sendLedger) save(path string) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sent-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(append(b, '\n')); err == nil {
		err = tmp.Close()
	} else {
		tmp.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// itemKey identifies a note within a session; itemDigest changes when the
// note is edited, so an edited note is sent again.
func itemKey(it reviewnotes.Item) string {
	if it.ID != "" {
		return it.ID
	}
	return "digest:" + itemDigest(it)
}

func itemDigest(it reviewnotes.Item) string {
	h := sha256.Sum256([]byte(it.Anchor() + "\n" + it.Text))
	return hex.EncodeToString(h[:8])
}

func (l sendLedger) sent(target, session string, it reviewnotes.Item) bool {
	return l.Deliveries[target][session][itemKey(it)] == itemDigest(it)
}

func (l sendLedger) record(target, session string, items []reviewnotes.Item) {
	if l.Deliveries[target] == nil {
		l.Deliveries[target] = map[string]map[string]string{}
	}
	if l.Deliveries[target][session] == nil {
		l.Deliveries[target][session] = map[string]string{}
	}
	for _, it := range items {
		l.Deliveries[target][session][itemKey(it)] = itemDigest(it)
	}
}

// lockLedger serializes sends that share the ledger, so two concurrent sends
// of one session cannot both deliver the same notes. A lock older than
// staleLock is taken over: its holder died.
func lockLedger(path string) (func(), error) {
	const staleLock = 10 * time.Minute
	lock := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(lock) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if st, serr := os.Stat(lock); serr == nil && time.Since(st.ModTime()) > staleLock {
			os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another hev review send holds %s; retry when it finishes", lock)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type sendOptions struct {
	session                    string
	includeLive, dryRun, again bool
}

func newReviewSendCommand(d func() reviewDeps, o *reviewOptions) *cobra.Command {
	var so sendOptions
	c := &cobra.Command{
		Use:   "send RUN",
		Short: "Send your hunk review notes to the run as one anchored message",
		Long: `Read the notes you left in the live hunk window for RUN (hunk session
comment list --type all --json): your notes and your replies to agent notes.
Each becomes an anchored item, path:line and text. They are shown, then sent
to the run as one message: hev steer --level urgent for a live run when this
build has hev steer, otherwise hev dispatch say (which reworks an ended run).

The window must show the run's head in the run's repository: --repo, else the
repository hev review RUN opened, else the current checkout or the review
cache clone. Windows in other repositories are never read.

Notes already delivered to RUN from the same window are skipped; an edited
note is sent again. --resend sends every note. --include-live also takes live
agent comments, the kind hunk session comment add creates, for notes added
from a shell. --dry-run prints without sending or recording.`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewSend(cmd, d(), *o, args[0], so)
		},
	}
	c.Flags().BoolVar(&so.dryRun, "dry-run", false, "Print the message without sending it")
	c.Flags().BoolVar(&so.includeLive, "include-live", false, "Also send live agent comments (hunk session comment add)")
	c.Flags().BoolVar(&so.again, "resend", false, "Send notes already delivered to the run too")
	c.Flags().StringVar(&so.session, "session", "", "hunk session ID (default: the window showing the run's head in the run's repository)")
	return c
}

func runReviewSend(cmd *cobra.Command, d reviewDeps, o reviewOptions, name string, so sendOptions) error {
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
	head, mergeBase := run.Row.Head, ""
	if _, f, err := fetchRun(ctx, d, run, "", false); err == nil {
		head, mergeBase = f.Head, f.MergeBase
	} else if head == "" {
		return err
	}
	repo, strict, err := sendRepo(d, o.repo, name, run.Row.Repo, mergeBase)
	if err != nil {
		return err
	}
	if _, err := d.lookPath("hunk"); err != nil {
		return errors.New(hunkInstallHint)
	}
	list, err := d.output("", "hunk", "session", "list", "--json")
	if err != nil {
		return err
	}
	session, err := pickHunkSession(list, head, so.session, repo, strict)
	if err != nil {
		return err
	}
	raw, err := d.output("", "hunk", "session", "comment", "list", session, "--type", "all", "--json")
	if err != nil {
		return err
	}
	all, err := reviewnotes.ParseHunkComments(raw, so.includeLive)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(stdout, "No reviewer notes in the hunk session; nothing to send.")
		return nil
	}
	path, err := ledgerPath(d)
	if err != nil {
		return err
	}
	if !so.dryRun {
		unlock, err := lockLedger(path)
		if err != nil {
			return err
		}
		defer unlock()
	}
	ledger, err := loadLedger(path)
	if err != nil {
		return err
	}
	target := run.Host + "/" + name
	items := all
	if !so.again {
		items = nil
		for _, it := range all {
			if !ledger.sent(target, session, it) {
				items = append(items, it)
			}
		}
	}
	skipped := len(all) - len(items)
	if len(items) == 0 {
		fmt.Fprintf(stdout, "All %d notes in hunk session %s were already sent to %s; nothing new to send (--resend sends them again).\n", len(all), session, name)
		return nil
	}
	msg := reviewnotes.FormatItems(name, head, items)
	fmt.Fprint(stdout, msg)
	if skipped > 0 {
		fmt.Fprintf(stdout, "\n(%d already sent to %s, skipped; --resend sends them again)\n", skipped, name)
	}
	via := sendRoute(d, run.Row.State)
	if so.dryRun {
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
	if err := d.run("", strings.NewReader(msg), stdout, cmd.ErrOrStderr(), self, args...); err != nil {
		return fmt.Errorf("sending to %s: %w (nothing recorded; the notes will be sent next time)", name, err)
	}
	ledger.record(target, session, items)
	if err := ledger.save(path); err != nil {
		return fmt.Errorf("sent, but recording the delivery in %s failed: %w", path, err)
	}
	return nil
}
