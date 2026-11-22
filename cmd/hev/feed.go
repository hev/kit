package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/feed"
	"github.com/hev/kit/internal/feedsend"
	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newFeedCommand()) }

func newFeedCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "feed",
		Short: "The Slack activity feed: summarized agent turns, notices, board feedback and evidence",
		Long: `The feed posts what agents on this host are doing to one Slack channel, one
thread per session, dispatch run or orchestrator, with each turn summarized by
a small model. Configure it with [feed] in ~/.hev/config.toml (https://hevkit.com/docs/config-reference).

Hooks only spool events; hev feed send posts them and is started for you.`,
	}
	c.AddCommand(&cobra.Command{
		Use:   "send",
		Short: "Post spooled feed events, then exit when idle (started by hooks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := feed.Load(secrets.ConfigPath())
			if err != nil {
				return err
			}
			if !cfg.Enabled {
				return nil
			}
			return feedsend.New(cfg, feed.Dir()).Run(cmd.Context())
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the feed's config, spool and last sender log lines",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return feedStatus(cmd.OutOrStdout())
		},
	})
	var run string
	ev := &cobra.Command{
		Use:   "evidence DIR",
		Short: "Post an acceptance-evidence recording (its result.json) to the feed",
		Long: `Post an acceptance-evidence recording directory (the recorder's output, with
result.json, final.png or poster.jpg, and recording.mp4) to the feed. Kit
already does this when an agent runs the recorder in a hooked session; use
this for a recording made elsewhere. --run names the dispatch run whose thread
it belongs in (orch-NAME for an orchestrator).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(args[0] + "/result.json"); err != nil {
				return fmt.Errorf("%s: no result.json: %w", args[0], err)
			}
			if !(feed.Observer{DispatchRoot: dispatch.Root()}).ObserveEvidence(args[0], run) {
				return errors.New("the feed is off here, does not post evidence, or this is client work")
			}
			fmt.Fprintln(cmd.OutOrStdout(), "queued for the feed")
			return nil
		},
	}
	ev.Flags().StringVar(&run, "run", "", "the dispatch run (or orch-NAME) the recording belongs to")
	c.AddCommand(ev)
	c.AddCommand(newFeedReplayCommand())
	return c
}

func feedStatus(out io.Writer) error {
	cfg, err := feed.Load(secrets.ConfigPath())
	if err != nil {
		fmt.Fprintln(out, "feed: "+err.Error())
	}
	if !cfg.Enabled {
		fmt.Fprintln(out, "feed: off (no [feed] in "+secrets.ConfigPath()+")")
		return nil
	}
	dir := feed.Dir()
	model := "off (no model_key: compact lines)"
	if cfg.ModelKey != "" {
		model = cfg.Model
	}
	fmt.Fprintf(out, "feed: slack %s, events %s, boards %s, model %s\n", cfg.Channel, strings.Join(cfg.Events, ","), strings.Join(cfg.Boards, ","), model)
	if len(cfg.Skip) > 0 {
		fmt.Fprintf(out, "skip: %s\n", strings.Join(cfg.Skip, ","))
	}
	fmt.Fprintf(out, "spool: %d waiting in %s; sender running: %v\n", len(feed.Pending(dir)), feed.SpoolDir(dir), feed.SenderRunning(dir))
	s := feedsend.Sender{Dir: dir}
	if until, ok := s.BackedOff(); ok {
		fmt.Fprintf(out, "paused until %s\n", until.Local().Format("15:04:05"))
	}
	if b, err := os.ReadFile(dir + "/sender.log"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 5 {
			lines = lines[len(lines)-5:]
		}
		fmt.Fprintln(out, "log:")
		for _, l := range lines {
			fmt.Fprintln(out, "  "+l)
		}
	}
	return nil
}

// printPoster prints posts instead of sending them (replay --print).
type printPoster struct {
	out io.Writer
	n   int
}

func (p *printPoster) Post(_ context.Context, m attention.SlackMessage) (string, string, error) {
	p.n++
	ts := fmt.Sprintf("%d.000100", p.n)
	where := "NEW THREAD"
	if m.ThreadTS != "" {
		where = "in thread " + m.ThreadTS
	}
	fmt.Fprintf(p.out, "--- post %s (%s)\n%s\n", ts, where, m.Text)
	return "C-PRINT", ts, nil
}

func (p *printPoster) Upload(_ context.Context, f attention.SlackFile) error {
	fmt.Fprintf(p.out, "--- upload %s (%d bytes) into thread %s\n", f.Name, len(f.Data), f.ThreadTS)
	return nil
}

func newFeedReplayCommand() *cobra.Command {
	var print_, literal bool
	c := &cobra.Command{
		Use:   "replay FILE",
		Short: "Run recorded events (JSONL) through the feed, for checking its output",
		Long: `Replay recorded events through the feed into a scratch state directory and post
them. Each line is a hook event (~/.hev/hooks/events.jsonl), an attention notice
or outbox record (~/.hev/attention/outbox/*.json), a board-post event, or
{"evidence_dir": DIR, "run": NAME}.

--print writes the posts to stdout instead of Slack. Otherwise posts go to
[feed] channel, or to HEV_SLACK_API_URL (a fake server) when set; with
--fake-token the token is not read (only with HEV_SLACK_API_URL).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := feed.Load(secrets.ConfigPath())
			if err != nil {
				return err
			}
			if !cfg.Enabled {
				return errors.New("no [feed] in " + secrets.ConfigPath())
			}
			cfg.Events = feed.AllKinds
			cfg.Skip = []string{feed.KindPreTool, feed.KindPostTool}
			dir, err := os.MkdirTemp("", "hev-feed-replay-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			obs := feed.Observer{Config: &cfg, Dir: dir, DispatchRoot: dispatch.Root(), Kick: func(string) {}, Replay: true}
			if err := replayLines(args[0], obs); err != nil {
				return err
			}
			s := feedsend.New(cfg, dir)
			s.Idle = time.Millisecond
			if print_ {
				p := &printPoster{out: cmd.OutOrStdout()}
				s.Post, s.Gap = p, time.Millisecond
				literal = true
			} else if literal && os.Getenv(attention.SlackAPIEnv) == "" {
				return errors.New("--fake-token needs HEV_SLACK_API_URL (a fake Slack)")
			}
			if literal {
				s.Token = func(context.Context, string) (string, error) { return "xoxb-fake", nil }
			}
			if err := s.Run(cmd.Context()); err != nil {
				return err
			}
			if left := len(feed.Pending(dir)); left > 0 {
				return fmt.Errorf("%d events not posted; see %s/sender.log", left, dir)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&print_, "print", false, "print posts instead of sending them")
	c.Flags().BoolVar(&literal, "fake-token", false, "use a fake bot token (fake Slack only)")
	return c
}

func replayLines(path string, obs feed.Observer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var probe struct {
			Schema      string          `json:"schema"`
			Event       string          `json:"event"`
			Notice      json.RawMessage `json:"notice"`
			EvidenceDir string          `json:"evidence_dir"`
			Run         string          `json:"run"`
		}
		if json.Unmarshal(line, &probe) != nil {
			continue
		}
		switch {
		case probe.EvidenceDir != "":
			obs.ObserveEvidence(probe.EvidenceDir, probe.Run)
		case len(probe.Notice) > 0:
			var n struct{ Event string }
			_ = json.Unmarshal(probe.Notice, &n)
			obs.ObserveLifecycle(n.Event, probe.Notice)
		case probe.Schema == "hev.hook.event/v1":
			obs.ObserveHook(line)
		case probe.Event != "":
			obs.ObserveLifecycle(probe.Event, line)
		}
		// Keep spool order equal to file order.
		time.Sleep(time.Millisecond)
	}
	return sc.Err()
}
