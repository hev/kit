package kitcapture

import (
	"context"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Archive is the capture target of a signed-in machine: the local file
// archive, with every change also queued for the kit API. Reads are the
// local archive's.
type Archive struct {
	*filearchive.Store
	Outbox *Outbox
	// AuthorID and AuthorName label session rows, as on a Layer client;
	// the kit API overwrites them from the key.
	AuthorID, AuthorName string
}

// New wraps the local archive files with outbox.
func New(files *filearchive.Store, outbox *Outbox) *Archive {
	return &Archive{Store: files, Outbox: outbox}
}

// Write stores chunks locally and queues them.
func (a *Archive) Write(rows []layer.Row) (layer.WriteResult, error) {
	res, err := a.Store.Write(rows)
	if err != nil {
		return res, err
	}
	return res, EnqueueRows(a.Outbox, KindChunks, rows)
}

// WriteBlocks stores blocks locally and queues them.
func (a *Archive) WriteBlocks(rows []trace.BlockRow) (layer.WriteResult, error) {
	res, err := a.Store.WriteBlocks(rows)
	if err != nil {
		return res, err
	}
	return res, EnqueueRows(a.Outbox, KindBlocks, rows)
}

// WriteSessions stores session rows locally, then queues the rows as the
// local archive now holds them: the archive merges a write into what it
// had (outcomes, summaries), and the API, which upserts whole rows, must
// get the merged row.
func (a *Archive) WriteSessions(rows []trace.SessionRow) (layer.WriteResult, error) {
	if a.AuthorID != "" {
		rows = append([]trace.SessionRow(nil), rows...)
		for i := range rows {
			rows[i].AuthorID, rows[i].AuthorName = a.AuthorID, a.AuthorName
		}
	}
	res, err := a.Store.WriteSessions(rows)
	if err != nil {
		return res, err
	}
	return res, a.queueSessions(idsOf(rows))
}

// PatchSessionSummaries patches locally and queues the patched rows.
func (a *Archive) PatchSessionSummaries(rows []trace.SessionRow) (layer.WriteResult, error) {
	res, err := a.Store.PatchSessionSummaries(rows)
	if err != nil || res.RowsUpserted == 0 {
		return res, err
	}
	return res, a.queueSessions(idsOf(rows))
}

// DeleteSessionArchive deletes locally and queues the delete.
func (a *Archive) DeleteSessionArchive(id string) error {
	if err := a.Store.DeleteSessionArchive(id); err != nil {
		return err
	}
	return a.Outbox.EnqueueDelete(id)
}

func idsOf(rows []trace.SessionRow) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		id := r.ID
		if id == "" {
			id = r.SessionID
		}
		ids = append(ids, id)
	}
	return ids
}

// queueSessions queues the stored rows for ids.
func (a *Archive) queueSessions(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	stored, err := a.Store.ListSessionRows(-1, []any{"id", "In", ids})
	if err != nil {
		return err
	}
	return EnqueueRows(a.Outbox, KindSessions, stored)
}

// PatchOutcomes patches locally and queues the patched row.
func (a *Archive) PatchOutcomes(ctx context.Context, row trace.SessionRow) error {
	if err := a.Store.PatchOutcomes(ctx, row); err != nil {
		return err
	}
	return a.queueSessions(idsOf([]trace.SessionRow{row}))
}
