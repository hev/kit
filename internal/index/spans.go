package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

// spanWriter is the archive side of RunTraceSpans.
type spanWriter interface {
	WriteTraceSpans([]trace.SpanRow) (layer.WriteResult, error)
}

// RunTraceSpans publishes the trace facts of this host's dispatch run records
// (root, normally dispatch.Root()) to the archive's spans namespace, so a
// dashboard on another host sharing the archive shows these runs in its
// traces. Ask text and order titles pass through the redaction scrubber.
// Rows whose content has not changed since the last publish are skipped.
func RunTraceSpans(root string, cl spanWriter, st *State, dryRun bool) (*Report, error) {
	scrubber, err := redact.Load()
	if err != nil {
		return nil, fmt.Errorf("load redaction: %w", err)
	}
	rep := &Report{Redactions: redact.Counts{}}
	scrub := func(s string) string {
		out, counts := scrubber.Text(s)
		rep.Redactions.Add(counts)
		return out
	}
	rows := dispatch.TraceSpanRows(dispatch.ReadRunRecords(root), layer.Hostname(), scrub)
	var changed []trace.SpanRow
	sigs := map[string]string{}
	for _, row := range rows {
		b, _ := json.Marshal(row)
		sum := sha256.Sum256(append(b, scrubber.Identity()...))
		sig := hex.EncodeToString(sum[:])
		key := "span:" + row.ID
		rep.UnitsSeen++
		if st.Units[key] == sig {
			rep.UnitsSkipped++
			continue
		}
		changed = append(changed, row)
		sigs[key] = sig
	}
	if dryRun || len(changed) == 0 {
		return rep, nil
	}
	for start := 0; start < len(changed); start += 200 {
		end := min(start+200, len(changed))
		res, err := cl.WriteTraceSpans(changed[start:end])
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("trace spans: %v", err))
			continue
		}
		rep.RowsUpserted += res.RowsUpserted
		for _, row := range changed[start:end] {
			st.Units["span:"+row.ID] = sigs["span:"+row.ID]
			rep.UnitsIndexed++
		}
	}
	return rep, nil
}
