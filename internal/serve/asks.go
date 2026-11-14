package serve

import (
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/asks"
	"net/http"
)

type Span = asks.Span
type TraceSummary = asks.TraceSummary
type AskFacet = asks.AskFacet
type TraceContext = asks.TraceContext

func (s *Server) WithDispatchRoot(root string) *Server { s.dispatchRoot = root; return s }
func (s *Server) asksView() *asks.Server {
	v := &asks.Server{JSON: writeJSON, Error: writeError, ListRows: s.listRows, LocalRecords: func() []dispatch.RunRecord {
		root := s.dispatchRoot
		if root == "" {
			root = dispatch.Root()
		}
		return dispatch.ReadRunRecords(root)
	}}
	if s.archiveAsks {
		v.LocalRecords = nil
	}
	if r, ok := s.reader.(asks.Reader); ok {
		v.Reader = r
	}
	return v
}
func (s *Server) asks(w http.ResponseWriter, r *http.Request)     { s.asksView().List(w, r) }
func (s *Server) askTrace(w http.ResponseWriter, r *http.Request) { s.asksView().Detail(w, r) }
func (s *Server) sessionTraceContext(row trace.SessionRow) *TraceContext {
	return s.asksView().SessionTraceContext(row)
}
