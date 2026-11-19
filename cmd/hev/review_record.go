package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/reviewnotes"
	"github.com/spf13/cobra"
)

type recordOptions struct {
	head, file, reviewer string
	clean                bool
}

func newReviewRecordCommand(d func() reviewDeps, o *reviewOptions) *cobra.Command {
	var ro recordOptions
	c := &cobra.Command{
		Use:   "record RUN --head SHA (--file FINDINGS.json | --clean)",
		Short: "Record a review's findings, or a clean review, beside the run",
		Long: `Store a review of RUN at one head beside the run on its host, in
RUN/reviews/ID.json. FINDINGS.json is hunk's agent-context format (as in
review-notes.json): every annotation is a finding tagged ["finding","fail"]
or ["finding","note"] with a confidence (low, medium or high), anchored with
newRange (or oldRange for deleted lines). A head in the file must match
--head. --clean (or a file with no annotations) records a clean review: the
head and no findings.

Records are durable: hev review RUN and hev review RUN --json merge them for
the head they name and drop them, by name, for any other head, and hev
dispatch show RUN lists them per head. Send unsent findings to the run with
hev review send RUN. In an orchestrator round the reviewer is the
orchestrator, and recording needs the rework authority.`,
		Example: `  hev review record fix-ci --head 8c1d2e4f0a1b --file findings.json
  hev review record fix-ci --head 8c1d2e4f0a1b --clean
  hev review send fix-ci`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewRecord(cmd, d(), *o, args[0], ro)
		},
	}
	c.Flags().StringVar(&ro.head, "head", "", "Commit SHA the review covers (required)")
	c.Flags().StringVar(&ro.file, "file", "", "Findings JSON in hunk's agent-context format; - reads standard input")
	c.Flags().BoolVar(&ro.clean, "clean", false, "Record a clean review: the head and no findings")
	c.Flags().StringVar(&ro.reviewer, "reviewer", "", "Reviewer name (default: this session's sender; an orchestrator round is always o-NAME)")
	return c
}

// readFindings loads the findings document for record.
func readFindings(stdin io.Reader, ro recordOptions) (reviewnotes.Notes, error) {
	switch {
	case ro.clean && ro.file != "":
		return reviewnotes.Notes{}, errors.New("--clean records no findings; drop --file or --clean")
	case ro.clean:
		return reviewnotes.Notes{Files: []reviewnotes.File{}}, nil
	case ro.file == "":
		return reviewnotes.Notes{}, errors.New("pass --file FINDINGS.json, or --clean for a review with no findings")
	}
	var b []byte
	var err error
	if ro.file == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(ro.file)
	}
	if err != nil {
		return reviewnotes.Notes{}, err
	}
	return reviewnotes.Parse(b)
}

func runReviewRecord(cmd *cobra.Command, d reviewDeps, o reviewOptions, name string, ro recordOptions) error {
	if strings.TrimSpace(ro.head) == "" {
		return errors.New("--head SHA is required: the commit you reviewed")
	}
	notes, err := readFindings(cmd.InOrStdin(), ro)
	if err != nil {
		return err
	}
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
	reviewer := ro.reviewer
	if reviewer == "" {
		reviewer = dispatch.LocalSender()
	}
	if reviewer == "" {
		reviewer = os.Getenv("USER")
	}
	if reviewer == "" {
		reviewer = "reviewer"
	}
	c, cancel := rpcCtx(cmd.Context())
	defer cancel()
	resp, err := d.rpc(c, run.H, dispatch.Request{Action: "review-record", Name: name, ReviewRecord: &dispatch.ReviewRecordRequest{Head: ro.head, Reviewer: reviewer, Notes: notes}})
	if err != nil {
		if strings.Contains(err.Error(), "unknown action") {
			return fmt.Errorf("host %s runs a hev without review-record; update hev there (hev dispatch hosts update)", run.Host)
		}
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), resp.Message)
	return nil
}
