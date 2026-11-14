package view

import "html/template"

var cors = CORS
var searchOpts = SearchOptions
var publicBase = PublicBase
var cleanRel = CleanRelativePath
var roleOf = RoleOf
var initial = Initial

// Assets contains the board's embedded styles, avatars and canvas helpers.
var Assets = ui

// Templates exposes the parsed local page templates.
func (s *Server) Templates() *template.Template { return s.tmpl }

// Patterns lists the registered HTTP routes.
func (s *Server) Patterns() []string { return append([]string(nil), s.patterns...) }

// WithReadOnly configures local read-only behavior before serving requests.
func (s *Server) WithReadOnly(value bool) *Server { s.readOnly = value; return s }
