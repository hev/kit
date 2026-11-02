// Package peers implements terminal and dispatch peer messaging without a backend.
package peers

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"time"

	"github.com/hev/kit/internal/platformops"
)

type Message map[string]any

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (e *Engine) path(session string) string {
	return filepath.Join(e.State, "inbox", unsafeName.ReplaceAllString(session, "_")+".jsonl")
}
func (e *Engine) lock(session string) (func(), error) {
	p := e.path(session)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}
	return platformops.LockPath(p+".lock", false)
}
func rows(path string) ([]Message, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Message
	r := bufio.NewReader(f)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			var m Message
			if json.Unmarshal(b, &m) == nil && m != nil {
				out = append(out, m)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Preserve incomplete tails, and fsync before reporting delivery or receipts.
func appendRow(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() > 0 {
		last := make([]byte, 1)
		if _, err = f.ReadAt(last, st.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			if _, err = f.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }
func (e *Engine) view(session, identity string) ([]Message, error) {
	ms, err := rows(e.path(session))
	if err != nil {
		return nil, err
	}
	receipts, err := rows(e.path(session) + ".receipts.jsonl")
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		m["current"] = identity != "" && m["recipient_incarnation"] == identity
		m["consumed_at"], m["acknowledged_at"] = nil, nil
		m["transport_state"] = "queued"
		for _, r := range receipts {
			if r["id"] == m["id"] && r["recipient_incarnation"] == m["recipient_incarnation"] {
				switch r["kind"] {
				case "consumed":
					m["consumed_at"] = r["ts"]
				case "acknowledged":
					m["acknowledged_at"] = r["ts"]
					m["transport_state"] = "acknowledged"
				}
			}
		}
	}
	return ms, nil
}
func (e *Engine) hold(session, identity string, m Message, verify ...func() error) (Message, error) {
	if identity == "" {
		return nil, fmt.Errorf("cannot establish recipient incarnation")
	}
	unlock, err := e.lock(session)
	if err != nil {
		return nil, err
	}
	defer unlock()
	for _, check := range verify {
		if err := check(); err != nil {
			return nil, err
		}
	}
	if m["id"] == nil || m["id"] == "" {
		m["id"] = newID()
	}
	m["recipient_incarnation"], m["recipient_session"], m["transport_state"] = identity, session, "queued"
	ms, err := rows(e.path(session))
	if err != nil {
		return nil, err
	}
	for _, old := range ms {
		if old["id"] == m["id"] {
			for _, k := range []string{"recipient_incarnation", "from", "from_agent", "from_cwd", "message", "thread", "order", "run"} {
				if !reflect.DeepEqual(old[k], m[k]) {
					return nil, fmt.Errorf("message ID reused with different content or recipient")
				}
			}
			return old, nil
		}
	}
	m["ts"] = now()
	return m, appendRow(e.path(session), m)
}

// receipt validates the entire batch before appending a single receipt.
func (e *Engine) receipt(session, identity, kind string, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("%s requires message IDs", kind)
	}
	ms, err := e.view(session, identity)
	if err != nil {
		return err
	}
	byID := map[string]Message{}
	for _, m := range ms {
		if m["current"] == true {
			if id, ok := m["id"].(string); ok {
				byID[id] = m
			}
		}
	}
	for _, id := range ids {
		m, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown current message ID %s", id)
		}
		if kind == "acknowledged" && m["consumed_at"] == nil {
			return fmt.Errorf("ACK requires already consumed messages")
		}
	}
	seen := map[string]bool{}
	for _, id := range ids {
		field := "consumed_at"
		if kind == "acknowledged" {
			field = "acknowledged_at"
		}
		if seen[id] || byID[id][field] != nil {
			continue
		}
		seen[id] = true
		if err := appendRow(e.path(session)+".receipts.jsonl", Message{"id": id, "recipient_incarnation": identity, "kind": kind, "ts": now()}); err != nil {
			return err
		}
	}
	return nil
}
