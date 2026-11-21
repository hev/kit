package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/harness"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/trace"
	"github.com/spf13/cobra"
)

// statsBackfillJournal records a capture-facts backfill's fixed window and
// the transcripts already written, so an interrupted --apply resumes.
type statsBackfillJournal struct {
	Version   int               `json:"version"`
	Namespace string            `json:"namespace"`
	From      int64             `json:"from"`
	Until     int64             `json:"until"`
	Done      map[string]string `json:"done"`
}

// statsBackfillCoverage is one harness's preview: what the rows a backfill
// would write carry.
type statsBackfillCoverage struct {
	Units             int `json:"units"`
	UnitsDone         int `json:"units_done"`
	UnreadableUnits   int `json:"unreadable_units"`
	OutsideWindow     int `json:"units_outside_window"`
	Sessions          int `json:"sessions"`
	BlockRows         int `json:"block_rows"`
	WithSkills        int `json:"with_skills"`
	SkillLoadsTool    int `json:"skill_loads_tool"`
	SkillLoadsRead    int `json:"skill_loads_read"`
	SkillLoadsCommand int `json:"skill_loads_command"`
	StartTranscript   int `json:"start_context_transcript"`
	StartDerived      int `json:"start_context_derived"`
	StartMixed        int `json:"start_context_mixed"`
	StartNone         int `json:"start_context_none"`
	ToolCalls         int `json:"tool_calls"`
	ToolCallsResulted int `json:"tool_calls_with_result"`
	ToolCallsExitCode int `json:"tool_calls_with_exit_code"`
	ToolCallsErrored  int `json:"tool_calls_errored"`
	WithCommands      int `json:"sessions_with_kit_commands"`
	EditedSessions    int `json:"sessions_edited"`
}

type statsBackfillReport struct {
	Mode      string                            `json:"mode"`
	From      int64                             `json:"from"`
	Until     int64                             `json:"until"`
	Journal   string                            `json:"journal"`
	Harnesses map[string]*statsBackfillCoverage `json:"harnesses"`
	Written   *struct {
		Units       int `json:"units"`
		BlockRows   int `json:"block_rows"`
		SessionRows int `json:"session_rows"`
	} `json:"written,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

const tracesBackfillLong = `Rewrite the read side (block and session rows) of recent sessions from this
host's local transcripts, so they carry the capture facts hev traces stats
reads: skill loads with how they were loaded, the starting context, tool
outcomes and per-session tool facts. No chunks are written and nothing is
embedded.

Without --apply it only previews: it parses the transcripts in the window
and reports, per harness, what the rows would carry. With --apply it writes
them, journaling each transcript as it lands in ~/.hev/traces-backfill.json;
run it again to resume. The window is fixed on the first run (--since back
from then) and kept until --restart.

  hev traces backfill                     preview the last 30 days
  hev traces backfill --apply             write them (resumable)
  hev traces backfill --apply --limit 50  write at most 50 transcripts this run`

func newTracesBackfillCmd() *cobra.Command {
	var since, namespace string
	var apply, restart, asJSON bool
	var limit int
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Preview or write capture facts for recent sessions from local transcripts",
		Long:  tracesBackfillLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			window, err := parseSinceDuration(since)
			if err != nil {
				return err
			}
			if window > 90*24*time.Hour {
				return fmt.Errorf("--since is bounded to 90d")
			}
			if limit < 0 {
				return fmt.Errorf("--limit must be 0 (no limit) or more")
			}
			cfg, err := daemon.LoadConfig()
			if err != nil {
				return err
			}
			w, err := currentWorkspace()
			if err != nil {
				return err
			}
			sources := trace.Sources(map[string]string{harness.Pi: cfg.PiSessionsDir, harness.OpenCode: cfg.OpenCodeDataDir, harness.Agy: cfg.AgyDataDir})
			home, _ := os.UserHomeDir()
			journalPath := filepath.Join(home, ".hev", "traces-backfill.json")
			if w.IsolatedStorage() {
				sources = workspaceTraceSources(w)
				journalPath = filepath.Join(w.Root(), "traces-backfill.json")
			}

			open := func() (index.Archive, *trace.ImageCapture, error) { return tracesWithImages(namespace) }
			return runTracesBackfill(cmd.OutOrStdout(), cmd.ErrOrStderr(), sources, open, journalPath, namespace, window, apply, restart, asJSON, limit, time.Now())
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window of transcripts to cover, back from the first run (at most 90d)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the rows (default: preview only)")
	cmd.Flags().BoolVar(&restart, "restart", false, "Forget the journal and start a new window")
	cmd.Flags().IntVar(&limit, "limit", 0, "With --apply, write at most this many transcripts this run")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the report as JSON")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Layer namespace (default $HEVLAYER_NAMESPACE or hev-traces)")
	return cmd
}

func runTracesBackfill(out, errOut io.Writer, sources []trace.Source, open func() (index.Archive, *trace.ImageCapture, error), journalPath, namespace string, window time.Duration, apply, restart, asJSON bool, limit int, now time.Time) error {
	j := statsBackfillJournal{Version: 1, Namespace: namespace, Until: now.UnixMilli(), From: now.Add(-window).UnixMilli(), Done: map[string]string{}}
	if !restart {
		if b, err := os.ReadFile(journalPath); err == nil {
			var old statsBackfillJournal
			if err := json.Unmarshal(b, &old); err != nil {
				return fmt.Errorf("read %s: %w (use --restart to start over)", journalPath, err)
			}
			if old.Version == 1 && old.Namespace == namespace {
				j = old
				if j.Done == nil {
					j.Done = map[string]string{}
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	// The window is closed at both ends. A transcript last touched after
	// Until is left to the live indexer, which (running the same capture)
	// rewrote it when it changed; leaving it out keeps a resumed backfill to
	// the fixed set it previewed, however many transcripts arrive meanwhile.
	// A unit whose last activity cannot be told is left out too.
	inWindow := func(u trace.Unit) bool {
		at := u.Modified
		if at == 0 {
			info, err := os.Stat(u.Key)
			if err != nil {
				return false
			}
			at = info.ModTime().UnixMilli()
		}
		return at >= j.From && at <= j.Until
	}
	pending := func(u trace.Unit) bool { return inWindow(u) && j.Done[u.Key] != u.Signature }

	rep := statsBackfillReport{Mode: "preview", From: j.From, Until: j.Until, Journal: journalPath, Harnesses: map[string]*statsBackfillCoverage{}}
	if apply {
		rep.Mode = "apply"
	}
	if !apply {
		for _, src := range sources {
			if _, err := os.Stat(sourceRoot(src)); err != nil {
				continue
			}
			units, err := src.Units()
			if err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", src.Describe(), err))
				continue
			}
			h := sourceHarness(src)
			c := &statsBackfillCoverage{}
			rep.Harnesses[h] = c
			for _, u := range units {
				if !inWindow(u) {
					c.OutsideWindow++
					continue
				}
				c.Units++
				if j.Done[u.Key] == u.Signature {
					c.UnitsDone++
					continue
				}
				turns, err := src.Read(u)
				if err != nil {
					c.UnreadableUnits++
					continue
				}
				_, _ = trace.CaptureImages(turns, nil)
				sessions, blocks := index.ReadSideRows(turns, layer.Hostname())
				previewCoverage(c, sessions, blocks)
			}
		}
		return printBackfill(out, rep, asJSON)
	}

	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	if err := platformops.MkdirAll(filepath.Dir(journalPath), 0o755); err != nil {
		return err
	}
	release, err := platformops.LockPath(journalPath+".lock", true)
	if err != nil {
		return fmt.Errorf("backfill journal busy: %w", err)
	}
	defer release()
	cl, images, err := open()
	if err != nil {
		return err
	}
	var mu sync.Mutex
	save := func() error {
		mu.Lock()
		defer mu.Unlock()
		return saveBackfillJSON(journalPath, j)
	}
	if err := save(); err != nil {
		return err
	}
	written := &struct {
		Units       int `json:"units"`
		BlockRows   int `json:"block_rows"`
		SessionRows int `json:"session_rows"`
	}{}
	remaining := limit
	for _, src := range sources {
		if _, err := os.Stat(sourceRoot(src)); err != nil {
			continue
		}
		if limit > 0 && remaining <= 0 {
			break
		}
		fmt.Fprintf(errOut, "backfilling %s\n", src.Describe())
		one, err := index.Run(src, cl, index.LoadState(), index.Options{
			ReadSide: true, Force: true, Limit: remaining, Images: images,
			Select: pending,
			Done: func(u trace.Unit) {
				mu.Lock()
				j.Done[u.Key] = u.Signature
				mu.Unlock()
				if err := save(); err != nil {
					fmt.Fprintf(errOut, "warning: journal not saved: %v\n", err)
				}
			},
			Progress: func(done, total int, unit string) {
				fmt.Fprintf(errOut, "\r\033[K  %d/%d %s", done, total, truncLeft(unit, 60))
			},
		})
		fmt.Fprintln(errOut)
		if err != nil {
			return err
		}
		written.Units += one.UnitsIndexed
		written.BlockRows += one.BlockRowsUpserted
		written.SessionRows += one.SessionRowsUpserted
		rep.Errors = append(rep.Errors, one.Errors...)
		if limit > 0 {
			remaining -= one.UnitsIndexed + len(one.Errors)
		}
	}
	rep.Written = written
	return printBackfill(out, rep, asJSON)
}

// previewCoverage adds one unit's would-be rows to a harness's preview.
func previewCoverage(c *statsBackfillCoverage, sessions []trace.SessionRow, blocks []trace.BlockRow) {
	c.BlockRows += len(blocks)
	for _, s := range sessions {
		c.Sessions++
		if len(s.Skills) > 0 {
			c.WithSkills++
		}
		source := "none"
		if s.StartContext != nil && s.StartContext.Source != "" {
			source = s.StartContext.Source
		}
		switch source {
		case "transcript":
			c.StartTranscript++
		case "derived":
			c.StartDerived++
		case "mixed":
			c.StartMixed++
		default:
			c.StartNone++
		}
		for _, n := range s.CommandCounts {
			if n > 0 {
				c.WithCommands++
				break
			}
		}
		if s.ToolsBeforeEdit >= 0 {
			c.EditedSessions++
		}
	}
	for _, b := range blocks {
		for _, l := range b.SkillLoads {
			switch l.Via {
			case "tool":
				c.SkillLoadsTool++
			case "read":
				c.SkillLoadsRead++
			case "command":
				c.SkillLoadsCommand++
			}
		}
		if b.BlockType != "tool_use" {
			continue
		}
		c.ToolCalls++
		if b.Resulted {
			c.ToolCallsResulted++
			if !b.OK {
				c.ToolCallsErrored++
			}
		}
		if b.ExitCode != nil {
			c.ToolCallsExitCode++
		}
	}
}

func printBackfill(out io.Writer, rep statsBackfillReport, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Fprintf(out, "%s: transcripts touched %s – %s (journal %s)\n", rep.Mode,
		time.UnixMilli(rep.From).UTC().Format(time.RFC3339), time.UnixMilli(rep.Until).UTC().Format(time.RFC3339), rep.Journal)
	if len(rep.Harnesses) > 0 {
		var names []string
		for h := range rep.Harnesses {
			names = append(names, h)
		}
		sort.Strings(names)
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "HARNESS\tUNITS\tOUTSIDE\tDONE\tSESSIONS\tBLOCKS\tSKILLS\tLOADS tool/read/cmd\tCONTEXT transcript/derived/mixed/none\tTOOL CALLS\tWITH RESULT\tEXIT CODE\tERRORS\tKIT CMDS\tEDITED")
		for _, h := range names {
			c := rep.Harnesses[h]
			fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d/%d/%d\t%d/%d/%d/%d\t%d\t%d\t%d\t%d\t%d\t%d\n", formatHarness(h), c.Units, c.OutsideWindow, c.UnitsDone, c.Sessions, c.BlockRows, c.WithSkills,
				c.SkillLoadsTool, c.SkillLoadsRead, c.SkillLoadsCommand, c.StartTranscript, c.StartDerived, c.StartMixed, c.StartNone,
				c.ToolCalls, c.ToolCallsResulted, c.ToolCallsExitCode, c.ToolCallsErrored, c.WithCommands, c.EditedSessions)
		}
		tw.Flush()
	}
	if rep.Written != nil {
		fmt.Fprintf(out, "written: %d transcripts, %d block rows, %d session rows\n", rep.Written.Units, rep.Written.BlockRows, rep.Written.SessionRows)
	}
	if rep.Mode == "preview" {
		fmt.Fprintln(out, "preview only — nothing written; run with --apply to write")
	}
	for _, e := range rep.Errors {
		fmt.Fprintf(out, "error: %s\n", e)
	}
	return nil
}
