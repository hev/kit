// Package kitcapture is hevd's capture target on a machine signed in to a
// kit service. Rows go to the local file archive first, as on a keyless
// machine, and each write also leaves a batch in an on-disk outbox that a
// drainer posts to the kit API. Capture never waits on the network: if the
// API is down the outbox grows, and it drains when the API answers again.
package kitcapture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
//
// An outbox belongs to one team: its batches live under teams/<team id>/ of
// the outbox root, and Drain sends them only with a key whose /v1/whoami
// names that team. Signing in to another team opens another directory, and
// the first team's batches wait, untouched, until it is signed in again.
// Batches written before outboxes were bound to a team sit directly in the
// root; nothing ever sends them (see Unbound).
type Outbox struct {
	root string
	dir  string
	team string
	seq  atomic.Int64
	mu   sync.Mutex

	// MaxBatches and MaxBytes bound what the outbox holds, queued and
	// failed/ together (defaults DefaultMaxBatches, DefaultMaxBytes). At
	// the bound, the oldest failed/ batches are removed first; if that is
	// not enough, a new batch is refused with ErrFull and a log line. The
	// rows stay in the local archive either way.
	MaxBatches int
	MaxBytes   int64
	// Logger gets the outbox's warnings (slog.Default when nil).
	Logger *slog.Logger

	usageMu   sync.Mutex
	usage     usage
	usageAt   time.Time
	refused   atomic.Int64
	refusedAt atomic.Int64 // unix nanos of the last refusal
}

// Default outbox bounds: room for a long outage of a busy machine, well
// short of filling a disk.
const (
	DefaultMaxBatches = 20_000
	DefaultMaxBytes   = 1 << 30
)

// ErrFull is Enqueue's answer when the outbox is at its bound.
var ErrFull = errors.New("kit outbox is full")

// ErrTeamMismatch is Drain's answer when the key it was given belongs to a
// different team than the outbox. Nothing is sent.
var ErrTeamMismatch = errors.New("kit key belongs to a different team than this outbox")

type usage struct {
	batches int
	bytes   int64
}

// ErrNoTeam is Drain's answer for an outbox opened without a team id: a
// sign-in that recorded none. Its batches are held and never sent.
var ErrNoTeam = errors.New("kit sign-in records no team id, so queued batches are held and not sent; run `hev login` again")

// OpenOutbox opens (creating) the outbox for team under root. team is the
// team id the sign-in recorded ([kit] team_id), not a label of the
// client's choosing. With no team id, capture still queues (into
// no-team/, so it never blocks) but Drain sends nothing: ErrNoTeam.
func OpenOutbox(root, team string) (*Outbox, error) {
	team = strings.TrimSpace(team)
	dir := filepath.Join(root, "no-team")
	if team != "" {
		dir = filepath.Join(root, "teams", teamDirName(team))
	}
	if err := os.MkdirAll(filepath.Join(dir, "failed"), 0o700); err != nil {
		return nil, err
	}
	return &Outbox{root: root, dir: dir, team: team}, nil
}

// teamDirName is a directory name for a team id: the id itself when it is
// plainly safe, else a hash of it.
func teamDirName(team string) string {
	safe := team != "." && team != ".." && len(team) <= 100
	for _, r := range team {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			safe = false
			break
		}
	}
	if safe {
		return team
	}
	sum := sha256.Sum256([]byte(team))
	return "h-" + hex.EncodeToString(sum[:16])
}

// Dir is where this team's batches live.
func (o *Outbox) Dir() string { return o.dir }

// Team is the team id the outbox is bound to.
func (o *Outbox) Team() string { return o.team }

// Unbound lists batches left in the outbox root by a kit that did not bind
// batches to a team. They are never sent: nothing records which team they
// were captured for. They stay on disk for the owner to inspect or delete;
// the rows themselves are in the local archive.
func (o *Outbox) Unbound() ([]string, error) {
	return pendingIn(o.root)
}

func (o *Outbox) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

func (o *Outbox) limits() (int, int64) {
	n, b := o.MaxBatches, o.MaxBytes
	if n <= 0 {
		n = DefaultMaxBatches
	}
	if b <= 0 {
		b = DefaultMaxBytes
	}
	return n, b
}

// Refused is how many batches this process could not queue because the
// outbox was full, and when the last one was.
func (o *Outbox) Refused() (int64, time.Time) {
	n := o.refused.Load()
	if n == 0 {
		return 0, time.Time{}
	}
	return n, time.Unix(0, o.refusedAt.Load())
}

// Usage counts what the outbox holds, queued and failed/ together.
func (o *Outbox) Usage() (batches int, bytes int64, err error) {
	u, err := o.scanUsage()
	return u.batches, u.bytes, err
}

func (o *Outbox) scanUsage() (usage, error) {
	var u usage
	for _, d := range []string{o.dir, filepath.Join(o.dir, "failed")} {
		entries, err := os.ReadDir(d)
		if err != nil {
			return u, err
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			u.batches++
			u.bytes += info.Size()
		}
	}
	return u, nil
}

// reserve makes room for one batch of size n, or reports ErrFull. Usage is
// rescanned at most every few seconds (other processes enqueue and the
// drainer removes); between scans it counts this process's own batches.
func (o *Outbox) reserve(n int64) error {
	o.usageMu.Lock()
	defer o.usageMu.Unlock()
	maxN, maxB := o.limits()
	if time.Since(o.usageAt) > 5*time.Second || o.usage.batches+1 > maxN || o.usage.bytes+n > maxB {
		u, err := o.scanUsage()
		if err != nil {
			return err
		}
		o.usage, o.usageAt = u, time.Now()
	}
	if o.usage.batches+1 > maxN || o.usage.bytes+n > maxB {
		o.pruneFailed(maxN, maxB, n)
	}
	if o.usage.batches+1 > maxN || o.usage.bytes+n > maxB {
		total := o.refused.Add(1)
		o.refusedAt.Store(time.Now().UnixNano())
		o.logger().Error("kit outbox full: a new batch was not queued for the kit API; its rows are kept in the local archive only",
			"outbox", o.dir, "batches", o.usage.batches, "bytes", o.usage.bytes, "max_batches", maxN, "max_bytes", maxB, "refused_total", total)
		return fmt.Errorf("%w (%d batches, %d bytes; limit %d batches, %d bytes) at %s", ErrFull, o.usage.batches, o.usage.bytes, maxN, maxB, o.dir)
	}
	o.usage.batches++
	o.usage.bytes += n
	return nil
}

// pruneFailed removes the oldest refused batches (and their notes) until a
// batch of size n fits, logging what it removed.
func (o *Outbox) pruneFailed(maxN int, maxB, n int64) {
	failedDir := filepath.Join(o.dir, "failed")
	names, err := pendingIn(failedDir)
	if err != nil {
		return
	}
	removed := 0
	for _, name := range names {
		if o.usage.batches+1 <= maxN && o.usage.bytes+n <= maxB {
			break
		}
		path := filepath.Join(failedDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if os.Remove(path) != nil {
			continue
		}
		_ = os.Remove(path + ".error")
		o.usage.batches--
		o.usage.bytes -= info.Size()
		removed++
	}
	if removed > 0 {
		o.logger().Warn("kit outbox at its bound: removed the oldest refused batches from failed/", "removed", removed, "outbox", o.dir)
	}
}

// Enqueue stores one batch body for kind. It writes the file whole (temp
// file, fsync, rename, fsync the directory), so a drainer never reads half
// a batch and a queued batch survives a crash. At the outbox's bound it
// returns ErrFull (and logs it) instead.
func (o *Outbox) Enqueue(kind string, body []byte) error {
	if err := o.reserve(int64(len(body))); err != nil {
		return err
	}
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(o.dir, name)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return syncDir(o.dir)
}

// syncDir makes a rename in dir durable. Windows cannot open a directory
// for sync; NTFS journals the rename.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
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
func (o *Outbox) Pending() ([]string, error) { return pendingIn(o.dir) }

func pendingIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
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
// retryable failure (API down, 429, 5xx), a rejected key (401) or a team
// read-only for billing (402) stops it; then it returns that error and
// leaves the rest, the batch it was sending included, for next time. A
// batch the API refuses for good (400, 403, 413, ...) moves to failed/ so
// it cannot block the queue. Before sending anything it asks /v1/whoami
// which team c's key belongs to and sends nothing (ErrTeamMismatch) unless
// that is the outbox's team. progress, when set, is called after each
// batch.
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
	if len(names) == 0 {
		return res, nil
	}
	if o.team == "" {
		return res, ErrNoTeam
	}
	who, err := c.Whoami(ctx)
	if err != nil {
		return res, err
	}
	if who.Team.ID != o.team {
		return res, fmt.Errorf("%w: the key is for team %q, the outbox at %s is for team %q; its batches wait for that team's sign-in", ErrTeamMismatch, who.Team.ID, o.dir, o.team)
	}
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
			if kitapi.Retryable(err) || errors.Is(err, kitapi.ErrUnauthorized) || kitapi.ReadOnly(err) || ctx.Err() != nil {
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
