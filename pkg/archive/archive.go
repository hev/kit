// Package archive is kit's trace archive for programs outside this module:
// the row types a capture daemon writes (sessions, blocks and chunks), and
// the two stores that hold them, a Layer namespace reached with the caller's
// own credentials and a file archive on local disk.
//
// The types are aliases of kit's internal ones, so a row read or written here
// is exactly the row hevd produces; nothing is copied or re-encoded.
package archive

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// Row types, as hevd writes them.
type (
	// SessionRow is one session's summary row (the "-sessions" namespace).
	SessionRow = trace.SessionRow
	// BlockRow is one transcript block (the "-blocks" namespace).
	BlockRow = trace.BlockRow
	// Row is one searchable transcript chunk (the base namespace).
	Row = layer.Row
	// Hit is a search result over chunks.
	Hit = layer.Hit
	// WriteResult is what a batch write cost.
	WriteResult = layer.WriteResult
)

// Batch limits of the kit API's capture routes. A client splits larger
// batches; a server answers 413 above them.
const (
	MaxBatchRows  = 500
	MaxBatchBytes = 4 << 20
)

// Writer is what capture writes to. Writes are upserts keyed by row ID, so
// sending a batch twice leaves one copy.
type Writer interface {
	Write(rows []Row) (WriteResult, error)
	WriteBlocks(rows []BlockRow) (WriteResult, error)
	WriteSessions(rows []SessionRow) (WriteResult, error)
}

// Reader is what the dashboard and search read. Both stores implement it,
// as well as SearchHits and PhraseHits.
type Reader interface {
	ListSessionRows(topK int, filter any) ([]SessionRow, error)
	ListBlockRows(sessionID string) ([]BlockRow, error)
	SearchHits(query string, topK int, filter any) ([]Hit, error)
	PhraseHits(q string, limit int, filter any) ([]Hit, bool, error)
}

// Store is a full trace archive: write, read, and delete one session's rows.
type Store interface {
	Writer
	Reader
	DeleteSessionArchive(id string) error
}

// LayerStore is a trace archive in a Layer namespace. Namespace is the base
// name; sessions and blocks live in "<namespace>-sessions" and
// "<namespace>-blocks".
type LayerStore = layer.Client

// LayerConfig is how a server reaches its own Layer namespace.
type LayerConfig struct {
	Endpoint  string
	Key       string
	Namespace string
	// Store names the store behind Endpoint (see layer capabilities); empty
	// is the hosted default.
	Store string
	// Model is the embedding model; empty is kit's default.
	Model string
	// HTTP, when set, replaces the client's own HTTP client. It must not
	// inject credentials of its own.
	HTTP *http.Client
}

// OpenLayer is a Layer-backed store with the caller's credentials. It reads
// no config file and no environment.
func OpenLayer(cfg LayerConfig) (*LayerStore, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Key) == "" || strings.TrimSpace(cfg.Namespace) == "" {
		return nil, errors.New("archive: endpoint, key and namespace are required")
	}
	c, err := layer.New(cfg.Endpoint, cfg.Key, cfg.Namespace, cfg.Model).WithStore(strings.ToLower(strings.TrimSpace(cfg.Store)))
	if err != nil {
		return nil, err
	}
	if cfg.HTTP != nil {
		c.HTTP = cfg.HTTP
	}
	return c, nil
}

// FileStore is a trace archive in files under root/<namespace>.
type FileStore = filearchive.Store

// OpenFiles opens (creating) a file archive. It is the same store a keyless
// hevd writes, and a good fake for tests.
func OpenFiles(root, namespace string) (*FileStore, error) {
	return filearchive.Open(root, namespace)
}

// DeleteSessionsBefore removes every session that ended before cutoff, with
// its blocks and chunks, and returns how many it removed. It reads session
// rows only, so a retention sweep never pages through transcript text.
func DeleteSessionsBefore(ctx context.Context, s Store, cutoff time.Time) (int, error) {
	rows, err := s.ListSessionRows(-1, nil)
	if err != nil {
		return 0, err
	}
	limit := cutoff.UnixMilli()
	n := 0
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		end := r.End
		if end == 0 {
			end = r.Start
		}
		if end == 0 || end >= limit {
			continue
		}
		if err := s.DeleteSessionArchive(r.SessionID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Hostname is the host label kit stamps on rows captured here.
func Hostname() string { return layer.Hostname() }

var _ Store = (*LayerStore)(nil)
var _ Store = (*FileStore)(nil)
