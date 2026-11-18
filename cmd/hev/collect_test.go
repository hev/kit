package main

import (
	"errors"
	"strings"
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

func TestCollectRefusesToStartWithoutNamespaces(t *testing.T) {
	t.Setenv("HEV_COLLECT_SERVICE_KEY", "svc-key-for-test")
	saved := collectNamespaces
	t.Cleanup(func() { collectNamespaces = saved })
	for _, list := range []string{"", " , ,"} {
		collectNamespaces = list
		if err := runCollect(collectCmd, nil); !errors.Is(err, collect.ErrNoNamespaces) {
			t.Fatalf("runCollect with --namespaces %q = %v, want ErrNoNamespaces", list, err)
		}
	}
	collectNamespaces = "story-prod,Story_Bad"
	err := runCollect(collectCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "Story_Bad") {
		t.Fatalf("runCollect with an invalid namespace = %v", err)
	}
	if strings.Contains(err.Error(), "svc-key-for-test") {
		t.Fatalf("error carries the key: %v", err)
	}
}

func TestCollectFlagDefaults(t *testing.T) {
	if f := collectCmd.Flags().Lookup("listen"); f == nil || f.DefValue != "127.0.0.1:4318" {
		t.Fatalf("--listen default = %v", f)
	}
	if f := collectCmd.Flags().Lookup("traces-namespace"); f == nil || f.DefValue != "hev-app-traces" {
		t.Fatalf("--traces-namespace default = %v", f)
	}
	if f := collectCmd.Flags().Lookup("namespaces"); f == nil {
		t.Fatal("--namespaces is missing")
	}
}
