package v1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/platformops"
)

// Serialize account mutations in a single server process. Stripes bound the
// synchronization table independently of the number of accounts.
var accountLocks = func() [256]chan struct{} {
	var locks [256]chan struct{}
	for i := range locks {
		locks[i] = make(chan struct{}, 1)
	}
	return locks
}()

func lockAccount(ctx context.Context, dir string) (func(), error) {
	ch := accountLocks[sha256.Sum256([]byte(dir))[0]]
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	}
}

type savedResponse struct {
	Location string
	Body     []byte
}

const replayWindow = 24 * time.Hour
const replayCapacity = 10000
const replayByteBudget = 256 << 20

func (h *handler) replay(w http.ResponseWriter, r *http.Request, create func(http.ResponseWriter)) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		create(w)
		return
	}
	if utf8.RuneCountInString(key) > 255 {
		fail(w, store.ErrInvalid)
		return
	}
	dir := filepath.Join(h.dir, "replays")
	sum := sha256.Sum256([]byte(callerOf(r).Author + "\x00" + r.URL.Path + "\x00" + key))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
	now := time.Now()
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fail(w, err)
		return
	}
	if err == nil && now.Sub(info.ModTime()) < replayWindow {
		raw, err := os.ReadFile(path)
		var res savedResponse
		if err != nil || json.Unmarshal(raw, &res) != nil || len(res.Body) == 0 {
			Error(w, 502, "upstream_error", "The stored create response is unavailable.")
			return
		}
		writeCreated(w, res)
		return
	}
	if err := prepareReplayDir(dir, now); err != nil {
		fail(w, err)
		return
	}
	// A reservation keeps an interrupted or ambiguous upstream write from
	// becoming a duplicate when the client retries after a process restart.
	if err := platformops.WriteFileAtomic(path, []byte("{}"), 0600); err != nil {
		fail(w, err)
		return
	}
	rec := httptest.NewRecorder()
	create(rec)
	if rec.Code != 201 {
		if rec.Code < 500 {
			if err := os.Remove(path); err != nil {
				fail(w, err)
				return
			}
		}
		for name, values := range rec.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}
	res := savedResponse{Location: rec.Header().Get("Location"), Body: rec.Body.Bytes()}
	raw, err := json.Marshal(res)
	if err == nil {
		err = platformops.WriteFileAtomic(path, raw, 0600)
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeCreated(w, res)
}

func prepareReplayDir(dir string, now time.Time) error {
	if err := platformops.MkdirAll(dir, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	active := 0
	var bytes int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) >= replayWindow {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		} else {
			active++
			bytes += info.Size()
		}
	}
	if active >= replayCapacity || bytes >= replayByteBudget {
		return errors.New("board replay capacity reached")
	}
	return nil
}

func writeCreated(w http.ResponseWriter, res savedResponse) {
	if res.Location != "" {
		w.Header().Set("Location", res.Location)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	_, _ = w.Write(res.Body)
}
