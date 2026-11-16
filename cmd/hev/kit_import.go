package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hev/kit/internal/board/settings"
	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/kitapi"
)

// kitImportState is how far `hev login --import` got, per kit key, so a
// second run picks up where the first stopped and sends nothing twice that
// the API could not deduplicate (board posts have no client-side id).
type kitImportState struct {
	KeyID    string                       `json:"key_id"`
	Sessions map[string]bool              `json:"sessions"`
	Threads  map[string]kitImportedThread `json:"threads"`
}

type kitImportedThread struct {
	Remote int64 `json:"remote"`
	Posts  int   `json:"posts"` // how many of the local thread's posts are sent
	// Sending is 1 + the index of a post whose POST was started and not
	// known to have finished (0: none). Saved before the POST, so a run
	// that died or lost the answer looks for the post on the API before
	// sending it again: board writes carry no idempotency key.
	Sending int `json:"sending,omitempty"`
}

func kitImportStatePath(keyID string) string {
	return filepath.Join(filepath.Dir(daemon.KitOutboxDir()), "import-"+keyID+".json")
}

func loadKitImportState(keyID string) (*kitImportState, error) {
	st := &kitImportState{KeyID: keyID, Sessions: map[string]bool{}, Threads: map[string]kitImportedThread{}}
	raw, err := os.ReadFile(kitImportStatePath(keyID))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, st); err != nil {
		return nil, fmt.Errorf("import state %s: %w", kitImportStatePath(keyID), err)
	}
	if st.Sessions == nil {
		st.Sessions = map[string]bool{}
	}
	if st.Threads == nil {
		st.Threads = map[string]kitImportedThread{}
	}
	return st, nil
}

func (s *kitImportState) save() error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := kitImportStatePath(s.KeyID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return platformops.WriteFileAtomic(path, raw, 0o600)
}

// runKitImport backfills this machine's file archive and file board to the
// kit API. Sessions are queued in the outbox (rows are upserts, so a resend
// is harmless) and the outbox is drained with progress; board threads are
// recreated through the API, with the original poster and time quoted at
// the top of each post because the API attributes posts to the key.
// Progress is kept per key, so an interrupted import resumes.
func runKitImport(ctx context.Context, out io.Writer) error {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Kit.SignedIn() {
		return fmt.Errorf("not signed in; run hev login")
	}
	state, err := loadKitImportState(cfg.Kit.KeyID)
	if err != nil {
		return err
	}
	target, err := daemon.OpenTargetRead(cfg, "")
	if err != nil {
		return err
	}
	if target.Kit == nil {
		return fmt.Errorf("signed in, but the capture target is not the kit API")
	}
	c := kitClient(cfg.Kit)

	queued, err := kitQueueSessions(target.Kit, state, out)
	if err != nil {
		return err
	}
	pending, _ := target.Kit.Outbox.Pending()
	fmt.Fprintf(out, "Traces: %d sessions queued now, %d batches to send.\n", queued, len(pending))
	total := len(pending)
	res, err := target.Kit.Outbox.Drain(ctx, c, func(r kitcapture.DrainResult) {
		done := r.Sent + r.Failed
		if done == total || done%25 == 0 {
			fmt.Fprintf(out, "  sent %d/%d batches (%d rows)\n", done, total, r.Rows)
		}
	})
	if err != nil {
		return fmt.Errorf("traces import stopped with %d batches left (run hev login --import again to resume; hev d also drains them): %w", res.Pending, err)
	}
	if res.Failed > 0 {
		fmt.Fprintf(out, "  %d batches were refused; they are kept in %s\n", res.Failed, filepath.Join(target.Kit.Outbox.Dir(), "failed"))
	}

	threads, posts, err := kitImportBoard(ctx, c, state, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Board: %d threads and %d posts imported.\n", threads, posts)
	return nil
}

// kitQueueSessions queues every session in the local archive not yet
// queued under this key, saving progress as it goes.
func kitQueueSessions(a *kitcapture.Archive, state *kitImportState, out io.Writer) (int, error) {
	rows, err := a.Store.ListSessionRows(-1, nil)
	if err != nil {
		return 0, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SessionID < rows[j].SessionID })
	todo := 0
	for _, r := range rows {
		if !state.Sessions[r.SessionID] {
			todo++
		}
	}
	if todo > 0 {
		fmt.Fprintf(out, "Queuing %d of %d local sessions for the kit API…\n", todo, len(rows))
	}
	n := 0
	for _, r := range rows {
		id := r.SessionID
		if state.Sessions[id] {
			continue
		}
		blocks, err := a.Store.ListBlockRows(id)
		if err != nil {
			return n, err
		}
		chunks, err := a.Store.SessionRows(id)
		if err != nil {
			return n, err
		}
		own := chunks[:0]
		for _, ch := range chunks {
			if ch.SessionID == id {
				own = append(own, ch)
			}
		}
		// Sessions last, so a session row never arrives before its blocks.
		if err := kitcapture.EnqueueRows(a.Outbox, kitcapture.KindChunks, own); err != nil {
			return n, err
		}
		if err := kitcapture.EnqueueRows(a.Outbox, kitcapture.KindBlocks, blocks); err != nil {
			return n, err
		}
		if err := kitcapture.EnqueueRows(a.Outbox, kitcapture.KindSessions, []trace.SessionRow{r}); err != nil {
			return n, err
		}
		state.Sessions[id] = true
		n++
		if n%50 == 0 {
			if err := state.save(); err != nil {
				return n, err
			}
			fmt.Fprintf(out, "  queued %d/%d\n", n, todo)
		}
	}
	return n, state.save()
}

// kitImportBoard recreates this machine's file board through the kit API.
// A board kept in Layer is not this machine's to copy and is skipped.
func kitImportBoard(ctx context.Context, c *kitapi.Client, state *kitImportState, out io.Writer) (int, int, error) {
	dir := settings.Dir()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return 0, 0, nil
	}
	st, err := boardstore.NewFiles(dir)
	if err != nil {
		return 0, 0, err
	}
	var all []boardstore.Thread
	for offset := 0; ; offset += 500 {
		page, err := st.Threads(ctx, boardstore.ListOpts{Limit: 500, Offset: offset})
		if err != nil {
			return 0, 0, err
		}
		all = append(all, page...)
		if len(page) < 500 {
			break
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	// Board writes are not idempotent, so the import sends each one once
	// itself and never lets the client retry a POST blindly: after a
	// failure it first looks for the post on the API.
	once := *c
	once.Retries = 0
	threads, posts := 0, 0
	for _, t := range all {
		key := fmt.Sprint(t.ID)
		done := state.Threads[key]
		ps, err := st.Posts(ctx, t.ID)
		if err != nil {
			return threads, posts, err
		}
		if done.Posts >= len(ps) {
			continue
		}
		for i := done.Posts; i < len(ps); i++ {
			p := ps[i]
			body := kitImportBody(t, p)
			sent := false
			for attempt := 0; !sent; attempt++ {
				if done.Sending == i+1 {
					// A send of this post may have landed: look before resending.
					remote, found, err := kitImportFind(ctx, c, t, done, i, body)
					if err != nil {
						return threads, posts, fmt.Errorf("board import: checking whether thread #%d post %d already arrived (run hev login --import again to resume): %w", t.ID, i+1, err)
					}
					if found {
						if i == 0 {
							done.Remote = remote
						}
						break
					}
				}
				done.Sending = i + 1
				state.Threads[key] = done
				if err := state.save(); err != nil {
					return threads, posts, err
				}
				if i == 0 {
					rt, err := once.CreateThread(ctx, kitapi.ThreadIn{Board: t.Board, Title: t.Title, Tags: t.Tags, Body: body})
					if err == nil {
						done.Remote = rt.ID
						sent = true
					} else if !kitapi.Retryable(err) || attempt >= 4 || ctx.Err() != nil {
						return threads, posts, fmt.Errorf("board import stopped at thread #%d (run hev login --import again to resume): %w", t.ID, err)
					}
				} else {
					_, err := once.Reply(ctx, done.Remote, kitapi.PostIn{Body: body})
					if err == nil {
						sent = true
					} else if !kitapi.Retryable(err) || attempt >= 4 || ctx.Err() != nil {
						return threads, posts, fmt.Errorf("board import stopped at thread #%d post %d (run hev login --import again to resume): %w", t.ID, i+1, err)
					}
				}
				if !sent {
					if err := kitImportPause(ctx, attempt); err != nil {
						return threads, posts, err
					}
				}
			}
			if i == 0 {
				threads++
			}
			done.Posts, done.Sending = i+1, 0
			posts++
			state.Threads[key] = done
			if err := state.save(); err != nil {
				return threads, posts, err
			}
		}
		if t.Pinned {
			_ = c.Pin(ctx, done.Remote, true)
		}
		if t.Locked {
			_ = c.Lock(ctx, done.Remote, true)
		}
	}
	return threads, posts, nil
}

// kitImportBody is what the import posts for local post p of thread t. It
// names the local thread, so kitImportFind can recognise it.
func kitImportBody(t boardstore.Thread, p boardstore.Post) string {
	return fmt.Sprintf("_Imported from local thread #%d: posted by %s on %s, %s._\n\n%s", t.ID, p.Author, orUnknown(p.Host), p.Created.UTC().Format("2006-01-02 15:04Z"), p.Body)
}

// kitImportFind reports whether post i of local thread t (with body) is
// already on the API: for the first post, a thread on the same board with
// the same title whose first post is body (and its id); for a reply, a post
// in the remote thread with body.
func kitImportFind(ctx context.Context, c *kitapi.Client, t boardstore.Thread, done kitImportedThread, i int, body string) (int64, bool, error) {
	if i > 0 {
		v, err := c.Thread(ctx, done.Remote)
		if err != nil {
			return 0, false, err
		}
		for _, p := range v.Posts {
			if p.Body == body {
				return done.Remote, true, nil
			}
		}
		return 0, false, nil
	}
	for offset := 0; ; offset += 100 {
		page, err := c.Threads(ctx, url.Values{"board": {t.Board}, "limit": {"100"}, "offset": {fmt.Sprint(offset)}})
		if err != nil {
			return 0, false, err
		}
		for _, rt := range page {
			if rt.Title != t.Title {
				continue
			}
			v, err := c.Thread(ctx, rt.ID)
			if err != nil {
				return 0, false, err
			}
			if len(v.Posts) > 0 && v.Posts[0].Body == body {
				return rt.ID, true, nil
			}
		}
		if len(page) < 100 {
			return 0, false, nil
		}
	}
}

// kitImportPause waits before the import's own retry of a board write.
func kitImportPause(ctx context.Context, attempt int) error {
	t := time.NewTimer(kitImportBackoff(attempt))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var kitImportBackoff = func(attempt int) time.Duration { return 250 * time.Millisecond << min(attempt, 5) }

func orUnknown(s string) string {
	if s == "" {
		return "an unknown host"
	}
	return s
}
