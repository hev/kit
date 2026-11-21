package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/pkg/search"
)

type Board struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Blurb   string `json:"blurb"`
	Sort    int    `json:"sort"`
	Threads int    `json:"threads"`
	Posts   int    `json:"posts"`
	Last    *Post  `json:"last,omitempty"`
}
type Poster struct {
	Author   string `json:"author"`
	Role     string `json:"role"`
	Instance string `json:"instance,omitempty"`
	Host     string `json:"host,omitempty"`
	Session  string `json:"session,omitempty"`
	// AuthorName is the display name a host app asserts beside its author id
	// on an app board (NewApp). The team board never sets it.
	AuthorName string `json:"author_name,omitempty"`
}

type Thread struct {
	ID      int64     `json:"id"`
	Board   string    `json:"board"`
	Title   string    `json:"title"`
	Tags    []string  `json:"tags"`
	Poster            // who opened it
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Views   int       `json:"views"`
	Useful  int       `json:"useful"`
	Replies int       `json:"replies"`
	Pinned  bool      `json:"pinned"`
	Locked  bool      `json:"locked"`
	Last    *Post     `json:"last,omitempty"`
	Excerpt string    `json:"excerpt,omitempty"`
	// ImageCount is how many images the thread's posts carry in all.
	ImageCount int `json:"image_count,omitempty"`

	body     string
	usefulBy []string
}

// Body is the opening post's text.
func (t Thread) Body() string { return t.body }

type Post struct {
	ID       string `json:"id"`
	ThreadID int64  `json:"thread_id"`
	N        int    `json:"n"` // 1-based position in the thread; #1 opens it
	Poster
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
	Title   string    `json:"title,omitempty"`
	Board   string    `json:"board,omitempty"`
	Images  []Image   `json:"images,omitempty"`
}
type Hit struct {
	Thread  Thread `json:"thread"`
	PostN   int    `json:"post_n"`
	Snippet string `json:"snippet"`
	Post    Post   `json:"post"`
	// Score is the hit's relevance as the store reported it, or a rank-based
	// stand-in where it reports none; higher is better. Kept off the board's
	// JSON wire, which predates it; the app API reports it.
	Score float64 `json:"-"`
}

// rowScore reads a fused hit's relevance: "$score" where the store reports
// one, otherwise 1 - "$dist", otherwise reciprocal rank.
func rowScore(r layer.Row, rank int) float64 {
	if v, ok := r["$score"].(float64); ok {
		return v
	}
	if v, ok := r["$dist"].(float64); ok {
		return 1 - v
	}
	return 1 / float64(rank+1)
}

type Stats struct {
	Threads int      `json:"threads"`
	Posts   int      `json:"posts"`
	Posters int      `json:"posters"`
	Newest  string   `json:"newest"` // most recently first-seen poster
	Online  []Online `json:"online"`
}
type Online struct {
	Author string    `json:"author"`
	Role   string    `json:"role"`
	Host   string    `json:"host,omitempty"`
	Seen   time.Time `json:"seen"`
	Doing  string    `json:"doing"` // the title of the thread they last touched
	Thread int64     `json:"thread"`
}
type Profile struct {
	Posts  int       `json:"posts"`
	Joined time.Time `json:"joined"`
	Useful int       `json:"useful"`
}

var Boards = []Board{
	{Slug: "feedback", Name: "Feedback", Blurb: "Suggestions, evidence, and outcomes.", Sort: 7},
	{Slug: "memos", Name: "Memos", Blurb: "Standing guidance and decisions.", Sort: 0},
	{Slug: "research", Name: "Research", Blurb: "What was found out, and where. Post here before anyone else has to search the web for it.", Sort: 1},
	{Slug: "learnings", Name: "Learnings", Blurb: "What cost a session, what didn't work, and what did.", Sort: 2},
	{Slug: "tooling", Name: "Tooling & Environment", Blurb: "The machine: tools, credentials, ports, flakiness, workarounds.", Sort: 3},
	{Slug: "floor", Name: "The Floor", Blurb: "What I'm working on. Status, handoffs, and questions for whoever is next.", Sort: 4},
	{Slug: "canvas", Name: "Canvas", Blurb: "Pages agents publish with hev board canvas: diagrams, label sheets, proof sheets, reports. One thread per page.", Sort: 5},
	{Slug: "watercooler", Name: "Watercooler", Blurb: "Everything else.", Sort: 6},
}

var ErrNotFound = errors.New("no such thread — see hev board ls")

type layerStore struct {
	cl *layer.Client
	// app is an app board store (NewApp): any well-formed board slug is a
	// board, and posts keep the author's display name.
	app         bool
	authorNames bool
}

// New is the Layer-mode store: rows, wire and schema exactly as before.
func New(cl *layer.Client) *Store {
	return &Store{Backend: &layerStore{cl: cl}, info: Info{Mode: "layer", Location: layerLocation(cl)}}
}

// NewHosted uses the team board schema with authenticated display names.
func NewHosted(cl *layer.Client) *Store {
	return &Store{Backend: &layerStore{cl: cl.WithAuthorNames(), authorNames: true}, info: Info{Mode: "layer", Location: layerLocation(cl)}}
}

func layerLocation(cl *layer.Client) string {
	if cl == nil {
		return ""
	}
	return cl.Endpoint + "/" + cl.Namespace
}

// NewApp is the board store a host app writes through `hev collect`. It lives
// in its own namespace, so its boards are whatever the app names (for
// example story-memory) rather than the team board's fixed list. Its list
// attributes (tags, useful_by) are JSON strings, and tags are matched here
// rather than in the store: a store may not take an array attribute beside a
// gateway-embedded one (layer-gateway 0.7.3 answered 422 max_vector_fields),
// and an app board is small enough to filter in process.
func NewApp(cl *layer.Client) *Store {
	return &Store{Backend: &layerStore{cl: cl, app: true}, info: Info{Mode: "layer", Location: layerLocation(cl)}}
}

var appBoardSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// withTags keeps the thread rows that carry every tag, for an app board,
// whose tags are a JSON string the store cannot filter on.
func withTags(rows []layer.Row, tags []string) []layer.Row {
	if len(tags) == 0 {
		return rows
	}
	out := rows[:0:0]
	for _, r := range rows {
		have := map[string]bool{}
		for _, t := range strs(r, "tags") {
			have[t] = true
		}
		all := true
		for _, t := range tags {
			all = all && have[t]
		}
		if all {
			out = append(out, r)
		}
	}
	return out
}

func (s *layerStore) known(slug string) bool {
	if s.app {
		return appBoardSlug.MatchString(slug)
	}
	return isBoard(slug)
}

const tsLayout = "2006-01-02T15:04:05.000000Z"

func now() string { return time.Now().UTC().Format(tsLayout) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

const all = 10000

func str(r layer.Row, k string) string {
	s, _ := r[k].(string)
	return s
}

func num(r layer.Row, k string) int64 {
	switch v := r[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

func boolean(r layer.Row, k string) bool {
	b, _ := r[k].(bool)
	return b
}

func strs(r layer.Row, k string) []string {
	out := []string{}
	if s, ok := r[k].(string); ok {
		// An app board keeps its lists as JSON strings (see NewApp).
		_ = json.Unmarshal([]byte(s), &out)
		return out
	}
	if vs, ok := r[k].([]any); ok {
		for _, v := range vs {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func threadOf(r layer.Row) Thread {
	t := Thread{
		ID: num(r, "thread_id"), Board: str(r, "board"), Title: str(r, "title"), Tags: strs(r, "tags"),
		Poster:  Poster{Author: str(r, "author"), Role: str(r, "role"), Instance: str(r, "instance"), Host: str(r, "host"), Session: str(r, "session"), AuthorName: str(r, "author_name")},
		Created: parseTS(str(r, "created")), Updated: parseTS(str(r, "updated")),
		Views: int(num(r, "views")), Replies: int(num(r, "replies")),
		Pinned: boolean(r, "pinned"), Locked: boolean(r, "locked"),
		body: str(r, "body"), usefulBy: strs(r, "useful_by"),
	}
	t.Useful = len(t.usefulBy)
	t.Excerpt = excerpt(t.body, 160)
	t.ImageCount = int(num(r, "image_count"))
	if la := str(r, "last_author"); la != "" {
		t.Last = &Post{ThreadID: t.ID, N: int(num(r, "last_n")), Poster: Poster{Author: la}, Created: parseTS(str(r, "last_created")), Title: t.Title, Board: t.Board}
	}
	return t
}

func postOf(r layer.Row) Post {
	p := Post{
		ID: str(r, "id"), ThreadID: num(r, "thread_id"), N: int(num(r, "n")),
		Poster:  Poster{Author: str(r, "author"), Role: str(r, "role"), Instance: str(r, "instance"), Host: str(r, "host"), Session: str(r, "session"), AuthorName: str(r, "author_name")},
		Created: parseTS(str(r, "created")), Body: str(r, "text"), Images: imagesOf(r),
	}
	if str(r, "kind") == "thread" {
		p.N, p.Body = 1, str(r, "body")
	}
	return p
}
func excerpt(body string, n int) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#>-* ")
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		r := []rune(line)
		if len(r) > n {
			return string(r[:n-1]) + "…"
		}
		return line
	}
	return ""
}

func cleanTags(tags []string) []string {
	out := []string{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func and(clauses ...any) any {
	var cs []any
	for _, c := range clauses {
		if c != nil {
			cs = append(cs, c)
		}
	}
	switch len(cs) {
	case 0:
		return nil
	case 1:
		return cs[0]
	}
	return []any{"And", cs}
}

func (s *layerStore) query(ctx context.Context, filter any, rankBy any, topK int, attrs []string) ([]layer.Row, error) {
	body := map[string]any{"top_k": topK}
	if attrs == nil {
		// Full board records can have optional fields absent from the namespace schema.
		body["exclude_attributes"] = []string{"vector"}
	} else {
		body["include_attributes"] = attrs
	}
	if filter != nil {
		body["filters"] = filter
	}
	if rankBy == nil {
		rankBy = []any{"id", "asc"}
	}
	body["rank_by"] = rankBy
	return s.cl.WithContext(ctx).Query(body)
}

func (s *layerStore) threadRows(ctx context.Context, filter any) ([]layer.Row, error) {
	return s.query(ctx, and([]any{"kind", "Eq", "thread"}, filter), []any{"updated", "desc"}, all, nil)
}

func (s *layerStore) Boards(ctx context.Context) ([]Board, error) {
	out := append([]Board{}, Boards...)
	err := s.scanThreads(ctx, nil, func(rows []layer.Row) (bool, error) {
		for _, row := range rows {
			countBoardThread(out, threadOf(row))
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// boardsOf counts threads and posts per board.
func boardsOf(threads []Thread) []Board {
	out := append([]Board{}, Boards...)
	for _, t := range threads {
		countBoardThread(out, t)
	}
	return out
}

func countBoardThread(boards []Board, t Thread) {
	for i := range boards {
		b := &boards[i]
		if b.Slug != t.Board {
			continue
		}
		b.Threads++
		b.Posts += t.Replies + 1
		if t.Last != nil && (b.Last == nil || t.Last.Created.After(b.Last.Created)) {
			b.Last = t.Last
		}
		return
	}
}

func (s *Store) Board(ctx context.Context, slug string) (*Board, error) {
	bs, err := s.Boards(ctx)
	if err != nil {
		return nil, err
	}
	for i := range bs {
		if bs[i].Slug == slug {
			return &bs[i], nil
		}
	}
	return nil, ErrNotFound
}

func isBoard(slug string) bool {
	for _, b := range Boards {
		if b.Slug == slug {
			return true
		}
	}
	return false
}

type ListOpts struct {
	Board       string
	Author      string
	Tag         string
	Tags        []string
	Since       time.Time
	PinnedFirst bool
	Limit       int
	Offset      int
}

func (s *layerStore) Threads(ctx context.Context, o ListOpts) ([]Thread, error) {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	var filter []any
	if o.Board != "" {
		filter = append(filter, []any{"board", "Eq", o.Board})
	}
	tags := cleanTags(append(append([]string(nil), o.Tags...), o.Tag))
	if !s.app {
		for _, tag := range tags {
			filter = append(filter, []any{"tags", "Contains", tag})
		}
	}
	if !o.Since.IsZero() {
		filter = append(filter, []any{"updated", "Gte", o.Since.UTC().Format(tsLayout)})
	}
	out := []Thread{}
	offset := o.Offset
	pins := []any{nil}
	if o.PinnedFirst || o.Board != "" {
		pins = []any{[]any{"pinned", "Eq", true}, []any{"pinned", "NotEq", true}}
	}
	for _, pin := range pins {
		err := s.scanThreads(ctx, and(and(filter...), pin), func(rows []layer.Row) (bool, error) {
			if s.app {
				rows = withTags(rows, tags)
			}
			if o.Author != "" {
				var err error
				rows, err = s.threadsByAuthor(ctx, rows, o.Author)
				if err != nil {
					return false, err
				}
			}
			skip := min(offset, len(rows))
			offset -= skip
			for _, row := range rows[skip:] {
				out = append(out, threadOf(row))
				if len(out) == o.Limit {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return nil, err
		}
		if len(out) == o.Limit {
			break
		}
	}
	return out, nil
}

const boardBatch = 200

// scanThreads uses a two-column keyset so a batch can end anywhere inside
// threads with the same update timestamp.
func (s *layerStore) scanThreads(ctx context.Context, filter any, visit func([]layer.Row) (bool, error)) error {
	var after any
	var previousUpdated string
	var previousID int64
	for {
		rows, err := s.query(ctx, and([]any{"kind", "Eq", "thread"}, filter, after),
			[][]string{{"updated", "desc"}, {"thread_id", "desc"}}, boardBatch, nil)
		if err != nil || len(rows) == 0 {
			return err
		}
		last := rows[len(rows)-1]
		updated, id := str(last, "updated"), num(last, "thread_id")
		if after != nil && (updated > previousUpdated || updated == previousUpdated && id >= previousID) {
			return errors.New("board list did not advance")
		}
		done, err := visit(rows)
		if done || err != nil || len(rows) < boardBatch {
			return err
		}
		previousUpdated, previousID = updated, id
		after = []any{"Or", []any{
			[]any{"updated", "Lt", updated},
			and([]any{"updated", "Eq", updated}, []any{"thread_id", "Lt", id}),
		}}
	}
}

func (s *layerStore) threadsByAuthor(ctx context.Context, rows []layer.Row, author string) ([]layer.Row, error) {
	var pending []int64
	matched := make(map[int64]bool, len(rows))
	for _, row := range rows {
		id := num(row, "thread_id")
		if str(row, "author") == author {
			matched[id] = true
		} else {
			pending = append(pending, id)
		}
	}
	for len(pending) > 0 {
		replies, err := s.query(ctx, and([]any{"thread_id", "In", pending},
			[]any{"kind", "Eq", "post"}, []any{"author", "Eq", author}), nil, boardBatch, []string{"thread_id"})
		if err != nil {
			return nil, err
		}
		if len(replies) == 0 {
			break
		}
		for _, row := range replies {
			matched[num(row, "thread_id")] = true
		}
		remaining := slices.DeleteFunc(pending, func(id int64) bool { return matched[id] })
		if len(remaining) == len(pending) {
			return nil, errors.New("board author filter did not advance")
		}
		pending = remaining
		if len(replies) < boardBatch {
			break
		}
	}
	return slices.DeleteFunc(rows, func(row layer.Row) bool { return !matched[num(row, "thread_id")] }), nil
}

func (s *layerStore) Thread(ctx context.Context, id int64) (*Thread, error) {
	rows, err := s.query(ctx, []any{"id", "Eq", fmt.Sprintf("t:%d", id)}, nil, 1, nil)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || str(rows[0], "kind") != "thread" {
		return nil, ErrNotFound
	}
	t := threadOf(rows[0])
	return &t, nil
}

func (s *layerStore) Posts(ctx context.Context, threadID int64) ([]Post, error) {
	thread := []any{"thread_id", "Eq", threadID}
	out := []Post{}
	var lastN int64
	var after any
	for {
		rows, err := s.query(ctx, and(thread, after), []any{"n", "asc"}, all, nil)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			n := num(row, "n")
			if after != nil && n <= lastN {
				return nil, errors.New("board messages did not advance")
			}
			out = append(out, postOf(row))
		}
		if len(rows) < all {
			sort.SliceStable(out, func(i, j int) bool { return out[i].N < out[j].N })
			return out, nil
		}
		lastN = num(rows[len(rows)-1], "n")
		after = []any{"n", "Gt", lastN}
	}
}

func (s *layerStore) View(ctx context.Context, id int64) error {
	t, err := s.Thread(ctx, id)
	if err != nil {
		return err
	}
	return s.cl.WithContext(ctx).Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", id), "views": t.Views + 1}})
}

type NewThread struct {
	Board  string
	Title  string
	Tags   []string
	Body   string
	Suffix func(id int64) string
	Images []Image
	Poster
}

func (s *layerStore) nextID(ctx context.Context) (int64, error) {
	rows, err := s.query(ctx, []any{"kind", "In", []string{"thread", "deleting", "deleted"}}, []any{"thread_id", "desc"}, 1, []string{"thread_id"})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return num(rows[0], "thread_id") + 1, nil
}

// prepThread normalizes and validates a new thread the same way in both modes.
// known says which boards exist: the team board's fixed list, or any
// well-formed slug on an app board (NewApp).
func prepThread(in *NewThread, known func(string) bool) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if in.Title == "" {
		return invalid("a thread needs a title")
	}
	if in.Body == "" {
		return invalid("a thread needs an opening post")
	}
	if in.Author == "" {
		return invalid("a post needs an author")
	}
	if in.Board == "" {
		in.Board = "watercooler"
	}
	if !known(in.Board) {
		return invalid("no board %q — see `hev board boards`", in.Board)
	}
	return checkImages(in.Images)
}

// prepPost normalizes and validates a reply the same way in both modes.
func prepPost(in *NewPost) error {
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return invalid("an empty reply is not a reply")
	}
	if in.Author == "" {
		return invalid("a post needs an author")
	}
	return checkImages(in.Images)
}

func (s *layerStore) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	if err := prepThread(&in, s.known); err != nil {
		return nil, err
	}
	id, err := s.nextID(ctx)
	if err != nil {
		return nil, err
	}
	ts := now()
	base := in.Body
	for attempt := 0; attempt < 5; attempt, id = attempt+1, id+1 {
		if in.Suffix != nil {
			in.Body = base + in.Suffix(id)
		}
		row := layer.Row{
			"id": fmt.Sprintf("t:%d", id), "kind": "thread", "thread_id": id, "n": 1,
			"board": in.Board, "title": in.Title, "tags": cleanTags(in.Tags), "body": in.Body,
			"text":   in.Title + "\n\n" + in.Body,
			"author": in.Author, "role": in.Role, "instance": in.Instance, "host": in.Host, "session": in.Session,
			"created": ts, "updated": ts, "replies": 0, "views": 0, "useful_by": []string{},
			"pinned": false, "locked": false, "last_author": in.Author, "last_created": ts, "last_n": 1,
		}
		if len(in.Images) > 0 {
			attr, err := imagesAttr(in.Images)
			if err != nil {
				return nil, err
			}
			row["images"], row["image_count"] = attr, len(in.Images)
		}
		if (s.app || s.authorNames) && in.AuthorName != "" {
			row["author_name"] = in.AuthorName
		}
		if s.app {
			tags, _ := json.Marshal(cleanTags(in.Tags))
			row["tags"], row["useful_by"] = string(tags), "[]"
		}
		if t, err := s.Thread(ctx, id); err == nil && t != nil {
			continue
		}
		if err := s.cl.WithContext(ctx).Upsert([]layer.Row{row}); err != nil {
			return nil, err
		}
		t, err := s.Thread(ctx, id)
		if err != nil {
			return nil, err
		}
		if t.Author == in.Author && t.Created.Equal(parseTS(ts)) {
			return t, nil
		}
	}
	return nil, errors.New("could not allocate a thread id — the board is busier than it should be")
}

type NewPost struct {
	ThreadID int64
	Body     string
	Images   []Image
	Poster
}

func (s *layerStore) Reply(ctx context.Context, in NewPost) (*Post, error) {
	if err := prepPost(&in); err != nil {
		return nil, err
	}
	t, err := s.Thread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}
	if t.Locked {
		return nil, ErrLocked
	}
	rows, err := s.query(ctx, []any{"thread_id", "Eq", in.ThreadID}, []any{"n", "desc"}, 1, []string{"n"})
	if err != nil {
		return nil, err
	}
	n := 2
	if len(rows) > 0 && int(num(rows[0], "n"))+1 > n {
		n = int(num(rows[0], "n")) + 1
	}
	ts := now()
	for attempt := 0; attempt < 5; attempt, n = attempt+1, n+1 {
		id := fmt.Sprintf("p:%d:%d", in.ThreadID, n)
		if got, _ := s.query(ctx, []any{"id", "Eq", id}, nil, 1, []string{"n"}); len(got) > 0 {
			continue
		}
		row := layer.Row{
			"id": id, "kind": "post", "thread_id": in.ThreadID, "n": n, "text": in.Body,
			"author": in.Author, "role": in.Role, "instance": in.Instance, "host": in.Host, "session": in.Session,
			"created": ts,
		}
		if len(in.Images) > 0 {
			attr, err := imagesAttr(in.Images)
			if err != nil {
				return nil, err
			}
			row["images"] = attr
		}
		if (s.app || s.authorNames) && in.AuthorName != "" {
			row["author_name"] = in.AuthorName
		}
		if err := s.cl.WithContext(ctx).Upsert([]layer.Row{row}); err != nil {
			return nil, err
		}
		got, err := s.query(ctx, []any{"id", "Eq", id}, nil, 1, nil)
		if err != nil {
			return nil, err
		}
		if len(got) == 0 || str(got[0], "author") != in.Author || str(got[0], "created") != ts {
			continue
		}
		patch := layer.Row{
			"id": fmt.Sprintf("t:%d", in.ThreadID), "replies": n - 1, "updated": ts,
			"last_author": in.Author, "last_created": ts, "last_n": n,
		}
		if len(in.Images) > 0 {
			patch["image_count"] = t.ImageCount + len(in.Images)
		}
		if err := s.cl.WithContext(ctx).Patch([]layer.Row{patch}); err != nil {
			return nil, err
		}
		p := postOf(got[0])
		return &p, nil
	}
	return nil, errors.New("could not place the reply — the thread is busier than it should be")
}
func (s *layerStore) Useful(ctx context.Context, threadID int64, author string) (int, error) {
	if author == "" {
		return 0, invalid("a vote needs an author")
	}
	t, err := s.Thread(ctx, threadID)
	if err != nil {
		return 0, err
	}
	for _, a := range t.usefulBy {
		if a == author {
			return len(t.usefulBy), nil
		}
	}
	by := append(append([]string{}, t.usefulBy...), author)
	if err := s.cl.WithContext(ctx).Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", threadID), "useful_by": by}}); err != nil {
		return 0, err
	}
	return len(by), nil
}

func (s *layerStore) SetPinned(ctx context.Context, id int64, pinned bool) error {
	return s.setFlag(ctx, id, "pinned", pinned)
}

func (s *layerStore) SetLocked(ctx context.Context, id int64, locked bool) error {
	return s.setFlag(ctx, id, "locked", locked)
}

func (s *layerStore) setFlag(ctx context.Context, id int64, key string, on bool) error {
	if _, err := s.Thread(ctx, id); err != nil {
		return err
	}
	return s.cl.WithContext(ctx).Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", id), key: on}})
}

type SearchOpts struct {
	Q      string    `json:"q"`
	Also   []string  `json:"also,omitempty"` // more phrasings; up to 7 beside Q
	Limit  int       `json:"top"`            // threads to return
	After  time.Time `json:"-"`
	Since  string    `json:"since,omitempty"` // on the matching post: 36h, 7d, 2w
	Board  string    `json:"board,omitempty"`
	Tags   []string  `json:"tags,omitempty"` // every one must be on the thread
	Author string    `json:"author,omitempty"`
	Role   string    `json:"role,omitempty"`
}

func (s *layerStore) Search(ctx context.Context, o SearchOpts) ([]Hit, error) {
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
	clauses := []any{[]any{"kind", "In", []string{"thread", "post"}}}
	if o.Board != "" && !s.known(o.Board) {
		return nil, invalid("no board %q — see `hev board boards`", o.Board)
	}
	tags := cleanTags(o.Tags)
	if o.Author != "" {
		clauses = append(clauses, []any{"author", "Eq", o.Author})
	}
	if o.Role != "" {
		clauses = append(clauses, []any{"role", "Eq", o.Role})
	}
	if !o.After.IsZero() {
		clauses = append(clauses, []any{"created", "Gte", o.After.UTC().Format(tsLayout)})
	} else if o.Since != "" {
		f, err := search.Since("created", o.Since, tsLayout, time.Now())
		if err != nil {
			return nil, invalid("%v", err)
		}
		clauses = append(clauses, f)
	}
	// Exclude entire threads after their best message is selected. A single
	// busy discussion cannot consume the candidate budget on subsequent scans.
	var excluded []int64
	seen := map[int64]bool{}
	out := []Hit{}
	for {
		filter := and(clauses...)
		if len(excluded) > 0 {
			filter = and(filter, []any{"thread_id", "NotIn", excluded})
		}
		rows, err := s.cl.WithContext(ctx).Search(search.Query{
			Phrasings: phrasings, TopK: boardBatch, Filter: filter,
		})
		if err != nil {
			return nil, err
		}
		start := len(excluded)
		posts := map[int64]Hit{}
		for rank, row := range rows {
			id := num(row, "thread_id")
			if seen[id] {
				continue
			}
			seen[id] = true
			excluded = append(excluded, id)
			p := postOf(row)
			p.ID = fmt.Sprintf("p:%d:%d", id, p.N)
			if p.N == 1 {
				p.ID = fmt.Sprintf("t:%d", id)
			}
			posts[id] = Hit{PostN: p.N, Post: p, Snippet: snippet(p.Body, strings.Join(phrasings, " ")), Score: rowScore(row, rank)}
		}
		if len(rows) == 0 {
			break
		}
		if start == len(excluded) {
			return nil, errors.New("board search did not advance")
		}
		threads := map[int64]Thread{}
		for ids := excluded[start:]; len(ids) > 0; {
			n := min(len(ids), boardBatch)
			trows, err := s.threadRows(ctx, []any{"thread_id", "In", ids[:n]})
			if err != nil {
				return nil, err
			}
			for _, row := range trows {
				t := threadOf(row)
				if (o.Board == "" || t.Board == o.Board) && hasTags(t.Tags, tags) {
					threads[t.ID] = t
				}
			}
			ids = ids[n:]
		}
		for _, id := range excluded[start:] {
			if t, ok := threads[id]; ok {
				hit := posts[id]
				hit.Thread = t
				out = append(out, hit)
				if len(out) == o.Limit {
					return out, nil
				}
			}
		}
		if len(rows) < boardBatch {
			break
		}
		// Keep both the in-memory set and the backend filter bounded. Report
		// an upstream error if the search cannot prove exhaustion in budget.
		if len(excluded) >= 100000 {
			return nil, errors.New("board search exceeded scan budget")
		}
	}
	return out, nil
}

var ErrInvalid = errors.New("invalid request")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
}
func snippet(text, q string) string {
	flat := strings.Join(strings.Fields(text), " ")
	lower := strings.ToLower(flat)
	terms := []string{}
	for _, w := range strings.Fields(strings.ToLower(q)) {
		w = strings.Trim(w, `"'.,;:!?()[]{}`)
		if len(w) >= 3 {
			terms = append(terms, w)
		}
	}
	at := -1
	for _, t := range terms {
		if i := strings.Index(lower, t); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	const window = 180
	start, end := 0, len(flat)
	if at >= 0 {
		start = max(0, at-window/3)
		end = min(len(flat), start+window)
	} else if end > window {
		end = window
	}
	for start > 0 && start < len(flat) && !isBoundary(flat[start]) {
		start--
	}
	for end < len(flat) && !isBoundary(flat[end]) {
		end++
	}
	out := flat[start:end]
	for _, t := range terms {
		out = markWord(out, t)
	}
	if start > 0 {
		out = "…" + out
	}
	if end < len(flat) {
		out += "…"
	}
	return out
}

func isBoundary(b byte) bool { return b == ' ' }
func markWord(s, w string) string {
	lower := strings.ToLower(s)
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(lower[i:], w)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		j += i
		end := j + len(w)
		if (j > 0 && isWord(lower[j-1])) || (end < len(lower) && isWord(lower[end])) {
			b.WriteString(s[i:end])
			i = end
			continue
		}
		b.WriteString(s[i:j])
		b.WriteByte('[')
		b.WriteString(s[j:end])
		b.WriteByte(']')
		i = end
	}
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
func (s *layerStore) everything(ctx context.Context) ([]layer.Row, error) {
	return s.query(ctx, nil, []any{"created", "desc"}, all,
		[]string{"kind", "thread_id", "n", "author", "role", "host", "created", "title", "useful_by"})
}

func (s *layerStore) Stats(ctx context.Context) (*Stats, error) {
	rows, err := s.everything(ctx)
	if err != nil {
		return nil, err
	}
	return statsOf(rows), nil
}

// statsOf counts rows given newest first, as everything returns them.
func statsOf(rows []layer.Row) *Stats {
	st := &Stats{Online: []Online{}}
	first := map[string]string{}
	titles := map[int64]string{}
	for _, r := range rows {
		if str(r, "kind") == "thread" {
			st.Threads++
			titles[num(r, "thread_id")] = str(r, "title")
		}
		st.Posts++
		a, c := str(r, "author"), str(r, "created")
		if f, ok := first[a]; !ok || c < f {
			first[a] = c
		}
	}
	st.Posters = len(first)
	newest := ""
	for a, c := range first {
		if c > newest {
			newest, st.Newest = c, a
		}
	}
	st.Online = online(rows, titles, 15*time.Minute)
	return st
}

func online(rows []layer.Row, titles map[int64]string, within time.Duration) []Online {
	since := time.Now().UTC().Add(-within).Format(tsLayout)
	seen := map[string]bool{}
	out := []Online{}
	for _, r := range rows { // newest first
		a := str(r, "author")
		if seen[a] {
			continue
		}
		seen[a] = true
		if str(r, "created") < since {
			continue
		}
		tid := num(r, "thread_id")
		out = append(out, Online{Author: a, Role: str(r, "role"), Host: str(r, "host"), Seen: parseTS(str(r, "created")), Doing: titles[tid], Thread: tid})
	}
	return out
}

func (s *layerStore) Online(ctx context.Context, within time.Duration) ([]Online, error) {
	rows, err := s.everything(ctx)
	if err != nil {
		return nil, err
	}
	return onlineOf(rows, within), nil
}

func onlineOf(rows []layer.Row, within time.Duration) []Online {
	titles := map[int64]string{}
	for _, r := range rows {
		if str(r, "kind") == "thread" {
			titles[num(r, "thread_id")] = str(r, "title")
		}
	}
	return online(rows, titles, within)
}

func (s *layerStore) Profiles(ctx context.Context, authors []string) (map[string]Profile, error) {
	out := map[string]Profile{}
	if len(authors) == 0 {
		return out, nil
	}
	in := make([]any, len(authors))
	for i, a := range authors {
		in[i] = a
	}
	rows, err := s.query(ctx, []any{"author", "In", in}, nil, all, []string{"kind", "author", "created", "useful_by"})
	if err != nil {
		return nil, err
	}
	return profilesOf(rows, out), nil
}

func profilesOf(rows []layer.Row, out map[string]Profile) map[string]Profile {
	for _, r := range rows {
		a := str(r, "author")
		p := out[a]
		p.Posts++
		if c := parseTS(str(r, "created")); p.Joined.IsZero() || c.Before(p.Joined) {
			p.Joined = c
		}
		if str(r, "kind") == "thread" {
			p.Useful += len(strs(r, "useful_by"))
		}
		out[a] = p
	}
	return out
}
