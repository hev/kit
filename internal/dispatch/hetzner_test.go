package dispatch

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestHetznerHTTPCreateAdoptPinAndDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HC_TEST_TOKEN", "private-token")
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	key, _ := ssh.NewPublicKey(pub)
	t.Setenv("HC_TEST_SSH", string(ssh.MarshalAuthorizedKey(key)))
	bootstrap := filepath.Join(t.TempDir(), "bootstrap")
	if e := os.WriteFile(bootstrap, []byte("#!/bin/sh\ntrue\n"), 0600); e != nil {
		t.Fatal(e)
	}
	h := Host{Name: "example", Type: "hetzner", Mode: "persistent", APIKeyEnv: "HC_TEST_TOKEN", SSHPublicKeyEnv: "HC_TEST_SSH", BootstrapFile: bootstrap}
	created, deleted := false, false
	var userData string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("missing env credential")
		}
		server := map[string]any{"id": 1, "name": "kit-example", "status": "running", "labels": map[string]string{"kit-managed": "true", "hev-dispatch": "example", "hev-mode": "persistent"}, "public_net": map[string]any{"ipv4": map[string]string{"ip": "192.0.2.1"}}, "location": map[string]string{"name": "nbg1"}, "server_type": map[string]any{"name": "cax11", "prices": []any{map[string]any{"location": "nbg1", "price_monthly": map[string]string{"gross": "4.00"}}}}}
		var out any
		switch {
		case r.Method == "GET" && r.URL.Path == "/servers":
			if r.URL.Query().Get("label_selector") != "kit-managed=true,hev-dispatch=example" {
				t.Error("missing ownership selector")
			}
			items := []any{}
			if created && !deleted {
				items = append(items, server)
			}
			out = map[string]any{"servers": items}
		case r.Method == "GET" && r.URL.Path == "/servers/1":
			if deleted {
				w.WriteHeader(404)
				return
			}
			out = map[string]any{"server": server}
		case r.Method == "POST" && r.URL.Path == "/servers":
			var b map[string]any
			if e := json.NewDecoder(r.Body).Decode(&b); e != nil {
				t.Fatal(e)
			}
			userData = b["user_data"].(string)
			if _, exists := b["ssh_keys"]; exists {
				t.Error("client public keys must not require duplicate project SSH-key objects")
			}
			if strings.Contains(userData, "private-token") {
				t.Error("token leaked into cloud-init")
			}
			if !strings.Contains(userData, "ed25519_private") || !strings.Contains(userData, "ssh_authorized_keys") {
				t.Error("missing pinned bootstrap keys")
			}
			created = true
			out = map[string]any{"server": server}
		case r.Method == "DELETE" && r.URL.Path == "/servers/1":
			deleted = true
			out = map[string]any{}
		case r.Method == "GET" && r.URL.Path == "/ssh_keys":
			out = map[string]any{"ssh_keys": []any{}}
		case r.Method == "GET" && r.URL.Path == "/firewalls":
			out = map[string]any{"firewalls": []any{}}
		case r.Method == "POST" && r.URL.Path == "/ssh_keys":
			out = map[string]any{"ssh_key": map[string]int{"id": 2}}
		case r.Method == "POST" && r.URL.Path == "/firewalls":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			rules := b["rules"].([]any)
			if len(rules) != 1 || rules[0].(map[string]any)["port"] != "22" {
				t.Error("firewall not SSH-only")
			}
			out = map[string]any{"firewall": map[string]int{"id": 3}}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	p, _ := NewHetzner(h)
	p.endpoint = srv.URL
	p.client = srv.Client()
	spec := MachineSpec{Name: "kit-example", Host: "example", From: "ubuntu-24.04", Size: "cax11", Region: "nbg1", Labels: map[string]string{"hev.dispatch": "example"}}
	m, e := p.Create(context.Background(), spec)
	if e != nil {
		t.Fatal(e)
	}
	if m.HostKey == "" || !strings.HasPrefix(m.HostKey, "192.0.2.1 ssh-ed25519 ") {
		t.Fatal("missing pre-generated pin")
	}
	adopted, e := p.Create(context.Background(), spec)
	if e != nil || adopted.HostKey != m.HostKey {
		t.Fatalf("adoption lost pin: %v", e)
	}
	shown, e := p.Show(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	shownJSON, _ := json.Marshal(shown)
	if !strings.Contains(string(shownJSON), `"gross":"4.00"`) {
		t.Fatal("show lost API monthly-cap price")
	}
	transport, e := p.Transport(context.Background(), m.ID)
	if e != nil || transport.HostKey != m.HostKey {
		t.Fatal("transport pin mismatch", e)
	}
	if e = os.Remove(p.keyPath(m.Name)); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Transport(context.Background(), m.ID); e == nil {
		t.Fatal("accepted unpinned adoption")
	}
	if e = p.Stop(context.Background(), m.ID); e != nil || !deleted {
		t.Fatalf("Stop must DELETE: %v", e)
	}
}
func TestHetznerTokenOnlyFromEnv(t *testing.T) {
	p, _ := NewHetzner(Host{APIKeyEnv: "MISSING_HETZNER_TEST"})
	t.Setenv("MISSING_HETZNER_TEST", "")
	if e := p.api(context.Background(), "GET", "/servers", nil, nil); e == nil {
		t.Fatal("accepted missing token")
	}
	h := Host{Type: "hetzner", Cap: 3, APIKeyEnv: "literal-token!", SSHPublicKeyEnv: "PUBLIC", BootstrapFile: "bootstrap"}
	if _, e := validateHostType(h, "example"); e == nil {
		t.Fatal("accepted literal token")
	}
}
func TestHetznerPersistentSharesReceiverCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TEST_HC_TOKEN", "test")
	h := Host{Name: "example", Type: "hetzner", Mode: "persistent", Cap: 3, APIKeyEnv: "TEST_HC_TOKEN", Namespace: "example-traces"}
	t.Setenv("HEVLAYER_NAMESPACE", "other-traces")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server := map[string]any{"id": 1, "name": "kit-example", "status": "running", "labels": map[string]string{"kit-managed": "true", "hev-dispatch": "example"}, "public_net": map[string]any{"ipv4": map[string]string{"ip": "192.0.2.1"}}, "location": map[string]string{"name": "nbg1"}, "server_type": map[string]any{"name": "cax11", "prices": []any{map[string]any{"location": "nbg1", "price_monthly": map[string]string{"gross": "4.00"}}}}}
		if r.Method != "GET" {
			t.Error("shared receiver must not create per-order servers")
		}
		if r.URL.Path == "/servers" {
			_ = json.NewEncoder(w).Encode(map[string]any{"servers": []any{server}})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"server": server})
		}
	}))
	defer srv.Close()
	p, _ := NewHetzner(h)
	p.endpoint = srv.URL
	p.client = srv.Client()
	_ = os.MkdirAll(filepath.Dir(p.keyPath("kit-example")), 0700)
	_ = os.WriteFile(p.keyPath("kit-example"), []byte("ssh-ed25519 pin"), 0600)
	old := hetznerSSHExec
	defer func() { hetznerSSHExec = old }()
	starts := 0
	runtimeConfigured := false
	hetznerSSHExec = func(_ context.Context, tr MachineTransport, cmd string) (string, error) {
		if tr.SSH != "user@192.0.2.1" {
			t.Error("not same server")
		}
		if strings.Contains(cmd, "sudo install -o user") && strings.Contains(cmd, "/run/hev-runtime.env") {
			fragment := strings.Split(cmd, "printf %s '")[1]
			data, e := base64.StdEncoding.DecodeString(strings.Split(fragment, "'")[0])
			if e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(string(data), "export HEVLAYER_NAMESPACE='example-traces'") {
				t.Error("host namespace not supplied to capture runtime")
			}
			runtimeConfigured = true
		}
		if strings.Contains(cmd, "hev dispatch rpc") {
			encoded := strings.Split(cmd, "'")[1]
			data, e := base64.StdEncoding.DecodeString(encoded)
			if e != nil {
				t.Fatal(e)
			}
			var request Request
			if e = json.Unmarshal(data, &request); e != nil {
				t.Fatal(e)
			}
			if request.Cap != 3 {
				t.Errorf("receiver cap = %d, want 3", request.Cap)
			}
			starts++
			return fmt.Sprintf(`{"cap":3,"running":%d}`, starts), nil
		}
		return "", nil
	}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("order-%d", i)
		order := "---\nname: " + name + "\nhost: example\nrepo: example/project\nvalidation: none\n---\nTask\n## Acceptance\n- Done\n"
		o, e := Parse(order)
		if e != nil {
			t.Fatal(e)
		}
		r, e := hetznerPersistentWithProvider(context.Background(), h, Request{Action: "start", Name: name, Order: order, Meta: Meta{Order: o}}, p)
		if e != nil || r.Cap != 3 {
			t.Fatalf("shared start: %+v %v", r, e)
		}
	}
	if !runtimeConfigured {
		t.Fatal("runtime credentials were not configured")
	}
	if starts != 3 {
		t.Fatal("missing receiver admissions")
	}
}

func TestHetznerReconcileTerminalLeftoverDeletesAfterFetch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HC_TOKEN", "test")
	h := Host{Name: "example", Type: "hetzner", Mode: "run", APIKeyEnv: "HC_TOKEN"}
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/servers/1" && r.Method == "DELETE":
			deleted = true
			fmt.Fprint(w, `{}`)
		case r.URL.Path == "/servers/1":
			if deleted {
				w.WriteHeader(404)
				return
			}
			fmt.Fprint(w, `{"server":{"id":1,"name":"d-one","status":"running","labels":{"kit-managed":"true","hev-dispatch":"example","hev-order":"one"},"public_net":{"ipv4":{"ip":"192.0.2.1"}}}}`)
		case r.URL.Path == "/servers":
			fmt.Fprint(w, `{"servers":[]}`)
		case r.URL.Path == "/firewalls":
			fmt.Fprint(w, `{"firewalls":[]}`)
		case r.URL.Path == "/ssh_keys":
			fmt.Fprint(w, `{"ssh_keys":[]}`)
		default:
			t.Errorf("unexpected HTTP %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	p, _ := NewHetzner(h)
	p.endpoint = srv.URL
	p.client = srv.Client()
	_ = os.MkdirAll(filepath.Dir(p.keyPath("d-one")), 0700)
	_ = os.WriteFile(p.keyPath("d-one"), []byte("ssh-ed25519 pin"), 0600)
	files := map[string]string{"meta.json": `{"name":"one","host":"example"}`, "result.json": `{"end":"2026-01-01T00:00:00Z","exit_code":0}`, "report.md": "complete"}
	archive := boatArchive(t, files)
	old := hetznerSSHExec
	defer func() { hetznerSSHExec = old }()
	hetznerSSHExec = func(_ context.Context, _ MachineTransport, cmd string) (string, error) {
		if strings.Contains(cmd, "hev dispatch rpc") {
			b, _ := json.Marshal(Response{Files: files})
			return string(b), nil
		}
		if strings.Contains(cmd, "tar --exclude=wt") {
			return "/tmp/hev-record-one.test/part-aa\n", nil
		}
		if strings.Contains(cmd, "cat --") {
			return string(archive), nil
		}
		return "", nil
	}
	if e := boatCleanup(p, "1", false); e == nil || deleted {
		t.Fatal("deleted unfetched records")
	}
	if e := reconcileBoatTerminal(context.Background(), p, "1", h, "one"); e != nil {
		t.Fatal(e)
	}
	if !deleted {
		t.Fatal("terminal leftover was not deleted")
	}
	if _, e := cachedBoatFiles("one", "1", "example"); e != nil {
		t.Fatal("record not fetched before delete", e)
	}
}
