// Package outboxrelay projects a transactional outbox into an eventually
// consistent document store. It deliberately provides no credential discovery
// or background activation.
package outboxrelay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrUncertain = errors.New("document delivery requires reconciliation")
var ErrConflict = errors.New("document identity has different content")

// Document is immutable. ID derives from the destination-scoped idempotency key,
// never the reusable order label or a database sequence used as a global cursor.
type Document struct {
	ID      string          `json:"id"`
	Key     string          `json:"idempotency_key"`
	Order   string          `json:"order_id,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

func NewDocument(destination, key, order string, payload json.RawMessage) (Document, error) {
	// Canonicalize JSON so database JSONB formatting does not cause false conflicts.
	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return Document{}, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return Document{}, err
	}
	identity, _ := json.Marshal([]string{destination, key})
	sum := sha256.Sum256(identity)
	return Document{ID: hex.EncodeToString(sum[:]), Key: key, Order: order, Payload: canonical}, nil
}

// Layer must send one immutable document per call; batch atomicity is not
// assumed. Inspect returns a positive observation, or nil when absent/unknown.
// Absence is never permission to retry an uncertain write.
type Layer interface {
	Put(context.Context, Document) error
	Inspect(context.Context, string) (*Document, error)
}

type Delivery struct {
	OutboxID int64
	Document Document
	// Claimed means this call durably changed pending -> sending before any I/O.
	// Otherwise it is reconciliation only, including expired sends after restart.
	Claimed bool
}

type Store interface {
	Next(context.Context) (*Delivery, error)
	Complete(context.Context, Delivery) error
}

type Relay struct {
	Store Store
	Layer Layer
}

// Step processes at most one document. Any Put error is ambiguous; even a
// positive transport response lost before Complete must be inspected on restart.
func (r Relay) Step(ctx context.Context) (bool, error) {
	d, err := r.Store.Next(ctx)
	if err != nil || d == nil {
		return false, err
	}
	if d.Claimed {
		if err := r.Layer.Put(ctx, d.Document); err != nil {
			return true, fmt.Errorf("%w: %v", ErrUncertain, err)
		}
	} else {
		found, err := r.Layer.Inspect(ctx, d.Document.ID)
		if err != nil {
			return true, err
		}
		if found == nil {
			return true, ErrUncertain
		}
		want, _ := json.Marshal(d.Document)
		got, _ := json.Marshal(found)
		if string(want) != string(got) {
			return true, ErrConflict
		}
	}
	return true, r.Store.Complete(ctx, *d)
}
