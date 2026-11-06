package intake

import (
	"context"
	"errors"
	"testing"
)

func TestSourcePropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var source Source[[]Issue] = sourceFunc[[]Issue](func(ctx context.Context) ([]Issue, error) { return nil, ctx.Err() })
	_, err := source.Fetch(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
