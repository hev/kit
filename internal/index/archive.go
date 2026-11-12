package index

import (
	"context"
	"reflect"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Archive is the storage surface the indexer writes through. *layer.Client
// is the Layer implementation (unchanged wire, unchanged rows);
// filearchive.Store keeps the same rows in files on a host with no key.
type Archive interface {
	ChunkRunes() int
	Write(rows []layer.Row) (layer.WriteResult, error)
	WriteBlocks(rows []trace.BlockRow) (layer.WriteResult, error)
	WriteSessions(rows []trace.SessionRow) (layer.WriteResult, error)
	PatchSessionSummaries(rows []trace.SessionRow) (layer.WriteResult, error)
	ArchiveIdentityRows(suffix string) ([]layer.Row, error)
	DeleteSessionArchive(id string) error
	InstructionVersions(path, host string) ([]layer.InstructionVersion, error)
	WriteInstructionVersions(rows []layer.InstructionVersion) error
	OutcomePage(ctx context.Context, after string, since int64, limit int) ([]trace.SessionRow, error)
	PatchOutcomes(ctx context.Context, row trace.SessionRow) error
}

var _ Archive = (*layer.Client)(nil)

// absent reports a missing archive, including a nil *layer.Client held in
// the interface (a dry run has no client).
func absent(cl Archive) bool {
	if cl == nil {
		return true
	}
	v := reflect.ValueOf(cl)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
