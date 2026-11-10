package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCloudRecoveryWatchStaysAliveUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- WatchCloudHosts(ctx, Config{}) }()
	select {
	case err := <-done:
		t.Fatalf("watch exited after a single pass: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not stop on cancellation")
	}
}
