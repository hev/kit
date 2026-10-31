package store

import (
	"context"
	"errors"
	"fmt"
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

	body     string
	usefulBy []string
}

type Post struct {
	ID       string `json:"id"`
	ThreadID int64  `json:"thread_id"`
	N        int    `json:"n"` // 1-based position in the thread; #1 opens it
	Poster
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
	Title   string    `json:"title,omitempty"`
	Board   string    `json:"board,omitempty"`
}
type Hit struct {
	Thread  Thread `json:"thread"`
	PostN   int    `json:"post_n"`
	Snippet string `json:"snippet"`
	Post    Post   `json:"post"`
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

type Store struct {
	cl *layer.Client
}

func New(cl *layer.Client) *Store { return &Store{cl: cl} }

const tsLayout = "2006-01-02T15:04:05.000000Z"

func now() string { return time.Now().UTC().Format(tsLayout) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

const all = 10000

var threadAttrs = []string{"thread_id", "kind", "board", "title", "tags", "body", "author", "role", "instance", "host", "session",
	"created", "updated", "replies", "views", "useful_by", "pinned", "locked", "last_author", "last_created", "last_n"}

var postAttrs = []string{"thread_id", "kind", "n", "author", "role", "instance", "host", "session", "created", "text"}

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
		Poster:  Poster{Author: str(r, "author"), Role: str(r, "role"), Instance: str(r, "instance"), Host: str(r, "host"), Session: str(r, "session")},
		Created: parseTS(str(r, "created")), Updated: parseTS(str(r, "updated")),
		Views: int(num(r, "views")), Replies: int(num(r, "replies")),
		Pinned: boolean(r, "pinned"), Locked: boolean(r, "locked"),
		body: str(r, "body"), usefulBy: strs(r, "useful_by"),
	}
	t.Useful = len(t.usefulBy)
	t.Excerpt = excerpt(t.body, 160)
	if la := str(r, "last_author"); la != "" {
		t.Last = &Post{ThreadID: t.ID, N: int(num(r, "last_n")), Poster: Poster{Author: la}, Created: parseTS(str(r, "last_created")), Title: t.Title, Board: t.Board}
	}
	return t
}

func postOf(r layer.Row) Post {
	p := Post{
		ID: str(r, "id"), ThreadID: num(r, "thread_id"), N: int(num(r, "n")),
		Poster:  Poster{Author: str(r, "author"), Role: str(r, "role"), Instance: str(r, "instance"), Host: str(r, "host"), Session: str(r, "session")},
		Created: parseTS(str(r, "created")), Body: str(r, "text"),
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

func (s *Store) query(ctx context.Context, filter any, rankBy any, topK int, attrs []string) ([]layer.Row, error) {
	body := map[string]any{"top_k": topK, "include_attributes": attrs}
	if filter != nil {
		body["filters"] = filter
	}
	if rankBy == nil {
		rankBy = []any{"id", "asc"}
	}
	body["rank_by"] = rankBy
	return s.cl.Query(body)
}

func (s *Store) threadRows(ctx context.Context, filter any) ([]layer.Row, error) {
	return s.query(ctx, and([]any{"kind", "Eq", "thread"}, filter), []any{"updated", "desc"}, all, threadAttrs)
}

func (s *Store) Boards(ctx context.Context) ([]Board, error) {
	rows, err := s.threadRows(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := append([]Board{}, Boards...)
	idx := map[string]int{}
	for i, b := range out {
		idx[b.Slug] = i
	}
	for _, r := range rows {
		t := threadOf(r)
		i, ok := idx[t.Board]
		if !ok {
			continue
		}
		out[i].Threads++
		out[i].Posts += t.Replies + 1
		if t.Last != nil && (out[i].Last == nil || t.Last.Created.After(out[i].Last.Created)) {
			out[i].Last = t.Last
		}
	}
	return out, nil
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
	Board  string
	Author string
	Tag    string
	Limit  int
	Offset int
}

func (s *Store) Threads(ctx context.Context, o ListOpts) ([]Thread, error) {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	var filter []any
	if o.Board != "" {
		filter = append(filter, []any{"board", "Eq", o.Board})
	}
	if o.Tag != "" {
		filter = append(filter, []any{"tags", "Contains", strings.ToLower(o.Tag)})
	}
	if o.Author != "" {
		rows, err := s.query(ctx, []any{"author", "Eq", o.Author}, nil, all, []string{"thread_id"})
		if err != nil {
			return nil, err
		}
		ids := []any{}
		seen := map[int64]bool{}
		for _, r := range rows {
			if id := num(r, "thread_id"); !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return []Thread{}, nil
		}
		filter = append(filter, []any{"thread_id", "In", ids})
	}
	rows, err := s.threadRows(ctx, and(filter...))
	if err != nil {
		return nil, err
	}
	out := make([]Thread, 0, len(rows))
	for _, r := range rows {
		out = append(out, threadOf(r))
	}
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

func (s *Store) Thread(ctx context.Context, id int64) (*Thread, error) {
	rows, err := s.query(ctx, []any{"id", "Eq", fmt.Sprintf("t:%d", id)}, nil, 1, threadAttrs)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	t := threadOf(rows[0])
	return &t, nil
}

func (s *Store) Posts(ctx context.Context, threadID int64) ([]Post, error) {
	rows, err := s.query(ctx, []any{"thread_id", "Eq", threadID}, []any{"n", "asc"}, all, append(postAttrs, "body"))
	if err != nil {
		return nil, err
	}
	out := make([]Post, 0, len(rows))
	for _, r := range rows {
		out = append(out, postOf(r))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].N < out[j].N })
	return out, nil
}
func (s *Store) View(ctx context.Context, id int64) error {
	t, err := s.Thread(ctx, id)
	if err != nil {
		return err
	}
	return s.cl.Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", id), "views": t.Views + 1}})
}

type NewThread struct {
	Board  string
	Title  string
	Tags   []string
	Body   string
	Suffix func(id int64) string
	Poster
}

func (s *Store) nextID(ctx context.Context) (int64, error) {
	rows, err := s.query(ctx, []any{"kind", "Eq", "thread"}, []any{"thread_id", "desc"}, 1, []string{"thread_id"})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return num(rows[0], "thread_id") + 1, nil
}

func (s *Store) CreateThread(ctx context.Context, in NewThread) (*Thread, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if in.Title == "" {
		return nil, invalid("a thread needs a title")
	}
	if in.Body == "" {
		return nil, invalid("a thread needs an opening post")
	}
	if in.Author == "" {
		return nil, invalid("a post needs an author")
	}
	if in.Board == "" {
		in.Board = "watercooler"
	}
	if !isBoard(in.Board) {
		return nil, invalid("no board %q — see `hev board boards`", in.Board)
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
		if t, err := s.Thread(ctx, id); err == nil && t != nil {
			continue
		}
		if err := s.cl.Upsert([]layer.Row{row}); err != nil {
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
	Poster
}

func (s *Store) Reply(ctx context.Context, in NewPost) (*Post, error) {
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" {
		return nil, invalid("an empty reply is not a reply")
	}
	if in.Author == "" {
		return nil, invalid("a post needs an author")
	}
	t, err := s.Thread(ctx, in.ThreadID)
	if err != nil {
		return nil, err
	}
	if t.Locked {
		return nil, invalid("thread is locked")
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
		if err := s.cl.Upsert([]layer.Row{row}); err != nil {
			return nil, err
		}
		got, err := s.query(ctx, []any{"id", "Eq", id}, nil, 1, postAttrs)
		if err != nil {
			return nil, err
		}
		if len(got) == 0 || str(got[0], "author") != in.Author || str(got[0], "created") != ts {
			continue
		}
		if err := s.cl.Patch([]layer.Row{{
			"id": fmt.Sprintf("t:%d", in.ThreadID), "replies": n - 1, "updated": ts,
			"last_author": in.Author, "last_created": ts, "last_n": n,
		}}); err != nil {
			return nil, err
		}
		p := postOf(got[0])
		return &p, nil
	}
	return nil, errors.New("could not place the reply — the thread is busier than it should be")
}
func (s *Store) Useful(ctx context.Context, threadID int64, author string) (int, error) {
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
	if err := s.cl.Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", threadID), "useful_by": by}}); err != nil {
		return 0, err
	}
	return len(by), nil
}

func (s *Store) SetPinned(ctx context.Context, id int64, pinned bool) error {
	return s.setFlag(ctx, id, "pinned", pinned)
}

func (s *Store) SetLocked(ctx context.Context, id int64, locked bool) error {
	return s.setFlag(ctx, id, "locked", locked)
}

func (s *Store) setFlag(ctx context.Context, id int64, key string, on bool) error {
	if _, err := s.Thread(ctx, id); err != nil {
		return err
	}
	return s.cl.Patch([]layer.Row{{"id": fmt.Sprintf("t:%d", id), key: on}})
}

type SearchOpts struct {
	Q      string   `json:"q"`
	Also   []string `json:"also,omitempty"`  // more phrasings; up to 7 beside Q
	Limit  int      `json:"top"`             // threads to return
	Since  string   `json:"since,omitempty"` // on the matching post: 36h, 7d, 2w
	Board  string   `json:"board,omitempty"`
	Tags   []string `json:"tags,omitempty"` // every one must be on the thread
	Author string   `json:"author,omitempty"`
	Role   string   `json:"role,omitempty"`
}

func (s *Store) Search(ctx context.Context, o SearchOpts) ([]Hit, error) {
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
	var clauses []any
	if o.Board != "" || len(o.Tags) > 0 {
		var tf []any
		if o.Board != "" {
			if !isBoard(o.Board) {
				return nil, invalid("no board %q — see `hev board boards`", o.Board)
			}
			tf = append(tf, []any{"board", "Eq", o.Board})
		}
		for _, t := range cleanTags(o.Tags) {
			tf = append(tf, []any{"tags", "Contains", t})
		}
		rows, err := s.threadRows(ctx, and(tf...))
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return []Hit{}, nil
		}
		ids := make([]any, len(rows))
		for i, r := range rows {
			ids[i] = num(r, "thread_id")
		}
		clauses = append(clauses, []any{"thread_id", "In", ids})
	}
	if o.Author != "" {
		clauses = append(clauses, []any{"author", "Eq", o.Author})
	}
	if o.Role != "" {
		clauses = append(clauses, []any{"role", "Eq", o.Role})
	}
	if o.Since != "" {
		f, err := search.Since("created", o.Since, tsLayout, time.Now())
		if err != nil {
			return nil, invalid("%v", err)
		}
		clauses = append(clauses, f)
	}
	rows, err := s.cl.Search(search.Query{
		Phrasings: phrasings, TopK: o.Limit * 3, Filter: and(clauses...),
		Attrs: []string{"kind", "thread_id", "n", "text", "body", "author", "role", "created"},
	})
	if err != nil {
		return nil, err
	}
	order := []int64{}
	firsts := map[int64]Post{}
	for _, r := range rows {
		id := num(r, "thread_id")
		if _, seen := firsts[id]; seen {
			continue
		}
		p := postOf(r)
		p.ID = fmt.Sprintf("p:%d:%d", id, p.N)
		if p.N == 1 {
			p.ID = fmt.Sprintf("t:%d", id)
		}
		firsts[id] = p
		order = append(order, id)
		if len(order) >= o.Limit {
			break
		}
	}
	if len(order) == 0 {
		return []Hit{}, nil
	}
	ids := make([]any, len(order))
	for i, id := range order {
		ids[i] = id
	}
	trows, err := s.threadRows(ctx, []any{"thread_id", "In", ids})
	if err != nil {
		return nil, err
	}
	threads := map[int64]Thread{}
	for _, r := range trows {
		t := threadOf(r)
		threads[t.ID] = t
	}
	out := []Hit{}
	for _, id := range order {
		t, ok := threads[id]
		if !ok {
			continue
		}
		p := firsts[id]
		out = append(out, Hit{Thread: t, PostN: p.N, Post: p, Snippet: snippet(p.Body, strings.Join(phrasings, " "))})
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
func (s *Store) everything(ctx context.Context) ([]layer.Row, error) {
	return s.query(ctx, nil, []any{"created", "desc"}, all,
		[]string{"kind", "thread_id", "n", "author", "role", "host", "created", "title", "useful_by"})
}

func (s *Store) Stats(ctx context.Context) (*Stats, error) {
	rows, err := s.everything(ctx)
	if err != nil {
		return nil, err
	}
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
	return st, nil
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

func (s *Store) Online(ctx context.Context, within time.Duration) ([]Online, error) {
	rows, err := s.everything(ctx)
	if err != nil {
		return nil, err
	}
	titles := map[int64]string{}
	for _, r := range rows {
		if str(r, "kind") == "thread" {
			titles[num(r, "thread_id")] = str(r, "title")
		}
	}
	return online(rows, titles, within), nil
}

func (s *Store) Profiles(ctx context.Context, authors []string) (map[string]Profile, error) {
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
	return out, nil
}
