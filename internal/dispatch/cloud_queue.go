package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var cloudQueueLauncher = func(h Host) error { return launchBoatCommand(h, "queue", "cloud-sweep") }
var cloudQueuePoll = 5 * time.Second

func cloudQueueDir(h Host) string               { return filepath.Join(Root(), ".remote", h.Name, ".queue") }
func cloudQueuePath(h Host, name string) string { return filepath.Join(cloudQueueDir(h), name+".json") }
func cloudQueueEntries(h Host) ([]queueEntry, error) {
	files, err := os.ReadDir(cloudQueueDir(h))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []queueEntry
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		var e queueEntry
		if err = readJSON(filepath.Join(cloudQueueDir(h), f.Name()), &e); err != nil {
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

// Called with the dispatcher's root lock held, just like receiver enqueue.
// Requests store names of credentials only, never their resolved values.
func enqueueCloud(h Host, q Request) (Response, error) {
	o, err := Parse(q.Order)
	if err != nil {
		return Response{}, err
	}
	if o.Name != q.Name || q.Meta.Name != q.Name {
		return Response{}, fmt.Errorf("invalid queued cloud order")
	}
	if exists(cloudQueuePath(h, q.Name)) {
		return Response{}, fmt.Errorf("%w: %s", errRunExists, q.Name)
	}
	entries, err := cloudQueueEntries(h)
	if err != nil {
		return Response{}, err
	}
	at := time.Now().UTC()
	if len(entries) > 0 && !at.After(entries[len(entries)-1].Enqueued) {
		at = entries[len(entries)-1].Enqueued.Add(time.Nanosecond)
	}
	q.Meta.LaunchEnv = nil
	q.Meta.LaunchResolved = false
	q.ClassToken = ""
	q.ReserveClass = false
	if err = os.MkdirAll(cloudQueueDir(h), 0700); err != nil {
		return Response{}, err
	}
	if err = WriteJSON(cloudQueuePath(h, q.Name), queueEntry{q, at}); err != nil {
		return Response{}, err
	}
	if err = cloudQueueLauncher(h); err != nil {
		return Response{}, fmt.Errorf("order queued; queue watcher failed: %w", err)
	}
	return Response{Message: fmt.Sprintf("queued %s (position %d)", q.Name, len(entries)+1)}, nil
}
func cloudClassFull(h Host, machines []Machine, class, self string) bool {
	limit := h.Classes[class]
	if class == "" || limit < 1 {
		return false
	}
	live := 0
	for _, m := range machines {
		if !m.live() || m.Labels["hev.order"] == self {
			continue
		}
		if m.Labels["hev.class"] == class {
			live++
			continue
		}
		var meta Meta
		if readJSON(filepath.Join(boatRecordDir(h.Name, m.Labels["hev.order"]), "meta.json"), &meta) == nil && meta.Class == class {
			live++
		}
	}
	return live >= limit
}

// A detached sweeper survives the submitting CLI and serializes with other
// sweepers. Restart explicitly with dispatch cloud-sweep queue --host HOST.
func SweepCloudQueue(ctx context.Context, h Host) error {
	unlock, err := lockBoat(h.Name, "queue", "sweep")
	if err != nil {
		return nil
	}
	defer unlock()
	for {
		entries, err := cloudQueueEntries(h)
		if err != nil || len(entries) == 0 {
			return err
		}
		q := entries[0].Request
		q.Dequeue = true
		_, err = boatRPC(ctx, h, q)
		if err != nil && !strings.Contains(err.Error(), "host full") && !strings.Contains(err.Error(), "queue head changed") && !errors.Is(err, errClassFull) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			recordBoatAdmission(h, q, err)
			if e := os.Remove(cloudQueuePath(h, q.Name)); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cloudQueuePoll):
		}
	}
}
