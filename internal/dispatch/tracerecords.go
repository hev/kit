package dispatch

import (
	"os"
	"path/filepath"
	"sort"
)

// RunRecord is one run directory as the trace read model sees it: its
// current metadata, attempt history, every attempt's result and its
// validation records. Records written before tracing read the same way and
// simply carry no trace fields.
type RunRecord struct {
	Name        string
	Meta        Meta
	History     History
	Results     []Result
	Validations []ValidationRecord
}

// ReadRunRecords reads every run under root (normally Root()). Unreadable
// or partial runs are skipped, never fatal: the dashboard shows what exists.
func ReadRunRecords(root string) []RunRecord {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []RunRecord
	for _, d := range dirs {
		if !d.IsDir() || !ValidName(d.Name()) {
			continue
		}
		dir := filepath.Join(root, d.Name())
		var m Meta
		if readJSON(filepath.Join(dir, "meta.json"), &m) != nil || m.Imported {
			continue
		}
		rec := RunRecord{Name: d.Name(), Meta: m, Results: runResults(dir), Validations: validationRecords(dir)}
		_ = readJSON(filepath.Join(dir, "history.json"), &rec.History)
		if len(rec.History.Attempts) == 0 {
			rec.History.Attempts = []Attempt{{Number: 1, Revision: 1, Meta: m}}
		}
		sort.SliceStable(rec.Results, func(i, j int) bool { return rec.Results[i].Start.Before(rec.Results[j].Start) })
		out = append(out, rec)
	}
	return out
}
