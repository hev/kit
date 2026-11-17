package filearchive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Evaluations (`hev eval put`) live in evals.jsonl, one row per (session,
// ts) in the shape Layer stores. A write is conditional the way Layer's is:
// a row whose id is already there is left alone, so a repeated put changes
// nothing.

func (s *Store) evalsPath() string { return filepath.Join(s.dir, "evals.jsonl") }

// WriteEvals stores evaluations that are not stored yet.
func (s *Store) WriteEvals(evals []trace.Eval) (layer.WriteResult, error) {
	rows, err := layer.EvalRows(evals)
	if err != nil || len(rows) == 0 {
		return layer.WriteResult{}, err
	}
	added := 0
	err = s.locked(func() error {
		prev, err := readFile(s.evalsPath())
		if err != nil {
			return err
		}
		lines, err := splitLines(prev)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, line := range lines {
			have[idOf(line)] = true
		}
		buf := bytes.NewBuffer(append([]byte(nil), prev...))
		if buf.Len() > 0 && !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
			buf.WriteByte('\n')
		}
		for _, row := range rows {
			id, _ := row["id"].(string)
			if have[id] {
				continue
			}
			b, err := json.Marshal(row)
			if err != nil {
				return err
			}
			buf.Write(b)
			buf.WriteByte('\n')
			have[id] = true
			added++
		}
		if added == 0 {
			return nil
		}
		return s.casWrite(s.evalsPath(), prev, buf.Bytes())
	})
	return layer.WriteResult{RowsUpserted: added}, err
}

// ListEvalRows reads the evaluations that match filter, by id.
func (s *Store) ListEvalRows(filter any) ([]layer.EvalRow, error) {
	return s.listEvals(filter, false)
}

// ListEvalMarks is ListEvalRows without the evaluator's prose.
func (s *Store) ListEvalMarks(filter any) ([]layer.EvalRow, error) {
	return s.listEvals(filter, true)
}

func (s *Store) listEvals(filter any, marksOnly bool) ([]layer.EvalRow, error) {
	match, err := compileFilter(filter)
	if err != nil {
		return nil, err
	}
	var lines [][]byte
	if err := s.lockedRead(func() (err error) { lines, err = readLines(s.evalsPath()); return err }); err != nil {
		return nil, err
	}
	rows := []layer.EvalRow{}
	for _, line := range lines {
		if filter != nil {
			m, err := decodeRow(line)
			if err != nil {
				return nil, fmt.Errorf("archive eval: %w", err)
			}
			if !match(m) {
				continue
			}
		}
		var r layer.EvalRow
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("archive eval: %w", err)
		}
		if marksOnly {
			r.Text, r.Summary, r.Evidence, r.Findings = "", "", "{}", "[]"
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// deleteEvals removes a session's evaluations; the caller holds the lock.
func (s *Store) deleteEvals(sessionID string) error {
	prev, err := readFile(s.evalsPath())
	if err != nil || prev == nil {
		return err
	}
	lines, err := splitLines(prev)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, line := range lines {
		var r struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(line, &r) == nil && r.SessionID == sessionID {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if bytes.Equal(buf.Bytes(), prev) {
		return nil
	}
	return s.casWrite(s.evalsPath(), prev, buf.Bytes())
}
