package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

// traceStore is what the trace commands read and write through: the Layer
// client when a key is configured, the file archive when none is.
type traceStore interface {
	index.Archive
	ListSessionRows(topK int, filter any) ([]trace.SessionRow, error)
	ListBlockRows(sessionID string) ([]trace.BlockRow, error)
	SessionRows(prefix string) ([]layer.Row, error)
	SearchHits(query string, topK int, filter any) ([]layer.Hit, error)
	SearchPhrasings(phrasings []string, topK int, filter any) ([]layer.Hit, error)
	SummariesServed() error
}

// lexicalStore is a store whose search matches words only (no embeddings).
type lexicalStore interface{ Lexical() bool }

func isLexical(s any) bool {
	l, ok := s.(lexicalStore)
	return ok && l.Lexical()
}

// errNeedsKey is returned by commands that only exist on Layer.
var errNeedsKey = errors.New("this needs a Layer API key: no HEVLAYER_API_KEY or HEVLAYER_API_KEY_OP_REF is set and [layer].api_key is empty, so traces are kept in files on this host")

var retiredOnce sync.Once

// openTarget resolves the storage mode for the trace commands and says, once
// per process, when the config still names the retired Postgres lane.
func openTarget(namespace string, errOut io.Writer) (daemon.Target, error) {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return daemon.Target{}, err
	}
	t, err := daemon.OpenTarget(cfg, namespace)
	if err != nil {
		return daemon.Target{}, err
	}
	if t.Retired && errOut != nil {
		retiredOnce.Do(func() { fmt.Fprintf(errOut, "%s\n\n", daemon.RetiredMessage(cfg)) })
	}
	return t, nil
}

// traces opens the trace archive in whichever mode this host is in.
func traces(namespace string) (traceStore, error) {
	t, err := openTarget(namespace, os.Stderr)
	if err != nil {
		return nil, err
	}
	if t.Files != nil {
		return t.Files, nil
	}
	return t.Layer, nil
}

// layerOf is the Layer client behind a store, or nil for the file archive.
func layerOf(s traceStore) *layer.Client {
	c, _ := s.(*layer.Client)
	return c
}
