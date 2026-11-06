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
	"sync"
	"time"
)

type request struct {
	Account string
	Refs    []Ref
	Status  bool
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
		r.Status = &s
	} else {
		var err error
		r.Values, err = c.Resolve(ctx, q.Refs)
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
	if err := Validate(refs); err != nil {
		return nil, err
	}
	r, err := query(ctx, request{Refs: refs})
	if errors.Is(err, ErrUnavailable) {
		return New().Resolve(ctx, refs)
	}
	return r.Values, err
}
func ReadStatus(ctx context.Context) (*Status, error) {
	r, err := query(ctx, request{Status: true})
	return r.Status, err
}

// Hold the endpoint lock for the full listener lifetime, including shutdown.
type lockedListener struct {
	net.Listener
	once    sync.Once
	release func()
}

func (l *lockedListener) Close() error { err := l.Listener.Close(); l.once.Do(l.release); return err }

func accountID() string {
	token := os.Getenv("OP_SERVICE_ACCOUNT_TOKEN")
	if token == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}
