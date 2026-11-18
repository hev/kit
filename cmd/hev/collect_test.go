package main

import (
	"errors"
	"testing"

	"github.com/hev/kit/internal/collect"
)

func TestCollectRefusesToStartWithoutAServiceKey(t *testing.T) {
	t.Setenv("HEV_COLLECT_SERVICE_KEY", "")
	err := runCollect(collectCmd, nil)
	if !errors.Is(err, collect.ErrNoKey) {
		t.Fatalf("runCollect without a key = %v, want ErrNoKey", err)
	}
}

func TestCollectFlagDefaults(t *testing.T) {
	if f := collectCmd.Flags().Lookup("listen"); f == nil || f.DefValue != "127.0.0.1:4318" {
		t.Fatalf("--listen default = %v", f)
	}
}
