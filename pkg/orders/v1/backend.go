// Package orders defines the versioned backend boundary for order operations.
package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hev/kit/internal/dispatch"
)

// Request contains API operations only. In particular it carries no launch
// environment, workspace, sender-selected capacity or forwarding authority.
type Request struct {
	Action      string          `json:"action"`
	ID          string          `json:"id,omitempty"`
	Host        string          `json:"host,omitempty"`
	Order       string          `json:"order,omitempty"`
	Trace       string          `json:"trace,omitempty"`
	Actor       string          `json:"actor,omitempty"`
	Note        string          `json:"note,omitempty"`
	Resolve     string          `json:"resolve,omitempty"`
	Level       string          `json:"level,omitempty"`
	HaltTimeout string          `json:"halt_timeout,omitempty"`
	Offset      int64           `json:"offset,omitempty"`
	Limit       int             `json:"limit,omitempty"`
	Round       int             `json:"round,omitempty"`
	Evaluation  json.RawMessage `json:"evaluation,omitempty"`
	IfMatch     string          `json:"if_match,omitempty"`
}

// Reply carries the projection of the dispatch record used by the API.
type Reply struct {
	Rows           []map[string]any `json:"rows,omitempty"`
	Order          map[string]any   `json:"order,omitempty"`
	Receipt        map[string]any   `json:"receipt,omitempty"`
	Log            string           `json:"log,omitempty"`
	Offset         int64            `json:"offset,omitempty"`
	Active         bool             `json:"active,omitempty"`
	EvaluationETag string           `json:"evaluation_etag,omitempty"`
}

type Backend interface {
	Execute(ctx context.Context, team string, request Request) (Reply, error)
}

type BackendFunc func(context.Context, string, Request) (Reply, error)

func (f BackendFunc) Execute(ctx context.Context, team string, request Request) (Reply, error) {
	return f(ctx, team, request)
}

// Route names one execution account provisioned for a single team. The SSH
// target must be an operator-trusted dispatch account, never a request value.
type Route struct {
	Host string
	SSH  string
}

// RPCBackend uses one operator-configured execution account per team. Each
// account must have its own dispatch root and admission policy.
type RPCBackend map[string]Route

func (b RPCBackend) Execute(ctx context.Context, team string, req Request) (Reply, error) {
	route, ok := b[team]
	if !ok || !dispatch.ValidLabel(route.Host) || (req.Host != "" && req.Host != route.Host) {
		return Reply{}, ErrForbidden
	}
	req.Host = route.Host
	for other, target := range b {
		if other != team && target.SSH == route.SSH {
			return Reply{}, ErrForbidden
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return execute(ctx, route, req)
}

var (
	ErrInvalid      = errors.New("invalid request")
	ErrForbidden    = errors.New("host or operation unavailable")
	ErrNotFound     = errors.New("order not found")
	ErrConflict     = errors.New("order state conflict")
	ErrRejected     = errors.New("order rejected by execution host")
	ErrPrecondition = errors.New("evaluation precondition failed")
	ErrUnavailable  = errors.New("execution host unavailable")
)
