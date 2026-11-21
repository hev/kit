// Package peers defines the versioned backend boundary for peer operations.
package peers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hev/kit/internal/dispatch"
	peerstore "github.com/hev/kit/internal/peers"
)

// Request carries the verified caller's host and the requested peer operation.
// The caller host comes from authentication, not from the JSON payload.
type Request struct {
	Action     string          `json:"action"`
	ID         string          `json:"id,omitempty"`
	Box        string          `json:"box,omitempty"`
	CallerKey  string          `json:"caller_key,omitempty"`
	CallerHost string          `json:"caller_host,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

type Reply struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type Backend interface {
	Execute(ctx context.Context, team string, request Request) (Reply, error)
}

type BackendFunc func(context.Context, string, Request) (Reply, error)

func (f BackendFunc) Execute(ctx context.Context, team string, request Request) (Reply, error) {
	return f(ctx, team, request)
}

type Route struct {
	Host string
	SSH  string
}

// RPCBackend maps authenticated teams to operator-provisioned, isolated
// peers stores. Local and SSH operations use the same inbox machinery.
type RPCBackend map[string]Route

func (b RPCBackend) Execute(ctx context.Context, team string, req Request) (Reply, error) {
	route, ok := b[team]
	if !ok || !dispatch.ValidLabel(route.Host) {
		return Reply{}, ErrForbidden
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	payload, err := json.Marshal(req)
	if err != nil {
		return Reply{}, err
	}
	var out dispatch.Response
	if route.SSH == "" {
		out = peerstore.HandleAPIRPC(ctx, string(payload))
	} else {
		out, err = dispatch.RPC(ctx, dispatch.Host{SSH: route.SSH}, dispatch.Request{Action: "peers-api", Note: string(payload)})
	}
	if err != nil || out.Error != "" {
		return Reply{}, ErrUnavailable
	}
	var reply Reply
	if err := json.Unmarshal([]byte(out.Log), &reply); err != nil {
		return Reply{}, ErrUnavailable
	}
	return reply, nil
}

var (
	ErrForbidden   = errors.New("host or operation unavailable")
	ErrUnavailable = errors.New("execution host unavailable")
)
