package nativehost

import (
	"errors"
	"fmt"
	"testing"
)

func TestReceiverWaitRetainsUnknownLocalSlot(t *testing.T) {
	for _, tt := range []struct {
		name      string
		wait      func() error
		remaining int
	}{
		{"known successful exit", func() error { return nil }, 0},
		{"known unsuccessful exit", func() error { return errors.New("root nonzero after owned cleanup") }, 0},
		{"unknown owned tree", func() error { return ErrHeld }, 1},
		{"wrapped unknown", func() error { return fmt.Errorf("cleanup: %w", ErrHeld) }, 1},
		{"missing waiter", nil, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := &host{active: 1}
			h.waitReceiver(tt.wait)
			if h.active != tt.remaining {
				t.Fatalf("local slots=%d want %d", h.active, tt.remaining)
			}
		})
	}
}
