package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// EvictWorkspaceSlots visits a snapshot of slot caches from oldest to newest.
// In dry runs it counts eligible slots if pressure continues, without creating locks.
func EvictWorkspaceSlots(ctx context.Context, root string, dryRun bool, underPressure func() (bool, error)) (int, error) {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.New("cannot inspect workspace slot root")
	}
	pools, err := os.ReadDir(root)
	if err != nil {
		return 0, errors.New("cannot list workspace slot pools")
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var slots []candidate
	for _, pool := range pools {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !pool.IsDir() {
			continue
		}
		dir := filepath.Join(root, pool.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return 0, errors.New("cannot list workspace slots")
		}
		for _, entry := range entries {
			if !isWorkspaceSlot(entry) {
				continue
			}
			if info, err := entry.Info(); err == nil && info.IsDir() {
				slots = append(slots, candidate{filepath.Join(dir, entry.Name()), info.ModTime()})
			}
		}
	}
	sort.SliceStable(slots, func(i, j int) bool { return slots[i].mod.Before(slots[j].mod) })
	var removed int
	var evictionErr error
	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return removed, errors.Join(evictionErr, err)
		}
		pressure, err := underPressure()
		if err != nil {
			return removed, errors.Join(evictionErr, errors.New("could not recheck workspace slot disk space"))
		}
		if !pressure {
			break
		}
		evicted, err := evictWorkspaceSlot(slot.path, slot.mod, dryRun)
		if err != nil && evictionErr == nil {
			evictionErr = err
		}
		if evicted {
			removed++
		}
	}
	return removed, evictionErr
}

func evictWorkspaceSlot(slot string, modified time.Time, dryRun bool) (bool, error) {
	if dryRun {
		lock, err := os.OpenFile(slot+".lock", os.O_RDWR, 0)
		if err == nil {
			defer lock.Close()
			if platformops.LockFile(lock, true) != nil {
				return false, nil
			}
		} else if !os.IsNotExist(err) {
			return false, errors.New("cannot inspect workspace slot lock")
		}
	} else {
		release, err := platformops.LockPath(slot+".lock", true)
		if platformops.IsLockBusy(err) {
			return false, nil
		}
		if err != nil {
			return false, errors.New("cannot lock workspace slot for eviction")
		}
		// Leave the lock file in place so all leases keep locking the same inode.
		defer release()
	}
	info, err := os.Lstat(slot)
	// A lease may have completed since enumeration and refreshed the slot.
	if err != nil || !info.IsDir() || !info.ModTime().Equal(modified) {
		return false, nil
	}
	if !dryRun {
		if err := os.RemoveAll(slot); err != nil {
			return false, errors.New("cannot remove workspace slot cache")
		}
	}
	return true, nil
}
