package intake

import "context"

// Source supplies a snapshot without deciding which work is authorized or admitted.
// Tracker implementations normalize their records to Issue; rules remain in Poller.
type Source[T any] interface {
	Fetch(context.Context) (T, error)
}
type sourceFunc[T any] func(context.Context) (T, error)

func (f sourceFunc[T]) Fetch(ctx context.Context) (T, error) { return f(ctx) }

type trackerSnapshot struct {
	Issues []Issue
	Bot    string
}

func (p *Poller) trackerSource() Source[trackerSnapshot] {
	return sourceFunc[trackerSnapshot](func(ctx context.Context) (trackerSnapshot, error) {
		issues, bot, err := p.candidates(ctx)
		return trackerSnapshot{issues, bot}, err
	})
}
func (p *Poller) mentionSource(bot string) Source[[]MentionComment] {
	return sourceFunc[[]MentionComment](func(ctx context.Context) ([]MentionComment, error) { return p.mentionComments(ctx, bot) })
}
