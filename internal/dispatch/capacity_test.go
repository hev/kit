package dispatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalCapacityIncludesWaitingFIFO(t *testing.T) {
	queueTestHome(t)
	cap, running, free, queue, e := LocalCapacity()
	if e != nil || cap != 1 || running != 0 || free != 1 || queue != 0 {
		t.Fatal(cap, running, free, queue, e)
	}
	dir := filepath.Join(Root(), "started")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := reserve(dir); e != nil {
		t.Fatal(e)
	}
	_, running, free, _, e = LocalCapacity()
	if e != nil || running != 1 || free != 0 {
		t.Fatal(running, free, e)
	}
	if e := os.MkdirAll(filepath.Dir(queuePath("waiting")), 0700); e != nil {
		t.Fatal(e)
	}
	if e := WriteJSON(queuePath("waiting"), queueEntry{Request: queueRequest("waiting"), Enqueued: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}); e != nil {
		t.Fatal(e)
	}
	_, _, _, queue, e = LocalCapacity()
	if e != nil || queue != 1 {
		t.Fatal(queue, e)
	}
}
