package dispatch

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RunRecord is one run directory as the trace read model sees it: its
// current metadata, attempt history, every attempt's result and its
// validation records. Records written before tracing read the same way and
// simply carry no trace fields.
type RunRecord struct {
	Name    string
	Meta    Meta
	History History
	Results []Result
	// Reports holds each result's worker report (report.md, or the
	// report.md.N archived with it), parallel to Results; "" when none.
	Reports     []string
	Validations []ValidationRecord
}

// MaxTraceReport caps a report read into the trace model.
const MaxTraceReport = 32 << 10

// runResultsWithReports reads every result of a run with the worker report
// archived beside it. An attempt's result.json and report.md are renamed with
// separate clock reads, so an archived report pairs with the archived result
// whose stamp is nearest, within a few seconds.
func runResultsWithReports(dir string) ([]Result, []string) {
	stamp := func(p, base string) (int64, bool) {
		n, err := strconv.ParseInt(strings.TrimPrefix(filepath.Base(p), base+"."), 10, 64)
		return n, err == nil
	}
	reports := map[int64]string{}
	paths, _ := filepath.Glob(filepath.Join(dir, "report.md.*"))
	for _, p := range paths {
		if n, ok := stamp(p, "report.md"); ok {
			reports[n] = p
		}
	}
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		if len(b) > MaxTraceReport {
			return string(b[:MaxTraceReport]) + "\n\n… (report truncated)"
		}
		return string(b)
	}
	var results []Result
	var texts []string
	resultPaths, _ := filepath.Glob(filepath.Join(dir, "result.json.*"))
	for _, p := range resultPaths {
		n, ok := stamp(p, "result.json")
		if !ok {
			continue
		}
		var r Result
		if readJSON(p, &r) != nil {
			continue
		}
		best, bestGap := int64(0), int64(5e9)
		for k := range reports {
			gap := max(k-n, n-k)
			if gap < bestGap {
				best, bestGap = k, gap
			}
		}
		text := ""
		if p, ok := reports[best]; ok && bestGap < 5e9 {
			text = read(p)
			delete(reports, best)
		}
		results, texts = append(results, r), append(texts, text)
	}
	var r Result
	if readJSON(filepath.Join(dir, "result.json"), &r) == nil {
		results, texts = append(results, r), append(texts, read(filepath.Join(dir, "report.md")))
	}
	return results, texts
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
		rec := RunRecord{Name: d.Name(), Meta: m, Validations: validationRecords(dir)}
		rec.Results, rec.Reports = runResultsWithReports(dir)
		_ = readJSON(filepath.Join(dir, "history.json"), &rec.History)
		if len(rec.History.Attempts) == 0 {
			rec.History.Attempts = []Attempt{{Number: 1, Revision: 1, Meta: m}}
		}
		idx := make([]int, len(rec.Results))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(i, j int) bool { return rec.Results[idx[i]].Start.Before(rec.Results[idx[j]].Start) })
		results, reports := make([]Result, len(idx)), make([]string, len(idx))
		for i, k := range idx {
			results[i], reports[i] = rec.Results[k], rec.Reports[k]
		}
		rec.Results, rec.Reports = results, reports
		out = append(out, rec)
	}
	return out
}
