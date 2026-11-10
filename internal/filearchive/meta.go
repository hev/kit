package filearchive

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/platformops"
)

// meta.json records the redaction policy the archive was started under. An
// archive that has held only rows scrubbed by the current policy has no
// history for the redaction migration to rebuild.
type meta struct {
	Version   int    `json:"version"`
	Redaction string `json:"redaction,omitempty"`
}

func (s *Store) metaPath() string { return filepath.Join(s.dir, "meta.json") }

// RedactionBaseline returns the redaction policy every row in this archive
// was written under. On an archive that holds nothing yet it records policy
// first. "" means the archive predates the record and its history is unknown.
func (s *Store) RedactionBaseline(policy string) (string, error) {
	var out string
	err := s.locked(func() error {
		var m meta
		b, err := os.ReadFile(s.metaPath())
		if err == nil {
			if err := json.Unmarshal(b, &m); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
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
		m.Version, m.Redaction = 1, policy
		raw, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if err := platformops.WriteFileAtomic(s.metaPath(), raw, 0o600); err != nil {
			return err
		}
		out = policy
		return nil
	})
	return out, err
}
