package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/nodes"
	"github.com/hev/kit/internal/skillsources"
	"github.com/hev/kit/internal/version"
	"github.com/hev/kit/skills"
	"github.com/spf13/cobra"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skillHarness is a skills root (dir/skills) and the harness homes that read
// it. The shared ~/.agents/skills copy serves pi and OpenCode.
type skillHarness struct {
	name, dir string
	homes     []string
}

var skillHarnesses = []skillHarness{
	{"claude", ".claude", []string{".claude"}},
	{"codex", ".codex", []string{".codex"}},
	{"agents", ".agents", []string{".pi", filepath.Join(".config", "opencode")}},
}

// present reports the first installed harness home, if any.
func (h skillHarness) present(home string) string {
	for _, d := range h.homes {
		if fi, e := os.Stat(filepath.Join(home, d)); e == nil && fi.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return ""
}

// skillRoot is one harness's skills directory as this machine resolves it.
type skillRoot struct {
	skillHarness
	root string // home/dir/skills as named
	real string // root with every existing symlink resolved
	// aliasOf names an earlier root whose real directory contains this one:
	// writing or retiring here would act on that harness's copies.
	aliasOf string
}

// realPath resolves the longest existing prefix of p, so a skills root that
// does not exist yet still resolves through a linked harness home.
func realPath(p string) string {
	rest := ""
	for {
		if r, e := filepath.EvalSymlinks(p); e == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

func pathWithin(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// ownsSharedCopies reports whether hev already stamped a bundled skill in
// ~/.agents/skills; those copies stay current even after pi and OpenCode go.
func ownsSharedCopies(root string) bool {
	names, _ := fs.ReadDir(skills.FS, ".")
	for _, n := range names {
		dest := filepath.Join(root, n.Name())
		if fi, e := os.Lstat(dest); e != nil || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if _, e := os.Stat(filepath.Join(dest, ".kit-skill.json")); e == nil {
			return true
		}
	}
	return false
}

// activeSkillRoots lists the roots bundled skills go to, shared root first:
// it is written before any Codex copy it replaces is retired.
func activeSkillRoots(home string) []skillRoot {
	var roots []skillRoot
	for _, name := range []string{"agents", "claude", "codex"} {
		for _, h := range skillHarnesses {
			if h.name != name {
				continue
			}
			r := skillRoot{skillHarness: h, root: filepath.Join(home, h.dir, "skills")}
			if h.present(home) == "" && (h.name != "agents" || !ownsSharedCopies(r.root)) {
				continue
			}
			r.real = realPath(r.root)
			for _, prior := range roots {
				if pathWithin(r.real, prior.real) || pathWithin(prior.real, r.real) {
					r.aliasOf = prior.name
					break
				}
			}
			roots = append(roots, r)
		}
	}
	return roots
}

// sharedSkillsRoot is ~/.agents/skills when hev keeps a copy there.
func sharedSkillsRoot(home string) string {
	for _, r := range activeSkillRoots(home) {
		if r.name == "agents" {
			return r.root
		}
	}
	return ""
}

type skillStamp struct {
	Version string `json:"version"`
	Hash    string `json:"sha256"`
}
type skillState struct {
	Harness string `json:"harness"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Path    string `json:"path"`
}

func skillHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func inspectSkill(dest string, content []byte) string {
	if fi, e := os.Lstat(dest); e == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "locally edited"
	}
	b, e := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if os.IsNotExist(e) {
		return "missing"
	}
	if e != nil {
		return "locally edited"
	}
	var stamp skillStamp
	raw, e := os.ReadFile(filepath.Join(dest, ".kit-skill.json"))
	if os.IsNotExist(e) {
		if skillHash(b) == skillHash(content) {
			return "stale"
		}
		historical, _ := skills.FS.ReadFile(filepath.Base(dest) + "/historical-sha256.txt")
		for _, hash := range strings.Fields(string(historical)) {
			if hash == skillHash(b) {
				return "stale"
			}
		}
		return notHevs
	}
	if e != nil || json.Unmarshal(raw, &stamp) != nil {
		return "locally edited"
	}
	if stamp.Hash != skillHash(b) {
		return "locally edited"
	}
	if stamp.Version != version.Version || stamp.Hash != skillHash(content) {
		return "stale"
	}
	return "current"
}

// skillAudience is what this machine's dispatch access lets its agents do.
// Owner covers full access (and machines with no dispatch hosts); member covers
// joined restricted nodes. A machine targeting both kinds gets both.
type skillAudience struct{ Owner, Member bool }

var ownerAudience = skillAudience{Owner: true}

func (a skillAudience) String() string {
	switch {
	case a.Owner && a.Member:
		return "mixed"
	case a.Member:
		return "member"
	}
	return "owner"
}

// Bundled SKILL.md front matter may set metadata.audience to owner, member or
// all; omitted means every machine.
func skillTargeted(name string, content []byte, a skillAudience) (bool, error) {
	m, e := skillsources.Metadata(name, content)
	if e != nil {
		return false, e
	}
	if len(m["audience"]) == 0 {
		return true, nil
	}
	targeted := false
	for _, v := range m["audience"] {
		switch v {
		case "all":
			targeted = true
		case "owner":
			targeted = targeted || a.Owner
		case "member":
			targeted = targeted || a.Member
		default:
			return false, fmt.Errorf("%s: unknown audience %q", name, v)
		}
	}
	return targeted, nil
}

// detectSkillAudience classifies the configured dispatch hosts. A host is a
// member host when its name or SSH alias is a node joined with `hev nodes join`.
func detectSkillAudience(dispatchConfig string, joined []nodes.Joined) (skillAudience, error) {
	var doc struct {
		Hosts map[string]map[string]any `toml:"hosts"`
	}
	if _, e := toml.DecodeFile(dispatchConfig, &doc); os.IsNotExist(e) {
		return ownerAudience, nil
	} else if e != nil {
		return ownerAudience, fmt.Errorf("read %s: %w", dispatchConfig, e)
	}
	names := map[string]bool{}
	for _, j := range joined {
		names[j.Name] = true
	}
	var a skillAudience
	for name, h := range doc.Hosts {
		ssh, _ := h["ssh"].(string)
		local, _ := h["local"].(bool)
		if !local && (names[name] || names[ssh]) {
			a.Member = true
		} else {
			a.Owner = true
		}
	}
	if !a.Member {
		a.Owner = true
	}
	return a, nil
}

// machineSkillAudience never blocks an install: unreadable dispatch state keeps
// the owner skills every machine received before audiences existed.
func machineSkillAudience(home string, out io.Writer) skillAudience {
	joined, e := (nodes.Store{Dir: filepath.Join(home, ".config", "hev", "nodes")}).Joined()
	a := ownerAudience
	if e == nil {
		a, e = detectSkillAudience(dispatch.ConfigPath(""), joined)
	}
	if e != nil && out != nil {
		fmt.Fprintf(out, "  ! dispatch hosts unreadable (%v); using owner skills\n", e)
	}
	return a
}

// notHevs marks a same-name skill hev never stamped: the user's own, so it is
// reported but never replaced or removed, even with --force.
const notHevs = "not installed by hev"

func syncSkills(home string, audience skillAudience, repair, force bool, out io.Writer) ([]skillState, error) {
	names, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		return nil, err
	}
	var states []skillState
	roots := activeSkillRoots(home)
	// shared holds the skills ~/.agents/skills serves after its pass.
	shared := map[string]bool{}
	for _, r := range roots {
		if r.aliasOf != "" {
			// Same directory as an earlier root: acting again could retire the
			// copies that root just installed.
			if out != nil {
				fmt.Fprintf(out, "%s: %s is %s's skills directory; skipped\n", r.name, r.root, r.aliasOf)
			}
			continue
		}
		for _, skill := range names {
			if !skill.IsDir() {
				continue
			}
			content, e := skills.FS.ReadFile(skill.Name() + "/SKILL.md")
			if e != nil {
				return nil, e
			}
			targeted, e := skillTargeted(skill.Name(), content, audience)
			if e != nil {
				return nil, e
			}
			reason := "not for " + audience.String()
			// Codex reads ~/.agents/skills too and lists same-name skills twice.
			superseded := r.name == "codex" && shared[skill.Name()]
			if superseded {
				reason = "served by " + filepath.Join(home, ".agents", "skills", skill.Name())
			}
			dest := filepath.Join(r.root, skill.Name())
			state := inspectSkill(dest, content)
			if !targeted || superseded {
				// Retire bundled copies this machine's access cannot use or that the
				// shared copy already serves. Edited copies and symlinks stay unless forced.
				if state == "missing" {
					continue
				}
				if repair && state != notHevs && (state != "locally edited" || force) {
					// Remove a symlink itself, never its target.
					if e = os.RemoveAll(dest); e != nil {
						return nil, e
					}
					state = "removed"
				} else if state == "locally edited" || state == notHevs {
					state += ", " + reason
				} else {
					state = reason
				}
			} else if repair && state != "current" && state != notHevs && (state != "locally edited" || force) {
				// Remove a symlink itself, never its target.
				if e = os.RemoveAll(dest); e != nil {
					return nil, e
				}
				if e = os.MkdirAll(dest, 0755); e != nil {
					return nil, e
				}
				if e = os.WriteFile(filepath.Join(dest, "SKILL.md"), content, 0644); e != nil {
					return nil, e
				}
				raw, _ := json.Marshal(skillStamp{version.Version, skillHash(content)})
				if e = os.WriteFile(filepath.Join(dest, ".kit-skill.json"), raw, 0644); e != nil {
					return nil, e
				}
				if state == "missing" {
					state = "installed"
				} else {
					state = "repaired"
				}
			}
			if r.name == "agents" && targeted && state != "missing" && state != notHevs {
				shared[skill.Name()] = true
			}
			states = append(states, skillState{r.name, skill.Name(), state, dest})
			if out != nil {
				fmt.Fprintf(out, "%s/%s: %s\n", r.name, skill.Name(), state)
			}
		}
	}
	return states, nil
}

// installBundledSkills is the one bundled install path: `hev up`, `hev join`
// and `hev skills setup` share it, so audience and edit protection agree.
func installBundledSkills(home string, force bool, out io.Writer) (skillAudience, []skillState, error) {
	audience := machineSkillAudience(home, out)
	states, e := syncSkills(home, audience, true, force, out)
	return audience, states, e
}
func installSkills(home string) ([]string, error) {
	_, states, e := installBundledSkills(home, false, os.Stdout)
	var names []string
	for _, s := range states {
		if len(names) == 0 || names[len(names)-1] != s.Harness {
			names = append(names, s.Harness)
		}
	}
	return names, e
}

// writeSkillsRole sets [skills] role in place, preserving comments and other
// keys as the init config writer does. It reports whether the file changed.
func writeSkillsRole(path, role string) (bool, error) {
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return true, writeConfigFile(path, []byte("[skills]\nrole = \""+role+"\"\n"))
	}
	if e != nil {
		return false, e
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("config %s must be a regular file (not a symlink)", path)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return false, e
	}
	var current struct {
		Skills struct {
			Role string `toml:"role"`
		} `toml:"skills"`
	}
	if _, e = toml.Decode(string(raw), &current); e != nil {
		return false, fmt.Errorf("read config %s: %w", path, e)
	}
	if current.Skills.Role == role {
		return false, nil
	}
	patched, e := patchConfigStrings(raw, "skills", []string{"role"}, []string{role})
	if e != nil {
		return false, e
	}
	var check struct {
		Skills skillsources.Config `toml:"skills"`
	}
	if _, e = toml.Decode(string(patched), &check); e != nil || check.Skills.Role != role {
		return false, fmt.Errorf("cannot preserve config while updating skills role")
	}
	return true, writeConfigFile(path, patched)
}

// localDispatchHost names a dispatch host this machine runs itself, if any.
func localDispatchHost() (string, error) {
	var doc struct {
		Hosts map[string]struct {
			Local bool `toml:"local"`
		} `toml:"hosts"`
	}
	path := dispatch.ConfigPath("")
	if _, e := toml.DecodeFile(path, &doc); os.IsNotExist(e) {
		return "", nil
	} else if e != nil {
		return "", fmt.Errorf("read %s: %w", path, e)
	}
	var local []string
	for name, h := range doc.Hosts {
		if h.Local {
			local = append(local, name)
		}
	}
	sort.Strings(local)
	if len(local) == 0 {
		return "", nil
	}
	return local[0], nil
}

// runSkillsSetup prepares a machine's agents to send work: it only orchestrates
// the role setting, the bundled install and the source sync. It never starts
// capture or the daemon. An empty role keeps the configured one; with none
// configured it means sender, except on a machine that runs a dispatch host.
func runSkillsSetup(home, role string, force bool, out io.Writer) error {
	path := daemon.DefaultConfigPath()
	cfg, e := skillsources.Load(path)
	if e != nil {
		return e
	}
	if role == "" {
		role = cfg.Role
	}
	if role == "" {
		// A host's role picks which team skills install, so never guess it.
		if host, e := localDispatchHost(); e != nil {
			return e
		} else if host != "" {
			return fmt.Errorf("this machine runs dispatch host %q (local = true in %s); pass --role host, or --role sender if it only sends", host, dispatch.ConfigPath(""))
		}
		role = "sender"
	}
	if role != "sender" && role != "host" {
		return fmt.Errorf("skills role must be sender or host")
	}
	changed, e := writeSkillsRole(path, role)
	if e != nil {
		return e
	}
	cfg.Role = role
	note := "unchanged"
	if changed {
		note = "written to " + path
	}
	fmt.Fprintf(out, "role      %s (%s)\n", role, note)
	found := 0
	roots := map[string]skillRoot{}
	for _, r := range activeSkillRoots(home) {
		roots[r.name] = r
	}
	for _, h := range skillHarnesses {
		label := h.name
		if h.name == "agents" {
			label = "pi/OpenCode"
		}
		r, active := roots[h.name]
		dest := filepath.Join(home, h.dir, "skills")
		switch {
		case r.aliasOf != "":
			dest += " (same directory as " + r.aliasOf + "'s; written once)"
		case h.name == "codex" && roots["agents"].root != "":
			dest = roots["agents"].root + " (shared; Codex also reads it)"
		}
		if at := h.present(home); at != "" {
			found++
			fmt.Fprintf(out, "harness   %s: %s -> %s\n", label, at, dest)
			continue
		}
		var homes []string
		for _, d := range h.homes {
			homes = append(homes, filepath.Join(home, d))
		}
		if active {
			fmt.Fprintf(out, "shared    %s: no %s; keeping hev's copies current since Codex reads them too\n", dest, strings.Join(homes, " or "))
			continue
		}
		fmt.Fprintf(out, "missing   %s: no %s; start it once, then rerun hev skills setup\n", label, strings.Join(homes, " or "))
	}
	audience, states, e := installBundledSkills(home, force, nil)
	if e != nil {
		return e
	}
	switch audience.String() {
	case "member":
		fmt.Fprintln(out, "audience  member: every dispatch host is a joined node; owner-only skills are not installed")
	case "mixed":
		fmt.Fprintln(out, "audience  mixed: owner skills plus hev-member for joined nodes")
	default:
		fmt.Fprintln(out, "audience  owner")
	}
	kept := 0
	for _, s := range states {
		switch {
		case s.State == "current":
			fmt.Fprintf(out, "unchanged %s\n", s.Path)
		case strings.HasPrefix(s.State, "locally edited"):
			kept++
			fmt.Fprintf(out, "left      %s (%s)\n", s.Path, s.State)
		case strings.HasPrefix(s.State, notHevs):
			fmt.Fprintf(out, "left      %s (%s; rename or remove it to get hev's)\n", s.Path, s.State)
		default:
			fmt.Fprintf(out, "%-9s %s\n", s.State, s.Path)
		}
	}
	if kept > 0 {
		fmt.Fprintf(out, "kept      %d locally edited skill(s); rerun with --force to replace or remove them\n", kept)
	}
	if len(cfg.Sources) == 0 {
		fmt.Fprintf(out, "sources   none configured ([[skills.sources]] in %s)\n", path)
	}
	if e = skillsources.Run(home, cfg, true, out); e != nil {
		return fmt.Errorf("team skill sources: %w", e)
	}
	links, e := skillsources.Installed(home)
	if e != nil {
		return e
	}
	dests := make([]string, 0, len(links))
	for dest := range links {
		dests = append(dests, dest)
	}
	sort.Strings(dests)
	for _, dest := range dests {
		fmt.Fprintf(out, "source    %s -> %s\n", dest, links[dest])
	}
	if found == 0 {
		return fmt.Errorf("no Claude Code, Codex, pi or OpenCode home found under %s; start one once, then rerun hev skills setup", home)
	}
	return nil
}
func init() {
	c := &cobra.Command{Use: "skills", Short: "Check and repair bundled and source agent skills"}
	var role string
	var setupForce bool
	setup := &cobra.Command{Use: "setup", Short: "Set up agent skills on a sending machine (no daemon or capture)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		home, e := os.UserHomeDir()
		if e != nil {
			return e
		}
		return runSkillsSetup(home, role, setupForce, cmd.OutOrStdout())
	}}
	setup.Flags().StringVar(&role, "role", "", "Skills role: sender or host; omitted keeps the configured role, else sender (required on a machine that runs a dispatch host)")
	setup.Flags().BoolVar(&setupForce, "force", false, "Replace or remove locally edited bundled skills (symlink targets are untouched; skills hev never installed are always kept)")
	c.AddCommand(setup)
	for _, action := range []string{"check", "sync"} {
		repair := action == "sync"
		var force bool
		short := "Report bundled skill drift"
		if repair {
			short = "Install and repair bundled skills"
		}
		sub := &cobra.Command{Use: action, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			home, e := os.UserHomeDir()
			if e != nil {
				return e
			}
			cfg, e := skillsources.Load(daemon.DefaultConfigPath())
			if e != nil {
				return e
			}
			if e = skillsources.Run(home, cfg, repair, cmd.OutOrStdout()); e != nil {
				return e
			}
			states, e := syncSkills(home, machineSkillAudience(home, cmd.OutOrStdout()), repair, force, cmd.OutOrStdout())
			if e != nil {
				return e
			}
			if !repair {
				for _, state := range states {
					if state.State != "current" {
						return fmt.Errorf("bundled skills need synchronization")
					}
				}
			}
			return nil
		}}
		if repair {
			sub.Flags().BoolVar(&force, "force", false, "Replace locally edited skills (symlink targets are untouched; skills hev never installed are always kept)")
		}
		c.AddCommand(sub)
	}
	rootCmd.AddCommand(c)
}
