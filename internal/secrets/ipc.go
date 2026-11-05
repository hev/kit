package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type request struct {
	Refs   []Ref
	Status bool
}
type response struct {
	Values map[string]string
	Status *Status
	Error  string
}

func Serve(ctx context.Context, c *Cache) (func(), error) {
	l, err := listen()
	if err != nil {
		return nil, err
	}
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
			go serveConn(ctx, c, conn)
		}
	}()
	return func() { close(done); l.Close() }, nil
}
func serveConn(ctx context.Context, c *Cache, conn net.Conn) {
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
	if q.Status {
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
