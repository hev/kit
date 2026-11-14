package kitapi

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// Peers API contract: all writes derive author_id from the authenticated key;
// no input can select an author. All routes can return 401 (revoked/invalid
// key), 403 (not permitted), 400 (invalid input), 429 and 5xx. Errors use
// {"error":"message"}. Oversized writes return 413. Success is 200.
// POST /v1/peers/messages additionally returns 402 billing_read_only; retain
// the entry for later. Presence is ephemeral, not stored content: it keeps
// working when the team is read-only. Reads also remain available.
const (
	PeersMaxBytes    = 1 << 20
	PeersMaxSessions = 1000
	PeersTTLSeconds  = 180
)

type PeerSession struct {
	Name         string     `json:"name"`
	Harness      string     `json:"harness"`
	Model        string     `json:"model,omitempty"`
	Project      string     `json:"project"` // base name only, never a local path
	Branch       string     `json:"branch"`
	State        string     `json:"state"`
	StartedAt    *time.Time `json:"started_at,omitempty"` // omitted when unknown
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}

// PresenceIn replaces this host's sessions atomically. ObservedAt is a client
// timestamp, also the beacon revision: repeated/older revisions do not refresh
// expiry or overwrite newer data. Send a fresh revision on each timer tick.
// Host must equal the key's registered stable host label (otherwise 403).
// Sessions is a complete snapshot, including [] when there are none.
type PresenceIn struct {
	Host       string        `json:"host"`
	Version    string        `json:"version"`
	OS         string        `json:"os"`
	ObservedAt time.Time     `json:"observed_at"`
	Sessions   []PeerSession `json:"sessions"`
}
type PresenceAck struct {
	TTLSeconds int       `json:"ttl_seconds"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type PeerMachine struct {
	PresenceIn
	AuthorID  string    `json:"author_id"`
	ExpiresAt time.Time `json:"expires_at"`
}
type PresencePage struct {
	Machines   []PeerMachine `json:"machines"` // sorted by host, live only
	TTLSeconds int           `json:"ttl_seconds"`
}

// PeerMessageIn is one sender-originated message, not one recipient delivery.
// ID is a client-supplied full UUID (32 hex or canonical hyphenated form),
// opaque to the server and unique per team. An identical repeated append is a
// no-op; the same key with different content returns 409, never overwrites.
// Direct sends reuse their existing full ID. Chats mint one hosted-only UUID
// when first queued, persist it in a separate team-bound outbox and reuse it
// on every retry; existing peer wires and state files are unchanged.
// Sender/Recipients are HOST/NAME addresses. Kind is direct or chat; chat_id
// and title are for chats only. Body is redacted on the sending machine.
// Stored bytes are len(json.Marshal(PeerMessageIn)) UTF-8 bytes, once per
// unique ID, excluding server author/ingestion/index metadata and retries.
type PeerMessageIn struct {
	ID         string    `json:"id"`
	Sender     string    `json:"sender"`
	Recipients []string  `json:"recipients"`
	Time       time.Time `json:"time"`
	Kind       string    `json:"kind"`
	ChatID     string    `json:"chat_id,omitempty"`
	Title      string    `json:"title,omitempty"`
	Body       string    `json:"body"`
}
type PeerMessage struct {
	PeerMessageIn
	AuthorID    string `json:"author_id"`
	StoredBytes int    `json:"stored_bytes"`
}
type PeerMessagesPage struct {
	Messages   []PeerMessage `json:"messages"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// PeerMessages query: machine matches sender or recipient host; session is
// an exact HOST/NAME sender or recipient; chat matches chat_id. Filters AND.
// limit defaults 50, range 1..200. Newest client time first, ties ID descending.
// cursor is opaque and scoped to team+filters, exclusive of the last row;
// invalid/cross-filter cursors return 400. New inserts ahead of a cursor do
// not shift subsequent pages. No next_cursor means end. Presence has no paging.
func (c *Client) PeerMessages(ctx context.Context, q url.Values) (PeerMessagesPage, error) {
	var out PeerMessagesPage
	err := c.Do(ctx, "GET", "/v1/peers/messages", q, nil, &out)
	return out, err
}
func (c *Client) PeerPresence(ctx context.Context) (PresencePage, error) {
	var out PresencePage
	err := c.Do(ctx, "GET", "/v1/peers/presence", nil, nil, &out)
	return out, err
}
func (c *Client) BeaconPeers(ctx context.Context, in PresenceIn) (PresenceAck, error) {
	var out PresenceAck
	err := c.Do(ctx, "POST", "/v1/peers/presence", nil, in, &out)
	return out, err
}
func (c *Client) AppendPeerMessage(ctx context.Context, in PeerMessageIn) (PeerMessage, error) {
	var out PeerMessage
	err := c.Do(ctx, "POST", "/v1/peers/messages", nil, in, &out)
	return out, err
}
func PeerStoredBytes(in PeerMessageIn) int { b, _ := json.Marshal(in); return len(b) }
