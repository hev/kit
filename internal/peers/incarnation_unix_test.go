//go:build !windows

package peers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

// The reference peers hashes self_label() into a recipient incarnation, so
// the native engine must name its host the same way. On the laptop the raw
// hostname (Adams-MacBook-Pro) and the label (macbook) differ.
func TestHostLabelMatchesReference(t *testing.T) {
	for hostname, want := range map[string]string{
		"Adams-MacBook-Pro.local": "macbook",
		"Adams-MacBook-Air":       "macbook",
		"Adams-Mac-mini.lan":      "mini",
		"OVEN":                    "oven",
		"build-box.example.com":   "build-box",
	} {
		if got := dispatch.PeersLabel(hostname); got != want {
			t.Errorf("PeersLabel(%q) = %q, want %q", hostname, got, want)
		}
	}
	t.Setenv("PEERS_SELF", "")
	laptop(t)
	if got := New().Host; got != "macbook" {
		t.Fatalf("engine host %q, reference label macbook", got)
	}
}

// laptop makes this host look like the laptop to both engines: the native
// one reads hostname, the reference runs `hostname -s`.
func laptop(t *testing.T) {
	t.Helper()
	old := hostname
	t.Cleanup(func() { hostname = old })
	hostname = func() (string, error) { return "Adams-MacBook-Pro.local", nil }
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "hostname"), []byte("#!/bin/sh\necho Adams-MacBook-Pro\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

// A notice pulled from another host and delivered here must be current for
// the live recipient session, or inbox --pending and the reference's
// UserPromptSubmit hook never show it.
func TestPulledNoticeIsCurrentForLiveSession(t *testing.T) {
	const session = "foh-hev-2"
	for _, kv := range isolatedTmux(t, session, "sleep 60") {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	for _, k := range []string{"TMUX", "TMUX_PANE", "PEERS_SELF", "PEERS_FROM"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	laptop(t)
	ctx := context.Background()
	if out, err := exec.Command("tmux", "set-option", "-t", "="+session+":", "@peers-inbox", "on").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	e := New()
	e.State = t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := e.Run(ctx, args, strings.NewReader(""), &out, io.Discard); err != nil {
			t.Fatal(args, err, out.String())
		}
		return out.String()
	}
	// The argv dispatch.SendOrchestratorNotice runs for a pulled owner notice.
	id := "oven/o-kit-063-peers-cutover:reply:1"
	run("send", session, "Decision needed: scope", "--from", "oven/o-kit-063-peers-cutover", "--id", id, "--inbox-only")

	// The reference incarnation: sha256(self_label:raw:birth), birth printed
	// as Python prints a whole-second float.
	raw, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session+":", "#{pid}:#{session_id}:#{session_created}").Output()
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.SplitN(strings.TrimSpace(string(raw)), ":", 2)[0]
	ps := exec.Command("ps", "-o", "lstart=", "-p", pid)
	ps.Env = append(os.Environ(), "LC_ALL=C")
	lstart, err := ps.Output()
	if err != nil {
		t.Fatal(err)
	}
	birth, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(string(lstart)), " "), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("macbook:%s:%d.0", strings.TrimSpace(string(raw)), birth.Unix())))
	want := hex.EncodeToString(sum[:])

	var pending struct {
		Identity string    `json:"recipient_incarnation"`
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(run("inbox", "--peek", "--pending", "--json", "--session", session)), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Identity != want {
		t.Fatalf("live session incarnation %s, reference %s", pending.Identity, want)
	}
	if len(pending.Messages) != 1 || pending.Messages[0]["id"] != id || pending.Messages[0]["current"] != true || pending.Messages[0]["recipient_incarnation"] != want {
		t.Fatalf("pulled notice not pending and current: %+v", pending.Messages)
	}

	// With the reference available, it must agree: same incarnation, and
	// the notice is unread for the live session.
	reference := os.Getenv("PEERS_PYTHON_REFERENCE")
	if reference == "" {
		return
	}
	script := `import runpy,sys,json,os
p=runpy.run_path(sys.argv[1],run_name='reference')
g=p['hold'].__globals__
g['STATE_DIR']=sys.argv[2]
g['INBOX']=os.path.join(sys.argv[2],'inbox')
identity,_,_,unread=g['inbox_view'](sys.argv[3])
print(json.dumps({'identity':identity,'unread':[m['id'] for m in unread]}))`
	out, err := exec.Command("python3", "-c", script, reference, e.State, session).CombinedOutput()
	if err != nil {
		t.Fatalf("reference: %v: %s", err, out)
	}
	var py struct {
		Identity string   `json:"identity"`
		Unread   []string `json:"unread"`
	}
	if err := json.Unmarshal(out, &py); err != nil {
		t.Fatal(err, string(out))
	}
	if py.Identity != want || len(py.Unread) != 1 || py.Unread[0] != id {
		t.Fatalf("reference view: %+v, want identity %s and unread %s", py, want, id)
	}
}
