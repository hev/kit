//go:build linux || darwin

package nativehost

import (
	"context"
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupObservationIsImmutableAndHeldDoesNotFailHost(t *testing.T) {
	f := setup(t)
	birth, e := n.ProcessBirth(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	h := &host{profiles: map[string]Profile{f.p.ID: f.p}, birth: birth, registered: map[string]bool{}}
	h.observeStartup(context.Background())
	if !h.registered[f.p.ID] {
		t.Fatal("valid registration held")
	}
	paths, e := filepath.Glob(filepath.Join(f.p.StateRoot, "host-registration-*.json"))
	if e != nil || len(paths) != 1 {
		t.Fatal(paths, e)
	}
	before, e := os.ReadFile(paths[0])
	if e != nil {
		t.Fatal(e)
	}
	var receipt StartupRegistrationReceipt
	if json.Unmarshal(before, &receipt) != nil || receipt.State != "Observed" || receipt.HostBirth != birth || receipt.ProfileDigest != n.Digest(f.p) || receipt.Registration == nil {
		t.Fatalf("bad receipt: %s", before)
	}
	// Same birth/profile can never replace its first observation or grant dispatch
	// on an uncertain receipt write.
	h.observeStartup(context.Background())
	if h.registered[f.p.ID] {
		t.Fatal("duplicate startup observation granted dispatch")
	}
	after, _ := os.ReadFile(paths[0])
	if string(before) != string(after) {
		t.Fatal("receipt overwritten")
	}
	// A separate profile with a rejected registration records Held, without an
	// error that could shut down the independent capture loop.
	p := f.p
	p.ID = "held"
	p.NodeUID = "different"
	h = &host{profiles: map[string]Profile{p.ID: p}, birth: birth, registered: map[string]bool{}}
	h.observeStartup(context.Background())
	if h.registered[p.ID] {
		t.Fatal("invalid tuple registered")
	}
	paths, _ = filepath.Glob(filepath.Join(p.StateRoot, "host-registration-*.json"))
	if len(paths) != 2 {
		t.Fatal(paths)
	}
	found := false
	for _, path := range paths {
		b, _ := os.ReadFile(path)
		var r StartupRegistrationReceipt
		_ = json.Unmarshal(b, &r)
		if r.ProfileID == p.ID {
			found = true
			if r.State != "Held" || r.Registration != nil {
				t.Fatalf("bad held receipt: %s", b)
			}
		}
	}
	if !found {
		t.Fatal("missing held observation")
	}
	if _, e := os.Stat(f.effects); !os.IsNotExist(e) {
		t.Fatal("startup launched workload")
	}
}
