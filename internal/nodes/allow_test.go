package nodes

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGrantCapabilitiesDefaultDenyAndToggle(t *testing.T) {
	s := Store{t.TempDir()}
	path := filepath.Join(t.TempDir(), ".ssh", "authorized_keys")
	_, i := invitation(t, s)
	code, _ := Encode(Request{i, testKey(t)})
	if _, e := s.AcceptWith(code, path, "/bin/hev", false, []string{"shell"}); e == nil || !strings.Contains(e.Error(), "unknown capability") {
		t.Fatalf("accepted unknown capability: %v", e)
	}
	a, e := s.Accept(code, path, "/bin/hev", false)
	if e != nil {
		t.Fatal(e)
	}
	if a.Allows("repos") {
		t.Fatal("capability granted by default")
	}
	b, e := os.ReadFile(filepath.Join(s.Dir, "access-"+a.ID+".json"))
	if e != nil || strings.Contains(string(b), `"allow"`) {
		t.Fatalf("default grant changed shape: %s %v", b, e)
	}
	if g, e := s.Grant(a.ID, "hev dispatch rpc"); e != nil || g.Name != "teammate" || g.Allows("repos") {
		t.Fatalf("grant=%+v err=%v", g, e)
	}
	if _, e = s.SetAllow("teammate", "shell", true); e == nil {
		t.Fatal("allowed unknown capability")
	}
	if _, e = s.SetAllow("nobody", "repos", true); e == nil {
		t.Fatal("allowed unknown teammate")
	}
	for _, on := range []bool{true, true, false, false, true} {
		if _, e = s.SetAllow("teammate", "repos", on); e != nil {
			t.Fatal(e)
		}
		g, e := s.Grant(a.ID, "hev dispatch rpc")
		if e != nil || g.Allows("repos") != on || len(g.Allow) > 1 {
			t.Fatalf("on=%v grant=%+v err=%v", on, g, e)
		}
		if g.Key != a.Key || g.Line != a.Line || g.Path != a.Path || !g.Added.Equal(a.Added) {
			t.Fatalf("toggle changed the key grant: %+v", g)
		}
	}
	// The toggle never touches authorized_keys.
	keys, e := os.ReadFile(path)
	if e != nil || strings.Count(string(keys), "\n") != 1 || !strings.Contains(string(keys), a.Line) {
		t.Fatalf("authorized keys changed: %q %v", keys, e)
	}
	if e = s.Remove("teammate"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Grant(a.ID, "hev dispatch rpc"); e == nil {
		t.Fatal("revoked grant still authenticates")
	}
}

func TestAcceptWithRecordsCapability(t *testing.T) {
	s := Store{t.TempDir()}
	_, i := invitation(t, s)
	code, _ := Encode(Request{i, testKey(t)})
	a, e := s.AcceptWith(code, filepath.Join(t.TempDir(), "authorized_keys"), "/bin/hev", false, []string{"repos", "repos"})
	if e != nil {
		t.Fatal(e)
	}
	grants, e := s.Accesses()
	if e != nil || len(grants) != 1 || !slices.Equal(grants[0].Allow, []string{"repos"}) || !a.Allows("repos") {
		t.Fatalf("grants=%+v err=%v", grants, e)
	}
}

// Grants written before capabilities existed keep working, with none allowed.
func TestLegacyGrantHasNoCapabilities(t *testing.T) {
	s := Store{t.TempDir()}
	legacy := `{"id":"abc","name":"teammate","key":"k","path":"p","line":"l","added":"2026-01-01T00:00:00Z","last_used":"0001-01-01T00:00:00Z"}`
	if e := os.WriteFile(filepath.Join(s.Dir, "access-abc.json"), []byte(legacy), 0600); e != nil {
		t.Fatal(e)
	}
	g, e := s.Grant("abc", "hev dispatch rpc")
	if e != nil || g.Name != "teammate" || g.Allows("repos") {
		t.Fatalf("grant=%+v err=%v", g, e)
	}
}
