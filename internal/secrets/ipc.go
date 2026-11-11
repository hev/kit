package secrets

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type request struct {
	Account string
	Refs    []Ref
	Status  bool
	// Caller names the requesting process in the read log only.
	Caller string `json:",omitempty"`
}
type response struct {
	Values      map[string]string
	Status      *Status
	Unavailable bool
	Error       string
}

func Serve(ctx context.Context, c *Cache) (func(), error) {
	l, err := listen()
	if err != nil {
		return nil, err
	}
	account := accountID()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-done:
		}
	}()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serveConn(ctx, c, conn, account)
		}
	}()
	return func() { close(done); l.Close() }, nil
}
func serveConn(ctx context.Context, c *Cache, conn net.Conn, account string) {
	defer conn.Close()
	if !sameUser(conn) {
		return
	}
	conn.SetDeadline(time.Now().Add(45 * time.Second))
	var q request
	if json.NewDecoder(io.LimitReader(conn, 65536)).Decode(&q) != nil {
		return
	}
	r := response{}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if account == "" || q.Account != account {
		r.Unavailable = true
	} else if q.Status {
		s := c.Status(ctx)
		s.Daemon = true
		r.Status = &s
	} else {
		var err error
		r.Values, err = c.ResolveAs(ctx, q.Caller, q.Refs)
		if err != nil {
			r.Error = err.Error()
		}
	}
	json.NewEncoder(conn).Encode(r)
}

var ErrUnavailable = errors.New("local secrets daemon unavailable")

func query(ctx context.Context, q request) (response, error) {
	q.Account = accountID()
	conn, err := dial(ctx)
	if err != nil {
		return response{}, ErrUnavailable
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(45 * time.Second))
	if !serverUser(conn) {
		return response{}, errors.New("secrets endpoint is not owned by current user")
	}
	if err = json.NewEncoder(conn).Encode(q); err != nil {
		return response{}, errors.New("secrets request failed")
	}
	var r response
	if json.NewDecoder(io.LimitReader(conn, 4<<20)).Decode(&r) != nil {
		return r, errors.New("invalid secrets response")
	}
	if r.Unavailable {
		return r, ErrUnavailable
	}
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, nil
}
func Resolve(ctx context.Context, refs []Ref) (map[string]string, error) {
	return ResolveWith(ctx, refs, Options{})
}

// CacheDisabled reports HEV_SECRETS_CACHE=off: clients skip the local
// endpoint and behave as if no daemon ran (for a process whose PATH supplies a
// fixture op, for example).
func CacheDisabled() bool { return os.Getenv("HEV_SECRETS_CACHE") == "off" }

// IsRateLimit reports whether err is a rate-limit answer, from op or from the
// cache's cooldown, locally or through the endpoint.
func IsRateLimit(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "1Password ") && strings.Contains(err.Error(), "rate limit; retry after ")
}

// Options adjusts a client resolution.
type Options struct {
	// Caller names this process in the read log; empty uses DefaultCaller.
	Caller string
	// CacheOnly returns ErrUnavailable instead of resolving directly when no
	// endpoint serves this account, so the caller can keep its own fallback.
	CacheOnly bool
}

// ResolveWith asks the local endpoint and, unless opts.CacheOnly, resolves
// directly when no endpoint serves this account. A connected endpoint's error
// (a rate limit included) is returned as is and never retried directly.
func ResolveWith(ctx context.Context, refs []Ref, opts Options) (map[string]string, error) {
	if err := Validate(refs); err != nil {
		return nil, err
	}
	if opts.Caller == "" {
		opts.Caller = DefaultCaller("hev")
	}
	var r response
	err := ErrUnavailable
	if !CacheDisabled() {
		r, err = query(ctx, request{Refs: refs, Caller: opts.Caller})
	}
	if errors.Is(err, ErrUnavailable) {
		if opts.CacheOnly {
			return nil, ErrUnavailable
		}
		return New().ResolveAs(ctx, opts.Caller+" direct", refs)
	}
	return r.Values, err
}

// ReadStatus asks the endpoint for its counters and adds the host read log's
// 24-hour count. With no endpoint it still returns that count, with
// Daemon false, alongside ErrUnavailable.
func ReadStatus(ctx context.Context) (*Status, error) {
	r, err := query(ctx, request{Status: true})
	s := r.Status
	if s == nil {
		if !errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		s = &Status{ReadLog: ReadLogPath(), AccountLimits: "unavailable"}
	}
	if n, e := CountReadLog(time.Now().Add(-24 * time.Hour)); e == nil {
		s.HostReadsDay = &n
	}
	return s, err
}

// Hold the endpoint lock for the full listener lifetime, including shutdown.
type lockedListener struct {
	net.Listener
	once    sync.Once
	release func()
}

func (l *lockedListener) Close() error { err := l.Listener.Close(); l.once.Do(l.release); return err }

func accountID() string {
	token := serviceToken()
	if token == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}
