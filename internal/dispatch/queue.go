package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Queue entries live beside run directories, under the same receiver root lock.
// Persist requests, never credentials or resolved launch environment values.
type queueEntry struct {
	Request  Request   `json:"request"`
	Enqueued time.Time `json:"enqueued"`
}

func queuePath(name string) string { return filepath.Join(Root(), ".queue", name+".json") }
func queueEntries() ([]queueEntry, error) {
	entries := []queueEntry{}
	files, err := os.ReadDir(filepath.Join(Root(), ".queue"))
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		var e queueEntry
		if err := readJSON(filepath.Join(Root(), ".queue", f.Name()), &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Enqueued.Equal(entries[j].Enqueued) {
			return entries[i].Request.Name < entries[j].Request.Name
		}
		return entries[i].Enqueued.Before(entries[j].Enqueued)
	})
	return entries, nil
}

// enqueue is called with the root lock held. Validate before accepting custody.
func enqueue(q Request) (Response, error) {
	o, err := Parse(q.Order)
	if err != nil {
		return Response{}, err
	}
	if o.Name != q.Name || q.Meta.Name != q.Name || q.Meta.Parent != "" {
		return Response{}, fmt.Errorf("invalid queued order")
	}
	if exists(filepath.Join(Root(), q.Name)) || exists(queuePath(q.Name)) {
		return Response{}, fmt.Errorf("%w: %s", errRunExists, q.Name)
	}
	cfg, err := targetPolicy()
	if err != nil {
		return Response{}, err
	}
	workspace := cfg.Workspace
	if workspace == "" {
		workspace = q.Workspace
	}
	if err := admitRepo(cfg.Admission, resolveRepo(o.Repo, workspace), o.Repo, o.Host); err != nil {
		return Response{}, err
	}
	q.Meta.Order = o
	entries, err := queueEntries()
	if err != nil {
		return Response{}, err
	}
	q.Meta.Start = time.Now().UTC()
	// Serialize FIFO order even if the receiver wall clock moves backwards.
	if len(entries) > 0 && !q.Meta.Start.After(entries[len(entries)-1].Enqueued) {
		q.Meta.Start = entries[len(entries)-1].Enqueued.Add(time.Nanosecond)
	}
	q.Meta.LaunchEnv = nil
	q.Meta.LaunchResolved = false
	q.Meta.HarnessWrapper = nil
	if err := os.MkdirAll(filepath.Dir(queuePath(q.Name)), 0700); err != nil {
		return Response{}, err
	}
	if err := WriteJSON(queuePath(q.Name), queueEntry{q, q.Meta.Start}); err != nil {
		return Response{}, err
	}
	return Response{Message: fmt.Sprintf("queued %s (position %d)", q.Name, len(entries)+1)}, nil
}
func controlQueued(q Request) (Response, bool, error) {
	unlock, err := lockRoot()
	if err != nil {
		return Response{}, false, err
	}
	defer unlock()
	entries, err := queueEntries()
	if err != nil {
		return Response{}, false, err
	}
	for i, e := range entries {
		if e.Request.Name != q.Name {
			continue
		}
		r := Response{Message: fmt.Sprintf("queued %s (position %d): waiting for a free host slot and earlier FIFO orders", q.Name, i+1), Files: map[string]string{"order.md": e.Request.Order}}
		if q.Action == "stop" {
			err = os.Remove(queuePath(q.Name))
			r.Message = "cancelled queued " + q.Name
		}
		return r, true, err
	}
	return Response{}, false, nil
}

// SweepQueue is safe to call concurrently from hevd, a scheduled task, and
// finishing supervisors. Handle rechecks the FIFO head and capacity under lock.
// A dead supervisor has no reservation; queued work needs no surviving parent.
func SweepQueue() error {
	if !exists(filepath.Join(Root(), ".queue")) {
		return nil
	}
	for {
		unlock, err := lockRoot()
		if err != nil {
			return err
		}
		// Reap harnesses whose supervisor crashed so the backstop does not
		// overlap orphaned work with the next order.
		dirs, scanErr := os.ReadDir(Root())
		if scanErr != nil {
			unlock()
			return scanErr
		}
		for _, d := range dirs {
			dir := filepath.Join(Root(), d.Name())
			if d.IsDir() && ValidName(d.Name()) && !supervisorLive(dir) && !processRecordLive(filepath.Join(dir, "admission.json")) && processRecordLive(filepath.Join(dir, "harness.json")) {
				if err := stopHarness(dir); err != nil {
					unlock()
					return err
				}
				_ = os.Remove(filepath.Join(dir, "harness.json"))
			}
		}
		entries, err := queueEntries()
		if err == nil && len(entries) > 0 {
			// A receiver can die between creating a run and removing its queue
			// entry. Preserve that run for inspection and advance the FIFO once
			// its admission owner is gone; never launch the same name twice.
			dir := filepath.Join(Root(), entries[0].Request.Name)
			if exists(dir) && !processRecordLive(filepath.Join(dir, "admission.json")) {
				if !supervisorLive(dir) {
					var m Meta
					if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
						if err = WriteJSON(filepath.Join(dir, "meta.json"), entries[0].Request.Meta); err != nil {
							unlock()
							return err
						}
					}
					var result Result
					if readJSON(filepath.Join(dir, "result.json"), &result) != nil || result.End.IsZero() {
						result = Result{AdmissionFailed: true, ExitCode: 1, Start: entries[0].Enqueued, End: time.Now().UTC(), Error: "receiver interrupted during queue admission; inspect before rework"}
						if err = WriteJSON(filepath.Join(dir, "result.json"), result); err != nil {
							unlock()
							return err
						}
					}
				}
				err = os.Remove(queuePath(entries[0].Request.Name))
				unlock()
				if err != nil {
					return err
				}
				continue
			}
		}
		unlock()
		if err != nil || len(entries) == 0 {
			return err
		}
		q := entries[0].Request
		q.Dequeue = true
		_, err = Handle(q)
		if err != nil {
			// Capacity races and another sweeper claiming the head are normal.
			if strings.Contains(err.Error(), "host full") || strings.Contains(err.Error(), "queue head changed") {
				return nil
			}
			return err
		}
	}
}

// SelectQueueHost always searches free hosts first, then a reachable queue.
func SelectQueueHost(c Config, inv map[string]Response) (string, error) {
	if n, err := SelectHost(c, inv); err == nil {
		return n, nil
	}
	if r, ok := inv[c.DefaultHost]; ok && r.Error == "" {
		return c.DefaultHost, nil
	}
	for _, n := range SortedHosts(inv) {
		if inv[n].Error == "" {
			return n, nil
		}
	}
	return "", fmt.Errorf("no reachable hosts")
}
