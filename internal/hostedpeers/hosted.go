// Package hostedpeers records optional hosted observations. It never transports
// peer messages. Local delivery finishes before a durable entry is queued;
// a separate process performs all network IO.
package hostedpeers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/platformops"
	iredact "github.com/hev/kit/internal/redact"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/redact"
)

type Config struct {
	API, Key, Team     string
	Presence, Messages bool
	Root               string
}

// Load uses the normal sign-in file, without resolving credentials or running
// subprocesses. No key, API or team means no hosted activity.
func Load() (Config, error) {
	path, e := iredact.ConfigPath()
	if e != nil {
		return Config{}, e
	}
	var doc struct {
		Kit struct {
			API, Key string
			Team     string `toml:"team_id"`
		}
		Peers struct {
			Presence *bool `toml:"hosted_presence"`
			Messages *bool `toml:"hosted_messages"`
		}
	}
	if _, e = toml.DecodeFile(path, &doc); os.IsNotExist(e) {
		return Config{}, nil
	} else if e != nil {
		return Config{}, e
	}
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".hev", "kit", "peers")
	if v := os.Getenv("HEV_KIT_PEERS_OUTBOX"); v != "" {
		root = v
	}
	on := func(v *bool, env string) bool {
		b := v == nil || *v
		if s, ok := os.LookupEnv(env); ok {
			parsed, e := strconv.ParseBool(s)
			return e == nil && parsed
		}
		return b
	}
	return Config{API: doc.Kit.API, Key: doc.Kit.Key, Team: doc.Kit.Team, Root: root, Presence: on(doc.Peers.Presence, "HEV_PEERS_HOSTED_PRESENCE"), Messages: on(doc.Peers.Messages, "HEV_PEERS_HOSTED_MESSAGES")}, nil
}
func (c Config) SignedIn() bool { return c.API != "" && c.Key != "" && c.Team != "" }
func UUID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return hex.EncodeToString(b)
}
func qualified(host, a string) string {
	if strings.Contains(a, "/") {
		return a
	}
	return host + "/" + a
}
func Address(host, a string) string                  { return qualified(host, a) }
func (c Config) outbox() (*kitcapture.Outbox, error) { return kitcapture.OpenOutbox(c.Root, c.Team) }

// QueueMessage persists the UUID in the independent, team-bound outbox before
// any upload can start. It does not touch Python-compatible state. Chats have
// no wire ID: generate one here, once, rather than in the retry path.
func (c Config) QueueMessage(in kitapi.PeerMessageIn) error {
	if !c.SignedIn() || !c.Messages {
		return nil
	}
	if in.ID == "" {
		in.ID = UUID()
	}
	scrub, e := iredact.Load()
	if e != nil {
		return e
	}
	if scrub == nil {
		return errors.New("hosted peers requires capture redaction")
	}
	in.Body, _ = redact.Text(scrub, in.Body)
	in.Title, _ = redact.Text(scrub, in.Title)
	o, e := c.outbox()
	if e != nil {
		return e
	}
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	return o.Enqueue(kitcapture.KindPeerMessage, b)
}
func (c Config) QueuePresence(in kitapi.PresenceIn) error {
	if !c.SignedIn() || !c.Presence {
		return nil
	}
	o, e := c.outbox()
	if e != nil {
		return e
	}
	// Only the latest beacon matters; never replay an old presence after outage.
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	return atomicFile(filepath.Join(o.Dir(), "presence"), b)
}
func atomicFile(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".peers-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = platformops.Rename(f.Name(), path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return platformops.SyncDir(d)
}
func (c Config) keyTag() string {
	sum := sha256.Sum256([]byte(c.API + "\x00" + c.Key))
	return hex.EncodeToString(sum[:])
}

// Flush is called only in the uploader process (or explicitly by tests).
// 401 persists a stop marker for this API/key; only a new login key resumes it.
// 402/transient errors retain entries. Permanent message refusals go to failed/.
// Off switches suppress uploads of already queued data as well as new entries.
func (c Config) Flush(ctx context.Context, client *kitapi.Client) error {
	if !c.SignedIn() || (!c.Messages && !c.Presence) {
		return nil
	}
	o, e := c.outbox()
	if e != nil {
		return e
	}
	unlock, e := platformops.LockPath(filepath.Join(o.Dir(), ".hosted.lock"), true)
	if platformops.IsLockBusy(e) {
		return nil
	}
	if e != nil {
		return e
	}
	defer unlock()
	stop := filepath.Join(o.Dir(), "revoked-"+c.keyTag())
	if _, e = os.Stat(stop); e == nil {
		return nil
	}
	who, e := client.Whoami(ctx)
	if e == nil && who.Team.ID != c.Team {
		e = kitcapture.ErrTeamMismatch
	}
	verified := e == nil
	if e == nil && c.Presence {
		path := filepath.Join(o.Dir(), "presence")
		b, readErr := os.ReadFile(path)
		if readErr == nil {
			var in kitapi.PresenceIn
			if json.Unmarshal(b, &in) == nil && time.Since(in.ObservedAt) < time.Duration(kitapi.PeersTTLSeconds)*time.Second {
				_, e = client.BeaconPeers(ctx, in)
			}
			// Do not remove: a concurrent local beacon may have replaced it. Revision
			// idempotency makes re-upload harmless and expiry is never extended by it.
		}
	}
	if verified && c.Messages && !errors.Is(e, kitapi.ErrUnauthorized) {
		presenceErr := e
		_, e = o.Drain(ctx, client, nil)
		if e == nil {
			e = presenceErr
		}
	}
	if errors.Is(e, kitapi.ErrUnauthorized) {
		_ = atomicFile(stop, []byte("revoked\n"))
	}
	if e != nil {
		return e
	}
	return nil
}

// WarnOnce persists a generic warning, never an API URL/key or server error.
func (c Config) WarnOnce(w io.Writer) {
	if !c.SignedIn() {
		return
	}
	o, e := c.outbox()
	if e != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(o.Dir(), "warned"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e == nil {
		f.Close()
		fmt.Fprintln(w, "peers: hosted observations could not be uploaded; local peers continues; queued messages wait for a later beacon (run hev login if the key was revoked)")
	}
}

// Kick starts a bounded background uploader; no HTTP is done by this process.
// stdout is a private append-only diagnostic file so errors survive CLI exit.
func (c Config) Kick() {
	if !c.SignedIn() || (!c.Messages && !c.Presence) {
		return
	}
	o, e := c.outbox()
	if e != nil {
		return
	}
	exe, e := os.Executable()
	if e != nil {
		return
	}
	if strings.HasSuffix(exe, ".test") || strings.HasSuffix(exe, ".test.exe") {
		return
	} // tests drain explicitly, never launch the test runner
	log, e := os.OpenFile(filepath.Join(o.Dir(), "upload.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return
	}
	defer log.Close()
	args := []string{"peers-native", "_hosted-flush"}
	if strings.TrimSuffix(filepath.Base(exe), ".exe") == "peers" {
		args = []string{"_hosted-flush"}
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = log, log
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// Record is the fail-open local send hook. It performs no network calls.
func Record(host, id, sender string, recipients []string, kind, chat, title, body string) {
	c, e := Load()
	if e != nil || !c.SignedIn() || !c.Messages {
		return
	}
	to := make([]string, len(recipients))
	for i, a := range recipients {
		to[i] = qualified(host, a)
	}
	e = c.QueueMessage(kitapi.PeerMessageIn{ID: id, Sender: qualified(host, sender), Recipients: to, Time: time.Now().UTC(), Kind: kind, ChatID: chat, Title: title, Body: body})
	if e != nil {
		c.WarnOnce(os.Stderr)
		return
	}
	c.Kick()
}
