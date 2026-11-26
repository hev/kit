package kitcapture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/pkg/kitapi"
)

// ErrDrainBusy means another process owns this outbox's drain lock.
var ErrDrainBusy = errors.New("Club Kit outbox is already draining")

// BeginScan marks the initial scan across processes. The OS releases the
// marker if the daemon exits during capture.
func (o *Outbox) BeginScan() (end func()) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	unlock, _ := lockFile(ctx, o.scanPath())
	cancel()
	o.announce.Store(true)
	o.scanning.Store(true)
	if o.Notify != nil {
		o.Notify()
	}
	return sync.OnceFunc(func() {
		o.scanning.Store(false)
		if unlock != nil {
			unlock()
		}
		if o.Notify != nil {
			o.Notify()
		}
	})
}

func (o *Outbox) scanPath() string { return filepath.Join(o.dir, ".initial-scan.lock") }

func (o *Outbox) isScanning() bool {
	if o.scanning.Load() {
		return true
	}
	if _, err := os.Stat(o.scanPath()); err != nil {
		return false
	}
	unlock, err := platformops.LockPath(o.scanPath(), true)
	if unlock != nil {
		unlock()
	}
	return platformops.IsLockBusy(err)
}

func (o *Outbox) progressPath() string { return filepath.Join(o.dir, ".sync-progress") }

func (o *Outbox) savedProgress() kitapi.SyncProgress {
	var p kitapi.SyncProgress
	if raw, err := os.ReadFile(o.progressPath()); err == nil {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

// Progress reads the last published drain snapshot and the live file count.
func (o *Outbox) Progress() kitapi.SyncProgress {
	p := o.savedProgress()
	p.Scanning = o.isScanning()
	if names, err := o.Pending(); err == nil && len(names) != p.PendingBatches {
		if p.PendingBatches == 0 && !p.Scanning {
			p.TotalBatches = len(names)
		} else {
			p.TotalBatches += max(0, len(names)-p.PendingBatches)
		}
		p.PendingBatches = len(names)
		sessions := make(map[sessionKey]bool)
		for _, name := range names {
			if b, err := o.inspect(name); err == nil {
				for _, id := range b.sessions {
					sessions[id] = true
				}
			}
		}
		p.PendingSessions = len(sessions)
	}
	return p
}

func (o *Outbox) publish(p kitapi.SyncProgress) {
	body, _ := json.Marshal(p)
	_ = platformops.WriteFileAtomic(o.progressPath(), body, 0o600)
}

type drainQueue struct {
	known    map[string]*queuedBatch
	done     int
	progress kitapi.SyncProgress
}

func (q *drainQueue) refresh(ctx context.Context, o *Outbox) ([]*queuedBatch, error) {
	names, err := o.Pending()
	if err != nil {
		return nil, errors.New("could not list the Club Kit outbox")
	}
	files := make([]*queuedBatch, 0, len(names))
	live := make(map[string]*queuedBatch, len(names))
	sessions := make(map[sessionKey]bool)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b := q.known[name]
		if b == nil {
			b, err = o.inspect(name)
			if err != nil {
				return nil, errors.New("could not inspect a queued trace batch")
			}
		}
		for _, id := range b.sessions {
			sessions[id] = true
		}
		live[name] = b
		files = append(files, b)
	}
	q.known = live
	q.progress = kitapi.SyncProgress{
		Scanning:        o.isScanning(),
		PendingBatches:  len(files),
		PendingSessions: len(sessions),
		TotalBatches:    q.done + len(files),
	}
	return files, nil
}

// orderBatches preserves enqueue order for writes that share a session or
// row, and treats deletions as barriers. Independent writes can run in
// parallel, with the newest sessions preferred at each opportunity.
func orderBatches(files []*queuedBatch) {
	segment := 0
	prior := make(map[sessionKey]*queuedBatch)
	activity := make(map[sessionKey]int64)
	for _, b := range files {
		for _, k := range b.sessions {
			activity[k] = max(activity[k], b.activity)
		}
	}
	for _, b := range files {
		if b.kind == KindDelete {
			segment++
			clear(prior)
		}
		b.segment = segment
		if b.kind == KindDelete {
			segment++
			continue
		}
		for _, k := range b.sessions {
			b.activity = max(b.activity, activity[k])
		}
		for _, k := range b.orderKeys {
			if older := prior[k]; older != nil {
				older.after = append(older.after, b)
				b.waiting++
			}
			prior[k] = b
		}
	}
	slices.SortFunc(files, func(a, b *queuedBatch) int {
		if a.segment != b.segment {
			return a.segment - b.segment
		}
		if a.activity != b.activity {
			if a.activity > b.activity {
				return -1
			}
			return 1
		}
		if a.name > b.name {
			return -1
		}
		if a.name < b.name {
			return 1
		}
		return 0
	})
}

// nextUploads visits ready files in priority order, filling at most limit
// payloads. A dependency is released only after the preceding wave finishes.
func nextUploads(files []*queuedBatch, limit int) []upload {
	var wave []upload
	segment := -1
	for _, b := range files {
		if b.finished {
			continue
		}
		if segment == -1 {
			segment = b.segment
		}
		if b.segment != segment {
			break
		}
		if b.waiting != 0 {
			continue
		}
		placed := false
		for i := range wave {
			if wave[i].fits(b) {
				wave[i].add(b)
				placed = true
				break
			}
		}
		if !placed && len(wave) < limit {
			wave = append(wave, upload{files: []*queuedBatch{b}, bytes: b.bytes, rows: b.rows})
		}
	}
	return wave
}

// Drain processes the queue while holding the team drain lock. A caller can
// immediately report progress when another process owns the lock.
func (o *Outbox) Drain(ctx context.Context, c *kitapi.Client, progress func(DrainResult)) (DrainResult, error) {
	if err := ctx.Err(); err != nil {
		return DrainResult{}, err
	}
	unlock, err := platformops.LockPath(filepath.Join(o.dir, ".drain.lock"), true)
	if platformops.IsLockBusy(err) {
		return DrainResult{}, ErrDrainBusy
	}
	if err != nil {
		return DrainResult{}, errors.New("could not lock the Club Kit outbox")
	}
	defer unlock()

	previous := o.savedProgress()
	q := drainQueue{done: max(0, previous.TotalBatches-previous.PendingBatches)}
	files, err := q.refresh(ctx, o)
	res := DrainResult{Pending: len(files)}
	if err != nil {
		return res, err
	}
	starting := o.announce.Load()
	if len(files) == 0 && !starting && q.progress.Scanning == previous.Scanning && previous.PendingBatches == 0 {
		return res, nil
	}
	if !previous.Scanning && previous.PendingBatches == 0 {
		q.done = 0
		q.progress.TotalBatches = len(files)
	}
	o.publish(q.progress)
	if o.team == "" {
		return res, ErrNoTeam
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	who, err := c.Whoami(checkCtx)
	cancel()
	if err != nil {
		return res, safeDrainError(err)
	}
	if who.Team.ID != o.team {
		return res, ErrTeamMismatch
	}
	if starting && o.announce.Swap(false) {
		start := q.progress
		start.Scanning = true
		c.ReportSync(ctx, start)
	}
	// Capture can queue more files while identity and the first heartbeat
	// are in flight. Refresh before selecting the first upload.
	files, err = q.refresh(ctx, o)
	if err != nil {
		return res, err
	}
	res.Pending = len(files)
	o.publish(q.progress)
	probe := true
	for len(files) != 0 {
		orderBatches(files)
		for {
			limit := drainWorkers
			if probe {
				limit = 1 // Check service availability before opening the window.
			}
			wave := nextUploads(files, limit)
			if len(wave) == 0 {
				break
			}
			probe = false
			workErr := o.drainWave(ctx, c, wave, &q, &res, progress)
			if workErr != nil {
				return res, workErr
			}
		}
		files, err = q.refresh(ctx, o)
		if err != nil {
			return res, err
		}
	}
	o.publish(q.progress)
	// Report emptiness even mid-scan; a later kick reports any new work.
	if res.Sent+res.Failed > 0 || previous.PendingBatches > 0 ||
		(!starting && q.progress.Scanning != previous.Scanning) || (starting && !q.progress.Scanning) {
		c.ReportSync(ctx, q.progress)
	}
	return res, nil
}

func (o *Outbox) drainWave(ctx context.Context, c *kitapi.Client, wave []upload, q *drainQueue, res *DrainResult, progress func(DrainResult)) error {
	type outcome struct {
		uploadResult
		err error
	}
	results := make([]outcome, len(wave))
	p := q.progress
	var wg sync.WaitGroup
	for i, job := range wave {
		wg.Go(func() {
			results[i].uploadResult, results[i].err = o.post(ctx, c, job.files, p)
		})
	}
	wg.Wait()
	var firstErr error
	var sent, refused int
	for i, result := range results {
		res.Sent += result.sent
		res.Failed += result.failed
		res.Rows += result.rows
		sent += result.sent
		refused += result.failed
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		for _, b := range wave[i].files {
			b.finished = true
			for _, dependent := range b.after {
				dependent.waiting--
			}
		}
	}
	q.done += sent + refused
	if refused > 0 {
		if err := syncDir(filepath.Join(o.dir, "failed")); err != nil {
			return errors.New("could not sync the failed trace outbox")
		}
	}
	if sent+refused > 0 {
		if err := syncDir(o.dir); err != nil {
			return errors.New("could not sync the Club Kit outbox")
		}
	}
	if _, err := q.refresh(ctx, o); err != nil {
		return err
	}
	res.Pending = q.progress.PendingBatches
	o.publish(q.progress)
	if progress != nil {
		progress(*res)
	}
	return firstErr
}

// FormatProgress renders the same source file counts for both login commands.
func FormatProgress(p kitapi.SyncProgress) string {
	done := max(0, p.TotalBatches-p.PendingBatches)
	percent := 0
	if p.TotalBatches > 0 {
		percent = 100 * done / p.TotalBatches
	}
	return fmt.Sprintf("Syncing: %d / %d batches (%d%%), scanning=%t, %d sessions pending",
		done, p.TotalBatches, percent, p.Scanning, p.PendingSessions)
}
