package secrets

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/secrets/fakeop"
)

func TestTokenFileRotationWithoutRestart(t *testing.T) {
	for _, timer := range []bool{false, true} {
		name := "request"
		if timer {
			name = "timer"
		}
		t.Run(name, func(t *testing.T) {
			c := fake(t)
			f := fakeop.Install(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			dir, err := os.MkdirTemp("", "rot-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			t.Setenv(EndpointEnv, filepath.Join(dir, "s"))
			t.Setenv("HEV_SECRETS_CACHE", "")
			token := filepath.Join(home, "token")
			t.Setenv("HEV_OP_TOKEN_FILE", token)
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "token-A")
			if err := os.WriteFile(token, []byte("token-A"), 0600); err != nil {
				t.Fatal(err)
			}
			if timer {
				c.monitorEvery = 10 * time.Millisecond
			}
			ctx := context.Background()
			stop, err := Serve(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			old := []Ref{{"OLD", "op://test/old/value", time.Hour}}
			if _, err := ResolveWith(ctx, old, Options{CacheOnly: true}); err != nil {
				t.Fatal(err)
			}
			// The daemon retains its startup environment; only the file is rotated.
			if err := os.WriteFile(token, []byte("token-B"), 0600); err != nil {
				t.Fatal(err)
			}
			if timer {
				deadline := time.Now().Add(2 * time.Second)
				for c.rotatedToken.Load() == nil && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if c.rotatedToken.Load() == nil {
					t.Fatal("timer did not adopt token file")
				}
			}
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "token-B")
			t.Setenv("HEV_FAKE_EXPECT_TOKEN", "token-B")
			if err := Ping(ctx); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			_, held := c.entries[old[0].URI]
			c.mu.Unlock()
			if held {
				t.Fatal("value from token A survived rotation")
			}
			fresh := []Ref{{"NEW", "op://test/new/value", time.Hour}}
			for i, want := range []string{SourceRead, SourceCache} {
				res, err := ResolveResult(ctx, fresh, Options{CacheOnly: true})
				if err != nil || res.Via != ViaDaemon || res.Sources["NEW"] != want {
					t.Fatalf("call %d: via=%s source=%s err=%v", i, res.Via, res.Sources["NEW"], err)
				}
			}
			if c.LocalStatus().Budget.UsedDay != 2 {
				t.Fatal("rotation reset the host budget")
			}
			if c.LocalStatus().Requests != 2 {
				t.Fatal("repeated reference made an extra provider read")
			}
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "token-A")
			err = Ping(ctx)
			var mismatch *CredentialMismatch
			if !errors.As(err, &mismatch) || !strings.Contains(err.Error(), "different credential") || mismatch.TokenFileModified.IsZero() || mismatch.Caller == mismatch.Daemon {
				t.Fatalf("missing mismatch details: %v", err)
			}
			status, err := ReadLocalStatus(ctx)
			if !errors.As(err, &mismatch) || status == nil || status.CredentialMismatch == nil {
				t.Fatalf("status lost mismatch: %v", err)
			}
			t.Setenv("HEV_FAKE_EXPECT_TOKEN", "")
			res, err := ResolveResult(ctx, fresh, Options{Notice: io.Discard})
			if err != nil || res.Via != ViaCredentialMismatch || f.Count(t) != 1 {
				t.Fatalf("direct mismatch fallback: %s %v", res.Via, err)
			}
			log, err := os.ReadFile(ReadLogPath())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(log), "via="+ViaCredentialMismatch) {
				t.Fatal("read log lost mismatch cause")
			}
			if strings.Contains(string(log), "token-A") || strings.Contains(string(log), "token-B") {
				t.Fatal("token leaked")
			}
		})
	}
}

func TestRotationPreservesCooldownAndBudgetState(t *testing.T) {
	c := fake(t)
	token := filepath.Join(t.TempDir(), "token")
	t.Setenv("HEV_OP_TOKEN_FILE", token)
	os.WriteFile(token, []byte("A"), 0600)
	s := &endpointCredentials{fileToken: "A", accounts: map[string]string{"op": c.provider("op").Fingerprint()}}
	reset := time.Now().Add(time.Hour)
	c.state["op"].reset = reset
	c.state["op"].requests = 7
	c.entries["op://test/old/token"] = entry{value: "old"}
	os.WriteFile(token, []byte("B"), 0600)
	s.refresh(c)
	if len(c.entries) != 0 || c.state["op"].requests != 7 || !c.state["op"].reset.Equal(reset) {
		t.Fatal("rotation lost accounting or retained values")
	}
}
