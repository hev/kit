package kitcapture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

const (
	drainWorkers  = 4
	batchEnvelope = len(`{"rows":[]}`)
)

type sessionKey [16]byte

type queuedBatch struct {
	name, kind       string
	bytes, rows      int
	activity         int64
	sessions         []sessionKey
	orderKeys        []sessionKey
	invalid          bool
	segment, waiting int
	after            []*queuedBatch
	finished         bool
}

func keyFor(id string) sessionKey {
	sum := sha256.Sum256([]byte(id))
	return sessionKey(sum[:16])
}

// readBatch bounds reads even for old or damaged outbox files.
func (o *Outbox) readBatch(name string) ([]byte, error) {
	f, err := os.Open(filepath.Join(o.dir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, kitapi.MaxBatchBytes+1))
}

func rowsJSON(body []byte) ([]byte, error) {
	var envelope struct {
		Rows json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Rows) == 0 || envelope.Rows[0] != '[' {
		return nil, errors.New("missing rows array")
	}
	return envelope.Rows, nil
}

func (o *Outbox) inspect(name string) (*queuedBatch, error) {
	b := &queuedBatch{name: name, kind: kindOf(name)}
	body, err := o.readBatch(name)
	if err != nil {
		return nil, err
	}
	b.bytes = len(body)
	if len(body) > kitapi.MaxBatchBytes {
		b.invalid = true
		return b, nil
	}
	if b.kind == KindDelete {
		var row struct {
			SessionID string `json:"session_id"`
		}
		b.invalid = json.Unmarshal(body, &row) != nil || row.SessionID == ""
		if !b.invalid {
			b.sessions = []sessionKey{keyFor(row.SessionID)}
		}
		return b, nil
	}
	if b.kind != KindSessions && b.kind != KindBlocks && b.kind != KindChunks {
		b.invalid = true
		return b, nil
	}
	raw, err := rowsJSON(body)
	if err != nil {
		b.invalid = true
		return b, nil
	}
	type rowMetadata struct {
		SessionID string `json:"session_id"`
		ID        string `json:"id"`
		TS        string `json:"ts"`
		Start     int64  `json:"start"`
		End       int64  `json:"end"`
	}
	var rows []rowMetadata
	// Read at most one row past the cap, even if a file contains millions
	// of tiny JSON objects.
	dec := json.NewDecoder(bytes.NewReader(raw))
	_, _ = dec.Token() // rowsJSON already verified the array.
	for dec.More() && len(rows) <= kitapi.MaxBatchRows {
		rows = append(rows, rowMetadata{})
		if err := dec.Decode(&rows[len(rows)-1]); err != nil {
			b.invalid = true
			return b, nil
		}
	}
	if len(rows) > kitapi.MaxBatchRows {
		b.invalid = true
		return b, nil
	}
	b.rows = len(rows)
	sessions := make(map[sessionKey]bool)
	keys := make(map[sessionKey]bool)
	for _, r := range rows {
		b.activity = max(b.activity, r.Start, r.End)
		if t, err := time.Parse(time.RFC3339Nano, r.TS); err == nil {
			b.activity = max(b.activity, t.UnixMilli())
		}
		if r.SessionID != "" {
			k := keyFor(r.SessionID)
			if !sessions[k] {
				b.sessions = append(b.sessions, k)
				sessions[k] = true
			}
		}
		// Session keys preserve ordering across kinds. Row IDs also cover
		// legacy rows lacking a session ID.
		id := r.SessionID
		if id == "" {
			id = r.ID
		}
		if id != "" {
			k := keyFor(id)
			if !keys[k] {
				b.orderKeys = append(b.orderKeys, k)
				keys[k] = true
			}
		}
	}
	return b, nil
}

type upload struct {
	files       []*queuedBatch
	bytes, rows int
}

func (u *upload) fits(b *queuedBatch) bool {
	first := u.files[0]
	return !first.invalid && !b.invalid && first.kind != KindDelete &&
		first.kind == b.kind && first.rows > 0 && b.rows > 0 && u.bytes+b.bytes <= kitapi.MaxBatchBytes &&
		u.rows+b.rows <= kitapi.MaxBatchRows
}

func (u *upload) add(b *queuedBatch) {
	u.files = append(u.files, b)
	u.bytes += b.bytes
	u.rows += b.rows
}

// combine copies existing row encodings without changing their JSON values.
func (o *Outbox) combine(files []*queuedBatch) ([]byte, error) {
	if len(files) == 1 {
		return o.readBatch(files[0].name)
	}
	var body bytes.Buffer
	body.WriteString(`{"rows":[`)
	for _, b := range files {
		raw, err := o.readBatch(b.name)
		if err != nil {
			return nil, err
		}
		raw, err = rowsJSON(raw)
		if err != nil {
			return nil, err
		}
		rows := bytes.TrimSpace(raw[1 : len(raw)-1])
		if len(rows) == 0 {
			continue
		}
		if body.Len() > batchEnvelope-2 {
			body.WriteByte(',')
		}
		body.Write(rows)
	}
	body.WriteString(`]}`)
	if body.Len() > kitapi.MaxBatchBytes {
		return nil, errors.New("combined trace batch exceeds byte limit")
	}
	return body.Bytes(), nil
}

type uploadResult struct {
	sent, failed int
	rows         int
}

func (o *Outbox) post(ctx context.Context, c *kitapi.Client, files []*queuedBatch, p kitapi.SyncProgress) (uploadResult, error) {
	var result uploadResult
	b := files[0]
	if b.invalid {
		return o.refuse(b, "invalid or oversized trace batch")
	}
	body, err := o.combine(files)
	if err != nil {
		return result, errors.New("could not read a queued trace batch")
	}
	var rejected []int
	if b.kind == KindDelete {
		var row struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(body, &row) != nil || row.SessionID == "" {
			return o.refuse(b, "invalid delete batch")
		}
		deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = c.DeleteSession(deleteCtx, row.SessionID)
		cancel()
		var e *kitapi.Error
		if errors.As(err, &e) && e.Status == http.StatusNotFound {
			err = nil
		}
	} else {
		p.Scanning = o.isScanning()
		result.rows, rejected, err = c.PostSyncBatch(ctx, b.kind, body, p)
	}
	if err != nil {
		var e *kitapi.Error
		if !errors.As(err, &e) || e.Status/100 != 4 || kitapi.Retryable(err) || kitapi.ReadOnly(err) {
			return result, safeDrainError(err)
		}
		if len(files) == 1 {
			return o.refuse(b, "Club Kit API refused trace batch (HTTP "+strconv.Itoa(e.Status)+")")
		}
		// A refusal of the combined request does not establish that any
		// individual file is bad. Bisect until the refusal is isolated.
		body = nil
		mid := len(files) / 2
		left, err := o.post(ctx, c, files[:mid], p)
		if err != nil {
			return left, err
		}
		right, err := o.post(ctx, c, files[mid:], p)
		return uploadResult{left.sent + right.sent, left.failed + right.failed, left.rows + right.rows}, err
	}
	badRows := make(map[int]bool, len(rejected))
	total := 0
	for _, file := range files {
		total += file.rows
	}
	for _, i := range rejected {
		if i < 0 || i >= total {
			return uploadResult{}, &kitapi.Error{Status: http.StatusBadGateway, Message: "invalid rejected row index"}
		}
		badRows[i] = true
	}
	offset := 0
	for _, file := range files {
		bad := false
		for i := offset; i < offset+file.rows; i++ {
			bad = bad || badRows[i]
		}
		offset += file.rows
		if bad {
			if err := o.fail(file, "Club Kit API refused one or more rows"); err != nil {
				return result, err
			}
			result.failed++
			continue
		}
		if err := os.Remove(filepath.Join(o.dir, file.name)); err != nil {
			return result, errors.New("could not remove an accepted trace batch")
		}
		result.sent++
	}
	return result, nil
}

func (o *Outbox) refuse(b *queuedBatch, reason string) (uploadResult, error) {
	if err := o.fail(b, reason); err != nil {
		return uploadResult{}, err
	}
	return uploadResult{failed: 1}, nil
}

func (o *Outbox) fail(b *queuedBatch, reason string) error {
	dest := filepath.Join(o.dir, "failed", b.name)
	_ = os.WriteFile(dest+".error", []byte(reason+"\n"), 0o600)
	if err := os.Rename(filepath.Join(o.dir, b.name), dest); err != nil {
		_ = os.Remove(dest + ".error")
		return errors.New("could not move a refused trace batch to failed/")
	}
	return nil
}

func safeDrainError(err error) error {
	var e *kitapi.Error
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, kitapi.ErrUnauthorized):
		return kitapi.ErrUnauthorized
	case errors.As(err, &e):
		safe := &kitapi.Error{Status: e.Status, RetryAfter: e.RetryAfter}
		if kitapi.ReadOnly(err) {
			safe.Message = "billing_read_only"
		}
		return safe
	default:
		return errors.New("Club Kit API request failed")
	}
}
