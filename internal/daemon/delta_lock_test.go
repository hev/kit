package daemon

import (
	"github.com/hev/kit/internal/platformops"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDeltaOtherWriterIgnoresLock(t *testing.T) {
	for _, clear := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "clear"}[clear], func(t *testing.T) {
			p := t.TempDir() + "/config.toml"
			t.Setenv("HEV_CONFIG", p)
			os.WriteFile(p, []byte("[kit]\nkey='delta-stored'\n"), 0600)
			u, e := platformops.LockPath(p+".lock", false)
			if e != nil {
				t.Fatal(e)
			}
			doc, e := ReadInviteConfig()
			if e != nil {
				u()
				t.Fatal(e)
			}
			done := make(chan error, 1)
			go func() {
				if clear {
					done <- ClearKitConfig(doc)
				} else {
					done <- WriteKitConfig(doc, KitConfig{Key: "delta-new"})
				}
			}()
			select {
			case e := <-done:
				u()
				t.Fatalf("kit writer bypassed transaction lock: %v", e)
			case <-time.After(100 * time.Millisecond):
			}
			os.WriteFile(p, []byte("unrelated='concurrent edit'\n[kit]\nkey='delta-stored'\n"), 0600)
			u()
			if e := <-done; e != nil {
				t.Fatal(e)
			}
			b, _ := os.ReadFile(p)
			if !strings.Contains(string(b), "concurrent edit") {
				t.Fatal("lost unrelated edit")
			}
		})
	}
}
