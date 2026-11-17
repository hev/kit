package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

const EndpointEnv = "HEV_CLUSTER_ENDPOINT"

func Endpoint() string {
	if p := os.Getenv(EndpointEnv); p != "" {
		return p
	}
	return defaultEndpoint()
}

type lockedListener struct {
	net.Listener
	once    sync.Once
	release func()
}

func (l *lockedListener) Close() error { e := l.Listener.Close(); l.once.Do(l.release); return e }

type Request struct {
	Action  string `json:"action"`
	Address string `json:"address,omitempty"`
}
type Response struct {
	Status  *Status  `json:"status,omitempty"`
	Members []Member `json:"members,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func Serve(ctx context.Context, s *Service) (func(), error) {
	if s.cfg == nil {
		return func() {}, nil
	}
	l, e := listen()
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-done:
		}
	}()
	go func() {
		defer close(done)
		for {
			conn, e := l.Accept()
			if e != nil {
				return
			}
			go serveConn(ctx, s, conn)
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); l.Close(); <-done }) }, nil
}
func serveConn(ctx context.Context, s *Service, c net.Conn) {
	defer c.Close()
	if !sameUser(c) {
		return
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	var q Request
	if json.NewDecoder(io.LimitReader(c, 4096)).Decode(&q) != nil {
		return
	}
	r := Response{}
	var e error
	switch q.Action {
	case "status":
		st := s.Status()
		r.Status = &st
	case "ls":
		r.Members = s.Members()
	case "join":
		e = s.Join(q.Address)
	case "leave":
		e = s.Leave()
	default:
		e = errors.New("unknown cluster action")
	}
	if e != nil {
		r.Error = e.Error()
	}
	_ = json.NewEncoder(c).Encode(r)
}
func Query(ctx context.Context, q Request) (Response, error) {
	var r Response
	if validEndpoint(Endpoint()) != nil {
		return r, errors.New("invalid local cluster endpoint")
	}
	c, e := dial(ctx)
	if e != nil {
		return r, errors.New("local cluster daemon unavailable; start hevd")
	}
	defer c.Close()
	if !serverUser(c) {
		return r, errors.New("cluster endpoint owner mismatch")
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	if json.NewEncoder(c).Encode(q) != nil {
		return r, errors.New("cluster request failed")
	}
	if json.NewDecoder(io.LimitReader(c, 2*maxWire)).Decode(&r) != nil {
		return r, errors.New("invalid cluster response")
	}
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, nil
}
