// Package kitcapture is hevd's capture target on a machine signed in to a
// kit service. Rows go to the local file archive first, as on a keyless
// machine, and each write also leaves a batch in an on-disk outbox that a
// drainer posts to the kit API. Capture never waits on the network: if the
// API is down the outbox grows, and it drains when the API answers again.
package kitcapture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

// Kinds of outbox entry, named for the capture route they are posted to.
const (
	KindSessions = "sessions"
	KindBlocks   = "blocks"
	KindChunks   = "chunks"
	KindDelete   = "delete"
)

// Outbox is a directory of batches waiting for the kit API, one file per
// batch, posted oldest first. Any number of processes may enqueue; one
// drains at a time (a lock file guards it).
type Outbox struct {
	dir string
	seq atomic.Int64
	mu  sync.Mutex
}

// OpenOutbox opens (creating) the outbox in dir.
func OpenOutbox(dir string) (*Outbox, error) {
	if err := os.MkdirAll(filepath.Join(dir, "failed"), 0o700); err != nil {
		return nil, err
	}
	return &Outbox{dir: dir}, nil
}

// Dir is where the outbox lives.
func (o *Outbox) Dir() string { return o.dir }

// Enqueue stores one batch body for kind. It writes the file whole (temp
// file, then rename), so a drainer never reads half a batch.
func (o *Outbox) Enqueue(kind string, body []byte) error {
	name := fmt.Sprintf("%020d-%06d-%d-%s.json", time.Now().UnixNano(), o.seq.Add(1)%1_000_000, os.Getpid(), kind)
	tmp, err := os.CreateTemp(o.dir, ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(o.dir, name))
}

// EnqueueRows splits rows into contract-sized batches and enqueues each.
func EnqueueRows[T any](o *Outbox, kind string, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	bodies, err := kitapi.Batches(rows)
	if err != nil {
		return err
	}
	for _, b := range bodies {
		if err := o.Enqueue(kind, b); err != nil {
			return err
		}
	}
	return nil
}

// EnqueueDelete records that a session was deleted here.
func (o *Outbox) EnqueueDelete(sessionID string) error {
	b, _ := json.Marshal(map[string]string{"session_id": sessionID})
	return o.Enqueue(KindDelete, b)
}

// Pending lists the batches waiting, oldest first.
func (o *Outbox) Pending() ([]string, error) {
	entries, err := os.ReadDir(o.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func kindOf(name string) string {
	name = strings.TrimSuffix(name, ".json")
	return name[strings.LastIndex(name, "-")+1:]
}

// DrainResult says what one drain did.
type DrainResult struct {
	Sent    int // batches the API accepted
	Rows    int // rows it accepted
	Failed  int // batches it refused for good, moved to failed/
	Pending int // batches still waiting
}

// Drain posts pending batches oldest first until the outbox is empty or a
// retryable failure (API down, 429, 5xx) or a rejected key stops it; then
// it returns that error and leaves the rest for next time. A batch the API
// refuses for good (400, 403, 413, ...) moves to failed/ so it cannot block
// the queue. progress, when set, is called after each batch.
func (o *Outbox) Drain(ctx context.Context, c *kitapi.Client, progress func(DrainResult)) (DrainResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	unlock, err := lockFile(filepath.Join(o.dir, ".drain.lock"))
	if err != nil {
		return DrainResult{}, err
	}
	defer unlock()
	names, err := o.Pending()
	if err != nil {
		return DrainResult{}, err
	}
	res := DrainResult{Pending: len(names)}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		path := filepath.Join(o.dir, name)
		body, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			res.Pending--
			continue
		}
		if err != nil {
			return res, err
		}
		n, err := send(ctx, c, kindOf(name), body)
		if err != nil {
			if kitapi.Retryable(err) || errors.Is(err, kitapi.ErrUnauthorized) || ctx.Err() != nil {
				return res, err
			}
			var e *kitapi.Error
			if errors.As(err, &e) && e.Status == 404 && kindOf(name) == KindDelete {
				// Already gone there: the delete is done.
				err = nil
			} else {
				_ = os.WriteFile(path+".error", []byte(err.Error()+"\n"), 0o600)
				_ = os.Rename(path+".error", filepath.Join(o.dir, "failed", name+".error"))
				if rerr := os.Rename(path, filepath.Join(o.dir, "failed", name)); rerr != nil {
					return res, rerr
				}
				res.Failed++
				res.Pending--
				if progress != nil {
					progress(res)
				}
				continue
			}
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
		res.Sent++
		res.Rows += n
		res.Pending--
		if progress != nil {
			progress(res)
		}
	}
	return res, nil
}

func send(ctx context.Context, c *kitapi.Client, kind string, body []byte) (int, error) {
	switch kind {
	case KindSessions, KindBlocks, KindChunks:
		return c.PostBatch(ctx, "/v1/traces/"+kind, body)
	case KindDelete:
		var d struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(body, &d); err != nil || d.SessionID == "" {
			return 0, &kitapi.Error{Status: 400, Message: "bad delete entry"}
		}
		return 0, c.Do(ctx, "DELETE", "/v1/traces/sessions/"+url.PathEscape(d.SessionID), nil, nil, nil)
	}
	return 0, &kitapi.Error{Status: 400, Message: "unknown outbox kind " + strconv.Quote(kind)}
}
