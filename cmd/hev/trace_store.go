package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hev/kit/internal/archive"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/workspace"
)

// traceStore is what the trace commands read and write through: the Layer
// client when a key is configured, the file archive when none is.
type traceStore = archive.Store

func isLexical(s any) bool { return archive.Lexical(s) }

// errNeedsKey is returned by commands that only exist on Layer.
var errNeedsKey = errors.New("this needs a Layer API key: no HEVLAYER_API_KEY or HEVLAYER_API_KEY_REF is set and [layer].api_key is empty, so traces are kept in files on this host")

var retiredOnce sync.Once

// openTarget resolves the storage mode for the trace commands (creating no
// file archive until something is written) and says, once per process, when the config still names the retired local Docker lane.
func openTarget(namespace string, errOut io.Writer) (daemon.Target, error) {
	w, err := currentWorkspace()
	if err != nil {
		return daemon.Target{}, err
	}
	if w.IsolatedStorage() {
		return daemon.OpenWorkspaceTarget(w, namespace)
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return daemon.Target{}, err
	}
	if namespace == "" {
		namespace = cfg.LayerNamespace
	}
	definitions, err := workspace.Read(workspace.ConfigPath())
	if err != nil {
		return daemon.Target{}, err
	}
	if err := definitions.CheckNamespace(w.Name, namespace); err != nil {
		return daemon.Target{}, err
	}
	t, err := daemon.OpenTargetRead(cfg, namespace)
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
	s, _, err := tracesWithImages(namespace)
	return s, err
}

// tracesWithImages is traces plus where this host keeps transcript images
// (<archive root>/<namespace>/blobs, in either mode).
func tracesWithImages(namespace string) (traceStore, *trace.ImageCapture, error) {
	t, err := openTarget(namespace, os.Stderr)
	if err != nil {
		return nil, nil, err
	}
	return t.Store(), t.Images, nil
}

// layerOf is the Layer client behind a store, or nil for the file archive.
func layerOf(s traceStore) *layer.Client {
	c, _ := s.(*layer.Client)
	return c
}
