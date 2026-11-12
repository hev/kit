package store

import (
	"context"
	"time"
)

// Backend is where the board keeps threads and posts. Layer (New) and the
// host's filesystem (NewFiles) are the two implementations; storagemode
// decides which one a server opens.
type Backend interface {
	Boards(ctx context.Context) ([]Board, error)
	Threads(ctx context.Context, o ListOpts) ([]Thread, error)
	Thread(ctx context.Context, id int64) (*Thread, error)
	Posts(ctx context.Context, threadID int64) ([]Post, error)
	View(ctx context.Context, id int64) error
	CreateThread(ctx context.Context, in NewThread) (*Thread, error)
	Reply(ctx context.Context, in NewPost) (*Post, error)
	Useful(ctx context.Context, threadID int64, author string) (int, error)
	SetPinned(ctx context.Context, id int64, pinned bool) error
	SetLocked(ctx context.Context, id int64, locked bool) error
	Search(ctx context.Context, o SearchOpts) ([]Hit, error)
	Stats(ctx context.Context) (*Stats, error)
	Online(ctx context.Context, within time.Duration) ([]Online, error)
	Profiles(ctx context.Context, authors []string) (map[string]Profile, error)
}

// Info says which storage a store uses. Location is the endpoint/namespace in
// Layer mode and the directory in filesystem mode; it is for the operator's
// own startup line, not for the HTTP API.
type Info struct {
	Mode      string `json:"mode"`
	Location  string `json:"-"`
	KeySource string `json:"-"`
}

// Store is what the server and canvas use; every operation goes to Backend.
type Store struct {
	Backend
	info Info
}

func (s *Store) Info() Info { return s.info }
