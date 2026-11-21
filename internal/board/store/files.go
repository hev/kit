package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

// fileStore keeps the board on the host's filesystem, one JSON file per post:
//
//	<dir>/threads/<id>.json        a thread's opening post (board, title, tags)
//	<dir>/replies/<id>/<n>.json    reply n of thread id (n starts at 2)
//	<dir>/flags/<id>.json          views, useful_by, pinned and locked
//	<dir>/layout                   the layout version, written once
//	<dir>/.lock                    held while flags change
//
// Posts are written once and never change. A new post claims its number by
// creating its file exclusively (O_EXCL semantics, see create): two writers
// that pick the same number race on the create and the loser takes the next
// one. No lock is involved unless the filesystem lacks hard links. A post
// file appears whole or not at all.
// Flags change in place: each change is a read-modify-write under the
// directory lock (platformops.LockPath, the lock peers uses) that replaces
// the flags file by atomic rename. Flags are read under the lock too (on
// Windows a file being replaced cannot be opened); post files are read
// without it, and one that does not parse yet is skipped (see readPost).
type fileStore struct {
	dir string
	// app is an app board store (NewAppFiles): any well-formed board slug is
	// a board, as on NewApp.
	app bool
}

func (f *fileStore) known(slug string) bool {
	if f.app {
		return appBoardSlug.MatchString(slug)
	}
	return isBoard(slug)
}

// filePost is one post file. Board, Title and Tags are set on the opening
// post only.
type filePost struct {
	ID    int64    `json:"id"`
	N     int      `json:"n"`
	Board string   `json:"board,omitempty"`
	Title string   `json:"title,omitempty"`
	Tags  []string `json:"tags,omitempty"`
	Poster
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
	Images  []Image   `json:"images,omitempty"`
}

type fileFlags struct {
	Views    int      `json:"views"`
	UsefulBy []string `json:"useful_by"`
	Pinned   bool     `json:"pinned"`
	Locked   bool     `json:"locked"`
}

// fileThread is a thread as read from disk: its opening post, its replies in
// order and its flags.
type fileThread struct {
	filePost
	fileFlags
	Replies []filePost
}

const fileLayout = "2"

// inFlight is how long a post file that does not parse yet is taken to be one
// another process is still writing, rather than a damaged one.
const inFlight = time.Minute

// NewFiles is the filesystem-mode store rooted at dir. It creates the
// directory if needed, moves a board written by an earlier 0.7 build (one
// file per thread) to one file per post, and never opens a network
// connection.
func NewFiles(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("board directory is empty")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"threads", "replies", "flags"} {
		if err := platformops.MkdirAll(filepath.Join(abs, sub), 0o700); err != nil {
			return nil, fmt.Errorf("board directory: %w", err)
		}
	}
	f := &fileStore{dir: abs}
	if err := f.upgrade(); err != nil {
		return nil, fmt.Errorf("board directory %s: %w", abs, err)
	}
	return &Store{Backend: f, info: Info{Mode: "filesystem", Location: abs}}, nil
}

// NewAppFiles is NewApp with no Layer key: an app board store in files at
// dir (hev collect puts it under the archive root). Its boards are whatever
// the app names, as on NewApp; everything else is NewFiles.
func NewAppFiles(dir string) (*Store, error) {
	st, err := NewFiles(dir)
	if err != nil {
		return nil, err
	}
	st.Backend.(*fileStore).app = true
	return st, nil
}

func (f *fileStore) locked(fn func() error) error {
	unlock, err := platformops.LockPath(filepath.Join(f.dir, ".lock"), false)
	if err != nil {
		return fmt.Errorf("lock board directory: %w", err)
	}
	defer unlock()
	return fn()
}

func (f *fileStore) threadPath(id int64) string {
	return filepath.Join(f.dir, "threads", strconv.FormatInt(id, 10)+".json")
}

func (f *fileStore) replyDir(id int64) string {
	return filepath.Join(f.dir, "replies", strconv.FormatInt(id, 10))
}

func (f *fileStore) replyPath(id int64, n int) string {
	return filepath.Join(f.replyDir(id), strconv.Itoa(n)+".json")
}

func (f *fileStore) flagsPath(id int64) string {
	return filepath.Join(f.dir, "flags", strconv.FormatInt(id, 10)+".json")
}

// create writes a new post file only if none exists at path yet. It reports
// fs.ErrExist when another writer has the number.
//
// The post is written and synced to a temporary file beside path first, then
// published under the final name in one step, so a writer killed mid-post
// leaves at most a stray temporary file, never a claimed but empty or torn
// post. Publishing is a hard link: it fails if path exists, as O_EXCL does,
// and needs no lock. On a filesystem without hard links the temporary file is
// renamed onto path instead, under the board lock (lock) after checking path
// is still free: every board writer publishes under that lock or by link, so
// the rename never replaces another writer's post.
func create(path string, v any, lock func(func() error) error) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	tmp, err := platformops.CreateTemp(dir, ".post-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	err = linkFile(tmp.Name(), path)
	if errors.Is(err, fs.ErrExist) {
		return err
	}
	if err != nil {
		err = lock(func() error {
			if beforePublish != nil {
				beforePublish(path)
			}
			return renameIfAbsent(tmp.Name(), path)
		})
		if err != nil {
			return err
		}
	}
	if d, err := os.Open(dir); err == nil {
		platformops.SyncDir(d)
		d.Close()
	}
	return nil
}

// linkFile and beforePublish let tests take the no-hard-link path and stop a
// writer there just before it publishes, holding the lock.
var (
	linkFile      = os.Link
	beforePublish func(path string)
)

// renameIfAbsent moves tmp to path unless path exists, when it reports
// fs.ErrExist. The caller holds the board lock.
func renameIfAbsent(tmp, path string) error {
	if _, err := os.Lstat(path); err == nil {
		return &fs.PathError{Op: "create", Path: path, Err: fs.ErrExist}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return platformops.Rename(tmp, path)
}

// readPost reads one post file. create never leaves a post that does not
// parse, but another tool writing into the directory might: one that is
// empty or changed in the last minute is skipped (errSkip) the way a post not
// yet made is, and one that stays broken is an error.
func readPost(path string) (*filePost, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p filePost
	if err := json.Unmarshal(b, &p); err != nil {
		if st, serr := os.Stat(path); serr == nil && (st.Size() == 0 || time.Since(st.ModTime()) < inFlight) {
			return nil, errSkip
		}
		return nil, fmt.Errorf("board post %s: %w", path, err)
	}
	return &p, nil
}

var errSkip = errors.New("post still being written")

func (f *fileStore) readFlags(id int64) (fileFlags, error) {
	fl := fileFlags{UsefulBy: []string{}}
	b, err := os.ReadFile(f.flagsPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return fl, nil
	}
	if err != nil {
		return fl, err
	}
	if err := json.Unmarshal(b, &fl); err != nil {
		return fl, fmt.Errorf("board flags %d: %w", id, err)
	}
	if fl.UsefulBy == nil {
		fl.UsefulBy = []string{}
	}
	return fl, nil
}

func (f *fileStore) writeFlags(id int64, fl fileFlags) error {
	b, err := json.MarshalIndent(fl, "", "  ")
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(f.flagsPath(id), append(b, '\n'), 0o600)
}

// patch is a read-modify-write of one thread's flags under the lock.
func (f *fileStore) patch(id int64, fn func(*fileFlags) error) error {
	if _, err := os.Stat(f.threadPath(id)); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return f.locked(func() error {
		fl, err := f.readFlags(id)
		if err != nil {
			return err
		}
		if err := fn(&fl); err != nil {
			return err
		}
		return f.writeFlags(id, fl)
	})
}

// numbered lists the n in <dir>/<n>.json, ascending; temporary files and
// anything else are skipped.
func numbered(dir string) ([]int64, error) {
	entries, err := os.ReadDir(dir)
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
		if n, err := strconv.ParseInt(name, 10, 64); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (f *fileStore) ids() ([]int64, error) { return numbered(filepath.Join(f.dir, "threads")) }

// replies reads thread id's replies in order.
func (f *fileStore) replies(id int64) ([]filePost, error) {
	ns, err := numbered(f.replyDir(id))
	if err != nil {
		return nil, err
	}
	out := make([]filePost, 0, len(ns))
	for _, n := range ns {
		p, err := readPost(f.replyPath(id, int(n)))
		if errors.Is(err, errSkip) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

// flagsOf reads the flags of ids under the lock. Flags files are replaced by
// rename, and on Windows opening one while it is being replaced fails with a
// sharing violation, so flags are only ever read with the lock held. Post
// files are written once and need no lock.
func (f *fileStore) flagsOf(ids ...int64) (map[int64]fileFlags, error) {
	out := make(map[int64]fileFlags, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	err := f.locked(func() error {
		for _, id := range ids {
			fl, err := f.readFlags(id)
			if err != nil {
				return err
			}
			out[id] = fl
		}
		return nil
	})
	return out, err
}

func (f *fileStore) get(id int64) (*fileThread, error) {
	ft, err := f.posts(id)
	if err != nil {
		return nil, err
	}
	fls, err := f.flagsOf(id)
	if err != nil {
		return nil, err
	}
	ft.fileFlags = fls[id]
	return ft, nil
}

// posts reads a thread's opening post and replies, without its flags.
func (f *fileStore) posts(id int64) (*fileThread, error) {
	op, err := readPost(f.threadPath(id))
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errSkip) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rs, err := f.replies(id)
	if err != nil {
		return nil, err
	}
	return &fileThread{filePost: *op, Replies: rs}, nil
}

// read loads every thread.
func (f *fileStore) read() ([]*fileThread, error) {
	ids, err := f.ids()
	if err != nil {
		return nil, err
	}
	out := make([]*fileThread, 0, len(ids))
	var have []int64
	for _, id := range ids {
		t, err := f.posts(id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		have = append(have, id)
	}
	fls, err := f.flagsOf(have...)
	if err != nil {
		return nil, err
	}
	for _, t := range out {
		t.fileFlags = fls[t.ID]
	}
	return out, nil
}

func stamp() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func (ft *fileThread) updated() time.Time {
	if n := len(ft.Replies); n > 0 && ft.Replies[n-1].Created.After(ft.Created) {
		return ft.Replies[n-1].Created
	}
	return ft.Created
}

func (ft *fileThread) thread() Thread {
	t := Thread{
		ID: ft.ID, Board: ft.Board, Title: ft.Title, Tags: append([]string{}, ft.Tags...), Poster: ft.Poster,
		Created: ft.Created, Updated: ft.updated(), Views: ft.Views, Replies: len(ft.Replies),
		Pinned: ft.Pinned, Locked: ft.Locked, body: ft.Body, usefulBy: append([]string{}, ft.UsefulBy...),
	}
	t.Useful = len(t.usefulBy)
	t.Excerpt = excerpt(t.body, 160)
	t.ImageCount = len(ft.Images)
	for _, r := range ft.Replies {
		t.ImageCount += len(r.Images)
	}
	last := &Post{ThreadID: t.ID, N: 1, Poster: Poster{Author: ft.Author}, Created: ft.Created, Title: t.Title, Board: t.Board}
	if n := len(ft.Replies); n > 0 {
		r := ft.Replies[n-1]
		last.N, last.Poster, last.Created = r.N, Poster{Author: r.Author}, r.Created
	}
	t.Last = last
	return t
}

func (ft *fileThread) opener() Post {
	return Post{ID: fmt.Sprintf("t:%d", ft.ID), ThreadID: ft.ID, N: 1, Poster: ft.Poster, Body: ft.Body, Created: ft.Created, Images: served(ft.Images)}
}

func (ft *fileThread) reply(r filePost) Post {
	return Post{ID: fmt.Sprintf("p:%d:%d", ft.ID, r.N), ThreadID: ft.ID, N: r.N, Poster: r.Poster, Body: r.Body, Created: r.Created, Images: served(r.Images)}
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
	ft, err := f.get(id)
	if err != nil {
		return nil, err
	}
	t := ft.thread()
	return &t, nil
}

func (f *fileStore) Posts(ctx context.Context, threadID int64) ([]Post, error) {
	out := []Post{}
	ft, err := f.get(threadID)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out = append(out, ft.opener())
	for _, r := range ft.Replies {
		out = append(out, ft.reply(r))
	}
	return out, nil
}

func (f *fileStore) View(ctx context.Context, id int64) error {
	return f.patch(id, func(fl *fileFlags) error { fl.Views++; return nil })
}

// claim creates the first free numbered file at or after n in dir, writing
// the post fill(n) returns, and reports the number it got.
func (f *fileStore) claim(n int64, path func(int64) string, fill func(int64) any) (int64, error) {
	for ; ; n++ {
		err := create(path(n), fill(n), f.locked)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return n, err
	}
}

// next is one past the highest number in dir.
func next(dir string, first int64) (int64, error) {
	ns, err := numbered(dir)
	if err != nil {
		return 0, err
	}
	if len(ns) > 0 && ns[len(ns)-1] >= first {
		return ns[len(ns)-1] + 1, nil
	}
	return first, nil
}

func (f *fileStore) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	if err := prepThread(&in, f.known); err != nil {
		return nil, err
	}
	start, err := next(filepath.Join(f.dir, "threads"), 1)
	if err != nil {
		return nil, err
	}
	ts := stamp()
	var op filePost
	id, err := f.claim(start, f.threadPath, func(id int64) any {
		body := in.Body
		if in.Suffix != nil {
			body += in.Suffix(id)
		}
		op = filePost{ID: id, N: 1, Board: in.Board, Title: in.Title, Tags: cleanTags(in.Tags), Poster: in.Poster, Body: body, Created: ts, Images: stored(in.Images)}
		return op
	})
	if err != nil {
		return nil, err
	}
	op.ID = id
	ft := &fileThread{filePost: op, fileFlags: fileFlags{UsefulBy: []string{}}}
	t := ft.thread()
	return &t, nil
}

func (f *fileStore) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if err := prepPost(&in); err != nil {
		return nil, err
	}
	if _, err := readPost(f.threadPath(in.ThreadID)); errors.Is(err, os.ErrNotExist) || errors.Is(err, errSkip) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	fls, err := f.flagsOf(in.ThreadID)
	if err != nil {
		return nil, err
	}
	if fls[in.ThreadID].Locked {
		return nil, invalid("thread is locked")
	}
	dir := f.replyDir(in.ThreadID)
	if err := platformops.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	start, err := next(dir, 2)
	if err != nil {
		return nil, err
	}
	ts := stamp()
	var r filePost
	_, err = f.claim(start, func(n int64) string { return f.replyPath(in.ThreadID, int(n)) }, func(n int64) any {
		r = filePost{ID: in.ThreadID, N: int(n), Poster: in.Poster, Body: in.Body, Created: ts, Images: stored(in.Images)}
		return r
	})
	if err != nil {
		return nil, err
	}
	ft := &fileThread{filePost: filePost{ID: in.ThreadID}}
	p := ft.reply(r)
	return &p, nil
}

func (f *fileStore) Useful(ctx context.Context, threadID int64, author string) (int, error) {
	if author == "" {
		return 0, invalid("a vote needs an author")
	}
	n := 0
	err := f.patch(threadID, func(fl *fileFlags) error {
		if !hasTag(fl.UsefulBy, author) {
			fl.UsefulBy = append(fl.UsefulBy, author)
		}
		n = len(fl.UsefulBy)
		return nil
	})
	return n, err
}

func (f *fileStore) SetPinned(ctx context.Context, id int64, pinned bool) (bool, error) {
	changed := false
	err := f.patch(id, func(fl *fileFlags) error {
		changed = fl.Pinned != pinned
		fl.Pinned = pinned
		return nil
	})
	return changed && err == nil, err
}

func (f *fileStore) SetLocked(ctx context.Context, id int64, locked bool) error {
	return f.patch(id, func(fl *fileFlags) error { fl.Locked = locked; return nil })
}

// rowsOf is every post as the Layer attributes Stats, Online and Profiles
// read, newest first, so both modes count them with the same code.
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

// legacyThread is the one-file-per-thread layout earlier 0.7 builds wrote to
// threads/<id>.json.
type legacyThread struct {
	filePost
	fileFlags
	Replies *[]filePost `json:"replies"`
}

// upgrade moves a board in the one-file-per-thread layout to one file per
// post, once, under the lock. Replies and flags go to their own files first;
// the thread file is rewritten as its opening post last, so an upgrade cut
// short is finished by the next one.
func (f *fileStore) upgrade() error {
	marker := filepath.Join(f.dir, "layout")
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == fileLayout {
		return nil
	}
	return f.locked(func() error {
		if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == fileLayout {
			return nil
		}
		ids, err := f.ids()
		if err != nil {
			return err
		}
		for _, id := range ids {
			b, err := os.ReadFile(f.threadPath(id))
			if err != nil {
				return err
			}
			var lt legacyThread
			if err := json.Unmarshal(b, &lt); err != nil {
				return fmt.Errorf("thread %d: %w", id, err)
			}
			if lt.Replies == nil {
				continue
			}
			if len(*lt.Replies) > 0 {
				if err := platformops.MkdirAll(f.replyDir(id), 0o700); err != nil {
					return err
				}
			}
			// The legacy thread file stays the source of truth until it is
			// rewritten below, so each reply is (re)written whole from it:
			// a reply file an interrupted upgrade left empty or torn is
			// replaced, never trusted.
			for _, r := range *lt.Replies {
				r.ID = id
				rb, err := json.MarshalIndent(r, "", "  ")
				if err != nil {
					return err
				}
				if err := platformops.WriteFileAtomic(f.replyPath(id, r.N), append(rb, '\n'), 0o600); err != nil {
					return err
				}
			}
			if lt.UsefulBy == nil {
				lt.UsefulBy = []string{}
			}
			if err := f.writeFlags(id, lt.fileFlags); err != nil {
				return err
			}
			op := lt.filePost
			op.ID, op.N = id, 1
			ob, err := json.MarshalIndent(op, "", "  ")
			if err != nil {
				return err
			}
			if err := platformops.WriteFileAtomic(f.threadPath(id), append(ob, '\n'), 0o600); err != nil {
				return err
			}
		}
		return platformops.WriteFileAtomic(marker, []byte(fileLayout+"\n"), 0o600)
	})
}

func (f *fileStore) Search(ctx context.Context, o SearchOpts) ([]Hit, error) {
	var phrasings [][]term
	var raw []string
	for _, p := range append([]string{o.Q}, o.Also...) {
		if p = strings.TrimSpace(p); p != "" {
			raw = append(raw, p)
		}
	}
	if len(raw) == 0 {
		return []Hit{}, nil
	}
	if len(raw) > search.MaxPhrasings {
		return nil, invalid("at most %d phrasings in one search, got %d", search.MaxPhrasings, len(raw))
	}
	var words []string
	for _, p := range raw {
		ts := termsOf(p)
		phrasings = append(phrasings, ts)
		for _, t := range ts {
			words = append(words, t.folded)
			if t.raw != "" {
				words = append(words, t.raw)
			}
		}
	}
	if len(words) == 0 {
		return []Hit{}, nil
	}
	if o.Limit <= 0 {
		o.Limit = 10
	}
	if o.Board != "" && !f.known(o.Board) {
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
	fts, only, err := f.candidates(ctx, words)
	if err != nil {
		return nil, err
	}
	tags := cleanTags(o.Tags)
	var docs []grepDoc
	for _, ft := range fts {
		inThread := o.Board == "" || ft.Board == o.Board
		for _, t := range tags {
			inThread = inThread && hasTag(ft.Tags, t)
		}
		if !inThread {
			continue
		}
		keep := func(p Poster, created time.Time, path string) bool {
			return (o.Author == "" || p.Author == o.Author) && (o.Role == "" || p.Role == o.Role) &&
				(since.IsZero() || !created.Before(since)) && (only == nil || only[path])
		}
		if path := f.threadPath(ft.ID); keep(ft.Poster, ft.Created, path) {
			docs = append(docs, grepDoc{
				thread: ft, path: path,
				post: Post{ID: fmt.Sprintf("t:%d", ft.ID), ThreadID: ft.ID, N: 1, Poster: Poster{Author: ft.Author, Role: ft.Role}, Body: ft.Body, Created: ft.Created},
				text: strings.ToLower(ft.Title + "\n" + strings.Join(ft.Tags, " ") + "\n" + ft.Body),
			})
		}
		for _, r := range ft.Replies {
			if path := f.replyPath(ft.ID, r.N); keep(r.Poster, r.Created, path) {
				docs = append(docs, grepDoc{
					thread: ft, path: path,
					post: Post{ID: fmt.Sprintf("p:%d:%d", ft.ID, r.N), ThreadID: ft.ID, N: r.N, Poster: Poster{Author: r.Author, Role: r.Role}, Body: r.Body, Created: r.Created},
					text: strings.ToLower(r.Body),
				})
			}
		}
	}
	out := []Hit{}
	seen := map[int64]bool{}
	for _, d := range grepRank(docs, phrasings) {
		if seen[d.post.ThreadID] {
			continue
		}
		seen[d.post.ThreadID] = true
		out = append(out, Hit{Thread: d.thread.thread(), PostN: d.post.N, Post: d.post, Snippet: snippet(d.post.Body, strings.Join(words, " "))})
		if len(out) >= o.Limit {
			break
		}
	}
	return out, nil
}

// candidates is the threads search reads. With rg on PATH it is only the
// threads that have a post file holding some word, and only names those
// files; without rg (or when rg fails) it is every thread, and only is nil.
func (f *fileStore) candidates(ctx context.Context, words []string) (fts []*fileThread, only map[string]bool, err error) {
	rg := rgPath()
	if rg == "" {
		fts, err = f.read()
		return fts, nil, err
	}
	threads, replies := filepath.Join(f.dir, "threads"), filepath.Join(f.dir, "replies")
	hits, ok := rgFiles(ctx, rg, words, []string{threads, replies})
	if !ok {
		fts, err = f.read()
		return fts, nil, err
	}
	only = map[string]bool{}
	ids := map[int64]bool{}
	for p := range hits {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(f.dir, abs)
		}
		only[abs] = true
		var idPart string
		if rel, err := filepath.Rel(threads, abs); err == nil && !strings.HasPrefix(rel, "..") {
			idPart = strings.TrimSuffix(rel, ".json")
		} else if rel, err := filepath.Rel(replies, abs); err == nil && !strings.HasPrefix(rel, "..") {
			idPart = filepath.Dir(rel)
		}
		if id, err := strconv.ParseInt(idPart, 10, 64); err == nil && id > 0 {
			ids[id] = true
		}
	}
	sorted := make([]int64, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, id := range sorted {
		ft, err := f.get(id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		fts = append(fts, ft)
	}
	return fts, only, nil
}
