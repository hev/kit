package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/hev/kit/internal/reviews"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newReviewCommand()) }

func newReviewCommand() *cobra.Command {
	c := &cobra.Command{Use: "review", Short: "Record, run, and search exact-head code reviews"}
	c.AddCommand(newReviewPut(), newReviewAdd(), newReviewResolve(), newReviewShow(),
		newReviewQuery(false), newReviewQuery(true), newReviewRun())
	return c
}

func newReviewPut() *cobra.Command {
	var r reviews.Review
	var file string
	c := &cobra.Command{
		Use: "put", Aliases: []string{"create"}, Short: "Record a review for a commit",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in := cmd.InOrStdin()
			switch file {
			case "":
				// Only read implicit stdin when it is a pipe, redirected file, or supplied reader.
				if f, ok := in.(*os.File); ok {
					info, err := f.Stat()
					if err != nil || info.Mode()&os.ModeCharDevice != 0 {
						in = nil
					}
				}
			case "-":
			default:
				f, err := os.Open(file)
				if err != nil {
					return errors.New("could not open findings file")
				}
				defer f.Close()
				in = f
			}
			if in != nil {
				data, err := io.ReadAll(io.LimitReader(in, reviews.MaxBytes+1))
				if err != nil || len(data) > reviews.MaxBytes {
					return errors.New("could not read findings within size limit")
				}
				if text := strings.TrimSpace(string(data)); text != "" || file != "" {
					if !strings.HasPrefix(text, "[") {
						return errors.New("findings JSON must be an array")
					}
					if err := reviews.Decode(strings.NewReader(text), &r.Findings); err != nil {
						return err
					}
				}
			}
			return saveReview(cmd, r)
		},
	}
	f := c.Flags()
	f.StringVar(&r.Repo, "repo", "", "Repository name or canonical identifier")
	f.StringVar(&r.HeadSHA, "head", "", "Full head commit SHA")
	f.StringVar(&r.BaseSHA, "base", "", "Full base commit SHA")
	f.StringVar(&r.Order, "order", "", "Dispatch order name")
	f.StringVar(&r.PR, "pr", "", "Pull request number or URL")
	f.StringVar(&r.Reviewer, "reviewer", "", "Reviewer identity")
	f.StringVar(&r.Harness, "harness", "", "Reviewer harness")
	f.StringVar(&r.Model, "model", "", "Reviewer model")
	f.StringVar(&r.Verdict, "verdict", "comment", "Review verdict (pass, rework, comment)")
	f.StringVarP(&r.Summary, "summary", "m", "", "Review summary")
	f.StringVar(&file, "findings-file", "", "Read a JSON array of findings from a file or - for stdin")
	return c
}

func saveReview(cmd *cobra.Command, r reviews.Review) error {
	s, err := reviews.Open(reviews.Root())
	if err != nil {
		return err
	}
	r, err = s.Create(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), r.ID, r.Verdict)
	return err
}

var lineRange = regexp.MustCompile(`^([0-9]+)(?:-([0-9]+))?$`)

func parseReviewLines(text string) (int, int, error) {
	if text == "" {
		return 0, 0, nil
	}
	match := lineRange.FindStringSubmatch(text)
	if match != nil {
		if match[2] == "" {
			match[2] = match[1]
		}
		start, e1 := strconv.Atoi(match[1])
		end, e2 := strconv.Atoi(match[2])
		if e1 == nil && e2 == nil && start > 0 && end >= start {
			return start, end, nil
		}
	}
	return 0, 0, errors.New("lines must be a positive inclusive START-END range")
}

func newReviewAdd() *cobra.Command {
	var f reviews.Finding
	var lines string
	c := &cobra.Command{
		Use: "add ID", Short: "Append an anchored finding", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start, end, err := parseReviewLines(lines)
			if err != nil {
				return err
			}
			f.StartLine, f.EndLine = start, end
			s, err := reviews.Open(reviews.Root())
			if err != nil {
				return err
			}
			added, err := s.Add(args[0], f)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), added.ID)
			return err
		},
	}
	c.Flags().StringVar(&f.File, "file", "", "Relative repository path")
	c.Flags().StringVar(&lines, "lines", "", "Inclusive range on the reviewed head (START-END)")
	c.Flags().StringVar(&f.Severity, "severity", "", "Finding severity (blocker, major, minor, nit)")
	c.Flags().StringVarP(&f.Body, "message", "m", "", "Finding body")
	return c
}

func newReviewResolve() *cobra.Command {
	var status, note string
	c := &cobra.Command{
		Use: "resolve ID FINDING-ID", Short: "Mark a finding fixed or waived", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := reviews.Open(reviews.Root())
			if err != nil {
				return err
			}
			return s.Resolve(args[0], args[1], status, note)
		},
	}
	c.Flags().StringVar(&status, "status", "", "Resolution status (fixed, waived)")
	c.Flags().StringVarP(&note, "message", "m", "", "Resolution note")
	return c
}

func newReviewShow() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use: "show ID", Short: "Show a review and its findings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := reviews.Open(reviews.Root())
			if err != nil {
				return err
			}
			r, err := s.Get(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return printReviewJSON(cmd.OutOrStdout(), r)
			}
			printReview(cmd.OutOrStdout(), r, true)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return c
}

func newReviewQuery(search bool) *cobra.Command {
	var filter reviews.Filter
	var asJSON bool
	c := &cobra.Command{Use: "ls", Short: "List reviews newest first", Args: cobra.NoArgs}
	f := c.Flags()
	f.StringVar(&filter.Repo, "repo", "", "Filter by repository")
	f.BoolVar(&filter.Open, "open", false, "Require an open finding")
	f.BoolVar(&asJSON, "json", false, "Print JSON")
	f.IntVar(&filter.Limit, "limit", 50, "Maximum results (1-200)")
	if search {
		c.Use, c.Short, c.Args = "search QUERY", "Search review summaries, findings, and paths", cobra.ExactArgs(1)
		f.StringVar(&filter.Path, "path", "", "Filter findings by path substring")
		f.StringVar(&filter.Severity, "severity", "", "Filter findings by severity")
	} else {
		f.StringVar(&filter.Order, "order", "", "Filter by dispatch order")
		f.StringVar(&filter.Head, "head", "", "Filter by full head SHA")
		f.StringVar(&filter.Verdict, "verdict", "", "Filter by verdict")
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		s, err := reviews.Open(reviews.Root())
		if err != nil {
			return err
		}
		var result []reviews.Review
		if search {
			result, err = s.Search(cmd.Context(), args[0], filter)
		} else {
			result, err = s.List(cmd.Context(), filter)
		}
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if asJSON {
			return printReviewJSON(out, result)
		}
		for _, r := range result {
			printReview(out, r, search)
		}
		return nil
	}
	return c
}

func printReviewJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printReview(w io.Writer, r reviews.Review, details bool) {
	fmt.Fprintf(w, "%s  %s  %s  %s  %s\n", r.ID, r.Verdict, r.Repo, r.HeadSHA, r.Summary)
	if !details {
		return
	}
	fmt.Fprintf(w, "Base: %s  Order: %s  PR: %s\n", r.BaseSHA, r.Order, r.PR)
	fmt.Fprintf(w, "Reviewer: %s  Harness: %s  Model: %s\n", r.Reviewer, r.Harness, r.Model)
	fmt.Fprintf(w, "Created: %s  Updated: %s\n", r.CreatedAt, r.UpdatedAt)
	for _, f := range r.Findings {
		location := f.File
		if f.StartLine != 0 {
			location += fmt.Sprintf(":%d-%d", f.StartLine, f.EndLine)
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s\n    %s\n", f.ID, f.Status, f.Severity, location, f.Body)
		if f.ResolvedAt != nil {
			fmt.Fprintf(w, "    Resolved %s  %s\n", f.ResolvedAt, f.ResolutionNote)
		}
	}
}
