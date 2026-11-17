package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	edgeA = "0.7.0-edge.aaaaaaa"
	edgeB = "0.7.0-edge.bbbbbbb"
)

func TestUpdateEdgeVerifiedSideBySideAtomicSwitch(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish("0.6.2", publishOpt{noManifest: true})
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	u.Config.PostUpdate = []string{"systemctl --user restart my-wrapper"}

	r, err := u.Run(context.Background(), Request{Channel: ChannelEdge, SaveChannel: ChannelEdge})
	if err != nil {
		t.Fatal(err)
	}
	if r.Before != edgeA || r.After != edgeB || !r.Changed {
		t.Fatalf("report = %+v", r)
	}
	if got := h.running(t); got != edgeB {
		t.Fatalf("link runs %q, want %s", got, edgeB)
	}
	if !renameSwap {
		target, err := os.Readlink(h.link)
		if err != nil || target != filepath.Join(h.home, ".hev", "versions", edgeB, "hev") {
			t.Fatalf("link = %q, %v; want a symlink to the versioned build", target, err)
		}
	}
	// The replaced build is kept beside it for rollback.
	if b, err := os.ReadFile(filepath.Join(h.home, ".hev", "versions", edgeA, BinaryName(runtime.GOOS))); err != nil || !strings.Contains(string(b), edgeA) {
		t.Fatalf("previous build not kept: %v", err)
	}
	st, _ := u.LoadState()
	if st.Current != edgeB || st.Previous != edgeA || len(st.Links) != 1 {
		t.Fatalf("state = %+v", st)
	}
	if h.svc.calls != 1 {
		t.Fatalf("service restarted %d times, want 1", h.svc.calls)
	}
	if len(h.skills) != 1 || !strings.HasSuffix(h.skills[0], filepath.Join(edgeB, BinaryName(runtime.GOOS))) {
		t.Fatalf("skills synced from %v, want the new build", h.skills)
	}
	if len(h.hooks) != 1 || !strings.Contains(strings.Join(h.hookEnv[0], " "), "HEV_UPDATE_TO="+edgeB) {
		t.Fatalf("hooks = %v %v", h.hooks, h.hookEnv)
	}
	cfg, err := LoadConfig(os.Getenv("HEV_CONFIG"))
	if err != nil || cfg.Channel != ChannelEdge {
		t.Fatalf("channel not recorded: %+v %v", cfg, err)
	}
	// Again: nothing to do.
	u.Current = edgeB
	if r, err := u.Run(context.Background(), Request{Channel: ChannelEdge}); err != nil || r.Changed {
		t.Fatalf("second run = %+v, %v", r, err)
	}
}

func TestUpdateCheckReportsAndChangesNothing(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	r, err := u.Run(context.Background(), Request{Channel: ChannelEdge, Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Behind || r.Latest != edgeB || r.Changed {
		t.Fatalf("report = %+v", r)
	}
	if h.running(t) != edgeA || h.svc.calls != 0 {
		t.Fatal("--check changed the host")
	}
	if _, err := os.Stat(filepath.Join(h.home, ".hev", "versions")); !os.IsNotExist(err) {
		t.Fatalf("--check installed something: %v", err)
	}
}

func TestUpdateToNamedVersionAndReleaseFromChecksums(t *testing.T) {
	h := newHarness(t, edgeB)
	h.repo.publish("0.6.2", publishOpt{noManifest: true})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeB)
	// The newest release is older than this edge build: the channel alone
	// never moves a host backwards.
	r, err := u.Run(context.Background(), Request{Channel: ChannelRelease})
	if err != nil || r.Changed || !strings.Contains(r.Message, "newer than") {
		t.Fatalf("run = %+v, %v", r, err)
	}
	// Naming it does.
	r, err = u.Run(context.Background(), Request{To: "v0.6.2"})
	if err != nil {
		t.Fatal(err)
	}
	if r.After != "0.6.2" || h.running(t) != "0.6.2" {
		t.Fatalf("report = %+v, running %s", r, h.running(t))
	}
	if _, err := u.Run(context.Background(), Request{To: "0.6.2", Channel: ChannelEdge}); err == nil {
		t.Fatal("a release version on the edge channel was accepted")
	}
}

func TestUpdateRefusesDigestMismatch(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeB, publishOpt{tamper: true})
	u := h.updater(edgeA)
	_, err := u.Run(context.Background(), Request{Channel: ChannelEdge})
	mustErr(t, err, "digest mismatch")
	if h.running(t) != edgeA {
		t.Fatal("a build that failed verification was switched in")
	}
	if _, err := os.Stat(filepath.Join(h.home, ".hev", "versions", edgeB, BinaryName(runtime.GOOS))); !os.IsNotExist(err) {
		t.Fatalf("unverified build was unpacked: %v", err)
	}
	if h.svc.calls != 0 {
		t.Fatal("services restarted after a refused update")
	}
	st, _ := u.LoadState()
	if len(st.History) == 0 || st.History[len(st.History)-1].Result != "refused" {
		t.Fatalf("refusal not recorded: %+v", st.History)
	}
}

func TestUpdateRefusesInconsistentManifest(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeB, publishOpt{})
	// The manifest under edgeB's tag claims to be another build.
	h.repo.files[Tag(edgeB)+"/manifest.json"] = []byte(strings.ReplaceAll(string(h.repo.files[Tag(edgeB)+"/manifest.json"]), `"version":"`+edgeB, `"version":"0.7.0-edge.ccccccc`))
	_, err := h.updater(edgeA).Run(context.Background(), Request{To: edgeB})
	mustErr(t, err, "manifest names version")
	if h.running(t) != edgeA {
		t.Fatal("switched despite a bad manifest")
	}
}

func TestRollbackReturnsToReplacedBuild(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	if _, err := u.Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	u.Current = edgeB
	r, err := u.Run(context.Background(), Request{Rollback: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.After != edgeA || h.running(t) != edgeA {
		t.Fatalf("rollback = %+v, running %s", r, h.running(t))
	}
	st, _ := u.LoadState()
	if st.Current != edgeA || st.Previous != edgeB {
		t.Fatalf("state = %+v", st)
	}
	if h.svc.calls != 2 {
		t.Fatalf("service restarted %d times, want 2", h.svc.calls)
	}
	// Rolling back twice goes forward again; with no history there is nothing.
	if _, err := h.updater(edgeA).Run(context.Background(), Request{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	if h.running(t) != edgeB {
		t.Fatal("second rollback did not swap back")
	}
	fresh := newHarness(t, edgeA)
	_, err = fresh.updater(edgeA).Run(context.Background(), Request{Rollback: true})
	mustErr(t, err, "nothing to roll back to")
}

func TestFailedRestartRollsBack(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeB, publishOpt{})
	h.svc.fail = func() error {
		if h.running(t) == edgeB {
			return errors.New("hevd exited at start")
		}
		return nil
	}
	u := h.updater(edgeA)
	r, err := u.Run(context.Background(), Request{Channel: ChannelEdge})
	mustErr(t, err, "rolled back to "+edgeA)
	if !r.RolledBack || h.running(t) != edgeA {
		t.Fatalf("report = %+v, running %s", r, h.running(t))
	}
	if h.svc.calls != 2 {
		t.Fatalf("restart calls = %d, want the failed one and the one after rolling back", h.svc.calls)
	}
	if len(h.skills) != 0 || len(h.hooks) != 0 {
		t.Fatal("skills or hooks ran for a rolled-back update")
	}
	st, _ := u.LoadState()
	if st.Current != edgeA || st.History[len(st.History)-1].Result != "rolled back" {
		t.Fatalf("state = %+v", st)
	}
}

func TestAbsentServicesAreNotRestarted(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeB, publishOpt{})
	h.svc.present = false
	if _, err := h.updater(edgeA).Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	if h.svc.calls != 0 {
		t.Fatal("restarted a service that is not installed")
	}
}

func TestPackageManagerInstallLeftAlone(t *testing.T) {
	h := newHarness(t, "0.6.1")
	h.repo.publish("0.6.2", publishOpt{noManifest: true})
	cellar := filepath.Join(filepath.Dir(filepath.Dir(h.link)), "homebrew", "Cellar", "hev", "0.6.1", "bin")
	os.MkdirAll(cellar, 0o755)
	brewed := filepath.Join(cellar, "hev")
	os.WriteFile(brewed, fakeBinary("0.6.1"), 0o755)
	u := h.updater("0.6.1")
	u.Invoked = brewed
	_, err := u.Run(context.Background(), Request{})
	if !errors.Is(err, ErrManaged) {
		t.Fatalf("err = %v, want ErrManaged", err)
	}
	mustErr(t, err, "brew upgrade hev/tap/hev")
	if b, _ := os.ReadFile(brewed); !strings.Contains(string(b), "0.6.1") {
		t.Fatal("the Homebrew file was touched")
	}
	if PackageManager("/usr/local/Cellar/hev/0.6.2/bin/hev") != "Homebrew" || PackageManager("/nix/store/x-hev/bin/hev") != "Nix" || PackageManager(h.link) != "" {
		t.Fatal("PackageManager misdetects")
	}
}

func TestRenameSwapReplacesFileAndKeepsPrevious(t *testing.T) {
	saved := renameSwap
	renameSwap = true
	t.Cleanup(func() { renameSwap = saved })
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish(edgeB, publishOpt{})
	u := h.updater(edgeA)
	if _, err := u.Run(context.Background(), Request{Channel: ChannelEdge}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(h.link)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || h.running(t) != edgeB {
		t.Fatalf("link = %v %v, running %s", info, err, h.running(t))
	}
	u.Current = edgeB
	if _, err := u.Run(context.Background(), Request{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	if h.running(t) != edgeA {
		t.Fatal("rollback by rename did not restore the previous build")
	}
	if left, _ := filepath.Glob(h.link + ".*"); len(left) != 0 {
		t.Fatalf("left behind %v", left)
	}
}

func TestVersionsRefuseAnythingButAVersion(t *testing.T) {
	for _, bad := range []string{"", "latest", "../../etc/passwd", "https://example.com/hev", "0.7", "0.7.0-edge.", "0.7.0-rc1", "0.7.0-edge.zzzzzzz", "0.7.0;rm"} {
		if _, err := NormalizeVersion(bad); err == nil {
			t.Errorf("NormalizeVersion(%q) accepted", bad)
		}
	}
	if v, _ := NormalizeVersion("v0.7.1"); v != "0.7.1" || Tag(v) != "v0.7.1" {
		t.Error("release version")
	}
	if Tag(edgeA) != "edge-"+edgeA {
		t.Error("edge tag")
	}
	for _, c := range []struct {
		a, b string
		cmp  int
		ok   bool
	}{
		{"0.6.2", "0.7.0", -1, true}, {"0.7.0", edgeA, 1, true}, {edgeA, "0.6.2", 1, true},
		{edgeA, edgeB, 0, false}, {"dev", "0.7.0", 0, false}, {"0.7.1", "0.7.1", 0, true},
	} {
		if cmp, ok := Compare(c.a, c.b); cmp != c.cmp || ok != c.ok {
			t.Errorf("Compare(%s, %s) = %d %v", c.a, c.b, cmp, ok)
		}
	}
}

func TestEdgeLatestIsNewestBuild(t *testing.T) {
	h := newHarness(t, edgeA)
	h.repo.publish(edgeA, publishOpt{})
	h.repo.publish("0.6.2", publishOpt{noManifest: true})
	h.repo.publish(edgeB, publishOpt{publishedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)})
	b, err := h.repo.source().Latest(context.Background(), ChannelEdge)
	if err != nil || b.Version != edgeB || b.Sequence != 3 {
		t.Fatalf("latest edge = %+v, %v", b, err)
	}
	b, err = h.repo.source().Latest(context.Background(), ChannelRelease)
	if err != nil || b.Version != "0.6.2" || !b.FromChecksums {
		t.Fatalf("latest release = %+v, %v", b, err)
	}
}

func TestSetChannelKeepsTheRestOfTheConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	orig := "# my notes\n[layer]\nendpoint = \"x\" # keep\n\n[update]\nauto = true\npost_update = [\"echo hi\"]\n\n[capture]\nscan_interval = \"5m\"\n"
	os.WriteFile(path, []byte(orig), 0o600)
	if err := SetChannel(path, ChannelEdge); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil || c.Channel != ChannelEdge || !c.Auto || len(c.PostUpdate) != 1 {
		t.Fatalf("config = %+v %v", c, err)
	}
	if err := SetChannel(path, ChannelRelease); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "# my notes") || !strings.Contains(string(raw), "# keep") || strings.Count(string(raw), "channel =") != 1 {
		t.Fatalf("config rewritten:\n%s", raw)
	}
	empty := filepath.Join(t.TempDir(), "config.toml")
	if err := SetChannel(empty, ChannelEdge); err != nil {
		t.Fatal(err)
	}
	if c, _ := LoadConfig(empty); c.Channel != ChannelEdge {
		t.Fatal("channel not added to a new file")
	}
	if c, _ := LoadConfig(filepath.Join(t.TempDir(), "none.toml")); c.EffectiveChannel() != ChannelRelease || c.AutoEnabled() {
		t.Fatal("defaults are not release with auto off")
	}
}

// A host already on the newest build still records the channel it was told
// to follow (the first move to edge is done by hand, then this).
func TestChannelRecordedWhenAlreadyCurrent(t *testing.T) {
	h := newHarness(t, edgeB)
	h.repo.publish(edgeB, publishOpt{})
	r, err := h.updater(edgeB).Run(context.Background(), Request{Channel: ChannelEdge, SaveChannel: ChannelEdge})
	if err != nil || r.Changed {
		t.Fatalf("run = %+v, %v", r, err)
	}
	if c, _ := LoadConfig(os.Getenv("HEV_CONFIG")); c.Channel != ChannelEdge {
		t.Fatal("channel not recorded")
	}
}
