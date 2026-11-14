package peers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
)

func TestNativeHostedSendReusesInboxIDWithoutChangingWire(t *testing.T) {
	_, call := fixture(t)
	home := t.TempDir()
	config := filepath.Join(home, "config.toml")
	root := filepath.Join(home, "hosted")
	t.Setenv("HEV_CONFIG", config)
	t.Setenv("HEV_KIT_PEERS_OUTBOX", root)
	t.Setenv("HEV_PEERS_HOSTED_MESSAGES", "true")
	// Reserved test domain; no network is launched by the test executable.
	body := fmt.Sprintf("[kit]\napi=\"https://api.example.test\"\nkey=\"test\"\nteam_id=\"team\"\n[capture]\nredact_salt=%q\n", strings.Repeat("ab", 32))
	if e := os.WriteFile(config, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	id := "123456781234123412341234567890ab"
	out, e := call("", "send", "desk", "hello", "--from", "sender", "--id", id)
	if e != nil {
		t.Fatal(out, e)
	}
	o, e := kitcapture.OpenOutbox(root, "team")
	if e != nil {
		t.Fatal(e)
	}
	names, e := o.Pending()
	if e != nil || len(names) != 1 {
		t.Fatal(names, e)
	}
	b, e := os.ReadFile(filepath.Join(o.Dir(), names[0]))
	if e != nil {
		t.Fatal(e)
	}
	var msg kitapi.PeerMessageIn
	if e = json.Unmarshal(b, &msg); e != nil {
		t.Fatal(e)
	}
	if msg.ID != id || msg.Sender != "local/sender" || len(msg.Recipients) != 1 || msg.Recipients[0] != "local/desk" {
		t.Fatal(msg)
	}
	if _, e = call("", "send", "missing", "failure", "--from", "sender"); e == nil {
		t.Fatal("missing recipient succeeded")
	}
	names, e = o.Pending()
	if e != nil || len(names) != 1 {
		t.Fatal("failure queued", names, e)
	}
}
