// Package nativeinbox holds local hev kit presence and structured messages.
package nativeinbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hev/kit/internal/hevpaths"
	"github.com/hev/kit/internal/platformops"
)

const (
	Background      = "background"
	Interrupt       = "interrupt"
	MaxBytes        = 256 << 10
	MaxBodyBytes    = 16 << 10
	maxMessages     = 128
	refreshInterval = 5 * time.Second
)

var (
	// Names are generated locally. Restrict reads to this namespace as well.
	namePattern = regexp.MustCompile(`^kit-[1-9][0-9]{0,9}$`)
	errStore    = errors.New("native peer store unavailable")
	errFull     = errors.New("native peer inbox full")
)

type Presence struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Agent     string    `json:"agent"`
	Cwd       string    `json:"cwd"`
	Branch    string    `json:"branch"`
	Role      string    `json:"role"`
	State     string    `json:"state"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Birth     string    `json:"birth"`
	TmuxPane  string    `json:"tmux_pane,omitempty"`
}

type Message struct {
	ID   string    `json:"id"`
	TS   time.Time `json:"ts"`
	From string    `json:"from"`
	Body string    `json:"body"`
	Mode string    `json:"mode"`
}

type Store struct{ dir string }

// AtStateDir places the native inbox under the resolved peers state directory.
func AtStateDir(dir string) Store {
	if dir == "" {
		return Store{}
	}
	return Store{filepath.Join(dir, "native")}
}

// AtHome resolves the peers surface under home so legacy ~/.local/state/peers-v2
// hosts never get ~/.hev/peers/ created underneath them.
func AtHome(home string) Store {
	if home == "" {
		return Store{}
	}
	return AtStateDir(hevpaths.ResolveHome(home, hevpaths.Peers).Dir)
}

// Default resolves the active user's peers surface (honouring HEV_PEERS_STATE).
func Default() Store {
	return AtStateDir(hevpaths.Dir(hevpaths.Peers))
}

func name(pid int) string { return fmt.Sprintf("kit-%d", pid) }

func (s Store) presencePath(peer string) string { return filepath.Join(s.dir, peer+".json") }
func (s Store) inboxPath(peer string) string    { return filepath.Join(s.dir, peer+".inbox.jsonl") }

// One lock covers presence checks, replacement and draining. Keep the lock
// file in place so every process always locks the same inode.
func (s Store) lock(create bool) (func(), error) {
	if s.dir == "" {
		return nil, errStore
	}
	if create {
		if err := platformops.MkdirAll(s.dir, 0700); err != nil {
			return nil, errStore
		}
	}
	info, err := os.Lstat(s.dir)
	if os.IsNotExist(err) && !create {
		return nil, os.ErrNotExist
	}
	if err != nil || !info.IsDir() {
		return nil, errStore
	}
	for deadline := time.Now().Add(250 * time.Millisecond); ; {
		release, err := platformops.LockPath(filepath.Join(s.dir, ".lock"), true)
		if err == nil {
			return release, nil
		}
		if !platformops.IsLockBusy(err) || time.Now().After(deadline) {
			return nil, errStore
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readBounded(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, errStore
	}
	if !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, errStore
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errStore
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errStore
	}
	return data, nil
}

func remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errStore
	}
	return nil
}

func (s Store) record(peer string) (Presence, error) {
	var p Presence
	data, err := readBounded(s.presencePath(peer), MaxBytes)
	if err != nil {
		return p, err
	}
	if json.Unmarshal(data, &p) != nil || p.Name != peer || p.PID <= 0 || p.Name != name(p.PID) ||
		p.Agent != "hev" || p.Birth == "" || (p.State != "idle" && p.State != "working") {
		return Presence{}, errStore
	}
	return p, nil
}

func (s Store) live(peer string) (Presence, error) {
	p, err := s.record(peer)
	if err != nil {
		return p, err
	}
	if processBirth(p.PID) != p.Birth {
		// Under the same lock as registration, only this incarnation can be removed.
		if err := s.discard(peer); err != nil {
			return Presence{}, err
		}
		return Presence{}, os.ErrNotExist
	}
	return p, nil
}

func (s Store) discard(peer string) error {
	if err := remove(s.inboxPath(peer)); err != nil {
		return err
	}
	return remove(s.presencePath(peer))
}

func (s Store) Register(cwd, branch, pane string) (Presence, error) {
	release, err := s.lock(true)
	if err != nil {
		return Presence{}, err
	}
	defer release()
	pid := os.Getpid()
	now := time.Now().UTC()
	p := Presence{Name: name(pid), PID: pid, Agent: "hev", Cwd: cwd, Branch: branch,
		Role: "foh", State: "idle", StartedAt: now, UpdatedAt: now, Birth: processBirth(pid),
		TmuxPane: pane}
	if p.Birth == "" {
		return Presence{}, errStore
	}
	if old, err := s.live(p.Name); err == nil && old.Birth == p.Birth {
		return Presence{}, errors.New("native peer already registered")
	} else if err != nil && !os.IsNotExist(err) {
		return Presence{}, err
	}
	// Also discard any orphaned inbox from an interrupted previous write.
	if err := remove(s.inboxPath(p.Name)); err != nil {
		return Presence{}, err
	}
	return p, s.writePresence(p)
}

func (s Store) writePresence(p Presence) error {
	data, err := json.Marshal(p)
	if err != nil || len(data) > MaxBytes || platformops.WriteFileAtomic(s.presencePath(p.Name), data, 0600) != nil {
		return errStore
	}
	return nil
}

func (s Store) Update(owner Presence, working bool) error {
	release, err := s.lock(false)
	if err != nil {
		return err
	}
	defer release()
	p, err := s.owned(owner)
	if err != nil {
		return err
	}
	state := "idle"
	if working {
		state = "working"
	}
	if p.State == state && time.Since(p.UpdatedAt) < refreshInterval {
		return nil
	}
	p.State, p.UpdatedAt = state, time.Now().UTC()
	return s.writePresence(p)
}

func (s Store) owned(owner Presence) (Presence, error) {
	if owner.PID != os.Getpid() || owner.Name != name(owner.PID) {
		return Presence{}, errStore
	}
	p, err := s.record(owner.Name)
	if err != nil || !p.StartedAt.Equal(owner.StartedAt) || p.Birth != owner.Birth {
		return Presence{}, errStore
	}
	return p, nil
}

func (s Store) Close(owner Presence) error {
	release, err := s.lock(false)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.owned(owner); err != nil {
		return err
	}
	return s.discard(owner.Name)
}

func (s Store) List() ([]Presence, error) {
	release, err := s.lock(false)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer release()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, errStore
	}
	var rows []Presence
	for _, entry := range entries {
		peer, ok := strings.CutSuffix(entry.Name(), ".json")
		if ok && namePattern.MatchString(peer) {
			if p, err := s.live(peer); err == nil {
				rows = append(rows, p)
			}
		}
	}
	return rows, nil
}

func (s Store) Lookup(peer string) (Presence, error) {
	if !namePattern.MatchString(peer) {
		return Presence{}, os.ErrNotExist
	}
	release, err := s.lock(false)
	if err != nil {
		return Presence{}, err
	}
	defer release()
	return s.live(peer)
}

func (s Store) Send(recipient Presence, msg Message) error {
	if !namePattern.MatchString(recipient.Name) || !validMessage(msg) {
		return errors.New("invalid native peer message")
	}
	if len(msg.Body) > MaxBodyBytes {
		return errFull
	}
	release, err := s.lock(false)
	if err != nil {
		return err
	}
	defer release()
	p, err := s.live(recipient.Name)
	if err != nil || !p.StartedAt.Equal(recipient.StartedAt) || p.Birth != recipient.Birth {
		return errors.New("native peer is no longer available")
	}
	path := s.inboxPath(p.Name)
	data, err := readBounded(path, MaxBytes)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	msg.TS = time.Now().UTC()
	line, err := json.Marshal(msg)
	if err != nil {
		return errStore
	}
	if len(data)+len(line)+1 > MaxBytes || bytes.Count(data, []byte{'\n'}) >= maxMessages {
		return errFull
	}
	// Atomically append a complete JSONL record, without exposing partial writes to readers.
	if platformops.WriteFileAtomic(path, append(append(data, line...), '\n'), 0600) != nil {
		return errStore
	}
	return nil
}

func validMessage(msg Message) bool {
	return msg.ID != "" && msg.From != "" && strings.TrimSpace(msg.Body) != "" &&
		(msg.Mode == Background || msg.Mode == Interrupt)
}

func (s Store) Drain(owner Presence, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, nil
	}
	release, err := s.lock(false)
	if err != nil {
		return nil, err
	}
	defer release()
	if _, err := s.owned(owner); err != nil {
		return nil, err
	}
	path := s.inboxPath(owner.Name)
	data, err := readBounded(path, MaxBytes)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var messages []Message
	for len(data) > 0 && len(messages) < limit {
		line, rest, ok := bytes.Cut(data, []byte{'\n'})
		var msg Message
		if !ok || json.Unmarshal(line, &msg) != nil || !validMessage(msg) || msg.TS.IsZero() {
			return nil, errStore
		}
		messages = append(messages, msg)
		data = rest
	}
	if len(data) == 0 {
		err = remove(path)
	} else {
		err = platformops.WriteFileAtomic(path, data, 0600)
	}
	if err != nil {
		return nil, errStore
	}
	return messages, nil
}
