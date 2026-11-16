package filearchive

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
)

// meta.json names the archive and records the redaction policy it was
// started under.
//
// ID is random and made when the directory is: an archive deleted and
// started again is a different archive, so the indexer's unchanged-unit
// cache (scoped by ArchiveID) does not skip transcripts it no longer holds.
//
// Redaction is the policy every row was written under. An archive that has
// held only rows scrubbed by the current policy has no history for the
// redaction migration to rebuild.
type meta struct {
	Version   int    `json:"version"`
	ID        string `json:"id,omitempty"`
	Redaction string `json:"redaction,omitempty"`
}

func (s *Store) metaPath() string { return filepath.Join(s.dir, "meta.json") }

// readMeta reads meta.json; the caller holds the lock.
func (s *Store) readMeta() (meta, error) {
	var m meta
	b, err := os.ReadFile(s.metaPath())
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

func (s *Store) writeMeta(m meta) error {
	m.Version = 1
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return platformops.WriteFileAtomic(s.metaPath(), raw, 0o600)
}

// ensureID gives the archive its identity the first time it is opened.
func (s *Store) ensureID() error {
	return s.withLock(func() error {
		m, err := s.readMeta()
		if err != nil {
			return err
		}
		if m.ID == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			m.ID = hex.EncodeToString(b)
			if err := s.writeMeta(m); err != nil {
				return err
			}
		}
		s.id = m.ID
		return nil
	})
}

// RedactionBaseline returns the redaction policy every row in this archive
// was written under. On an archive that holds nothing yet it records policy
// first. "" means the archive's history is unknown.
func (s *Store) RedactionBaseline(policy string) (string, error) {
	var out string
	err := s.locked(func() error {
		m, err := s.readMeta()
		if err != nil {
			return err
		}
		if m.Redaction != "" {
			out = m.Redaction
			return nil
		}
		for _, kind := range []string{"chunks", "blocks", "sessions"} {
			keys, err := s.keys(kind)
			if err != nil {
				return err
			}
			if len(keys) > 0 {
				return nil
			}
		}
		m.Redaction = policy
		if err := s.writeMeta(m); err != nil {
			return err
		}
		out = policy
		return nil
	})
	return out, err
}
