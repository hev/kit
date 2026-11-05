package peers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBareNameSnapshotResolution(t *testing.T) {
	e, _ := fixture(t)
	dir := filepath.Join(e.State, "hosts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("remote", `{"host":"remote","panes":[{"address":"desk","role":"foh"}]}`)
	target, err := e.relayAlias("desk", nil)
	if err != nil || target != "remote/desk" {
		t.Fatal(target, err)
	}
	target, err = e.relayAlias("desk", []Peer{{Session: "desk"}})
	if err != nil || target != "desk" {
		t.Fatal(target, err)
	}
	write("second", `{"host":"second","panes":[{"address":"desk"}]}`)
	if _, err = e.relayAlias("desk", nil); err == nil {
		t.Fatal("ambiguous remote name accepted")
	}
	write("second", `{"host":"second","panes":[{"address":"desk","role":"offline"}]}`)
	target, err = e.relayAlias("desk", nil)
	if err != nil || target != "remote/desk" {
		t.Fatal(target, err)
	}
	write("remote", `{torn`)
	target, err = e.relayAlias("desk", nil)
	if err != nil || target != "desk" {
		t.Fatal(target, err)
	}
}
