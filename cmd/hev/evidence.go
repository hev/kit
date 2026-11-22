package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/hostlabel"
	"github.com/spf13/cobra"
)

// evidenceNotice is printed (to stderr) by every OSS search.
const evidenceNotice = "Local evidence search matches words in titles, summaries, claims, captions, file names and text items (semantic: false). Images and video match only through their captions, names and claims; visual and semantic search needs Pro."

func init() { rootCmd.AddCommand(newEvidenceCommand()) }

// openEvidence is the local evidence folder with kit's hooks wired.
var openEvidence = func(cmd *cobra.Command) (*evidence.Store, error) {
	s, err := evidence.Open(evidence.Root())
	if err != nil {
		return nil, err
	}
	s.Observe = evidence.HookObserver(cmd.Context())
	return s, nil
}

func newEvidenceCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "evidence",
		Short: "Keep proof packages: screenshots, videos and text from validation runs",
		Long: `Evidence packages hold the proof a run produced: images (PNG, JPEG, GIF,
WebP), short videos (MP4, WebM, MOV up to 60s and 100 MiB) and text, with
optional claims. On this machine they are folders under ~/.hev/evidence
(HEV_EVIDENCE_DIR or [evidence] dir in ~/.hev/config.toml), one per package:
manifest.json and the item files beside it. Text and every manifest string
are scrubbed of secrets before they are written.

See https://hevkit.com/docs/cli.`,
		SilenceUsage: true,
	}
	root.AddCommand(evidencePutCmd(), evidenceCreateCmd(), evidenceAddCmd(), evidenceClaimCmd(), evidenceSealCmd(),
		evidenceLsCmd(), evidenceShowCmd(), evidenceGetCmd(), evidenceSearchCmd())
	for _, c := range root.Commands() {
		c.SilenceUsage, c.SilenceErrors = true, true
	}
	return root
}

type evidenceMetaFlags struct {
	title, summary, source, order, session, gate, repo, branch, commit, host, author, role string
	tags                                                                                   []string
}

func (f *evidenceMetaFlags) add(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.title, "title", "", "Package title (required)")
	fl.StringVar(&f.summary, "summary", "", "What the package shows")
	fl.StringVar(&f.source, "source", "", "validator, browser-use, computer-use, spike, manual or api")
	fl.StringVar(&f.order, "order", os.Getenv("HEV_ORDER"), "Dispatch order name (default $HEV_ORDER)")
	fl.StringVar(&f.session, "session", "", "Harness session id")
	fl.StringVar(&f.gate, "gate", "", "Gate name")
	fl.StringVar(&f.repo, "repo", "", "Repository")
	fl.StringVar(&f.branch, "branch", "", "Branch")
	fl.StringVar(&f.commit, "commit", "", "Commit")
	fl.StringVar(&f.host, "host", "", "Host (default this machine)")
	fl.StringVar(&f.author, "author", "", "Author")
	fl.StringVar(&f.role, "role", "", "agent or human")
	fl.StringArrayVar(&f.tags, "tag", nil, "Tag (repeatable)")
	_ = cmd.MarkFlagRequired("title")
}

func (f *evidenceMetaFlags) meta() evidence.Meta {
	host := f.host
	if host == "" {
		host = hostlabel.Self(os.Hostname)
	}
	return evidence.Meta{Title: f.title, Summary: f.summary, Source: f.source, Order: f.order, Session: f.session, Gate: f.gate,
		Repo: f.repo, Branch: f.branch, Commit: f.commit, Host: host, Author: f.author, Role: f.role, Tags: f.tags}
}

type evidenceItemFlags struct {
	message, name, caption, url, app, step string
	meta                                   []string
}

func (f *evidenceItemFlags) add(cmd *cobra.Command, captions bool) {
	fl := cmd.Flags()
	fl.StringVarP(&f.message, "message", "m", "", "Add TEXT as a text item")
	fl.StringVar(&f.name, "name", "", "File name for -m or stdin text")
	if captions {
		fl.StringVar(&f.caption, "caption", "", "What the item shows")
		fl.StringVar(&f.url, "url", "", "Page URL captured (browser-use)")
		fl.StringVar(&f.app, "app", "", "Application or window (computer-use)")
		fl.StringVar(&f.step, "step", "", "Step name")
		fl.StringArrayVar(&f.meta, "meta", nil, "KEY=VALUE metadata (repeatable, at most 16)")
	}
}

func (f *evidenceItemFlags) metadata() (map[string]string, error) {
	if len(f.meta) == 0 {
		return nil, nil
	}
	m := map[string]string{}
	for _, kv := range f.meta {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--meta %q is not KEY=VALUE", kv)
		}
		m[k] = v
	}
	return m, nil
}

// itemInput reads one item: a file path, "-" for stdin, or -m text. --name
// names stdin and -m text, and a file only when renameFile is set (add).
func (f *evidenceItemFlags) itemInput(cmd *cobra.Command, arg string, renameFile bool) (evidence.ItemInput, error) {
	in := evidence.ItemInput{Caption: f.caption, URL: f.url, App: f.app, Step: f.step}
	md, err := f.metadata()
	if err != nil {
		return in, err
	}
	in.Metadata = md
	switch {
	case arg == "-":
		b, err := evidence.ReadCapped(cmd.InOrStdin(), evidence.DefaultLimits.MaxAnyBytes())
		if err != nil {
			return in, err
		}
		in.Data, in.Name = b, defaultString(f.name, "stdin.txt")
	case arg != "":
		in.Path = arg
		if renameFile {
			in.Name = f.name
		}
	default:
		in.Data, in.Name = []byte(f.message), defaultString(f.name, "message.txt")
	}
	return in, nil
}

func stdinPiped(cmd *cobra.Command) bool {
	if cmd.InOrStdin() != os.Stdin {
		return true
	}
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice == 0
}

func writeEvidenceJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func evidenceErr(err error) error {
	switch {
	case errors.Is(err, evidence.ErrConflict):
		return fmt.Errorf("conflict: %w", err)
	case errors.Is(err, evidence.ErrIntegrity):
		return fmt.Errorf("refused: %w", err)
	}
	return err
}

func evidencePutCmd() *cobra.Command {
	var mf evidenceMetaFlags
	var itf evidenceItemFlags
	var claims, verdicts, criteria []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "put --title TITLE [flags] FILE...",
		Short: "Create and seal a package in one step",
		Long:  "Create and seal a package from FILEs (\"-\" reads stdin) and -m text. Every file is checked before anything is written; one refused file writes nothing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var items []evidence.ItemInput
			for _, a := range args {
				in, err := itf.itemInput(cmd, a, false)
				if err != nil {
					return err
				}
				items = append(items, in)
			}
			if itf.message != "" {
				in, _ := itf.itemInput(cmd, "", false)
				items = append(items, in)
			}
			cl, err := claimInputs(claims, verdicts, criteria)
			if err != nil {
				return err
			}
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, err := s.Put(mf.meta(), items, cl)
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintln(cmd.OutOrStdout(), p.ID)
			fmt.Fprintf(cmd.ErrOrStderr(), "sealed %d items, %d claims in %s\n", len(p.Items), len(p.Claims), s.Path(p.ID))
			return nil
		},
	}
	mf.add(cmd)
	itf.add(cmd, false)
	cmd.Flags().StringArrayVar(&claims, "claim", nil, "Claim text (repeatable)")
	cmd.Flags().StringArrayVar(&verdicts, "verdict", nil, "Verdict for the claim in the same position: pass, fail, inconclusive or info")
	cmd.Flags().StringArrayVar(&criteria, "criterion", nil, "Acceptance criterion for the claim in the same position")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

// claimInputs pairs --claim with --verdict and --criterion by position; a
// single --verdict or --criterion applies to every claim.
func claimInputs(claims, verdicts, criteria []string) ([]evidence.ClaimInput, error) {
	if len(claims) == 0 && (len(verdicts) > 0 || len(criteria) > 0) {
		return nil, fmt.Errorf("--verdict and --criterion describe a --claim; add one")
	}
	pick := func(vals []string, i int, flag string) (string, error) {
		switch {
		case len(vals) == 0:
			return "", nil
		case len(vals) == 1:
			return vals[0], nil
		case len(vals) == len(claims):
			return vals[i], nil
		}
		return "", fmt.Errorf("give one --%s, or one per --claim (%d claims, %d values)", flag, len(claims), len(vals))
	}
	var out []evidence.ClaimInput
	for i, c := range claims {
		v, err := pick(verdicts, i, "verdict")
		if err != nil {
			return nil, err
		}
		cr, err := pick(criteria, i, "criterion")
		if err != nil {
			return nil, err
		}
		out = append(out, evidence.ClaimInput{Text: c, Verdict: v, Criterion: cr})
	}
	return out, nil
}

func evidenceCreateCmd() *cobra.Command {
	var mf evidenceMetaFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create --title TITLE [flags]",
		Short: "Start an open package and print its id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, err := s.Create(mf.meta())
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintln(cmd.OutOrStdout(), p.ID)
			return nil
		},
	}
	mf.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

func evidenceAddCmd() *cobra.Command {
	var itf evidenceItemFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "add ID [FILE | - | -m TEXT]",
		Short: "Add one item to an open package",
		Long:  "Add a file, stdin (\"-\", or piped with no FILE) or -m TEXT to an open package. Text from -m or stdin is named by --name.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 2 {
				arg = args[1]
			}
			switch {
			case arg != "" && itf.message != "":
				return fmt.Errorf("give a FILE or -m TEXT, not both")
			case arg == "" && itf.message == "":
				if !stdinPiped(cmd) {
					return fmt.Errorf("give a FILE, -m TEXT, or pipe the item on stdin")
				}
				arg = "-"
			}
			in, err := itf.itemInput(cmd, arg, true)
			if err != nil {
				return err
			}
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, it, err := s.AddItem(args[0], in)
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", it.ID, it.Name, it.MIME, evidenceBytes(it.Size))
			return nil
		},
	}
	itf.add(cmd, true)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

func evidenceClaimCmd() *cobra.Command {
	var verdict, criterion string
	var items []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "claim ID TEXT",
		Short: "Add a claim to an open package",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, c, err := s.AddClaim(args[0], evidence.ClaimInput{Text: args[1], Verdict: verdict, Criterion: criterion, ItemIDs: items})
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintln(cmd.OutOrStdout(), c.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&verdict, "verdict", "", "pass, fail, inconclusive or info")
	cmd.Flags().StringVar(&criterion, "criterion", "", "The acceptance criterion the claim is about")
	cmd.Flags().StringArrayVar(&items, "item", nil, "Supporting item id (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

func evidenceSealCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "seal ID",
		Short: "Seal an open package; it can no longer change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, err := s.Seal(args[0])
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sealed %s: %d items, %d claims\n", p.ID, len(p.Items), len(p.Claims))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

type evidenceFilterFlags struct {
	order, session, repo, gate, author, state, since, until string
	sources, modalities, verdicts, tags                     []string
}

func (f *evidenceFilterFlags) add(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.order, "order", "", "Only this dispatch order")
	fl.StringVar(&f.session, "session", "", "Only this session")
	fl.StringVar(&f.repo, "repo", "", "Only this repository")
	fl.StringVar(&f.gate, "gate", "", "Only this gate")
	fl.StringVar(&f.author, "author", "", "Only this author")
	fl.StringVar(&f.state, "state", "", "open or sealed")
	fl.StringVar(&f.since, "since", "", "Created within (7d, 36h) or after an RFC 3339 time")
	fl.StringVar(&f.until, "until", "", "Created before an RFC 3339 time (or this long ago)")
	fl.StringArrayVar(&f.sources, "source", nil, "Source (repeatable: any of)")
	fl.StringArrayVar(&f.modalities, "modality", nil, "text, image or video (repeatable: any of)")
	fl.StringArrayVar(&f.verdicts, "verdict", nil, "Claim verdict (repeatable: any of)")
	fl.StringArrayVar(&f.tags, "tag", nil, "Tag (repeatable: all of)")
}

// evidenceTime reads 7d / 36h as that long before now, or an RFC 3339 time.
func evidenceTime(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	d, err := parseSinceDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a duration (7d, 36h) or an RFC 3339 time", s)
	}
	return now.Add(-d), nil
}

func (f *evidenceFilterFlags) filter() (evidence.Filter, error) {
	now := time.Now()
	since, err := evidenceTime(f.since, now)
	if err != nil {
		return evidence.Filter{}, fmt.Errorf("--since: %w", err)
	}
	until, err := evidenceTime(f.until, now)
	if err != nil {
		return evidence.Filter{}, fmt.Errorf("--until: %w", err)
	}
	fl := evidence.Filter{Order: f.order, Session: f.session, Repo: f.repo, Gate: f.gate, Author: f.author, State: f.state,
		Sources: f.sources, Modalities: f.modalities, Verdicts: f.verdicts, Tags: f.tags, Since: since, Until: until}
	return fl, fl.Validate()
}

func evidenceLsCmd() *cobra.Command {
	var ff evidenceFilterFlags
	var asJSON bool
	var limit int
	cmd := &cobra.Command{
		Use:   "ls [filters]",
		Short: "List packages, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := ff.filter()
			if err != nil {
				return err
			}
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			list, err := s.List(f)
			if err != nil {
				return err
			}
			if limit > 0 && len(list) > limit {
				list = list[:limit]
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), map[string]any{"data": list, "next_cursor": nil})
			}
			if len(list) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no evidence packages in %s\n", s.Root)
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tCREATED\tSTATE\tITEMS\tVERDICTS\tORDER\tTITLE")
			for _, p := range list {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.CreatedAt.Local().Format("2006-01-02 15:04"), p.State,
					itemCounts(p.ItemCount), verdictList(p.Claims), evidenceDash(p.Order), evidenceLine(p.Title, 60))
			}
			return tw.Flush()
		},
	}
	ff.add(cmd)
	cmd.Flags().IntVar(&limit, "limit", 50, "At most this many packages (0: all)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print {data: [EvidencePackageSummary], next_cursor}")
	return cmd
}

func itemCounts(c evidence.Counts) string {
	var parts []string
	for _, x := range []struct {
		n    int
		name string
	}{{c.Image, "img"}, {c.Video, "vid"}, {c.Text, "txt"}} {
		if x.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", x.n, x.name))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func verdictList(cs []evidence.Claim) string {
	var out []string
	for _, c := range cs {
		if c.Verdict != "" {
			out = append(out, c.Verdict)
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, ",")
}

func evidenceDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func evidenceLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func evidenceBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func evidenceShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show ID",
		Short: "Show one package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			p, err := s.Load(args[0])
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			printEvidence(cmd.OutOrStdout(), p, s.Path(p.ID))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

func printEvidence(w io.Writer, p *evidence.Package, dir string) {
	fmt.Fprintf(w, "%s  %s\n", p.ID, p.Title)
	fmt.Fprintf(w, "state    %s", p.State)
	if p.SealedAt != nil {
		fmt.Fprintf(w, " (sealed %s)", p.SealedAt.Local().Format(time.RFC3339))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "created  %s\n", p.CreatedAt.Local().Format(time.RFC3339))
	fmt.Fprintf(w, "folder   %s\n", dir)
	for _, kv := range [][2]string{{"source", p.Source}, {"order", p.Order}, {"session", p.Session}, {"gate", p.Gate}, {"repo", p.Repo},
		{"branch", p.Branch}, {"commit", p.Commit}, {"host", p.Host}, {"author", p.Author}, {"tags", strings.Join(p.Tags, ", ")}} {
		if kv[1] != "" {
			fmt.Fprintf(w, "%-8s %s\n", kv[0], kv[1])
		}
	}
	if p.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", p.Summary)
	}
	if len(p.Claims) > 0 {
		fmt.Fprintln(w, "\nClaims:")
		for _, c := range p.Claims {
			fmt.Fprintf(w, "  [%s] %s\n", evidenceDash(c.Verdict), c.Text)
			if c.Criterion != "" {
				fmt.Fprintf(w, "         criterion: %s\n", c.Criterion)
			}
		}
	}
	if len(p.Items) > 0 {
		fmt.Fprintln(w, "\nItems:")
		for _, it := range p.Items {
			extra := ""
			if it.Width > 0 {
				extra = fmt.Sprintf(" %dx%d", it.Width, it.Height)
			}
			if it.DurationMs != nil {
				extra += fmt.Sprintf(" %.1fs", float64(*it.DurationMs)/1000)
			}
			if it.LineCount != nil {
				extra += fmt.Sprintf(" %d lines", *it.LineCount)
			}
			fmt.Fprintf(w, "  %s  %-6s %s  %s%s\n", it.Name, it.Modality, it.MIME, evidenceBytes(it.Size), extra)
			if it.Caption != "" {
				fmt.Fprintf(w, "       %s\n", it.Caption)
			}
		}
	}
}

func evidenceGetCmd() *cobra.Command {
	var out string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "get ID [--out DIR]",
		Short: "Verify a package's files and print its folder, or copy it to DIR",
		Long:  "Check every item against the sha256 in manifest.json. With --out, copy the manifest and items into DIR; a changed or missing file fails and copies nothing.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			var p *evidence.Package
			dir := ""
			if out != "" {
				p, err = s.Export(args[0], out)
				dir = out
			} else {
				p, err = s.Verify(args[0])
				if p != nil {
					dir = s.Path(p.ID)
				}
			}
			if err != nil {
				return evidenceErr(err)
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), p)
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			fmt.Fprintf(cmd.ErrOrStderr(), "verified %d items against manifest.json\n", len(p.Items))
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Copy the package into DIR")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the package (EvidencePackage)")
	return cmd
}

func evidenceSearchCmd() *cobra.Command {
	var ff evidenceFilterFlags
	var also []string
	var image, video string
	var limit int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "search QUERY [--also PHRASING]... [filters]",
		Short: "Word search over package text (OSS: lexical, not semantic)",
		Long: `Find packages holding every word of QUERY, or of any --also phrasing, in
their titles, summaries, claims, captions, file names, provenance or text
items. Hits are one per package, ranked by distinct words matched, then
newest. Images and video are found through their captions, names and claims;
searching by an image or a video (--image, --video) needs Pro.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if image != "" || video != "" {
				return fmt.Errorf("searching by an image or a video needs Pro: visual and semantic evidence search runs in hev layer. Local search matches words; try a caption or file name instead")
			}
			if len(args) == 0 {
				return fmt.Errorf("give a QUERY")
			}
			f, err := ff.filter()
			if err != nil {
				return err
			}
			s, err := openEvidence(cmd)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), evidenceNotice)
			r, err := s.Search(cmd.Context(), evidence.SearchQuery{Q: args[0], Also: also, Filter: f, Limit: limit})
			if err != nil {
				return err
			}
			if asJSON {
				return writeEvidenceJSON(cmd.OutOrStdout(), r)
			}
			if len(r.Data) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no matches")
				return nil
			}
			w := cmd.OutOrStdout()
			for _, h := range r.Data {
				fmt.Fprintf(w, "%s  %s  [%s]  %s\n", h.Package.ID, h.Package.CreatedAt.Local().Format("2006-01-02"), itemCounts(h.Package.ItemCount), evidenceLine(h.Package.Title, 80))
				for _, m := range h.Matches {
					where := m.Target
					if m.Name != "" {
						where = m.Name
					}
					fmt.Fprintf(w, "    %s: %s\n", where, m.Snippet)
				}
			}
			return nil
		},
	}
	ff.add(cmd)
	cmd.Flags().StringArrayVar(&also, "also", nil, "Another phrasing (OR; up to 7)")
	cmd.Flags().StringVar(&image, "image", "", "Search by an image (Pro only)")
	cmd.Flags().StringVar(&video, "video", "", "Search by a video (Pro only)")
	cmd.Flags().IntVar(&limit, "limit", 50, "At most this many packages (max 200)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print EvidenceSearchResponse")
	return cmd
}
