package filearchive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
)

// index.jsonl lists every session, one line each, so `hev traces ls` and the
// dashboard's newest-first views read one file and then only the sessions
// they show. Every session write and delete updates it under the archive
// lock with a conditional write. It is derived: a listing that does not name
// exactly the session files on disk (a crash between the two writes, an
// archive from a 0.7 preview, a file removed by hand) is rebuilt from them.

type listEntry struct {
	Key       string `json:"key"`
	ID        string `json:"id"`
	SessionID string `json:"session_id,omitempty"`
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	Harness   string `json:"harness,omitempty"`
	Workdir   string `json:"workdir,omitempty"`
}

func (s *Store) listingPath() string { return filepath.Join(s.dir, "index.jsonl") }

func entryOf(key string, obj map[string]json.RawMessage) *listEntry {
	b, _ := json.Marshal(obj)
	e := &listEntry{}
	_ = json.Unmarshal(b, e)
	e.Key = key
	return e
}

func entryFromBytes(key string, b []byte) (*listEntry, error) {
	e := &listEntry{}
	if err := json.Unmarshal(b, e); err != nil {
		return nil, fmt.Errorf("archive session %s: %w", key, err)
	}
	e.Key = key
	return e, nil
}

// loadListing reads index.jsonl; the caller holds the lock.
func (s *Store) loadListing() (map[string]*listEntry, []byte, error) {
	prev, err := readFile(s.listingPath())
	if err != nil {
		return nil, nil, err
	}
	lines, err := splitLines(prev)
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string]*listEntry, len(lines))
	for _, line := range lines {
		e := &listEntry{}
		if json.Unmarshal(line, e) != nil || e.Key == "" {
			continue // rebuilt below if this loses a session
		}
		out[e.Key] = e
	}
	return out, prev, nil
}

func (s *Store) saveListing(entries map[string]*listEntry, prev []byte) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		b, err := json.Marshal(entries[k])
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if bytes.Equal(buf.Bytes(), prev) {
		return nil
	}
	return s.casWrite(s.listingPath(), prev, buf.Bytes())
}

// relist applies changes to the listing (a nil entry removes the key); the
// caller holds the lock.
func (s *Store) relist(changes map[string]*listEntry) error {
	if len(changes) == 0 {
		return nil
	}
	entries, prev, err := s.loadListing()
	if err != nil {
		return err
	}
	for k, e := range changes {
		if e == nil {
			delete(entries, k)
		} else {
			entries[k] = e
		}
	}
	return s.saveListing(entries, prev)
}

// listing returns the listing, rebuilt first when it does not name exactly
// the session files on disk; the caller holds the lock.
func (s *Store) listing() (map[string]*listEntry, error) {
	keys, err := s.keys("sessions")
	if err != nil {
		return nil, err
	}
	entries, prev, err := s.loadListing()
	if err != nil {
		return nil, err
	}
	fresh := len(entries) == len(keys)
	for i := 0; fresh && i < len(keys); i++ {
		fresh = entries[keys[i]] != nil
	}
	if fresh {
		return entries, nil
	}
	fileKeys, objects, err := s.sessionFiles()
	if err != nil {
		return nil, err
	}
	entries = make(map[string]*listEntry, len(objects))
	for i, b := range objects {
		e, err := entryFromBytes(fileKeys[i], b)
		if err != nil {
			return nil, err
		}
		entries[fileKeys[i]] = e
	}
	return entries, s.saveListing(entries, prev)
}

// newestSessions reads the files of the n sessions that started last, in
// the order ListSessionRows sorts them.
func (s *Store) newestSessions(n int) ([][]byte, error) {
	entries, err := s.listing()
	if err != nil {
		return nil, err
	}
	list := make([]*listEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Start != list[j].Start {
			return list[i].Start > list[j].Start
		}
		return list[i].ID < list[j].ID
	})
	var out [][]byte
	for _, e := range list {
		if len(out) >= n {
			break
		}
		b, err := readFile(s.path("sessions", e.Key))
		if err != nil {
			return nil, err
		}
		if b != nil {
			out = append(out, b)
		}
	}
	return out, nil
}
