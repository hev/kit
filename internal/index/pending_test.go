package index

import (
	"errors"
	"github.com/hev/kit/internal/trace"
	"testing"
)

type pendingSource struct{ secretSource }

func (pendingSource) Read(trace.Unit) ([]trace.Turn, error) {
	return nil, errors.New("fixture read failure")
}
func TestPendingIncludesUnitsBeyondLimit(t *testing.T) {
	var pending uint32
	_, _ = Run(pendingSource{}, nil, &State{Units: map[string]string{}}, Options{Limit: 1, ObservePending: func(n uint32) { pending = n }})
	if pending != 2 {
		t.Fatalf("pending = %d; want both units despite limit", pending)
	}
}
