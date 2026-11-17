package main

// The skills shelf (RFC 0018): hev skills add, hev skills shelf (list, diff,
// bump, index) and hev skills search.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/skillsources"
	"github.com/spf13/cobra"
)

// shelfEnv is where the shelf commands read and write; tests replace it.
var shelfEnv = func() (home, config string, err error) {
	home, err = os.UserHomeDir()
	return home, daemon.DefaultConfigPath(), err
}

func readConfigText(path string) (string, os.FileMode, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", 0600, nil
	}
	if err != nil {
		return "", 0, err
	}
	mode := os.FileMode(0600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return string(b), mode, nil
}

func writeConfigText(path, text string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".hev-new"
	if err := os.WriteFile(tmp, []byte(text), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// changeShelf writes new config text, syncs sources and re-indexes the
// shelf; a failed sync puts the previous config back.
func changeShelf(home, config, old, next string, mode os.FileMode, out, errOut io.Writer) error {
	if err := writeConfigText(config, next, mode); err != nil {
		return err
	}
	cfg, err := skillsources.Load(config)
	if err == nil {
		err = skillsources.Run(home, cfg, true, out)
	}
	if err != nil {
		if old == "" {
			_ = os.Remove(config)
		} else if rerr := writeConfigText(config, old, mode); rerr != nil {
			return fmt.Errorf("%w; restoring %s: %v", err, config, rerr)
		}
		return fmt.Errorf("%w (config left as it was)", err)
	}
	if n, where, err := indexShelf(home, cfg); err != nil {
		fmt.Fprintf(errOut, "warning: pinned, but the skills collection was not re-indexed: %v; run hev skills shelf index\n", err)
	} else {
		fmt.Fprintf(out, "indexed %d chunks into %s\n", n, where)
	}
	return nil
}

func runSkillsAdd(arg, kind, name string, out, errOut io.Writer) error {
	spec, err := skillsources.ParseShorthand(arg)
	if err != nil {
		return err
	}
	if name == "" {
		name = spec.DefaultName()
	}
	home, config, err := shelfEnv()
	if err != nil {
		return err
	}
	ref, commit, err := skillsources.ResolveRemote(spec.URL, spec.Ref)
	if err != nil {
		return err
	}
	old, mode, err := readConfigText(config)
	if err != nil {
		return err
	}
	next, err := skillsources.AddSourceText(old, skillsources.Source{Name: name, Kind: kind, URL: spec.URL, Ref: ref, Subdir: spec.Subdir, Commit: commit})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "pack %s (%s): %s %s @ %s, pinned %s\n", name, kind, spec.URL, spec.Subdir, ref, commit)
	if err := changeShelf(home, config, old, next, mode, out, errOut); err != nil {
		return err
	}
	fmt.Fprintf(out, "added %s to %s; installed, not active: enable skills per run with skills: [%s/SKILL]\n", name, config, name)
	return nil
}

// shelfRow is one pack as hev skills shelf shows it.
type shelfRow struct {
	Name     string   `json:"pack"`
	Kind     string   `json:"kind"`
	Commit   string   `json:"commit"`
	License  string   `json:"license"`
	Author   string   `json:"author,omitempty"`
	URL      string   `json:"url"`
	Subdir   string   `json:"subdir,omitempty"`
	Ref      string   `json:"ref,omitempty"`
	State    string   `json:"state"`
	Skills   []string `json:"skills"`
	Licenses []string `json:"skill_licenses,omitempty"`
}

func shelfRows(home string, cfg skillsources.Config) ([]shelfRow, error) {
	packs, err := skillsources.Packs(home)
	if err != nil {
		return nil, err
	}
	var rows []shelfRow
	for _, s := range cfg.Sources {
		if !s.Shelf() {
			continue
		}
		r := shelfRow{Name: s.Name, Kind: s.Kind, Commit: s.Commit, Ref: s.Ref, Subdir: s.Subdir, URL: s.URL, State: "not synced", Skills: []string{}}
		if r.URL == "" {
			r.URL = s.Path
		}
		r.Author, _ = skillsources.Attribution(s.URL)
		if p, ok := packs[s.Name]; ok {
			r.License = p.License
			if p.Commit == s.Commit {
				r.State = "installed"
				ids := map[string]bool{}
				for _, sk := range p.Skills {
					r.Skills = append(r.Skills, sk.Name)
					ids[sk.License] = true
				}
				if len(ids) > 1 {
					for id := range ids {
						r.Licenses = append(r.Licenses, id)
					}
					sort.Strings(r.Licenses)
				}
			} else {
				r.State = "installed at " + p.Commit[:12] + "; run hev skills sync"
			}
		}
		if _, home := skillsources.Attribution(s.URL); home != "" {
			r.URL = home + "/tree/" + s.Commit
			if s.Subdir != "" {
				r.URL += "/" + s.Subdir
			}
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func printShelf(rows []shelfRow, out io.Writer) {
	if len(rows) == 0 {
		fmt.Fprintln(out, "the shelf is empty: add a pack with hev skills add OWNER/REPO[//SUBDIR][@REF] --kind mode|tool|domain")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PACK\tKIND\tCOMMIT\tLICENSE\tAUTHOR\tSKILLS")
	for _, r := range rows {
		lic := r.License
		if lic == "" {
			lic = "-"
		}
		skills := fmt.Sprintf("%d", len(r.Skills))
		if len(r.Skills) > 0 {
			list := r.Skills
			more := ""
			if len(list) > 6 {
				list, more = list[:6], fmt.Sprintf(", +%d", len(r.Skills)-6)
			}
			skills += ": " + strings.Join(list, ", ") + more
		}
		if r.State != "installed" {
			skills = r.State
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, r.Kind, r.Commit[:12], lic, r.Author, skills)
	}
	tw.Flush()
	fmt.Fprintln(out)
	for _, r := range rows {
		line := fmt.Sprintf("%s: %s", r.Name, r.URL)
		if len(r.Licenses) > 0 {
			line += " (skill licences: " + strings.Join(r.Licenses, ", ") + ")"
		}
		fmt.Fprintln(out, line)
	}
}

// skillsIndexPath is the keyless equivalent of the Layer skills collection.
func skillsIndexPath(home string) string {
	return filepath.Join(home, ".local", "state", "hev-skills", "index.json")
}

// shelfChunks chunks every configured pack installed at its pinned commit.
func shelfChunks(home string, cfg skillsources.Config) ([]skillsources.Chunk, []string, error) {
	packs, err := skillsources.Packs(home)
	if err != nil {
		return nil, nil, err
	}
	var chunks []skillsources.Chunk
	var commits []string
	for _, s := range cfg.Sources {
		p, ok := packs[s.Name]
		if !s.Shelf() || !ok || p.Commit != s.Commit {
			continue
		}
		c, err := skillsources.Chunks(p)
		if err != nil {
			return nil, nil, fmt.Errorf("pack %s: %w", p.Name, err)
		}
		chunks = append(chunks, c...)
		commits = append(commits, p.Commit)
	}
	return chunks, commits, nil
}

// indexShelf writes the shelf's pinned snapshots into the skills collection:
// <namespace>-skills on Layer when this host has a key, else the local
// index file. Only the operator's own namespace is written.
func indexShelf(home string, cfg skillsources.Config) (int, string, error) {
	chunks, _, err := shelfChunks(home, cfg)
	if err != nil {
		return 0, "", err
	}
	t, err := openTarget("", nil)
	if err != nil {
		return 0, "", err
	}
	if t.Layer != nil {
		if len(chunks) == 0 {
			return 0, layer.SkillsNamespace(t.Layer.Namespace), nil
		}
		if _, err := t.Layer.WriteSkillChunks(chunks); err != nil {
			return 0, "", err
		}
		return len(chunks), "Layer namespace " + layer.SkillsNamespace(t.Layer.Namespace), nil
	}
	path := skillsIndexPath(home)
	b, err := json.Marshal(chunks)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(path+".tmp", b, 0600); err != nil {
		return 0, "", err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return 0, "", err
	}
	return len(chunks), path + " (no Layer key: lexical)", nil
}

// searchSkills queries the skills collection, scoped to the pins installed
// now, so rows from a pin that was bumped away never answer.
func searchSkills(home string, cfg skillsources.Config, query, pack string, top int) ([]skillsources.Chunk, bool, error) {
	_, commits, err := shelfChunks(home, cfg)
	if err != nil {
		return nil, false, err
	}
	if len(commits) == 0 {
		return nil, false, nil
	}
	t, err := openTarget("", nil)
	if err != nil {
		return nil, false, err
	}
	if t.Layer != nil {
		var filter any = []any{"commit", "In", commits}
		if pack != "" {
			filter = layer.And(filter, []any{"pack", "Eq", pack})
		}
		hits, err := t.Layer.SearchSkills(query, top, filter)
		return hits, false, err
	}
	b, err := os.ReadFile(skillsIndexPath(home))
	if os.IsNotExist(err) {
		return nil, true, fmt.Errorf("no skills index yet; run hev skills shelf index")
	}
	if err != nil {
		return nil, true, err
	}
	var rows []skillsources.Chunk
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, true, err
	}
	pinned := map[string]bool{}
	for _, c := range commits {
		pinned[c] = true
	}
	terms := strings.Fields(strings.ToLower(query))
	type scored struct {
		c skillsources.Chunk
		n int
	}
	var hits []scored
	for _, r := range rows {
		if !pinned[r.Commit] || (pack != "" && r.Pack != pack) {
			continue
		}
		text := strings.ToLower(r.Text)
		n := 0
		for _, t := range terms {
			n += strings.Count(text, t)
			if strings.Contains(strings.ToLower(r.Skill), t) {
				n += 5
			}
		}
		if n > 0 {
			hits = append(hits, scored{r, n})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].n > hits[j].n })
	var out []skillsources.Chunk
	for i := 0; i < len(hits) && i < top; i++ {
		out = append(out, hits[i].c)
	}
	return out, true, nil
}

func addShelfCommands(c *cobra.Command) {
	var addKind, addName string
	add := &cobra.Command{
		Use:   "add OWNER/REPO[//SUBDIR][@REF]",
		Short: "Put an upstream skill pack on the shelf, pinned to a commit (installed, not active)",
		Long: `Add a [[skills.sources]] shelf entry for a GitHub skill pack, pin it to the
commit its ref points at now, install it to the pinned store and index it.

Accepts the GitHub forms npx skills add does: owner/repo, owner/repo//subdir,
owner/repo/subdir, @ref (branch, tag or full commit), github.com/owner/repo,
https://github.com/owner/repo/tree/REF/subdir and git@github.com:owner/repo.git.

A shelf pack is installed, not active: it is not linked into any harness skill
root. A run enables its skills with skills: [pack/skill, ...] in the order.

  hev skills add cursor/plugins//pstack --kind mode
  hev skills add mattpocock/skills//skills --name mattpocock --kind tool`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillsAdd(args[0], addKind, addName, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	add.Flags().StringVar(&addKind, "kind", "tool", "Pack kind: mode (owns the workflow; one per run), tool or domain")
	add.Flags().StringVar(&addName, "name", "", "Pack name used in skills: [pack/skill] (default: from the subdir or repository)")
	c.AddCommand(add)

	var shelfJSON bool
	shelf := &cobra.Command{
		Use:   "shelf",
		Short: "List shelf packs: pinned commit, licence, author, kind and skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, config, err := shelfEnv()
			if err != nil {
				return err
			}
			cfg, err := skillsources.Load(config)
			if err != nil {
				return err
			}
			rows, err := shelfRows(home, cfg)
			if err != nil {
				return err
			}
			if shelfJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if rows == nil {
					rows = []shelfRow{}
				}
				return enc.Encode(rows)
			}
			printShelf(rows, cmd.OutOrStdout())
			return nil
		},
	}
	shelf.Flags().BoolVar(&shelfJSON, "json", false, "Print packs as JSON")

	var diffTo string
	var diffPatch bool
	diff := &cobra.Command{
		Use:   "diff PACK",
		Short: "Show upstream changes to a pack since its pin (moves nothing)",
		Long: `Fetch the pack's upstream and show the commits on its ref that touch the
pack since the pinned commit, a diffstat, and with --patch the full diff. This
is what a review reads before hev skills shelf bump moves the pin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, config, err := shelfEnv()
			if err != nil {
				return err
			}
			cfg, err := skillsources.Load(config)
			if err != nil {
				return err
			}
			_, err = skillsources.Diff(home, cfg, args[0], diffTo, diffPatch, cmd.OutOrStdout())
			return err
		},
	}
	diff.Flags().StringVar(&diffTo, "to", "", "Compare with this commit instead of the ref's tip")
	diff.Flags().BoolVar(&diffPatch, "patch", false, "Include the full diff")

	var bumpTo string
	bump := &cobra.Command{
		Use:   "bump PACK --to SHA",
		Short: "Move a pack's pin to a reviewed commit, reinstall and re-index it",
		Long: `Move one pack's pin to the commit given with --to. A ref name is refused:
the pin moves only to a commit someone reviewed (hev skills shelf diff PACK
--to SHA --patch shows what it brings). The commit must be on the pack's ref.
Nothing moves a pin on its own.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if bumpTo == "" {
				return fmt.Errorf("bump needs --to SHA: the pin moves only to a commit you reviewed (hev skills shelf diff %s --patch)", args[0])
			}
			home, config, err := shelfEnv()
			if err != nil {
				return err
			}
			cfg, err := skillsources.Load(config)
			if err != nil {
				return err
			}
			s, full, err := skillsources.ResolveBump(home, cfg, args[0], bumpTo)
			if err != nil {
				return err
			}
			if full == s.Commit {
				fmt.Fprintf(cmd.OutOrStdout(), "pack %s is already pinned to %s\n", s.Name, full)
				return nil
			}
			old, mode, err := readConfigText(config)
			if err != nil {
				return err
			}
			next, err := skillsources.SetCommitText(old, s.Name, full)
			if err != nil {
				return err
			}
			if err := changeShelf(home, config, old, next, mode, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pack %s: pin moved %s -> %s\n", s.Name, s.Commit[:12], full[:12])
			return nil
		},
	}
	bump.Flags().StringVar(&bumpTo, "to", "", "The reviewed commit SHA to pin (required)")

	index := &cobra.Command{
		Use:   "index",
		Short: "Index the shelf's pinned snapshots into the skills collection",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, config, err := shelfEnv()
			if err != nil {
				return err
			}
			cfg, err := skillsources.Load(config)
			if err != nil {
				return err
			}
			n, where, err := indexShelf(home, cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "indexed %d chunks into %s\n", n, where)
			return nil
		},
	}
	shelf.AddCommand(diff, bump, index)
	c.AddCommand(shelf)

	var searchPack string
	var searchTop int
	var searchJSON bool
	searchCmd := &cobra.Command{
		Use:   "search QUERY",
		Short: "Search the shelf's pinned skills (the skills collection)",
		Long: `Search the skills collection: every pinned shelf pack, chunked and keyed by
(source, commit, path), with licence and attribution on every hit. Only pins
installed now answer. With a Layer key this is a hybrid query over
<namespace>-skills; without one it is a lexical scan of the local index.
RFC 0007's hev traces search --in skills will name the same collection.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, config, err := shelfEnv()
			if err != nil {
				return err
			}
			cfg, err := skillsources.Load(config)
			if err != nil {
				return err
			}
			hits, lexical, err := searchSkills(home, cfg, strings.Join(args, " "), searchPack, searchTop)
			if err != nil {
				return err
			}
			if len(hits) > searchTop {
				hits = hits[:searchTop]
			}
			if searchJSON {
				if hits == nil {
					hits = []skillsources.Chunk{}
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			if lexical {
				fmt.Fprintln(cmd.ErrOrStderr(), "search is lexical (no Layer API key): it matches words, not meaning")
			}
			if len(hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing found")
				return nil
			}
			for _, h := range hits {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s/%s  %s  commit %s  %s  by %s\n", h.Pack, h.Skill, h.Kind, h.Commit[:12], h.License, h.Author)
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", oneParagraph(h.Text, 300))
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  (%s)\n", h.Path, h.Homepage)
			}
			return nil
		},
	}
	searchCmd.Flags().StringVar(&searchPack, "pack", "", "Only this pack")
	searchCmd.Flags().IntVar(&searchTop, "top", 5, "Hits to show")
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Print hits as JSON (full chunk text, attribution and licence)")
	c.AddCommand(searchCmd)
}
