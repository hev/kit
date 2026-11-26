package kitapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// SyncProgress counts source outbox files, independent of HTTP coalescing.
type SyncProgress struct {
	Scanning        bool `json:"scanning"`
	PendingBatches  int  `json:"pending_batches"`
	TotalBatches    int  `json:"total_batches"`
	PendingSessions int  `json:"pending_sessions"`
}

func (p SyncProgress) setHeaders(h http.Header) {
	h.Set("X-Kit-Sync-Scanning", strconv.FormatBool(p.Scanning))
	h.Set("X-Kit-Sync-Pending-Batches", strconv.Itoa(p.PendingBatches))
	h.Set("X-Kit-Sync-Total-Batches", strconv.Itoa(p.TotalBatches))
	h.Set("X-Kit-Sync-Pending-Sessions", strconv.Itoa(p.PendingSessions))
}

// ReportSync sends a best-effort snapshot. Older servers may lack this route.
func (c *Client) ReportSync(ctx context.Context, p SyncProgress) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	body, _ := json.Marshal(p)
	once := *c
	once.Retries = 0
	_, _ = once.doRaw(ctx, "POST", "/v1/sync/status", nil, body, &p)
}

// PostSyncBatch sends trace rows with the queue snapshot. Rejected indices
// allow callers to retain the source files that contain refused rows.
func (c *Client) PostSyncBatch(ctx context.Context, kind string, body []byte, p SyncProgress) (int, []int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := c.doRaw(ctx, "POST", "/v1/traces/"+kind, nil, body, &p)
	if err != nil {
		return 0, nil, err
	}
	var a accepted
	if err := json.Unmarshal(raw, &a); err != nil {
		return 0, nil, &Error{Status: http.StatusBadGateway, Message: "invalid ingest response"}
	}
	rejected := make([]int, 0, len(a.Rejected))
	for _, row := range a.Rejected {
		rejected = append(rejected, row.Index)
	}
	return a.Accepted, rejected, nil
}
