// Package controlstate is an isolated client for the version 1 transactional
// coordination API. It does not schedule work or activate a controller.
package controlstate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const APIVersion = 1

// Error contains only a SQLSTATE, never SQL text, server details or credentials.
// Use errors.Is with the sentinel errors below.
type Error struct{ Code string }

func (e *Error) Error() string        { return "control state SQLSTATE " + e.Code }
func (e *Error) Is(target error) bool { t, ok := target.(*Error); return ok && t.Code == e.Code }

var (
	ErrStaleFence      = &Error{"K0001"}
	ErrStaleGeneration = &Error{"K0002"}
	ErrNotFound        = &Error{"K0003"}
	ErrConflict        = &Error{"K0004"}
	ErrInvalidState    = &Error{"K0005"}
	ErrTransport       = errors.New("control state connection failed; outcome may be unknown")
)

type VersionError struct{ Got, Want int }

func (e *VersionError) Error() string {
	return fmt.Sprintf("control state API version %d; require %d", e.Got, e.Want)
}
func safe(err error) error {
	if err == nil {
		return nil
	}
	var v *VersionError
	if errors.As(err, &v) {
		return v
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		return &Error{p.Code}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrTransport
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type Store struct {
	db   querier
	pool *pgxpool.Pool
}

// New checks the contract before exposing any calls. The caller owns pool and
// its TLS/connection configuration. No environment or credential files are read.
func New(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("control state pool required")
	}
	var v int
	if err := pool.QueryRow(ctx, "SELECT kitchen.api_version()", pgx.QueryExecModeExec).Scan(&v); err != nil {
		return nil, safe(err)
	}
	if v != APIVersion {
		return nil, &VersionError{v, APIVersion}
	}
	return &Store{db: pool, pool: pool}, nil
}

// Transaction makes related intents, events and outbox writes atomic. The
// callback is called once. Neither this method nor individual calls retry:
// after a lost commit acknowledgement reconcile or replay stable dedupe keys.
func (s *Store) Transaction(ctx context.Context, fn func(*Store) error) error {
	if s.pool == nil {
		return errors.New("nested control state transaction")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return safe(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err = fn(&Store{db: tx}); err != nil {
		return err
	}
	return safe(tx.Commit(ctx))
}
func scalar[T any](ctx context.Context, s *Store, q string, args ...any) (T, error) {
	var v T
	err := s.db.QueryRow(ctx, q, append([]any{pgx.QueryExecModeExec}, args...)...).Scan(&v)
	return v, safe(err)
}
func (s *Store) call(ctx context.Context, q string, args ...any) error {
	_, err := s.db.Exec(ctx, q, append([]any{pgx.QueryExecModeExec}, args...)...)
	return safe(err)
}
