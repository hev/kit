package index

import (
	"fmt"
	"sort"

	"github.com/hev/kit/internal/trace"
)

// ReadSide derives the session and block rows a source would index, with the
// same dispatch attribution (order, issue, sender and trace) Run writes, but
// keeps them in memory. It serves fixtures and local previews; nothing is
// written to Layer and no git enrichment runs.
func ReadSide(src trace.Source, host string) ([]trace.SessionRow, []trace.BlockRow, error) {
	units, err := src.Units()
	if err != nil {
		return nil, nil, fmt.Errorf("enumerate units: %w", err)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Key < units[j].Key })
	var sessions []trace.SessionRow
	var blocks []trace.BlockRow
	for _, u := range units {
		turns, err := src.Read(u)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", u.Key, err)
		}
		rows, b := ReadSideRows(turns, host)
		sessions = append(sessions, rows...)
		blocks = append(blocks, b...)
	}
	return sessions, blocks, nil
}

// ReadSideRows derives one unit's session and block rows the way Run
// does, without writing: dispatch attribution, context digest, trace and
// starting context (no git enrichment, no redaction).
func ReadSideRows(turns []trace.Turn, host string) ([]trace.SessionRow, []trace.BlockRow) {
	attribute := unitAttribution(nil)
	rows := trace.Sessions(turns, func(string) string { return "" }, host)
	for i := range rows {
		a := attribute(trace.Turn{SessionID: rows[i].SessionID, Workdir: rows[i].Workdir})
		rows[i].Order, rows[i].Issue, rows[i].Sender = a.order, a.issue, a.sender
		sessionContext(&rows[i])
		sessionTrace(&rows[i])
		sessionStartContext(&rows[i], "", nil)
	}
	return rows, trace.Blocks(turns)
}
