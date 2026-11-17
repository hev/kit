package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/facets"
	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
)

// facetFlags are --repo, --host, --tool, --area and --client, each
// repeatable and comma-separable, on ls, search, post and reply.
type facetFlags map[string]*list

func addFacetFlags(fs *flag.FlagSet, what string) facetFlags {
	out := facetFlags{}
	for _, n := range facets.Names {
		l := &list{}
		fs.Var(l, n, fmt.Sprintf("%s %s (repeatable)", what, n))
		out[n] = l
	}
	return out
}

func (f facetFlags) set() facets.Set {
	var s facets.Set
	for _, n := range facets.Names {
		for _, v := range *f[n] {
			s.Add(n, strings.Split(v, ",")...)
		}
	}
	return s.Normal()
}

// query adds the facet parameters to an API query.
func (f facetFlags) query(q url.Values) {
	s := f.set()
	for _, n := range facets.Names {
		for _, v := range s.Get(n) {
			q.Add(n, v)
		}
	}
}

// inferFacets is what `post` guesses when the poster names none: the repo
// from the working directory's git remote and the client from a client
// folder (a path under a directory called "clients"). The server keeps a
// guess only when the taxonomy knows the value.
func inferFacets(dir string) facets.Set {
	var s facets.Set
	if repo := repoOf(dir); repo != "" {
		s.Add("repo", repo)
	}
	if client := clientOf(dir); client != "" {
		s.Add("client", client)
	}
	return s.Normal()
}

var gitRemote = func(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func repoOf(dir string) string {
	remote := gitRemote(dir)
	if remote == "" {
		return ""
	}
	remote = strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	if i := strings.LastIndexAny(remote, "/:"); i >= 0 {
		remote = remote[i+1:]
	}
	return strings.ToLower(remote)
}

func clientOf(dir string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "clients" && parts[i+1] != "" {
			return strings.ToLower(parts[i+1])
		}
	}
	return ""
}

func facetsLine(s *facets.Set) string {
	if s == nil {
		return ""
	}
	return "  {" + s.String() + "}"
}

const facetsUsage = `hev board facets — the five filterable subjects on thread rows

  hev board facets backfill --from INVENTORY.json [--taxonomy FILE] [--source NAME]
                            [--apply [--undo FILE]] [--check-org ORG] [--verbose] [--json]
  hev board facets restore --from UNDO.jsonl [--apply]
  hev board facets schema facets|text [--apply]

Every command previews by default and writes nothing; --apply writes.
backfill and restore open the board's own store (the Layer namespace, or
the board directory in filesystem mode), so run them where the board serves.
`

func runFacets(args []string) error {
	cmd := first(args)
	if len(args) > 0 {
		args = args[1:]
	}
	switch cmd {
	case "backfill":
		return runBackfill(args, os.Stdout)
	case "restore":
		return runRestore(args, os.Stdout)
	case "schema":
		return runSchema(args, os.Stdout)
	case "", "-h", "--help", "help":
		fmt.Print(facetsUsage)
		return nil
	}
	return fmt.Errorf("unknown facets command %q\n\n%s", cmd, facetsUsage)
}

// openStore is the store backfill and restore run on; tests replace it.
var openStore = store.Open

// inventoryRow is the part of an inventory row the backfill reads.
type inventoryRow struct {
	ID         int64      `json:"id"`
	Facets     facets.Set `json:"facets"`
	Confidence string     `json:"confidence"`
}

func readInventory(path string) ([]inventoryRow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []inventoryRow
	if err := json.Unmarshal(b, &rows); err != nil {
		var wrapped struct {
			Rows []inventoryRow `json:"rows"`
		}
		if err2 := json.Unmarshal(b, &wrapped); err2 != nil || wrapped.Rows == nil {
			return nil, fmt.Errorf("inventory %s: want a JSON array of rows (or {\"rows\": [...]}): %v", path, err)
		}
		rows = wrapped.Rows
	}
	return rows, nil
}

// backfillPlan is what a backfill would do, row by row.
type backfillPlan struct {
	Source    string           `json:"source"`
	Taxonomy  string           `json:"taxonomy"`
	Read      int              `json:"rows_read"`
	OnBoard   int              `json:"on_board"`
	Missing   []int64          `json:"not_on_board"`
	Empty     []int64          `json:"no_facets"`
	Kept      []int64          `json:"kept_set_by_person"`
	Unchanged []int64          `json:"unchanged"`
	Changes   []backfillChange `json:"changes"`
	Rejected  []backfillReject `json:"rejected"`
	Low       int              `json:"low_confidence"`
	Coverage  []facetCoverage  `json:"coverage"`
	Threads   int              `json:"threads_with_facets"`
	Applied   int              `json:"applied,omitempty"`
	Undo      string           `json:"undo,omitempty"`
}

type backfillChange struct {
	ID       int64       `json:"id"`
	From     *facets.Set `json:"from,omitempty"`
	FromBy   string      `json:"from_by,omitempty"`
	To       facets.Set  `json:"to"`
	By       string      `json:"by"`
	set      facets.Set
	previous facets.Set
}

type backfillReject struct {
	ID      int64           `json:"id"`
	Rejects []facets.Reject `json:"rejects"`
}

type facetCoverage struct {
	Facet    string `json:"facet"`
	Threads  int    `json:"threads"`
	Values   int    `json:"values"`
	Distinct int    `json:"distinct"`
}

// planBackfill compares each inventory row with the live thread. A row is
// rejected whole when any value fails the taxonomy or the bounds; nothing is
// truncated. A thread whose facets a person set (author, triage) is kept.
func planBackfill(ctx context.Context, st *store.Store, rows []inventoryRow, tax *facets.Taxonomy, source string, repos facets.RepoChecker) (*backfillPlan, error) {
	threads, err := st.Threads(ctx, store.ListOpts{Limit: 1 << 30})
	if err != nil {
		return nil, err
	}
	live := map[int64]store.Thread{}
	for _, t := range threads {
		live[t.ID] = t
	}
	p := &backfillPlan{Source: source, Taxonomy: tax.Path, Read: len(rows)}
	if p.Taxonomy == "" {
		p.Taxonomy = "kit's generic example"
	}
	written := map[int64]facets.Set{}
	for _, r := range rows {
		t, ok := live[r.ID]
		if !ok {
			p.Missing = append(p.Missing, r.ID)
			continue
		}
		p.OnBoard++
		set, rejects := tax.Check(r.Facets, repos)
		if len(rejects) > 0 {
			p.Rejected = append(p.Rejected, backfillReject{ID: r.ID, Rejects: rejects})
			continue
		}
		if set.Empty() {
			p.Empty = append(p.Empty, r.ID)
			continue
		}
		if facets.UserSet(t.FacetsBy) {
			p.Kept = append(p.Kept, r.ID)
			if t.Facets != nil {
				written[r.ID] = *t.Facets
			}
			continue
		}
		by := source
		if strings.EqualFold(r.Confidence, "low") {
			by += facets.LowSuffix
			p.Low++
		}
		if err := facets.CheckBy(by); err != nil {
			return nil, fmt.Errorf("--source: %v", err)
		}
		written[r.ID] = set
		var prev facets.Set
		if t.Facets != nil {
			prev = *t.Facets
		}
		if prev.Equal(set) && t.FacetsBy == by {
			p.Unchanged = append(p.Unchanged, r.ID)
			continue
		}
		p.Changes = append(p.Changes, backfillChange{ID: r.ID, From: t.Facets, FromBy: t.FacetsBy, To: set, By: by, set: set, previous: prev})
	}
	p.Threads = len(written)
	for _, n := range facets.Names {
		c := facetCoverage{Facet: n}
		distinct := map[string]bool{}
		for _, s := range written {
			vs := s.Get(n)
			if len(vs) > 0 {
				c.Threads++
			}
			c.Values += len(vs)
			for _, v := range vs {
				distinct[v] = true
			}
		}
		c.Distinct = len(distinct)
		p.Coverage = append(p.Coverage, c)
	}
	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].ID < p.Changes[j].ID })
	return p, nil
}

// undoRecord is one line of the undo file: a thread's facets before the
// backfill wrote them, and what the backfill wrote.
type undoRecord struct {
	ID       int64       `json:"id"`
	Facets   *facets.Set `json:"facets,omitempty"`
	FacetsBy string      `json:"facets_by"`
	Wrote    string      `json:"wrote_by"`
	At       string      `json:"at"`
}

// githubRepos checks a repository outside the taxonomy's list against the
// org with a read-only gh call.
func githubRepos(org, repo string) (bool, error) {
	out, err := exec.Command("gh", "api", "repos/"+org+"/"+repo, "--jq", ".name").CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Not Found") {
			return false, nil
		}
		return false, fmt.Errorf("gh api: %s", strings.TrimSpace(string(out)))
	}
	return strings.EqualFold(strings.TrimSpace(string(out)), repo), nil
}

func runBackfill(args []string, w io.Writer) error {
	c := flags("facets backfill")
	from := c.fs.String("from", "", "the inventory JSON to read facets from")
	taxPath := c.fs.String("taxonomy", settings.Current.Taxonomy, "the taxonomy file (default board.taxonomy, else kit's generic example)")
	source := c.fs.String("source", "", "recorded as facets_by (default inventory-YYYYMMDD from the file's date)")
	apply := c.fs.Bool("apply", false, "write the facets; without it nothing is written")
	undo := c.fs.String("undo", "", "with --apply: where to save each thread's previous facets (default ./board-facets-undo-TIMESTAMP.jsonl)")
	org := c.fs.String("check-org", "", "check repos outside the taxonomy's list against this GitHub org (read-only gh api)")
	verbose := c.fs.Bool("verbose", false, "list each change")
	if _, err := c.parse(args); err != nil {
		return err
	}
	if *from == "" {
		return errors.New("--from INVENTORY.json is required")
	}
	rows, err := readInventory(*from)
	if err != nil {
		return err
	}
	tax, err := facets.Load(*taxPath)
	if err != nil {
		return err
	}
	var repos facets.RepoChecker
	if *org != "" {
		tax.Org, repos = *org, githubRepos
	}
	if *source == "" {
		*source = "inventory"
		if fi, err := os.Stat(*from); err == nil {
			*source += "-" + fi.ModTime().UTC().Format("20060102")
		}
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	ctx := context.Background()
	plan, err := planBackfill(ctx, st, rows, tax, *source, repos)
	if err != nil {
		return err
	}
	if *apply && len(plan.Changes) > 0 {
		if *undo == "" {
			*undo = "board-facets-undo-" + time.Now().UTC().Format("20060102T150405Z") + ".jsonl"
		}
		if err := writeUndo(*undo, plan.Changes); err != nil {
			return err
		}
		plan.Undo = *undo
		for _, ch := range plan.Changes {
			if err := st.SetFacets(ctx, ch.ID, ch.set, ch.By); err != nil {
				printPlan(w, plan, *apply, *verbose, *c.json)
				return fmt.Errorf("#%d: %w (wrote %d of %d; undo file %s)", ch.ID, err, plan.Applied, len(plan.Changes), *undo)
			}
			plan.Applied++
		}
	}
	printPlan(w, plan, *apply, *verbose, *c.json)
	return nil
}

func writeUndo(path string, changes []backfillChange) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("undo file: %w", err)
	}
	at := time.Now().UTC().Format(time.RFC3339)
	enc := json.NewEncoder(f)
	for _, ch := range changes {
		if err := enc.Encode(undoRecord{ID: ch.ID, Facets: ch.previous.Ptr(), FacetsBy: ch.FromBy, Wrote: ch.By, At: at}); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func ids(xs []int64) string {
	if len(xs) == 0 {
		return ""
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("#%d", x)
	}
	return " (" + strings.Join(parts, " ") + ")"
}

func printPlan(w io.Writer, p *backfillPlan, apply, verbose, asJSON bool) {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(p)
		return
	}
	mode := "preview: nothing written (add --apply to write)"
	if apply {
		mode = fmt.Sprintf("applied: %d threads written", p.Applied)
	}
	fmt.Fprintf(w, "facets backfill, %s\n", mode)
	fmt.Fprintf(w, "source %s · taxonomy %s\n\n", p.Source, p.Taxonomy)
	fmt.Fprintf(w, "rows read            %d\n", p.Read)
	fmt.Fprintf(w, "on the board         %d\n", p.OnBoard)
	fmt.Fprintf(w, "not on the board     %d%s\n", len(p.Missing), ids(p.Missing))
	fmt.Fprintf(w, "would change         %d\n", len(p.Changes))
	fmt.Fprintf(w, "unchanged            %d\n", len(p.Unchanged))
	fmt.Fprintf(w, "kept (set by person) %d%s\n", len(p.Kept), ids(p.Kept))
	fmt.Fprintf(w, "no facets in row     %d%s\n", len(p.Empty), ids(p.Empty))
	fmt.Fprintf(w, "rejected             %d\n", len(p.Rejected))
	fmt.Fprintf(w, "low confidence       %d (listed on /review once written)\n\n", p.Low)
	fmt.Fprintf(w, "coverage after the backfill: %d threads with at least one facet\n", p.Threads)
	fmt.Fprintf(w, "  %-7s %8s %7s %9s\n", "facet", "threads", "values", "distinct")
	for _, c := range p.Coverage {
		fmt.Fprintf(w, "  %-7s %8d %7d %9d\n", c.Facet, c.Threads, c.Values, c.Distinct)
	}
	if len(p.Rejected) > 0 {
		fmt.Fprintf(w, "\nrejected rows (nothing is truncated; fix the inventory or extend the taxonomy):\n")
		for _, r := range p.Rejected {
			for _, x := range r.Rejects {
				fmt.Fprintf(w, "  #%d  %s\n", r.ID, x)
			}
		}
	}
	if verbose && len(p.Changes) > 0 {
		fmt.Fprintf(w, "\nchanges:\n")
		for _, ch := range p.Changes {
			from := "none"
			if ch.From != nil {
				from = ch.From.String()
			}
			fmt.Fprintf(w, "  #%d  %s [%s] -> %s [%s]\n", ch.ID, from, ch.FromBy, ch.To.String(), ch.By)
		}
	}
	if p.Undo != "" {
		fmt.Fprintf(w, "\nundo file: %s (hev board facets restore --from %s --apply)\n", p.Undo, p.Undo)
	}
}

// runRestore puts back the facets an undo file recorded. A thread a person
// has edited since (facets_by no longer what the backfill wrote) is left
// alone.
func runRestore(args []string, w io.Writer) error {
	c := flags("facets restore")
	from := c.fs.String("from", "", "the undo file a backfill --apply wrote")
	apply := c.fs.Bool("apply", false, "write; without it nothing is written")
	if _, err := c.parse(args); err != nil {
		return err
	}
	if *from == "" {
		return errors.New("--from UNDO.jsonl is required")
	}
	f, err := os.Open(*from)
	if err != nil {
		return err
	}
	defer f.Close()
	var recs []undoRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var r undoRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return fmt.Errorf("undo file: %w", err)
		}
		recs = append(recs, r)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var restore, skipped []undoRecord
	for _, r := range recs {
		t, err := st.Thread(ctx, r.ID)
		if err != nil {
			skipped = append(skipped, r)
			continue
		}
		if t.FacetsBy != r.Wrote {
			skipped = append(skipped, r)
			continue
		}
		restore = append(restore, r)
	}
	done := 0
	if *apply {
		for _, r := range restore {
			var prev facets.Set
			if r.Facets != nil {
				prev = *r.Facets
			}
			if err := st.SetFacets(ctx, r.ID, prev, r.FacetsBy); err != nil {
				return fmt.Errorf("#%d: %w (restored %d of %d)", r.ID, err, done, len(restore))
			}
			done++
		}
	}
	verb := "would restore"
	if *apply {
		verb = "restored"
	}
	fmt.Fprintf(w, "%s %d threads; %d left alone (edited since, or gone)\n", verb, len(restore), len(skipped))
	if !*apply {
		fmt.Fprintln(w, "preview: nothing written (add --apply to write)")
	}
	return nil
}

// schemaClient is the Layer client the schema writes use; tests replace it.
var schemaClient = func() (*layer.Client, error) {
	st, err := openStore()
	if err != nil {
		return nil, err
	}
	if st.Info().Mode != "layer" {
		return nil, nil
	}
	return layer.Load()
}

// runSchema is the two schema writes, each idempotent: `facets` declares the
// six facet attributes filterable; `text` declares title and body
// filterable:false. They are separate so a problem with the second cannot
// hold up the first.
func runSchema(args []string, w io.Writer) error {
	c := flags("facets schema")
	apply := c.fs.Bool("apply", false, "write the schema; without it nothing is written")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	var want map[string]any
	switch first(pos) {
	case "facets":
		want = layer.FacetSchema()
	case "text":
		want = layer.TextSchema()
	default:
		return fmt.Errorf("which write? hev board facets schema facets|text [--apply]")
	}
	cl, err := schemaClient()
	if err != nil {
		return err
	}
	if cl == nil {
		fmt.Fprintln(w, "the board is in filesystem mode: there is no schema to write")
		return nil
	}
	changes, err := cl.PlanSchema(want)
	if err != nil {
		return err
	}
	if *c.json {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"namespace": cl.Namespace, "changes": changes, "applied": *apply && len(changes) > 0})
	}
	if len(changes) == 0 {
		if !*c.json {
			fmt.Fprintf(w, "%s: schema already as declared; nothing to write\n", cl.Namespace)
		}
		return nil
	}
	if !*c.json {
		fmt.Fprintf(w, "%s: %d attributes to declare\n", cl.Namespace, len(changes))
		for _, ch := range changes {
			from, _ := json.Marshal(ch.From)
			to, _ := json.Marshal(ch.To)
			fmt.Fprintf(w, "  %-13s %s -> %s\n", ch.Attr, from, to)
		}
	}
	if !*apply {
		if !*c.json {
			fmt.Fprintln(w, "preview: nothing written (add --apply to write)")
		}
		return nil
	}
	if err := cl.ApplySchema(changes); err != nil {
		return err
	}
	if !*c.json {
		fmt.Fprintln(w, "written (schema only, no rows)")
	}
	return nil
}
