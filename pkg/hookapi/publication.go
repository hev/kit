package hookapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Publication resolves a stored source reference to handler payloads. The
// reference must contain no event content. A zero Publication with no error
// indicates a candidate which did not commit.
type Publication struct {
	ID, Name, Host string
	Time           time.Time
	Full, Redacted json.RawMessage
}

type PublicationSource func(ctx context.Context, account string, reference json.RawMessage) (Publication, error)

type publication struct {
	Account, Key string
	Reference    json.RawMessage
	Created      time.Time
}

const publicationTTL = 30 * 24 * time.Hour

// StagePublication commits an obligation before the originating write. The
// resolver verifies that the write actually committed before publishing it.
func (s *Service) StagePublication(ctx context.Context, account, key string, reference json.RawMessage) error {
	if account == "" || key == "" || len(key) > 128 || len(reference) > 4096 || !json.Valid(reference) {
		return errors.New("invalid hook publication reference")
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("hook publication unavailable")
	}
	defer tx.Rollback()
	var count int
	err = tx.QueryRowContext(ctx, "SELECT count(*) FROM hook_rows WHERE account=? AND kind='publication'", account).Scan(&count)
	if err != nil || count >= 4096 {
		return errors.New("hook publication unavailable")
	}
	now := time.Now().UTC()
	p := publication{Account: account, Key: key, Reference: reference, Created: now}
	if err := s.writeRow(ctx, tx, account, stateRow{kind: "publication", id: key, at: now.UnixMilli(), due: now.UnixMilli(), value: p}); err != nil || tx.Commit() != nil {
		return errors.New("hook publication unavailable")
	}
	return nil
}

func (s *Service) publicationWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for range 16 {
				if ctx.Err() != nil || !s.publishNext(ctx) {
					break
				}
			}
		}
	}
}

func (s *Service) publishNext(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var account, key string
	var ciphertext []byte
	err := s.db.QueryRowContext(ctx, `SELECT account,id,body FROM hook_rows
		WHERE kind='publication' AND due!=0 AND due<=? ORDER BY due,account,id LIMIT 1`, time.Now().UnixMilli()).Scan(&account, &key, &ciphertext)
	if err != nil {
		return false
	}
	body, err := s.seal.Open(nil, nil, ciphertext, rowAAD(account, "publication", key))
	var p publication
	if err != nil || json.Unmarshal(body, &p) != nil {
		return false
	}
	now := time.Now().UTC()
	if now.Sub(p.Created) >= publicationTTL {
		return s.write(ctx, account, stateRow{kind: "publication", id: key}) == nil
	}
	payload, err := s.source(ctx, account, p.Reference)
	if err == nil && payload.ID != "" {
		err = s.publishReference(ctx, p, payload)
		if err == nil {
			return true
		}
	}
	// A missing post may still be committing. Give a candidate time to
	// become visible before discarding it.
	if err == nil && now.Sub(p.Created) >= time.Minute {
		return s.write(ctx, account, stateRow{kind: "publication", id: key}) == nil
	}
	return s.write(context.Background(), account, stateRow{kind: "publication", id: key, at: p.Created.UnixMilli(), due: now.Add(5 * time.Second).UnixMilli(), value: p}) == nil
}

func (s *Service) publishReference(ctx context.Context, p publication, payload Publication) error {
	if !validPublication(p.Account, payload.ID, payload.Name, payload.Full, payload.Redacted) || payload.Time.IsZero() {
		return errors.New("invalid resolved hook publication")
	}
	e := record{ID: payload.ID, Event: payload.Name, Harness: "kit", Host: payload.Host, Time: payload.Time, Body: payload.Redacted}
	_, err := s.publish(ctx, p.Account, e, payload.Full, false, time.Now(), &p)
	return err
}
