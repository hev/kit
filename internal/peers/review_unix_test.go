//go:build !windows

package peers

// Regression coverage for the native peers compatibility review.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func reviewSnapshot(t *testing.T, e *Engine, host, body string) {
	t.Helper()
	dir := filepath.Join(e.State, "hosts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, host+".json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// R1: an HTTP enqueue for a bare name that is not local is relayed onward,
// without --inbox-only, even though the handler rejects "/" targets.
func TestReviewHTTPEnqueueRelaysOnward(t *testing.T) {
	e, _ := fixture(t)
	r := &recordingTransport{}
	e.Transport = r
	reviewSnapshot(t, e, "third", `{"host":"third","panes":[{"address":"ghost","role":"foh"}]}`)
	srv := httptest.NewServer(e.Handler("secret"))
	defer srv.Close()
	b, _ := json.Marshal(enqueue{Target: "ghost", From: "origin/sender", Message: "hello"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/peers/enqueue", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var d delivery
	json.NewDecoder(resp.Body).Decode(&d)
	t.Logf("delivery=%+v forwarded host=%q args=%q", d, r.host, r.args)
	if r.host != "" {
		t.Errorf("HTTP enqueue was relayed to %s with args %q", r.host, r.args)
	}
}

// R2: HEVD transport rejects the --order/--run flags the engine now forwards.
func TestReviewHEVDRejectsProvenance(t *testing.T) {
	e, call := fixture(t)
	fallback := &recordingTransport{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(delivery{Status: "delivered to session inbox"})
	}))
	defer srv.Close()
	e.Transport = HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: fallback}
	if _, err := call("", "send", "remote/desk", "hello", "--from", "local/desk"); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	_, err := call("", "send", "remote/desk", "hello", "--from", "local/desk", "--order", "o-1", "--run", "r-1")
	t.Logf("with provenance: err=%v fallback=%q", err, fallback.host)
	if err != nil {
		t.Errorf("provenance send over configured hevd failed: %v", err)
	}
}

// R3: SSH send d-NAME drops --inbox-only, --id, --order and --run.
func TestReviewSSHRunSendRejectsUnsupportedFlags(t *testing.T) {
	for _, flag := range []string{"--inbox-only", "--id", "--order", "--run"} {
		args := []string{"send", "d-task", "hello", "--from", "a/b", flag}
		if flag != "--inbox-only" {
			args = append(args, "stable")
		}
		if err := (SSHTransport{}).Execute(context.Background(), "remote", args, strings.NewReader(""), io.Discard, io.Discard); err == nil {
			t.Fatal("accepted", flag)
		}
	}
	if err := (SSHTransport{}).Execute(context.Background(), "remote", []string{"send", "d-task", "-"}, strings.NewReader("  "), io.Discard, io.Discard); err == nil {
		t.Fatal("accepted empty message")
	}
}

// R4: a bare d-NAME naming a local run is routed to a remote snapshot instead.
func TestReviewLocalRunDPrefixRoutedRemote(t *testing.T) {
	e, _ := fixture(t)
	local := []Peer{{Session: "task", Run: true, Address: "local/task"}}
	if _, err := find(local, "d-task"); err != nil {
		t.Fatal("find should match the local run", err)
	}
	reviewSnapshot(t, e, "remote", `{"host":"remote","panes":[{"address":"d-task","role":"boh"}]}`)
	target, err := e.relayAlias("d-task", local)
	t.Logf("target=%q err=%v", target, err)
	if target != "d-task" {
		t.Errorf("local run d-task resolved to %q", target)
	}
}

// R5: an unsupported runtime token blocks plain sends, not only receipt mutation.
func TestReviewTokenBlocksSend(t *testing.T) {
	_, call := fixture(t)
	t.Setenv("PEERS_FROM", "")
	if _, err := call("", "send", "desk", "hello"); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	t.Setenv("PEERS_INBOX_TOKEN", strings.Repeat("a", 64))
	_, err := call("", "send", "desk", "hello")
	t.Logf("send with token: %v", err)
	if err != nil {
		t.Errorf("send without --from failed under PEERS_INBOX_TOKEN: %v", err)
	}
}

// R1b: the SSH RPC receiver re-resolves a bare target and relays it again.
func TestReviewRPCReceiverRelaysOnward(t *testing.T) {
	bin, state := t.TempDir(), t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin)
	t.Setenv("PEERS_SELF", "middle")
	t.Setenv("HEV_PEERS_STATE", state)
	log := filepath.Join(bin, "ssh.log")
	t.Setenv("REVIEW_SSH_LOG", log)
	os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\necho 'no server running' >&2\nexit 1\n"), 0700)
	os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\necho \"$@\" >> \"$REVIEW_SSH_LOG\"\n/bin/cat > /dev/null\nprintf '{\"log\":\"\"}\\n'\n"), 0700)
	os.MkdirAll(filepath.Join(state, "hosts"), 0700)
	os.WriteFile(filepath.Join(state, "hosts", "origin.json"), []byte(`{"host":"origin","panes":[{"address":"ghost","role":"foh"}]}`), 0600)
	note, _ := json.Marshal(RPCRequest{Args: []string{"send", "ghost", "hello", "--from", "origin/sender"}})
	r := HandleRPC(context.Background(), string(note))
	b, _ := os.ReadFile(log)
	t.Logf("response=%+v ssh invocations=%q", r, b)
	if len(b) > 0 {
		t.Errorf("RPC-received send was relayed again over SSH: %s", b)
	}
}

// R7: a stable-ID retry across implementations conflicts on from_agent/from_cwd.
func TestReviewMixedWriterRetry(t *testing.T) {
	reference := os.Getenv("PEERS_PYTHON_REFERENCE")
	if reference == "" {
		t.Skip("reference")
	}
	e, call := fixture(t)
	if _, err := call("seed", "inbox", "hold", "--id", "seed"); err != nil {
		t.Fatal(err)
	}
	ms, _ := rows(e.path("desk"))
	identity := ms[0]["recipient_incarnation"].(string)
	script := `import runpy,sys,os
p=runpy.run_path(sys.argv[1],run_name='reference')
g=p['hold'].__globals__
g['STATE_DIR']=sys.argv[2]
g['INBOX']=os.path.join(sys.argv[2],'inbox')
g['incarnation']=lambda *a,**k:sys.argv[3]
g['tmux']=lambda *a,**k:''
g['hold']('desk','[peer message from local/desk (claude, /work). Reply]','hello','local/desk',{'id':'retry-1','from_agent':'claude','from_cwd':'/work','order':'o-1'})
`
	if b, err := exec.Command("python3", "-c", script, reference, e.State, identity).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	_, err := call("", "send", "desk", "hello", "--from", "local/desk", "--id", "retry-1", "--order", "o-1")
	t.Logf("native retry of Python-written id: %v", err)
	if err != nil {
		t.Errorf("same logical retry rejected: %v", err)
	}
	if _, err := call("", "send", "desk", "hello", "--from", "local/desk", "--id", "retry-2", "--order", "o-1"); err != nil {
		t.Fatal(err)
	}
	reverse := strings.ReplaceAll(script, "retry-1", "retry-2")
	if b, err := exec.Command("python3", "-c", reverse, reference, e.State, identity).CombinedOutput(); err != nil {
		t.Fatalf("reverse retry: %v %s", err, b)
	}

}

// Soundness probe: concurrent Go and Python writers on one inbox.
func TestReviewMixedConcurrentHold(t *testing.T) {
	reference := os.Getenv("PEERS_PYTHON_REFERENCE")
	if reference == "" {
		t.Skip("reference")
	}
	e, call := fixture(t)
	if _, err := call("seed", "inbox", "hold", "--id", "seed"); err != nil {
		t.Fatal(err)
	}
	ms, _ := rows(e.path("desk"))
	identity := ms[0]["recipient_incarnation"].(string)
	script := `import runpy,sys,os
p=runpy.run_path(sys.argv[1],run_name='reference')
g=p['hold'].__globals__
g['STATE_DIR']=sys.argv[2]
g['INBOX']=os.path.join(sys.argv[2],'inbox')
g['incarnation']=lambda *a,**k:sys.argv[3]
g['tmux']=lambda *a,**k:''
for i in range(50):
    g['hold']('desk','h','x'*3000,'py',{'id':'py-%s-%d'%(sys.argv[4],i)})
`
	done := make(chan error, 8)
	for w := 0; w < 4; w++ {
		w := w
		go func() {
			b, err := exec.Command("python3", "-c", script, reference, e.State, identity, string(rune('a'+w))).CombinedOutput()
			if err != nil {
				err = fmt.Errorf("%v %s", err, b)
			}
			done <- err
		}()
		go func() {
			var err error
			for i := 0; i < 50 && err == nil; i++ {
				_, err = e.hold("desk", identity, Message{"id": fmt.Sprintf("go-%d-%d", w, i), "message": strings.Repeat("y", 3000)})
			}
			done <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	ms, _ = rows(e.path("desk"))
	if len(ms) != 401 {
		t.Fatalf("rows=%d", len(ms))
	}
}

// panicReader detects any attempted stdin read without blocking the suite.
type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("unexpected stdin read") }

func TestSSHNonMessageCommandsDoNotReadStdin(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncat >/dev/null\nprintf '{\"log\":\"ok\"}\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"read", "desk"}, {"ls", "--json"}, {"inbox", "--session", "desk", "--peek"}, {"send", "d-task", "hello", "--from", "a/b"}} {
		var out bytes.Buffer
		if err := (SSHTransport{}).Execute(context.Background(), "remote", args, panicReader{}, &out, io.Discard); err != nil {
			t.Fatal(args, err)
		}
	}
}

func TestRemoteRequestsRejectQualifiedTargets(t *testing.T) {
	e, call := fixture(t)
	e.remote = true
	r := &recordingTransport{}
	e.Transport = r
	for _, args := range [][]string{{"send", "third/desk", "hi", "--from", "origin/sender"}, {"read", "third/desk"}, {"ls", "--everywhere"}, {"inbox", "--session", "third/desk", "--peek"}} {
		if _, err := call("", args...); err == nil {
			t.Fatal("accepted", args)
		}
	}
	if r.host != "" {
		t.Fatal("forwarded", r.host)
	}
}

func TestTokenAllowsAttributionButRefusesReceipts(t *testing.T) {
	e, call := fixture(t)
	t.Setenv("PEERS_INBOX_TOKEN", "unsupported")
	if address, err := e.OwnAddress(context.Background()); err != nil || address != "local/desk" {
		t.Fatal(address, err)
	}
	if _, err := call("", "inbox", "--pending", "--json"); err == nil {
		t.Fatal("accepted unsupported receipt capability")
	}
}

func TestHTTPProvenanceStoredAndValidated(t *testing.T) {
	e, _ := fixture(t)
	srv := httptest.NewServer(e.Handler("secret"))
	defer srv.Close()
	transport := HEVDTransport{Hosts: map[string]Endpoint{"remote": {URL: srv.URL, Token: "secret"}}, Fallback: &recordingTransport{}}
	args := []string{"send", "desk", "hello", "--from", "origin/sender", "--order", "o-1", "--run", "r-1"}
	if err := transport.Execute(context.Background(), "remote", args, strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	ms, err := rows(e.path("desk"))
	if err != nil || len(ms) != 1 {
		t.Fatal(ms, err)
	}
	if ms[0]["order"] != "o-1" || ms[0]["run"] != "r-1" {
		t.Fatal(ms)
	}
	b, _ := json.Marshal(enqueue{Target: "desk", From: "origin/sender", Message: "hello", Order: "bad/order"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/peers/enqueue", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
}

func TestSSHRunUsesRestrictedActionAndHeader(t *testing.T) {
	bin := t.TempDir()
	capture := filepath.Join(bin, "request")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PEERS_RPC_CAPTURE", capture)
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\ncat >\"$PEERS_RPC_CAPTURE\"\nprintf '{\"message\":\"ok\"}\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := (SSHTransport{}).Execute(context.Background(), "remote", []string{"send", "d-task", "-", "--from", "origin/sender"}, strings.NewReader(" hello \n"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var q struct{ Action, Name, Note string }
	if err := json.Unmarshal(b, &q); err != nil {
		t.Fatal(err)
	}
	if q.Action != "peer-send" || q.Name != "task" || q.Note != messageHeader("origin/sender")+"\nhello" {
		t.Fatal(q)
	}
}
