package hostedpeers_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/hostedpeers"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
)

func fixture(t *testing.T) (hostedpeers.Config, *kitapitest.Server, *kitapi.Client) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HEV_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("HEV_PEERS_HOSTED_PRESENCE", "true")
	t.Setenv("HEV_PEERS_HOSTED_MESSAGES", "true")
	s := kitapitest.New(t, "key", kitapitest.Identity{Host: "host", TeamID: "team", AuthorID: "member"})
	c := hostedpeers.Config{API: s.API(), Key: "key", Team: "team", Root: filepath.Join(home, "hosted"), Messages: true, Presence: true}
	writeConfig(t, c)
	client := s.Client("key")
	client.Retries = 0
	return c, s, client
}
func writeConfig(t *testing.T, c hostedpeers.Config) {
	t.Helper()
	b := fmt.Sprintf("[kit]\napi=%q\nkey=%q\nteam_id=%q\n[capture]\nredact_salt=%q\n", c.API, c.Key, c.Team, strings.Repeat("ab", 32))
	if e := os.WriteFile(os.Getenv("HEV_CONFIG"), []byte(b), 0600); e != nil {
		t.Fatal(e)
	}
}
func msg() kitapi.PeerMessageIn {
	return kitapi.PeerMessageIn{Sender: "host/sender", Recipients: []string{"other/reader", "host/reader"}, Time: time.Now().UTC(), Kind: "chat", ChatID: "thread", Body: "hello"}
}
func pending(t *testing.T, c hostedpeers.Config) []string {
	t.Helper()
	o, e := testOutbox(c)
	if e != nil {
		t.Fatal(e)
	}
	names, e := o.Pending()
	if e != nil {
		t.Fatal(e)
	}
	return names
}
func TestHostedBeaconOnlyWhenLoggedInAndEachOffSwitch(t *testing.T) {
	c, s, client := fixture(t)
	ctx := context.Background()
	in := kitapi.PresenceIn{Host: "host", ObservedAt: time.Now(), Sessions: []kitapi.PeerSession{}}
	key := c.Key
	c.Key = ""
	if e := c.QueuePresence(in); e != nil {
		t.Fatal(e)
	}
	if e := c.QueueMessage(msg()); e != nil {
		t.Fatal(e)
	}
	if e := c.Flush(ctx, client); e != nil {
		t.Fatal(e)
	}
	if s.Requests("GET /v1/whoami") != 0 {
		t.Fatal("signed-out traffic")
	}
	c.Key = key
	c.Presence = false
	c.Messages = false
	_ = c.QueuePresence(in)
	_ = c.QueueMessage(msg())
	_ = c.Flush(ctx, client)
	if s.Requests("GET /v1/whoami") != 0 {
		t.Fatal("disabled traffic")
	}
	// Presence disabled independently while message upload remains enabled.
	c.Messages = true
	_ = c.QueueMessage(msg())
	if e := c.Flush(ctx, client); e != nil {
		t.Fatal(e)
	}
	if s.Requests("POST /v1/peers/presence") != 0 || s.Requests("POST /v1/peers/messages") != 1 {
		t.Fatal("presence off ignored")
	}
	// Message switch disables draining already queued entries, independently.
	_ = c.QueueMessage(msg())
	c.Messages = false
	c.Presence = true
	_ = c.QueuePresence(in)
	if e := c.Flush(ctx, client); e != nil {
		t.Fatal(e)
	}
	if s.Requests("POST /v1/peers/presence") != 1 || s.Requests("POST /v1/peers/messages") != 1 || len(pending(t, c)) != 1 {
		t.Fatal("message off ignored")
	}
}
func TestHostedReadOnlyQueueRecoveryDurableChatUUID(t *testing.T) {
	c, s, client := fixture(t)
	ctx := context.Background()
	s.SetReadOnly(true)
	if e := c.QueueMessage(msg()); e != nil {
		t.Fatal(e)
	}
	names := pending(t, c)
	if len(names) != 1 {
		t.Fatal(names)
	}
	o, _ := testOutbox(c)
	before, e := os.ReadFile(filepath.Join(o.Dir(), names[0]))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Flush(ctx, client); !kitapi.ReadOnly(e) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(o.Dir(), names[0]))
	if string(before) != string(after) {
		t.Fatal("retry changed UUID/body")
	}
	// Recreate the client as after a process crash. The durable entry owns ID.
	restarted := c
	s.SetReadOnly(false)
	if e = restarted.Flush(ctx, client); e != nil {
		t.Fatal(e)
	}
	page, e := client.PeerMessages(ctx, nil)
	if e != nil || len(page.Messages) != 1 || len(page.Messages[0].ID) != 32 || len(page.Messages[0].Recipients) != 2 || len(pending(t, c)) != 0 {
		t.Fatal(page, e)
	}
	// Lost successful response replay remains exactly one stored row.
	if e = o.Enqueue(kitcapture.KindPeerMessage, before); e != nil {
		t.Fatal(e)
	}
	if e = c.Flush(ctx, client); e != nil {
		t.Fatal(e)
	}
	page, e = client.PeerMessages(ctx, nil)
	if e != nil || len(page.Messages) != 1 {
		t.Fatal(page, e)
	}
}
func TestHostedUnauthorizedStopsRetries(t *testing.T) {
	c, s, client := fixture(t)
	_ = c.QueueMessage(msg())
	s.Revoke("key")
	if e := c.Flush(context.Background(), client); !errors.Is(e, kitapi.ErrUnauthorized) {
		t.Fatal(e)
	}
	n := s.Requests("GET /v1/whoami")
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	if s.Requests("GET /v1/whoami") != n || len(pending(t, c)) != 1 {
		t.Fatal("revoked key retried or entry removed")
	}
}
func TestHostedOfflineRetainsAndPermanentRefusalStops(t *testing.T) {
	c, s, client := fixture(t)
	_ = c.QueueMessage(msg())
	s.SetDown(true)
	if e := c.Flush(context.Background(), client); e == nil || len(pending(t, c)) != 1 {
		t.Fatal(e)
	}
	s.SetDown(false)
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	m := msg()
	m.ID = "custom-non-uuid"
	_ = c.QueueMessage(m)
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	if len(pending(t, c)) != 0 {
		t.Fatal("permanent refusal loops")
	}
	o, _ := testOutbox(c)
	files, _ := os.ReadDir(filepath.Join(o.Dir(), "failed"))
	if len(files) == 0 {
		t.Fatal("missing refusal evidence")
	}
}
func TestHostedRedactionBeforeUploadAndDirectID(t *testing.T) {
	c, _, client := fixture(t)
	m := msg()
	m.ID = "123456781234123412341234567890ab"
	secret := "ghp_" + "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	m.Body = "token=" + secret
	m.Title = secret
	if e := c.QueueMessage(m); e != nil {
		t.Fatal(e)
	}
	o, _ := testOutbox(c)
	for _, n := range pending(t, c) {
		b, _ := os.ReadFile(filepath.Join(o.Dir(), n))
		if strings.Contains(string(b), secret) {
			t.Fatal("raw secret in hosted queue")
		}
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	page, e := client.PeerMessages(context.Background(), nil)
	if e != nil || len(page.Messages) != 1 || strings.Contains(page.Messages[0].Body, secret) || page.Messages[0].ID != m.ID {
		t.Fatal(page, e)
	}
}
func TestHostedOutboxNeverReboundToAnotherTeam(t *testing.T) {
	c, s, client := fixture(t)
	_ = c.QueueMessage(msg())
	s.AddKey("other", kitapitest.Identity{TeamID: "other", Host: "host"})
	if e := c.Flush(context.Background(), s.Client("other")); !errors.Is(e, kitcapture.ErrTeamMismatch) {
		t.Fatal(e)
	}
	if s.Requests("POST /v1/peers/messages") != 0 || len(pending(t, c)) != 1 {
		t.Fatal("cross-team upload")
	}
	if e := c.Flush(context.Background(), client); e != nil {
		t.Fatal(e)
	}
}
func TestHostedConfigOffSwitches(t *testing.T) {
	c, _, _ := fixture(t)
	loaded, e := hostedpeers.Load()
	if e != nil || !loaded.SignedIn() || !loaded.Presence || !loaded.Messages {
		t.Fatal(loaded, e)
	}
	t.Setenv("HEV_PEERS_HOSTED_PRESENCE", "false")
	loaded, e = hostedpeers.Load()
	if e != nil || loaded.Presence || !loaded.Messages {
		t.Fatal(loaded, e)
	}
	t.Setenv("HEV_PEERS_HOSTED_MESSAGES", "0")
	loaded, e = hostedpeers.Load()
	if e != nil || loaded.Messages {
		t.Fatal(loaded, e)
	}
	t.Setenv("HEV_PEERS_HOSTED_PRESENCE", "")
	t.Setenv("HEV_PEERS_HOSTED_MESSAGES", "")
	// Unset overrides to exercise actual TOML switches.
	os.Unsetenv("HEV_PEERS_HOSTED_PRESENCE")
	os.Unsetenv("HEV_PEERS_HOSTED_MESSAGES")
	f, e := os.OpenFile(os.Getenv("HEV_CONFIG"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.WriteString("\n[peers]\nhosted_presence=false\nhosted_messages=false\n")
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	loaded, e = hostedpeers.Load()
	if e != nil || loaded.Presence || loaded.Messages {
		t.Fatal(loaded, e)
	}
	c.Key = ""
	writeConfig(t, c)
	loaded, e = hostedpeers.Load()
	if e != nil || loaded.SignedIn() {
		t.Fatal(loaded, e)
	}
}

func testOutbox(c hostedpeers.Config) (*kitcapture.Outbox, error) {
	return kitcapture.OpenOutbox(c.Root, c.Team)
}
func TestHostedPresenceRefusalDoesNotStarveMessageDrain(t *testing.T) {
	c, _, client := fixture(t)
	if e := c.QueuePresence(kitapi.PresenceIn{Host: "wrong-host", ObservedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if e := c.QueueMessage(msg()); e != nil {
		t.Fatal(e)
	}
	e := c.Flush(context.Background(), client)
	var ae *kitapi.Error
	if !errors.As(e, &ae) || ae.Status != 403 {
		t.Fatal("missing presence refusal", e)
	}
	page, e := client.PeerMessages(context.Background(), nil)
	if e != nil || len(page.Messages) != 1 || len(pending(t, c)) != 0 {
		t.Fatal("presence refusal starved message queue", page, e)
	}
}
