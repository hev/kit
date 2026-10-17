package outboxrelay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Postgres uses the version-1 kitchen function surface. The caller supplies an
// already-open database and live lease. All users of this destination must share
// this lease; do not mix this consumer with unfenced claim_outbox callers.
type Postgres struct {
	DB                            *sql.DB
	Destination, Consumer, Holder string
	Fence                         int64
	TTL                           int
}

func (p Postgres) lease() string { return "outbox-relay/" + p.Destination }
func (p Postgres) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if p.Destination == "" || p.Consumer == "" || p.Holder == "" || p.TTL < 1 || p.TTL > 86400 {
		return errors.New("destination, consumer, holder and ttl 1..86400 required")
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT kitchen.api_version()").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported kitchen API %d", version)
	}
	if _, err = tx.ExecContext(ctx, "SELECT kitchen.check_fence($1,$2::uuid,$3)", p.lease(), p.Holder, p.Fence); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Next never filters pending rows by outbox_id > cursor: sequence allocation
// order is not commit order. Expired sends are inspected rather than reclaimed.
func (p Postgres) Next(ctx context.Context) (result *Delivery, err error) {
	err = p.transaction(ctx, func(tx *sql.Tx) error {
		var id sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT kitchen.claim_outbox($1,$2)", p.Destination, p.TTL).Scan(&id); err != nil {
			return err
		}
		claimed := id.Valid
		if !claimed {
			// Do not sweep other destinations. Version 1 has no destination-scoped sweep;
			// an expired sending row can be reconciled and finished directly instead.
			err := tx.QueryRowContext(ctx, `SELECT outbox_id FROM kitchen.outbox
    WHERE destination=$1 AND (state='uncertain' OR (state='sending' AND claim_expires_at<=clock_timestamp()))
    ORDER BY outbox_id LIMIT 1`, p.Destination).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		var key, order string
		var payload []byte
		if err := tx.QueryRowContext(ctx, `SELECT idempotency_key,coalesce(order_id,''),payload FROM kitchen.outbox WHERE outbox_id=$1`, id.Int64).Scan(&key, &order, &payload); err != nil {
			return err
		}
		doc, err := NewDocument(p.Destination, key, order, payload)
		if err != nil {
			return err
		}
		result = &Delivery{OutboxID: id.Int64, Document: doc, Claimed: claimed}
		return nil
	})
	// Never expose a claimed row if commit failed or its outcome is unknown.
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p Postgres) Complete(ctx context.Context, d Delivery) error {
	return p.transaction(ctx, func(tx *sql.Tx) error {
		var state, key, order string
		var payload []byte
		if err := tx.QueryRowContext(ctx, `SELECT state,idempotency_key,coalesce(order_id,''),payload FROM kitchen.outbox WHERE outbox_id=$1 AND destination=$2 FOR UPDATE`, d.OutboxID, p.Destination).Scan(&state, &key, &order, &payload); err != nil {
			return err
		}
		stored, err := NewDocument(p.Destination, key, order, payload)
		if err != nil {
			return err
		}
		a, _ := json.Marshal(stored)
		b, _ := json.Marshal(d.Document)
		if string(a) != string(b) {
			return ErrConflict
		}
		if state != "sending" && state != "uncertain" && state != "sent" {
			return errors.New("outbox was not claimed")
		}
		if _, err = tx.ExecContext(ctx, "SELECT kitchen.finish_outbox($1,$2)", d.OutboxID, d.Document.ID); err != nil {
			return err
		}
		// One stream per outbox row: safe under rollback gaps and late commits. This
		// cursor is a completion receipt, not a global high-water mark.
		stream := fmt.Sprintf("outbox/%s/%d", p.Destination, d.OutboxID)
		return p.advance(ctx, tx, stream, 0, 1)
	})
}

func (p Postgres) advance(ctx context.Context, tx *sql.Tx, stream string, expect, next int64) error {
	var ok bool
	if err := tx.QueryRowContext(ctx, "SELECT kitchen.advance_cursor($1,$2,$3,$4,$5,$6::uuid,$7)", p.Consumer, stream, expect, next, p.lease(), p.Holder, p.Fence).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		var position int64
		if err := tx.QueryRowContext(ctx, "SELECT position FROM kitchen.cursors WHERE consumer=$1 AND stream=$2", p.Consumer, stream).Scan(&position); err != nil {
			return err
		}
		if position != next {
			return errors.New("cursor compare-and-set failed")
		}
	}
	return nil
}

// ProjectEvent enqueues the next immutable event and advances its per-order
// cursor in the same transaction. Repeated calls also rebuild into a NEW
// destination/consumer without changing original delivery records.
func (p Postgres) ProjectEvent(ctx context.Context, order string) (projected bool, err error) {
	err = p.transaction(ctx, func(tx *sql.Tx) error {
		stream := "events/" + p.Destination + "/" + order
		var position int64
		if err := tx.QueryRowContext(ctx, "SELECT coalesce((SELECT position FROM kitchen.cursors WHERE consumer=$1 AND stream=$2),0)", p.Consumer, stream).Scan(&position); err != nil {
			return err
		}
		var seq int64
		var payload []byte
		err := tx.QueryRowContext(ctx, `SELECT seq,jsonb_build_object('version',1,'order_id',order_id,'seq',seq,'dedupe_key',dedupe_key,'kind',kind,'payload',payload,'payload_digest',payload_digest,'source_peer',source_peer,'at',at) FROM kitchen.order_events WHERE order_id=$1 AND seq>$2 ORDER BY seq LIMIT 1`, order, position).Scan(&seq, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if seq != position+1 {
			return errors.New("non-dense event sequence")
		}
		keyBytes, _ := json.Marshal([]any{"order-event-v1", order, seq})
		if _, err = tx.ExecContext(ctx, "SELECT kitchen.enqueue_outbox($1,$2,$3::jsonb,$4)", p.Destination, string(keyBytes), string(payload), order); err != nil {
			return err
		}
		if err = p.advance(ctx, tx, stream, position, seq); err != nil {
			return err
		}
		projected = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return projected, nil
}

// RebuildOutboxRow copies one retained source intent into a new destination.
// Enumerate source IDs from a consistent snapshot, then repeat enumeration to
// include later commits if the source is still active. No original row is reset.
func (p Postgres) RebuildOutboxRow(ctx context.Context, source string, id int64) error {
	if source == p.Destination {
		return errors.New("rebuild requires a new destination")
	}
	return p.transaction(ctx, func(tx *sql.Tx) error {
		var key string
		var order sql.NullString
		var payload []byte
		if err := tx.QueryRowContext(ctx, `SELECT idempotency_key,order_id,payload FROM kitchen.outbox WHERE destination=$1 AND outbox_id=$2`, source, id).Scan(&key, &order, &payload); err != nil {
			return err
		}
		var orderValue any
		if order.Valid {
			orderValue = order.String
		}
		_, err := tx.ExecContext(ctx, "SELECT kitchen.enqueue_outbox($1,$2,$3::jsonb,$4)", p.Destination, key, string(payload), orderValue)
		return err
	})
}
