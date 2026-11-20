package filearchive

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Documents are chunk rows written by something other than the trace
// indexer: `hev collect` keeps an app's turn documents this way when there is
// no Layer key. They live where chunks do (chunks/<shard>/<key>.jsonl, one
// file per sessionID), are upserted by id under the archive lock like any
// chunk, and are found by the same search (SearchQuery, PhraseHits) because
// they carry "id", "text" and "ts" as chunks do. Every other attribute the
// caller sets is stored as written, so a filter can name it and Docs returns
// it; layer.Hit, which search returns, carries only the chunk attributes.

// WriteDocs upserts docs into sessionID's chunk file. Each doc needs a
// string "id"; "session_id" is set to sessionID so a search filtered on it
// reads only this file. Redaction is the caller's job, before this call.
func (s *Store) WriteDocs(sessionID string, docs []map[string]any) error {
	if sessionID == "" {
		return fmt.Errorf("documents need a session id")
	}
	if len(docs) == 0 {
		return nil
	}
	group := make(map[string][]byte, len(docs))
	for _, d := range docs {
		id, _ := d["id"].(string)
		if id == "" {
			return fmt.Errorf("document without id")
		}
		row := make(map[string]any, len(d)+1)
		for k, v := range d {
			row[k] = v
		}
		row["session_id"] = sessionID
		b, err := json.Marshal(row)
		if err != nil {
			return err
		}
		group[id] = b
	}
	return s.locked(func() error {
		_, err := s.upsertLines(s.path("chunks", keyOf(sessionID)), group)
		return err
	})
}

// Docs returns every row in sessionID's chunk file with all its attributes,
// in id order. A missing file (or a lazy store with no directory) is empty.
func (s *Store) Docs(sessionID string) ([]map[string]any, error) {
	if sessionID == "" {
		return nil, nil
	}
	var out []map[string]any
	err := s.lockedRead(func() error {
		lines, err := readLines(s.path("chunks", keyOf(sessionID)))
		if err != nil {
			return err
		}
		for _, line := range lines {
			row, err := decodeRow(line)
			if err != nil {
				return fmt.Errorf("archive document: %w", err)
			}
			if sid, _ := row["session_id"].(string); sid != sessionID {
				continue // a hashed key's file holds only its id; be sure
			}
			out = append(out, row)
		}
		return nil
	})
	return out, err
}

// NamespaceDir is <root>/<namespace>, checked as Open checks a namespace
// name, for a store that keeps its own files beside the archive's
// namespaces (hev collect's app boards). It creates nothing.
func NamespaceDir(root, namespace string) (string, error) {
	if !safeNamespace.MatchString(namespace) {
		return "", fmt.Errorf("archive namespace %q must be letters, digits, '.', '_' or '-'", namespace)
	}
	if root == "" {
		root = DefaultRoot()
	}
	return filepath.Abs(filepath.Join(root, namespace))
}
