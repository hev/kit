package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Service is something kit installed that runs from the hev binary and must
// be restarted onto a new build: the capture daemon under launchd, systemd or
// the Windows Task Scheduler. Restart returns once the service is confirmed
// running again, or an error.
type Service interface {
	Name() string
	Present() bool
	Restart(ctx context.Context) error
}

// Updater moves one host between builds. Everything outside the host's own
// home (the source, the service managers, the shell) is a field, so tests run
// it against a temporary root, a local fake source and fake services.
type Updater struct {
	// Home holds .hev/update (state, lock) and .hev/versions (builds).
	Home   string
	Source Source
	GOOS   string
	GOARCH string
	// Current is the running build's version (version.Version).
	Current string
	// Invoked is the path this hev was run as; Exe is that path resolved.
	Invoked string
	Exe     string
	// Extra are further paths the host's services run hev from, switched
	// with Invoked when they hold a kit binary (hev up's ~/.local/bin/hev).
	Extra  []string
	Config Config
	// ConfigPath is the host's config file, where SaveChannel is recorded.
	ConfigPath string
	Services   []Service
	// Smoke runs a freshly installed binary and returns what `version` prints.
	Smoke func(ctx context.Context, bin string) (string, error)
	// SyncSkills refreshes bundled skills from the new binary; nil skips it.
	SyncSkills func(ctx context.Context, bin string) error
	// RunHook runs one post_update command.
	RunHook func(ctx context.Context, command string, env []string) ([]byte, error)
	// By names who asked, for the history: "cli", "daemon", "dispatch:<sender>".
	By  string
	Out io.Writer
	Now func() time.Time
}

// Request is everything a caller may ask for. A dispatch sender reaches this
// with Channel, To and Check only.
type Request struct {
	Channel  string
	To       string
	Check    bool
	Rollback bool
	// Self is the daemon updating itself: it does not restart its own
	// service (it re-executes), and records a pending switch to confirm.
	Self bool
	// SaveChannel records Channel in the host's config (hev update --channel).
	SaveChannel string
}

// Report says what happened, for the CLI, the daemon log and a dispatch reply.
type Report struct {
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Channel string   `json:"channel"`
	Latest  string   `json:"latest,omitempty"`
	Behind  bool     `json:"behind,omitempty"`
	Changed bool     `json:"changed,omitempty"`
	Message string   `json:"message,omitempty"`
	Steps   []string `json:"steps,omitempty"`
	// RolledBack is set when a failed restart put the previous build back.
	RolledBack bool `json:"rolled_back,omitempty"`
}

func (r *Report) step(format string, args ...any) {
	r.Steps = append(r.Steps, fmt.Sprintf(format, args...))
}

func (u *Updater) now() time.Time {
	if u.Now != nil {
		return u.Now()
	}
	return time.Now()
}

func (u *Updater) logf(format string, args ...any) {
	if u.Out != nil {
		fmt.Fprintf(u.Out, format+"\n", args...)
	}
}

// ErrManaged is returned when a package manager owns the binary.
var ErrManaged = errors.New("installed by a package manager")

// ErrUpToDate is not a failure: the host already runs the build.
var ErrUpToDate = errors.New("already up to date")

// Run performs one request under the host's update lock.
func (u *Updater) Run(ctx context.Context, q Request) (Report, error) {
	r := Report{Before: u.Current, After: u.Current, Channel: u.Config.EffectiveChannel()}
	if q.Channel != "" {
		if !ValidChannel(q.Channel) {
			return r, fmt.Errorf("unknown channel %q (want release or edge)", q.Channel)
		}
		r.Channel = q.Channel
	}
	if q.To != "" {
		v, err := NormalizeVersion(q.To)
		if err != nil {
			return r, err
		}
		if q.Channel != "" && VersionChannel(v) != q.Channel {
			return r, fmt.Errorf("%s is on the %s channel, not %s", v, VersionChannel(v), q.Channel)
		}
		q.To = v
	}
	unlock, err := u.lock()
	if err != nil {
		return r, err
	}
	defer unlock()
	st, err := u.LoadState()
	if err != nil {
		return r, fmt.Errorf("update state: %w", err)
	}
	if q.Rollback {
		return u.rollback(ctx, &st, q, r)
	}
	var b Build
	if q.To != "" {
		b, err = u.Source.Find(ctx, q.To)
	} else {
		b, err = u.Source.Latest(ctx, r.Channel)
	}
	if err != nil {
		return r, err
	}
	r.Latest = b.Version
	cmp, known := Compare(u.Current, b.Version)
	if !known && VersionChannel(b.Version) == ChannelEdge && VersionChannel(u.Current) == ChannelEdge {
		if mine, err := u.Source.Find(ctx, u.Current); err == nil && mine.Sequence > 0 && b.Sequence > 0 {
			cmp, known = compareInt(mine.Sequence, b.Sequence), true
		}
	}
	r.Behind = u.Current != b.Version && (!known || cmp < 0)
	if q.Check {
		switch {
		case u.Current == b.Version:
			r.Message = fmt.Sprintf("%s is the newest %s build", u.Current, r.Channel)
		case r.Behind:
			r.Message = fmt.Sprintf("%s is available on %s (this host runs %s)", b.Version, r.Channel, u.Current)
		default:
			r.Message = fmt.Sprintf("this host runs %s, newer than %s's %s", u.Current, r.Channel, b.Version)
		}
		return r, nil
	}
	if q.SaveChannel != "" && q.SaveChannel != u.Config.Channel {
		// The channel is the host's choice, recorded whether or not a build
		// moves now: a host already on the newest edge build still follows it.
		if err := SetChannel(u.configPath(), q.SaveChannel); err != nil {
			return r, fmt.Errorf("record the channel: %w", err)
		}
		u.Config.Channel = q.SaveChannel
		r.step("channel set to %s in %s", q.SaveChannel, u.configPath())
	}
	if u.Current == b.Version {
		r.Message = fmt.Sprintf("already on %s (%s)", b.Version, r.Channel)
		return r, nil
	}
	if q.To == "" && known && cmp > 0 {
		r.Message = fmt.Sprintf("this host runs %s, newer than the newest %s build %s; name one with --to to move to it", u.Current, r.Channel, b.Version)
		return r, nil
	}
	return u.install(ctx, &st, q, b, r)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// label names a build's directory under versions.
func (u *Updater) label(v string) string {
	if _, err := NormalizeVersion(v); err == nil {
		return v
	}
	return "dev-" + u.now().UTC().Format("20060102T150405Z")
}

func (u *Updater) binIn(label string) string {
	return filepath.Join(u.versionsDir(), label, BinaryName(u.GOOS))
}

// install fetches, verifies, installs beside the current build, switches,
// restarts and refreshes. Any failure before the switch leaves the host as it
// was; a failed restart after it switches back.
func (u *Updater) install(ctx context.Context, st *State, q Request, b Build, r Report) (Report, error) {
	links, err := u.links(st)
	if err != nil {
		return r, err
	}
	label := b.Version
	dir := filepath.Join(u.versionsDir(), label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return r, err
	}
	archive := filepath.Join(dir, ".download")
	a, err := u.Source.Fetch(ctx, b, u.GOOS, u.GOARCH, archive)
	if err != nil {
		u.record(st, Event{Action: "update", From: u.Current, To: b.Version, Channel: r.Channel, Result: "refused", Detail: err.Error()})
		_ = u.saveState(*st)
		return r, err
	}
	r.step("fetched %s, sha256 %s verified against the %s manifest", a.Name, a.SHA256[:12], b.Tag)
	bin := u.binIn(label)
	err = extract(archive, a.Name, BinaryName(u.GOOS), bin)
	os.Remove(archive)
	if err != nil {
		return r, fmt.Errorf("unpack %s: %w", a.Name, err)
	}
	if u.Smoke != nil {
		got, err := u.Smoke(ctx, bin)
		if err != nil || strings.TrimSpace(got) != b.Version {
			return r, fmt.Errorf("the new binary does not run as %s (got %q, %v); nothing was switched", b.Version, strings.TrimSpace(got), err)
		}
	}
	r.step("installed beside the current build at %s", bin)
	prev, err := u.preserve(st, links)
	if err != nil {
		return r, fmt.Errorf("keep the current build for rollback: %w", err)
	}
	if err := u.switchAll(links, bin); err != nil {
		return r, err
	}
	r.step("switched %s", strings.Join(links, ", "))
	from := u.Current
	st.Links, st.Previous, st.Current = links, prev, label
	if q.Self {
		st.Pending = &Pending{From: prev, To: label, At: u.now()}
	} else if err := u.restart(ctx, &r); err != nil {
		rerr := u.switchAll(links, u.binIn(prev))
		st.Current, st.Previous = prev, ""
		r.RolledBack = rerr == nil
		detail := err.Error()
		if rerr != nil {
			detail += "; switching back also failed: " + rerr.Error()
		} else {
			_ = u.restart(ctx, &Report{})
			r.step("rolled back to %s", prev)
		}
		u.record(st, Event{Action: "update", From: from, To: b.Version, Channel: r.Channel, Result: "rolled back", Detail: detail})
		_ = u.saveState(*st)
		if rerr != nil {
			return r, fmt.Errorf("%s after switching to %s, and switching back failed: %v", err, b.Version, rerr)
		}
		return r, fmt.Errorf("%s after switching to %s; rolled back to %s", err, b.Version, prev)
	}
	r.After, r.Changed, r.Behind = b.Version, true, false
	u.afterSwitch(ctx, &r, bin, from, b.Version, r.Channel)
	u.record(st, Event{Action: "update", From: from, To: b.Version, Channel: r.Channel, Result: "ok"})
	u.prune(st)
	if err := u.saveState(*st); err != nil {
		r.step("could not record the update: %v", err)
	}
	r.Message = fmt.Sprintf("updated %s -> %s (%s)", from, b.Version, r.Channel)
	return r, nil
}

func (u *Updater) configPath() string {
	if u.ConfigPath != "" {
		return u.ConfigPath
	}
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(u.Home, ".hev", "config.toml")
}

// restart restarts every present service, stopping at the first failure.
func (u *Updater) restart(ctx context.Context, r *Report) error {
	for _, s := range u.Services {
		if !s.Present() {
			continue
		}
		if err := s.Restart(ctx); err != nil {
			return fmt.Errorf("restart %s failed: %w", s.Name(), err)
		}
		r.step("restarted %s", s.Name())
	}
	return nil
}

// afterSwitch refreshes skills and runs the host's own hooks. Neither undoes
// the update: the binary is good and running; what failed is reported.
func (u *Updater) afterSwitch(ctx context.Context, r *Report, bin, from, to, channel string) {
	if u.SyncSkills != nil {
		if err := u.SyncSkills(ctx, bin); err != nil {
			r.step("bundled skills not refreshed: %v", err)
		} else {
			r.step("bundled skills refreshed")
		}
	}
	for _, c := range u.Config.PostUpdate {
		if u.RunHook == nil {
			break
		}
		env := []string{"HEV_UPDATE_FROM=" + from, "HEV_UPDATE_TO=" + to, "HEV_UPDATE_CHANNEL=" + channel}
		out, err := u.RunHook(ctx, c, env)
		if err != nil {
			r.step("post_update %q failed: %v: %s", c, err, strings.TrimSpace(string(out)))
		} else {
			r.step("post_update %q ran", c)
		}
	}
}

// rollback switches back to the build the last update replaced.
func (u *Updater) rollback(ctx context.Context, st *State, q Request, r Report) (Report, error) {
	if st.Previous == "" || len(st.Links) == 0 {
		return r, fmt.Errorf("nothing to roll back to: no update has been recorded on this host")
	}
	bin := u.binIn(st.Previous)
	if _, err := os.Stat(bin); err != nil {
		return r, fmt.Errorf("the previous build %s is gone: %w", st.Previous, err)
	}
	if q.Check {
		r.Latest = st.Previous
		r.Message = fmt.Sprintf("--rollback would move this host from %s back to %s", st.Current, st.Previous)
		return r, nil
	}
	from, to := st.Current, st.Previous
	if err := u.switchAll(st.Links, bin); err != nil {
		return r, err
	}
	r.step("switched %s back to %s", strings.Join(st.Links, ", "), to)
	st.Current, st.Previous = to, from
	if !q.Self {
		if err := u.restart(ctx, &r); err != nil {
			u.record(st, Event{Action: "rollback", From: from, To: to, Result: "restart failed", Detail: err.Error()})
			_ = u.saveState(*st)
			return r, fmt.Errorf("rolled back to %s, but %w", to, err)
		}
	}
	st.Pending = nil
	r.After, r.Changed = to, true
	u.afterSwitch(ctx, &r, bin, from, to, VersionChannel(to))
	u.record(st, Event{Action: "rollback", From: from, To: to, Result: "ok"})
	if err := u.saveState(*st); err != nil {
		return r, err
	}
	r.Message = fmt.Sprintf("rolled back %s -> %s", from, to)
	return r, nil
}

// preserve makes sure the build being replaced is under versions, so it can
// be switched back to, and returns its label.
func (u *Updater) preserve(st *State, links []string) (string, error) {
	cur := links[0]
	if resolved, err := filepath.EvalSymlinks(cur); err == nil {
		if rel, err := filepath.Rel(u.versionsDir(), resolved); err == nil && !strings.HasPrefix(rel, "..") && filepath.Dir(rel) != "." {
			return strings.Split(filepath.ToSlash(rel), "/")[0], nil
		}
		cur = resolved
	}
	if st.Current != "" && sameContent(cur, u.binIn(st.Current)) {
		return st.Current, nil
	}
	label := u.label(u.Current)
	dst := u.binIn(label)
	if sameContent(cur, dst) {
		return label, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return label, copyExecutable(cur, dst)
}

// links are the paths to switch: those recorded by an earlier update, or the
// path hev was run as plus any service path holding a kit binary. A path a
// package manager owns stops the update.
func (u *Updater) links(st *State) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}
	for _, l := range st.Links {
		if _, err := os.Lstat(l); err == nil {
			add(l)
		}
	}
	if len(out) == 0 {
		add(u.Invoked)
		if u.Invoked == "" {
			add(u.Exe)
		}
	}
	for _, p := range u.Extra {
		if _, err := os.Lstat(p); err == nil {
			add(p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cannot tell where hev is installed")
	}
	for _, l := range out {
		if pm := PackageManager(l); pm != "" {
			return nil, managedError(pm, l)
		}
	}
	return out, nil
}

// switchAll points every link at bin, undoing the ones already switched if a
// later one fails.
func (u *Updater) switchAll(links []string, bin string) error {
	type done struct{ link, back string }
	var undo []done
	for _, l := range links {
		back, err := switchLink(l, bin)
		if err != nil {
			for i := len(undo) - 1; i >= 0; i-- {
				_ = restoreLink(undo[i].link, undo[i].back)
			}
			return fmt.Errorf("switch %s: %w", l, err)
		}
		undo = append(undo, done{l, back})
	}
	for _, d := range undo {
		discardBackup(d.back)
	}
	return nil
}

// prune keeps the current and previous builds and the three most recent
// others (a long run may still be executing one), and removes the rest. A
// build still in use (Windows) is left for the next prune.
func (u *Updater) prune(st *State) {
	entries, err := os.ReadDir(u.versionsDir())
	if err != nil {
		return
	}
	type v struct {
		name string
		mod  time.Time
	}
	var vs []v
	for _, e := range entries {
		if !e.IsDir() || e.Name() == st.Current || e.Name() == st.Previous {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		vs = append(vs, v{e.Name(), info.ModTime()})
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].mod.After(vs[j].mod) })
	for i, x := range vs {
		if i >= 3 {
			_ = os.RemoveAll(filepath.Join(u.versionsDir(), x.name))
		}
	}
}

func (u *Updater) lock() (func(), error) {
	if err := os.MkdirAll(u.dir(), 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(u.dir(), "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			if info, serr := os.Stat(path); serr == nil && u.now().Sub(info.ModTime()) > 30*time.Minute {
				_ = os.Remove(path)
				return u.lock()
			}
			return nil, fmt.Errorf("another update is in progress on this host (%s)", path)
		}
		return nil, err
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { _ = os.Remove(path) }, nil
}

// extract writes the archive's single binary to dst (via a temporary file and
// a rename). Only the entry named exactly like the binary is read; nothing
// else in the archive is written anywhere.
func extract(archive, name, binary, dst string) error {
	var src io.Reader
	var closer func()
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name == binary && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				src, closer = rc, func() { rc.Close() }
				break
			}
		}
	} else {
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if strings.TrimPrefix(h.Name, "./") == binary && h.Typeflag == tar.TypeReg {
				src = tr
				break
			}
		}
	}
	if src == nil {
		return fmt.Errorf("no %s in the archive", binary)
	}
	if closer != nil {
		defer closer()
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".hev-new-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(src, maxArchive)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".hev-copy-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func sameContent(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	return err == nil && bytes.Equal(x, y)
}

// DefaultSmoke runs `bin version`.
func DefaultSmoke(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	return string(out), err
}

// DefaultRunHook runs a post_update command through the host's shell.
func DefaultRunHook(ctx context.Context, command string, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	} else {
		c = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	c.Env = append(os.Environ(), env...)
	return c.CombinedOutput()
}
