// Package archive is the trace archive as kit's commands use it, whichever
// store holds it: *layer.Client (hev layer, with a key) or
// *filearchive.Store (files under ~/.hev/traces, with none). Callers take
// a Store, or the narrower Writer the indexer needs, and ask SearchCaps what
// search can promise; nothing outside the two implementations branches on
// which one it holds, except commands that only exist on Layer.
package archive

import (
	"context"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Writer is what capture writes through (hevd, `hev index`, the redaction
// migration, the outcome sweep).
type Writer interface {
	// ChunkRunes is the largest chunk the store takes.
	ChunkRunes() int
	Write(rows []layer.Row) (layer.WriteResult, error)
	WriteBlocks(rows []trace.BlockRow) (layer.WriteResult, error)
	WriteSessions(rows []trace.SessionRow) (layer.WriteResult, error)
	PatchSessionSummaries(rows []trace.SessionRow) (layer.WriteResult, error)
	// ArchiveIdentityRows lists ownership coordinates of chunks (""),
	// blocks ("-blocks") or sessions ("-sessions") for redaction.
	ArchiveIdentityRows(suffix string) ([]layer.Row, error)
	// DeleteSessionArchive removes everything stored for a session.
	DeleteSessionArchive(id string) error
	InstructionVersions(path, host string) ([]layer.InstructionVersion, error)
	WriteInstructionVersions(rows []layer.InstructionVersion) error
	OutcomePage(ctx context.Context, after string, since int64, limit int) ([]trace.SessionRow, error)
	PatchOutcomes(ctx context.Context, row trace.SessionRow) error
}

// Reader is the dashboard's minimum: ordered session listing and a
// session's blocks.
type Reader interface {
	ListSessionRows(topK int, filter any) ([]trace.SessionRow, error)
	ListBlockRows(sessionID string) ([]trace.BlockRow, error)
}

// Searcher is search over chunk text. On Layer a phrasing is matched by
// meaning and by words; on files by words only (see SearchCaps).
type Searcher interface {
	SearchHits(query string, topK int, filter any) ([]layer.Hit, error)
	// SearchPhrasings ORs phrasings of one question (--also).
	SearchPhrasings(phrasings []string, topK int, filter any) ([]layer.Hit, error)
	// PhraseHits finds contiguous text; truncated reports more than limit.
	PhraseHits(q string, limit int, filter any) ([]layer.Hit, bool, error)
	SearchCaps() layer.SearchCaps
}

// Evals is evaluation storage (`hev eval put`, grades in the dashboard).
type Evals interface {
	WriteEvals(evals []trace.Eval) (layer.WriteResult, error)
	ListEvalRows(filter any) ([]layer.EvalRow, error)
	ListEvalMarks(filter any) ([]layer.EvalRow, error)
	SearchEvals(q string, top int, filter any) ([]layer.Hit, error)
	PhraseEvals(q string, limit int, filter any) ([]layer.Hit, bool, error)
}

// Store is the whole archive.
type Store interface {
	Writer
	Reader
	Searcher
	Evals
	// SessionRows returns the chunks of every session whose id starts with
	// prefix, in transcript order (`hev traces show`).
	SessionRows(prefix string) ([]layer.Row, error)
	// SummariesServed is nil when the store takes summary patches.
	SummariesServed() error
}

// RegexSearcher is a store that can search by regular expression (files).
type RegexSearcher interface {
	SearchRegex(pattern string, topK int, filter any) ([]layer.Hit, error)
}

var _ Store = (*layer.Client)(nil)

// Lexical reports whether s matches words only (no semantic search).
func Lexical(s any) bool {
	c, ok := s.(interface{ SearchCaps() layer.SearchCaps })
	return ok && c.SearchCaps().Lexical
}
