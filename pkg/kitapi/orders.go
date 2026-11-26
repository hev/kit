package kitapi

import (
	"context"
	"net/http"
	"time"
)

// MirroredOrder is the metadata-only snapshot of a dispatch executed outside
// the hosted Orders API.
type MirroredOrder struct {
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Repo      string    `json:"repo,omitempty"`
	Base      string    `json:"base,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Harness   string    `json:"harness,omitempty"`
	State     string    `json:"state"`
	Title     string    `json:"title,omitempty"`
	Decision  string    `json:"decision,omitempty"`
	Error     string    `json:"error,omitempty"`
	PRURL     string    `json:"pr_url,omitempty"`
	TraceID   string    `json:"trace_id,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MirrorOrder sends one idempotent update, without retries inside its budget.
func (c *Client) MirrorOrder(ctx context.Context, order MirroredOrder) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := *c
	client.Retries = 0
	return client.Do(ctx, http.MethodPut, "/v1/orders/mirror", nil, order, nil)
}
