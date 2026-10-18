//go:build linux || darwin

package nativehost

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiverRPCIdentityBeforeFixedProfileExecution(t *testing.T) {
	f := setup(t)
	root := f.p.Runtime.Directory
	script := "#!/bin/sh\nprintf '%s' '{\"Node\":{\"StableID\":\"oven\"},\"UserProfile\":{\"ID\":123}}'\n"
	who := filepath.Join(root, "tailscale")
	if e := os.WriteFile(who, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	pin := FilePin{who, hash([]byte(script))}
	runtime := f.p.Runtime
	runtime.Files = append(runtime.Files, pin)
	r := Remote{"100.100.100.1:19761", "oven", "123", pin, runtime}
	if e := r.verify(); e != nil {
		t.Fatal(e)
	}
	receiver := "#!/bin/sh\n[ \"$#\" -eq 2 ] && [ \"$1\" = '--config' ] || exit 9\nprintf x >> '" + f.effects + "'\ncat\n"
	if e := os.WriteFile(f.p.SupervisorRef, []byte(receiver), 0700); e != nil {
		t.Fatal(e)
	}
	f.p.SupervisorDigest = hash([]byte(receiver))
	// Do not use setup's supervisor-PID cleanup for this byte-only receiver.
	f.effects = filepath.Join(root, "not-the-receiver-marker")
	call := json.RawMessage(`{"version":"native-receiver/v1","operation":"inspect","job_id":"original"}`)
	envelope := receiverEnvelope{ReceiverRPCVersion, f.p.ID, f.p.ConfigDigest, call}
	run := func(env receiverEnvelope, remote Remote) *httptest.ResponseRecorder {
		b, _ := json.Marshal(env)
		req := httptest.NewRequest("POST", "http://node/native-receiver/v1", bytes.NewReader(b))
		req.RemoteAddr = "100.110.73.23:12345"
		w := httptest.NewRecorder()
		receiverRPCHandler([]Profile{f.p}, remote).ServeHTTP(w, req)
		return w
	}
	bad := r
	bad.AllowedUserID = "456"
	if w := run(envelope, bad); w.Code != 503 {
		t.Fatal(w.Code)
	}
	marker := filepath.Join(root, "effects")
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("wrong identity executed receiver")
	}
	wrong := envelope
	wrong.ConfigDigest = hash([]byte("other"))
	if w := run(wrong, r); w.Code != 503 {
		t.Fatal(w.Code)
	}
	wrong = envelope
	wrong.Call = json.RawMessage(`{"version":"native-receiver/v1","operation":"inspect","job_id":"original","argv":["bad"]}`)
	if w := run(wrong, r); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if w := run(envelope, r); w.Code != 200 || w.Body.String() != string(call) {
		t.Fatal(w.Code, w.Body.String())
	}
	b, e := os.ReadFile(marker)
	if e != nil || string(b) != "x" {
		t.Fatal("expected exactly one fixed receiver execution", string(b), e)
	}
	os.Remove(marker)
}
func TestReceiverRPCBoundedProtocolAndTailnetOnly(t *testing.T) {
	for _, s := range []string{"127.0.0.1:19761", "0.0.0.0:19761", "100.110.73.23:80", "oven:19761"} {
		if tailAddress(s) {
			t.Fatal(s)
		}
	}
	if !tailAddress("100.110.73.23:19761") {
		t.Fatal("tailnet rejected")
	}
	if _, e := boundedRead(strings.NewReader(strings.Repeat("x", maxFrame+1))); e == nil {
		t.Fatal("oversized request")
	}
	for _, call := range []string{`{"version":"native-receiver/v1","operation":"prepare","job_id":"x"}`, `{"version":"native-receiver/v1","operation":"inspect","job_id":"x","job_id":"y"}`} {
		if receiverCall([]byte(call)) == nil {
			t.Fatal(call)
		}
	}
}
