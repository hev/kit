package dispatch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/platformops"
)

type WorkspaceConfig struct {
	Root            string `toml:"root"`
	Mode            string `toml:"mode"`
	Driver          string `toml:"driver"`
	AutoCompactDone bool   `toml:"auto_compact_done"`
}

func (w WorkspaceConfig) validate() error {
	switch w.Mode {
	case "", "auto", "oss", "pro":
	default:
		return errors.New("workspace mode must be auto, oss or pro")
	}
	switch w.Driver {
	case "", "cow", "git":
	default:
		return errors.New("workspace driver must be cow or git")
	}
	return nil
}

var errWorkspaceLicense = errors.New("workspace operation requires a active Kit Pro license (HEV_LICENSE)")

func workspacePro(w WorkspaceConfig) (bool, error) {
	if w.Mode == "oss" {
		return false, nil
	}
	licensed := workspaceEntitled(time.Now())
	if !licensed && w.Mode == "pro" {
		return false, errWorkspaceLicense
	}
	return licensed, nil
}

func dispatchWorkspaceCap() (int, error) {
	cfg, pro, err := workspacePolicy()
	if err != nil || !pro {
		return 0, err
	}
	return cfg.Cap, nil
}

func workspacePolicy() (Config, bool, error) {
	cfg, err := targetPolicy()
	if err != nil {
		return cfg, false, errors.New("cannot read receiver workspace configuration")
	}
	pro, err := workspacePro(cfg.WorkspacePolicy)
	return cfg, pro, err
}

func workspaceStatePath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, ".hev", name)
}

func workspaceEntitled(now time.Time) bool {
	return license.ReadProjection(now).Entitled()
}

func workspacePool(repo string) (string, error) {
	// Linked worktrees and repository aliases must agree on the same pool.
	common, err := command(repo, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", errors.New("cannot identify workspace repository")
	}
	common, err = filepath.EvalSymlinks(common)
	root := workspaceStatePath("workspace-slots")
	if err != nil || root == "" {
		return "", errors.New("cannot locate workspace slot pool")
	}
	hash := sha256.Sum256([]byte(common))
	return filepath.Join(root, hex.EncodeToString(hash[:])), nil
}

func leaseWorkspaceSlot(pool string, cap int) (string, *os.File, func(), error) {
	if err := os.MkdirAll(pool, 0700); err != nil {
		return "", nil, nil, errors.New("cannot create workspace slot pool")
	}
	for i := 0; i < cap; i++ {
		slot := filepath.Join(pool, "slot-"+strconv.Itoa(i))
		lock, err := os.OpenFile(slot+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return "", nil, nil, errors.New("cannot open workspace slot lock")
		}
		if err = platformops.LockFile(lock, true); err != nil {
			_ = lock.Close()
			if platformops.IsLockBusy(err) {
				continue
			}
			return "", nil, nil, errors.New("cannot lease workspace slot")
		}
		stamp := func() { _ = os.Chtimes(slot, time.Now(), time.Now()) }
		if err = os.MkdirAll(filepath.Join(slot, "cargo-target"), 0700); err != nil {
			_ = lock.Close()
			return "", nil, nil, errors.New("cannot initialize workspace slot")
		}
		stamp()
		return slot, lock, func() { stamp(); _ = lock.Close() }, nil
	}
	return "", nil, nil, errors.New("all workspace slots for this repository are leased")
}

func gcWorkspaceSlots(now time.Time) (int, error) {
	root := workspaceStatePath("workspace-slots")
	if root == "" {
		return 0, errors.New("workspace slot GC needs a home directory")
	}
	pools, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.New("cannot list workspace slot pools")
	}
	removed := 0
	for _, pool := range pools {
		if !pool.IsDir() {
			continue
		}
		dir := filepath.Join(root, pool.Name())
		slots, err := os.ReadDir(dir)
		if err != nil {
			return removed, errors.New("cannot list workspace slots")
		}
		for _, entry := range slots {
			if !isWorkspaceSlot(entry) {
				continue
			}
			slot := filepath.Join(dir, entry.Name())
			release, err := platformops.LockPath(slot+".lock", true)
			if platformops.IsLockBusy(err) {
				continue
			}
			if err != nil {
				return removed, errors.New("cannot lock workspace slot for GC")
			}
			info, statErr := os.Lstat(slot)
			if statErr == nil && info.IsDir() && now.Sub(info.ModTime()) > 7*24*time.Hour {
				err = os.RemoveAll(slot)
				if err == nil {
					removed++
				}
			}
			release()
			if err != nil {
				return removed, errors.New("cannot remove idle workspace slot")
			}
		}
	}
	return removed, nil
}

func cargoWorkspaceEnvironment(env []string, wt, slot string) []string {
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key == "CARGO_TARGET_DIR" || (runtime.GOOS == "windows" && strings.EqualFold(key, "CARGO_TARGET_DIR")) {
			return env
		}
	}
	if info, err := os.Stat(filepath.Join(wt, "Cargo.toml")); err == nil && info.Mode().IsRegular() {
		return append(env, "CARGO_TARGET_DIR="+filepath.Join(slot, "cargo-target"))
	}
	return env
}

func isWorkspaceSlot(entry os.DirEntry) bool {
	id, ok := strings.CutPrefix(entry.Name(), "slot-")
	i, err := strconv.Atoi(id)
	return entry.IsDir() && ok && err == nil && i >= 0 && strconv.Itoa(i) == id
}
