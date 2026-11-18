package collect

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	boardlayer "github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/layer"
)

// BoardPost is the body of POST /v1/app/boards/{board}/posts. The host app
// asserts author; kit does not check it.
type BoardPost struct {
	Author     string   `json:"author"`
	AuthorName string   `json:"authorName"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Tags       []string `json:"tags"`
}

// BoardPosted answers a post.
type BoardPosted struct {
	ID      string `json:"id"`
	Created string `json:"created"`
}

// BoardQuery is the body of POST /v1/app/boards/{board}/search. With no
// query, tags alone list the newest posts that carry every one of them.
type BoardQuery struct {
	Query string   `json:"query"`
	Tags  []string `json:"tags"`
	Limit int      `json:"limit"`
}

// BoardHit is one board search answer.
type BoardHit struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Author     string   `json:"author"`
	AuthorName string   `json:"authorName"`
	Created    string   `json:"created"`
	Tags       []string `json:"tags"`
	Score      float64  `json:"score"`
}

// Boards is the app board surface.
type Boards interface {
	Post(ctx context.Context, board string, p BoardPost) (BoardPosted, error)
	Search(ctx context.Context, board string, q BoardQuery) ([]BoardHit, error)
}

// ErrBadBoardRequest marks a request the board store refused as invalid.
var ErrBadBoardRequest = errors.New("invalid board request")

// StoreBoards serves app boards from kit's board store, in a namespace of
// their own (never the team board's), with the store's search route.
type StoreBoards struct {
	Store *store.Store
}

// NewStoreBoards opens the app board store over a kit Layer client whose
// store capabilities are resolved.
func NewStoreBoards(cl *layer.Client) *StoreBoards {
	return &StoreBoards{Store: store.NewApp(boardlayer.ForStore(cl))}
}

func wrapBoardErr(err error) error {
	if errors.Is(err, store.ErrInvalid) {
		return fmt.Errorf("%w: %v", ErrBadBoardRequest, err)
	}
	return err
}

func (b *StoreBoards) Post(ctx context.Context, board string, p BoardPost) (BoardPosted, error) {
	t, err := b.Store.CreateThread(ctx, store.NewThread{
		Board: board, Title: p.Title, Body: p.Body, Tags: p.Tags,
		Poster: store.Poster{Author: p.Author, AuthorName: p.AuthorName, Role: "app"},
	})
	if err != nil {
		return BoardPosted{}, wrapBoardErr(err)
	}
	return BoardPosted{ID: strconv.FormatInt(t.ID, 10), Created: t.Created.UTC().Format(time.RFC3339Nano)}, nil
}

func (b *StoreBoards) Search(ctx context.Context, board string, q BoardQuery) ([]BoardHit, error) {
	tags := cleanTags(q.Tags)
	if strings.TrimSpace(q.Query) == "" {
		return b.byTags(ctx, board, tags, q.Limit)
	}
	hits, err := b.Store.Search(ctx, store.SearchOpts{Q: q.Query, Board: board, Tags: tags, Limit: q.Limit})
	if err != nil {
		return nil, wrapBoardErr(err)
	}
	out := make([]BoardHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, hitOf(h.Thread, h.Score))
	}
	return out, nil
}

func (b *StoreBoards) byTags(ctx context.Context, board string, tags []string, limit int) ([]BoardHit, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("%w: a search needs a query or tags", ErrBadBoardRequest)
	}
	threads, err := b.Store.Threads(ctx, store.ListOpts{Board: board, Tag: tags[0], Limit: 1000})
	if err != nil {
		return nil, wrapBoardErr(err)
	}
	out := []BoardHit{}
	for i, t := range threads {
		if !hasAll(t.Tags, tags) {
			continue
		}
		out = append(out, hitOf(t, 1/float64(i+1)))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func hitOf(t store.Thread, score float64) BoardHit {
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	return BoardHit{
		ID: strconv.FormatInt(t.ID, 10), Title: t.Title, Body: t.Body(), Author: t.Author, AuthorName: t.AuthorName,
		Created: t.Created.UTC().Format(time.RFC3339Nano), Tags: tags, Score: score,
	}
}

func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasAll(have, want []string) bool {
	set := map[string]bool{}
	for _, t := range have {
		set[strings.ToLower(t)] = true
	}
	for _, t := range want {
		if !set[t] {
			return false
		}
	}
	return true
}
