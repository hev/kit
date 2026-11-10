package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/pkg/search"
)

// fileStore keeps the board on the host's filesystem, one JSON document per
// thread:
//
//	<dir>/threads/<id>.json   the thread, its opening post and its replies
//	<dir>/.lock               held across every read and write
//
// Every operation takes the directory lock (platformops.LockPath, the same
// lock peers uses), so any number of servers and clients on the host can
// share the directory. Each write replaces one thread file atomically.
type fileStore struct {
	dir string
}

type fileThread struct {
	ID    int64    `json:"id"`
	Board string   `json:"board"`
	Title string   `json:"title"`
	Tags  []string `json:"tags"`
	Poster
	Body     string      `json:"body"`
	Created  time.Time   `json:"created"`
	Updated  time.Time   `json:"updated"`
	Views    int         `json:"views"`
	UsefulBy []string    `json:"useful_by"`
	Pinned   bool        `json:"pinned"`
	Locked   bool        `json:"locked"`
	Replies  []fileReply `json:"replies"`
}

type fileReply struct {
	N int `json:"n"`
	Poster
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
}

// NewFiles is the filesystem-mode store rooted at dir. It creates the
// directory if needed and never opens a network connection.
func NewFiles(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("board directory is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := platformops.MkdirAll(filepath.Join(abs, "threads"), 0o700); err != nil {
		return nil, fmt.Errorf("board directory: %w", err)
	}
	return &Store{Backend: &fileStore{dir: abs}, info: Info{Mode: "filesystem", Location: abs}}, nil
}

func (f *fileStore) locked(fn func() error) error {
	unlock, err := platformops.LockPath(filepath.Join(f.dir, ".lock"), false)
	if err != nil {
		return fmt.Errorf("lock board directory: %w", err)
	}
	defer unlock()
	return fn()
}

func (f *fileStore) path(id int64) string {
	return filepath.Join(f.dir, "threads", strconv.FormatInt(id, 10)+".json")
}

func (f *fileStore) get(id int64) (*fileThread, error) {
	b, err := os.ReadFile(f.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var t fileThread
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("board thread %d: %w", id, err)
	}
	return &t, nil
}

func (f *fileStore) put(t *fileThread) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(f.path(t.ID), append(b, '\n'), 0o600)
}

// ids lists thread ids on disk; temporary files and anything else are skipped.
func (f *fileStore) ids() ([]int64, error) {
	entries, err := os.ReadDir(filepath.Join(f.dir, "threads"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() {
			continue
		}
		if id, err := strconv.ParseInt(name, 10, 64); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fileStore) all() ([]*fileThread, error) {
	ids, err := f.ids()
	if err != nil {
		return nil, err
	}
	out := make([]*fileThread, 0, len(ids))
	for _, id := range ids {
		t, err := f.get(id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// read loads every thread under the lock.
func (f *fileStore) read() (ts []*fileThread, err error) {
	err = f.locked(func() error {
		ts, err = f.all()
		return err
	})
	return ts, err
}

// update is a read-modify-write of one thread under the lock.
func (f *fileStore) update(id int64, fn func(*fileThread) error) error {
	return f.locked(func() error {
		t, err := f.get(id)
		if err != nil {
			return err
		}
		if err := fn(t); err != nil {
			return err
		}
		return f.put(t)
	})
}

func stamp() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func (ft *fileThread) thread() Thread {
	t := Thread{
		ID: ft.ID, Board: ft.Board, Title: ft.Title, Tags: append([]string{}, ft.Tags...), Poster: ft.Poster,
		Created: ft.Created, Updated: ft.Updated, Views: ft.Views, Replies: len(ft.Replies),
		Pinned: ft.Pinned, Locked: ft.Locked, body: ft.Body, usefulBy: append([]string{}, ft.UsefulBy...),
	}
	t.Useful = len(t.usefulBy)
	t.Excerpt = excerpt(t.body, 160)
	last := &Post{ThreadID: t.ID, N: 1, Poster: Poster{Author: ft.Author}, Created: ft.Created, Title: t.Title, Board: t.Board}
	if n := len(ft.Replies); n > 0 {
		r := ft.Replies[n-1]
		last.N, last.Poster, last.Created = r.N, Poster{Author: r.Author}, r.Created
	}
	t.Last = last
	return t
}

func (ft *fileThread) opener() Post {
	return Post{ID: fmt.Sprintf("t:%d", ft.ID), ThreadID: ft.ID, N: 1, Poster: ft.Poster, Body: ft.Body, Created: ft.Created}
}

func (ft *fileThread) reply(r fileReply) Post {
	return Post{ID: fmt.Sprintf("p:%d:%d", ft.ID, r.N), ThreadID: ft.ID, N: r.N, Poster: r.Poster, Body: r.Body, Created: r.Created}
}

func (ft *fileThread) postedBy(author string) bool {
	if ft.Author == author {
		return true
	}
	for _, r := range ft.Replies {
		if r.Author == author {
			return true
		}
	}
	return false
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (f *fileStore) Boards(ctx context.Context) ([]Board, error) {
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	ts := make([]Thread, len(fts))
	for i, ft := range fts {
		ts[i] = ft.thread()
	}
	return boardsOf(ts), nil
}

func (f *fileStore) Threads(ctx context.Context, o ListOpts) ([]Thread, error) {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	tag := strings.ToLower(o.Tag)
	out := []Thread{}
	for _, ft := range fts {
		if (o.Board != "" && ft.Board != o.Board) || (tag != "" && !hasTag(ft.Tags, tag)) || (o.Author != "" && !ft.postedBy(o.Author)) {
			continue
		}
		out = append(out, ft.thread())
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].ID > out[j].ID
	})
	if o.Board != "" {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Pinned && !out[j].Pinned })
	}
	if o.Offset >= len(out) {
		return []Thread{}, nil
	}
	out = out[o.Offset:]
	if len(out) > o.Limit {
		out = out[:o.Limit]
	}
	return out, nil
}

func (f *fileStore) Thread(ctx context.Context, id int64) (*Thread, error) {
	var t Thread
	err := f.locked(func() error {
		ft, err := f.get(id)
		if err != nil {
			return err
		}
		t = ft.thread()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (f *fileStore) Posts(ctx context.Context, threadID int64) ([]Post, error) {
	out := []Post{}
	err := f.locked(func() error {
		ft, err := f.get(threadID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		out = append(out, ft.opener())
		for _, r := range ft.Replies {
			out = append(out, ft.reply(r))
		}
		return nil
	})
	return out, err
}

func (f *fileStore) View(ctx context.Context, id int64) error {
	return f.update(id, func(ft *fileThread) error { ft.Views++; return nil })
}

func (f *fileStore) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	if err := prepThread(&in); err != nil {
		return nil, err
	}
	var t Thread
	err := f.locked(func() error {
		ids, err := f.ids()
		if err != nil {
			return err
		}
		id := int64(1)
		for _, have := range ids {
			if have >= id {
				id = have + 1
			}
		}
		if in.Suffix != nil {
			in.Body += in.Suffix(id)
		}
		ts := stamp()
		ft := &fileThread{
			ID: id, Board: in.Board, Title: in.Title, Tags: cleanTags(in.Tags), Poster: in.Poster, Body: in.Body,
			Created: ts, Updated: ts, UsefulBy: []string{}, Replies: []fileReply{},
		}
		if err := f.put(ft); err != nil {
			return err
		}
		t = ft.thread()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (f *fileStore) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if err := prepPost(&in); err != nil {
		return nil, err
	}
	var p Post
	err := f.update(in.ThreadID, func(ft *fileThread) error {
		if ft.Locked {
			return invalid("thread is locked")
		}
		ts := stamp()
		r := fileReply{N: len(ft.Replies) + 2, Poster: in.Poster, Body: in.Body, Created: ts}
		ft.Replies = append(ft.Replies, r)
		ft.Updated = ts
		p = ft.reply(r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (f *fileStore) Useful(ctx context.Context, threadID int64, author string) (int, error) {
	if author == "" {
		return 0, invalid("a vote needs an author")
	}
	n := 0
	err := f.locked(func() error {
		ft, err := f.get(threadID)
		if err != nil {
			return err
		}
		if hasTag(ft.UsefulBy, author) {
			n = len(ft.UsefulBy)
			return nil
		}
		ft.UsefulBy = append(ft.UsefulBy, author)
		n = len(ft.UsefulBy)
		return f.put(ft)
	})
	return n, err
}

func (f *fileStore) SetPinned(ctx context.Context, id int64, pinned bool) error {
	return f.update(id, func(ft *fileThread) error { ft.Pinned = pinned; return nil })
}

func (f *fileStore) SetLocked(ctx context.Context, id int64, locked bool) error {
	return f.update(id, func(ft *fileThread) error { ft.Locked = locked; return nil })
}

// rows is every post as the Layer attributes Stats, Online and Profiles read,
// newest first, so both modes count them with the same code.
func rowsOf(fts []*fileThread) []layer.Row {
	var rows []layer.Row
	for _, ft := range fts {
		by := make([]any, len(ft.UsefulBy))
		for i, a := range ft.UsefulBy {
			by[i] = a
		}
		rows = append(rows, layer.Row{
			"kind": "thread", "thread_id": ft.ID, "n": int64(1), "author": ft.Author, "role": ft.Role, "host": ft.Host,
			"created": ft.Created.UTC().Format(tsLayout), "title": ft.Title, "useful_by": by,
		})
		for _, r := range ft.Replies {
			rows = append(rows, layer.Row{
				"kind": "post", "thread_id": ft.ID, "n": int64(r.N), "author": r.Author, "role": r.Role, "host": r.Host,
				"created": r.Created.UTC().Format(tsLayout),
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return str(rows[i], "created") > str(rows[j], "created") })
	return rows
}

func (f *fileStore) Stats(ctx context.Context) (*Stats, error) {
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	return statsOf(rowsOf(fts)), nil
}

func (f *fileStore) Online(ctx context.Context, within time.Duration) ([]Online, error) {
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	return onlineOf(rowsOf(fts), within), nil
}

func (f *fileStore) Profiles(ctx context.Context, authors []string) (map[string]Profile, error) {
	out := map[string]Profile{}
	if len(authors) == 0 {
		return out, nil
	}
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	var rows []layer.Row
	for _, r := range rowsOf(fts) {
		if hasTag(authors, str(r, "author")) {
			rows = append(rows, r)
		}
	}
	return profilesOf(rows, out), nil
}

func (f *fileStore) Search(ctx context.Context, o SearchOpts) ([]Hit, error) {
	phrasings := []string{}
	for _, p := range append([]string{o.Q}, o.Also...) {
		if p = strings.TrimSpace(p); p != "" {
			phrasings = append(phrasings, p)
		}
	}
	if len(phrasings) == 0 {
		return []Hit{}, nil
	}
	if len(phrasings) > search.MaxPhrasings {
		return nil, invalid("at most %d phrasings in one search, got %d", search.MaxPhrasings, len(phrasings))
	}
	if o.Limit <= 0 {
		o.Limit = 10
	}
	if o.Board != "" && !isBoard(o.Board) {
		return nil, invalid("no board %q — see `hev board boards`", o.Board)
	}
	var since time.Time
	if o.Since != "" {
		d, err := search.ParseSince(o.Since)
		if err != nil {
			return nil, invalid("%v", err)
		}
		since = time.Now().Add(-d)
	}
	fts, err := f.read()
	if err != nil {
		return nil, err
	}
	tags := cleanTags(o.Tags)
	var corpus []lexDoc
	for _, ft := range fts {
		inThread := (o.Board == "" || ft.Board == o.Board)
		for _, t := range tags {
			inThread = inThread && hasTag(ft.Tags, t)
		}
		keep := func(p Poster, created time.Time) bool {
			return inThread && (o.Author == "" || p.Author == o.Author) && (o.Role == "" || p.Role == o.Role) &&
				(since.IsZero() || !created.Before(since))
		}
		corpus = append(corpus, lexDoc{
			thread: ft, post: Post{ID: fmt.Sprintf("t:%d", ft.ID), ThreadID: ft.ID, N: 1, Poster: Poster{Author: ft.Author, Role: ft.Role}, Body: ft.Body, Created: ft.Created},
			keep:   keep(ft.Poster, ft.Created),
			fields: []lexField{{ft.Title, 2}, {strings.Join(ft.Tags, " "), 2}, {ft.Body, 1}},
		})
		for _, r := range ft.Replies {
			corpus = append(corpus, lexDoc{
				thread: ft, post: Post{ID: fmt.Sprintf("p:%d:%d", ft.ID, r.N), ThreadID: ft.ID, N: r.N, Poster: Poster{Author: r.Author, Role: r.Role}, Body: r.Body, Created: r.Created},
				keep:   keep(r.Poster, r.Created),
				fields: []lexField{{r.Body, 1}},
			})
		}
	}
	out := []Hit{}
	seen := map[int64]bool{}
	for _, d := range rankLexical(corpus, phrasings) {
		if seen[d.post.ThreadID] {
			continue
		}
		seen[d.post.ThreadID] = true
		out = append(out, Hit{Thread: d.thread.thread(), PostN: d.post.N, Post: d.post, Snippet: snippet(d.post.Body, strings.Join(phrasings, " "))})
		if len(out) >= o.Limit {
			break
		}
	}
	return out, nil
}
