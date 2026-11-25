package prevallearn

import (
	"context"
	"net/url"
	"strconv"

	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/preval"
	"github.com/hev/kit/pkg/kitapi"
)

// NewHosted uses the same workspace-origin bearer for source reads and for the
// existing tenant board. The native model receives only the redacted sample.
func NewHosted(documents preval.HostedDocuments, model string) (*Learner, error) {
	board := hostedBoard{kitapi.New(documents.BaseURL, documents.Token)}
	learner, err := newLearner(board, "", model, documents.LearningMaterial)
	if err != nil {
		return nil, err
	}
	learner.secrets = []string{documents.Token}
	return learner, nil
}

type hostedBoard struct{ client *kitapi.Client }

func (b hostedBoard) Threads(ctx context.Context, options store.ListOpts) ([]store.Thread, error) {
	query := url.Values{"board": {options.Board}, "limit": {strconv.Itoa(options.Limit)}}
	if options.Tag != "" {
		query.Set("tag", options.Tag)
	}
	rows, err := b.client.Threads(ctx, query)
	if err != nil {
		return nil, errLearning
	}
	out := make([]store.Thread, 0, min(len(rows), options.Limit))
	for _, row := range rows[:min(len(rows), options.Limit)] {
		out = append(out, store.Thread{ID: row.ID, Excerpt: row.Excerpt})
	}
	return out, nil
}

func (b hostedBoard) CreateThread(ctx context.Context, thread store.NewThread) (*store.Thread, error) {
	row, err := b.client.CreateThread(ctx, kitapi.ThreadIn{Board: thread.Board, Title: thread.Title, Tags: thread.Tags, Body: thread.Body})
	if err != nil {
		return nil, errLearning
	}
	return &store.Thread{ID: row.ID}, nil
}
