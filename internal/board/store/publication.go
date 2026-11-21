package store

import (
	"context"
	"crypto/sha256"
	"time"
)

// PostReference identifies a candidate board write without copying its body.
// A publisher must verify it against the stored post before emitting an event.
type PostReference struct {
	ThreadID int64
	N        int
	Created  time.Time
	Author   string
	Host     string
	Digest   [32]byte
}

func (ref PostReference) Matches(p Post) bool {
	return ref.ThreadID == p.ThreadID && ref.N == p.N && ref.Created.Equal(p.Created) &&
		ref.Author == p.Author && ref.Host == p.Host && ref.Digest == sha256.Sum256([]byte(p.Body))
}

type publicationKey struct{}

// WithPostPublication registers a callback before each candidate write. If it
// fails, that candidate is not written. A recorded candidate may lose a write
// race, so the publisher must verify that it committed.
func WithPostPublication(ctx context.Context, stage func(context.Context, PostReference) error) context.Context {
	return context.WithValue(ctx, publicationKey{}, stage)
}

func stagePost(ctx context.Context, thread int64, n int, created time.Time, poster Poster, body string) error {
	stage, ok := ctx.Value(publicationKey{}).(func(context.Context, PostReference) error)
	if !ok {
		return nil
	}
	return stage(ctx, PostReference{thread, n, created, poster.Author, poster.Host, sha256.Sum256([]byte(body))})
}
