// Package server is the local board adapter.
package server

import (
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/pkg/board/view"
	"html/template"
	"net/http"
)

type Server struct {
	*view.Server
	readOnly bool
	patterns []string
	tmpl     *template.Template
}

func New(st *store.Store) (*Server, error) {
	s, err := view.New(st)
	if err != nil {
		return nil, err
	}
	return &Server{Server: s, readOnly: settings.Current.ReadOnly, patterns: s.Patterns(), tmpl: s.Templates()}, nil
}
func (s *Server) Handler() http.Handler { return s.Server.WithReadOnly(s.readOnly).Handler() }

type CanvasIn = view.CanvasIn
type CanvasOut = view.CanvasOut
type BlobLimits = view.BlobLimits

const MaxCanvas = view.MaxCanvas

var CanvasDir = view.CanvasDir
var BlobDir = view.BlobDir
var ui = view.Assets
var cors = view.CORS
var searchOpts = view.SearchOptions
var publicBase = view.PublicBase
var cleanRel = view.CleanRelativePath
var roleOf = view.RoleOf
var initial = view.Initial

type frame = view.Frame
