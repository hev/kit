package controlstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var obj = json.RawMessage(`{"test":true}`)

func require(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

// Only starts a fresh local cluster. Never accepts a DSN or existing database.
func cluster(t *testing.T) (*Store, *Store, *pgxpool.Pool) {
	t.Helper()
	bin, dir := os.Getenv("CONTROL_PG_BIN"), os.Getenv("CONTROL_SCHEMA_DIR")
	if bin == "" || dir == "" {
		if os.Getenv("CONTROL_PG_REQUIRED") == "1" {
			t.Fatal("local PostgreSQL binary and schema directory required")
		}
		t.Skip("real PostgreSQL integration requires CONTROL_PG_BIN and CONTROL_SCHEMA_DIR")
	}
	root, err := os.MkdirTemp("", "cs-pg-")
	require(t, err)
	t.Cleanup(func() { os.RemoveAll(root) })
	run := func(args ...string) {
		t.Helper()
		c := exec.Command(filepath.Join(bin, args[0]), args[1:]...)
		out, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("local PostgreSQL %s: %v %s", args[0], e, out)
		}
	}
	run("initdb", "-D", filepath.Join(root, "data"), "-U", "owner", "--auth=trust", "-E", "UTF8")
	sock := filepath.Join(root, "sock")
	require(t, os.Mkdir(sock, 0700))
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	require(t, e)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	run("pg_ctl", "-D", filepath.Join(root, "data"), "-w", "-l", filepath.Join(root, "log"), "-o", fmt.Sprintf("-p %d -k %s -c listen_addresses=''", port, sock), "start")
	t.Cleanup(func() {
		exec.Command(filepath.Join(bin, "pg_ctl"), "-D", filepath.Join(root, "data"), "-m", "immediate", "-w", "stop").Run()
	})
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(fmt.Sprintf("host=%s port=%d user=owner dbname=postgres sslmode=disable", sock, port))
	require(t, e)
	admin, e := pgxpool.NewWithConfig(ctx, cfg)
	require(t, e)
	t.Cleanup(admin.Close)
	_, e = admin.Exec(ctx, "CREATE ROLE kitchen_runtime LOGIN; CREATE ROLE kitchen_enroller LOGIN")
	require(t, e)
	files, e := filepath.Glob(filepath.Join(dir, "*.sql"))
	require(t, e)
	if len(files) < 2 {
		t.Fatal("both schema and enrollment migration required")
	}
	for _, f := range files {
		b, e := os.ReadFile(f)
		require(t, e)
		_, e = admin.Exec(ctx, string(b))
		require(t, e)
	}
	connect := func(user string) *Store {
		c := cfg.Copy()
		c.ConnConfig.User = user
		c.AfterConnect = checkVersion
		p, e := pgxpool.NewWithConfig(ctx, c)
		require(t, e)
		t.Cleanup(p.Close)
		s, e := New(ctx, p)
		require(t, e)
		return s
	}
	return connect("kitchen_runtime"), connect("kitchen_enroller"), admin
}
func peer(t *testing.T, s *Store) string {
	t.Helper()
	p, e := s.EnrollPeer(context.Background(), "tool", "synthetic", nil)
	require(t, e)
	return p
}
func chef(t *testing.T, s *Store, p, name string) Lease {
	t.Helper()
	l, e := s.AcquireLease(context.Background(), name, p, 60)
	require(t, e)
	if l == nil {
		t.Fatal("lease busy")
	}
	return *l
}
func TestPostgres(t *testing.T) {
	s, enroll, admin := cluster(t)
	ctx := context.Background()
	p, q := peer(t, enroll), peer(t, enroll)
	l := chef(t, s, p, "chef")
	t.Run("runtime permissions", func(t *testing.T) {
		_, e := s.EnrollPeer(ctx, "tool", "forbidden", nil)
		if !errors.Is(e, &Error{"42501"}) {
			t.Fatalf("runtime enrolled: %v", e)
		}
		for _, sql := range []string{"INSERT INTO kitchen.peers(kind,label) VALUES('tool','bad')", "UPDATE kitchen.leases SET fence=0", "DELETE FROM kitchen.orders", "CREATE TABLE kitchen.evil(x int)", "SELECT nextval('kitchen.messages_seq_seq')"} {
			_, e := s.pool.Exec(ctx, sql)
			if e == nil {
				t.Fatalf("runtime allowed %s", sql)
			}
		}
		_, e = s.PutOrder(ctx, "o", obj, nil)
		require(t, e)
	})
	t.Run("events and generation", func(t *testing.T) {
		n, e := s.AppendEvent(ctx, "o", "event", "note", obj, &p)
		require(t, e)
		if n != 1 {
			t.Fatal(n)
		}
		n, e = s.AppendEvent(ctx, "o", "event", "note", obj, &p)
		require(t, e)
		if n != 1 {
			t.Fatal(n)
		}
		_, e = s.AppendEvent(ctx, "o", "event", "different", obj, &p)
		if !errors.Is(e, ErrConflict) {
			t.Fatal(e)
		}
		_, e = s.AppendEvent(ctx, "missing", "e", "note", obj, nil)
		if !errors.Is(e, ErrNotFound) {
			t.Fatal(e)
		}
		_, e = s.PutOrder(ctx, "o", json.RawMessage(`{"new":1}`), nil)
		if !errors.Is(e, ErrStaleGeneration) {
			t.Fatal(e)
		}
		rows, e := s.Events(ctx, "o", 0, 10)
		require(t, e)
		var v []any
		require(t, json.Unmarshal(rows, &v))
		if len(v) != 1 {
			t.Fatal(string(rows))
		}
		ok, e := s.AdvanceCursor(ctx, "c", "events", 0, 1, &l)
		require(t, e)
		if !ok {
			t.Fatal("cursor")
		}
		ok, e = s.AdvanceCursor(ctx, "c", "events", 0, 2, &l)
		require(t, e)
		if ok {
			t.Fatal("stale cursor advanced")
		}
	})
	t.Run("concurrent claims and delayed finish", func(t *testing.T) {
		id, e := s.IntendAction(ctx, "o", "fake-effect", "action", obj, nil, l)
		require(t, e)
		var wg sync.WaitGroup
		claims := make(chan *ActionClaim, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); c, e := s.ClaimAction(ctx, l, 30, strptr("o")); errs <- e; claims <- c }()
		}
		wg.Wait()
		close(claims)
		close(errs)
		for e := range errs {
			require(t, e)
		}
		var c *ActionClaim
		count := 0
		for v := range claims {
			if v != nil {
				c = v
				count++
			}
		}
		if count != 1 || c.ID != id || c.Attempt != 1 {
			t.Fatalf("claims=%d claim=%+v", count, c)
		}
		require(t, s.FinishAction(ctx, *c, "failed", nil, 0))
		next, e := s.ClaimAction(ctx, l, 30, strptr("o"))
		require(t, e)
		if next == nil || next.Attempt != 2 {
			t.Fatal(next)
		}
		if e = s.FinishAction(ctx, *c, "done", nil, 0); !errors.Is(e, ErrStaleFence) {
			t.Fatal(e)
		}
		require(t, s.FinishAction(ctx, *next, "uncertain", nil, 0))
		if e = s.ReconcileAction(ctx, id, true, "", nil); !errors.Is(e, ErrInvalidState) {
			t.Fatal(e)
		}
		require(t, s.ReconcileAction(ctx, id, true, "fake target inspected", nil))
	})
	t.Run("messages outbox enrollment assignment", func(t *testing.T) {
		require(t, s.Heartbeat(ctx, p, 30, obj))
		m, e := s.SendMessage(ctx, p, q, strptr("o"), "message", obj)
		require(t, e)
		m2, e := s.SendMessage(ctx, p, q, strptr("o"), "message", obj)
		require(t, e)
		if m != m2 {
			t.Fatal("duplicate message")
		}
		body, e := s.Inbox(ctx, q, 0, 20)
		require(t, e)
		var v []any
		require(t, json.Unmarshal(body, &v))
		if len(v) != 1 {
			t.Fatal(string(body))
		}
		ok, e := s.AckMessage(ctx, m, q)
		require(t, e)
		if !ok {
			t.Fatal("ack")
		}
		ok, e = s.AckMessage(ctx, m, q)
		require(t, e)
		if ok {
			t.Fatal("duplicate ack")
		}
		id, e := s.EnqueueOutbox(ctx, "fake-layer", "outbox", obj, strptr("o"))
		require(t, e)
		claim, e := s.ClaimOutbox(ctx, "fake-layer", 30)
		require(t, e)
		if claim == nil || *claim != id {
			t.Fatal(claim)
		}
		require(t, s.FinishOutbox(ctx, id, "fake:receipt"))
		_, e = s.Outbox(ctx, id)
		require(t, e)
		w, e := enroll.EnrollWorker(ctx, p, "codex", strptr("synthetic"))
		require(t, e)
		st, e := enroll.EnrollStation(ctx, "test-station", "synthetic", obj)
		require(t, e)
		a, e := s.Assign(ctx, "o", "part", &st, w, 30, "artifact:context")
		require(t, e)
		require(t, s.ReportAttempt(ctx, a.Attempt, w, a.Fence, "succeeded", obj))
		require(t, enroll.EnrollObligation(ctx, "accept", "o", "synthetic condition"))
		_, e = s.SubmitCandidate(ctx, "accept", "sha:fake")
		require(t, e)
		require(t, s.DecideCandidate(ctx, "accept", "sha:fake", "pass", "fake evidence"))
	})
	t.Run("stale fence", func(t *testing.T) {
		ok, e := s.ReleaseLease(ctx, l)
		require(t, e)
		if !ok {
			t.Fatal("release")
		}
		newLease := chef(t, s, q, "chef")
		if newLease.Fence <= l.Fence {
			t.Fatal(newLease)
		}
		if e = s.CheckFence(ctx, l); !errors.Is(e, ErrStaleFence) {
			t.Fatal(e)
		}
		_, e = s.IntendAction(ctx, "o", "fake", "stale", obj, nil, l)
		if !errors.Is(e, ErrStaleFence) {
			t.Fatal(e)
		}
	})
	t.Run("transaction rollback and killed connection retry", func(t *testing.T) {
		abort := errors.New("abort")
		e := s.Transaction(ctx, func(tx *Store) error {
			_, e := tx.AppendEvent(ctx, "o", "rolled-back", "note", obj, nil)
			require(t, e)
			return abort
		})
		if !errors.Is(e, abort) {
			t.Fatal(e)
		}
		// Kill this client's backend after its insert but before commit. Its stable
		// dedupe key can be retried after the outcome is reconciled.
		e = s.Transaction(ctx, func(tx *Store) error {
			_, e := tx.AppendEvent(ctx, "o", "network-kill", "note", obj, nil)
			if e != nil {
				return e
			}
			pid, e := scalar[int](ctx, tx, "SELECT pg_backend_pid()")
			if e != nil {
				return e
			}
			_, e = admin.Exec(ctx, "SELECT pg_terminate_backend($1)", pid)
			require(t, e)
			return nil
		})
		if e == nil {
			t.Fatal("killed transaction committed")
		}
		n, e := s.AppendEvent(ctx, "o", "network-kill", "note", obj, nil)
		require(t, e)
		if n != 2 {
			t.Fatal(n)
		}
		n, e = s.AppendEvent(ctx, "o", "network-kill", "note", obj, nil)
		require(t, e)
		if n != 2 {
			t.Fatal(n)
		}
		// Also model loss of a committed response: replay returns its original seq.
	})
	t.Run("committed acknowledgement loss", func(t *testing.T) {
		var armed, dropped, committed atomic.Bool
		cfg := admin.Config().Copy()
		cfg.ConnConfig.User = "kitchen_runtime"
		cfg.AfterConnect = checkVersion
		cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
			c, e := (&net.Dialer{}).DialContext(ctx, network, address)
			if e != nil {
				return nil, e
			}
			return &lostAckConn{Conn: c, armed: &armed, dropped: &dropped, confirmed: &committed, confirm: func() bool {
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					var n int
					e := admin.QueryRow(context.Background(), "SELECT count(*) FROM kitchen.order_events WHERE order_id='o' AND dedupe_key='lost-ack'").Scan(&n)
					if e == nil && n == 1 {
						return true
					}
					time.Sleep(5 * time.Millisecond)
				}
				return false
			}}, nil
		}
		pool, e := pgxpool.NewWithConfig(ctx, cfg)
		require(t, e)
		defer pool.Close()
		client, e := New(ctx, pool)
		require(t, e)
		armed.Store(true)
		e = client.Transaction(ctx, func(tx *Store) error { _, e := tx.AppendEvent(ctx, "o", "lost-ack", "note", obj, nil); return e })
		if e == nil || !dropped.Load() || !committed.Load() {
			t.Fatalf("loss not proven: err=%v dropped=%v committed=%v", e, dropped.Load(), committed.Load())
		}
		n, e := client.AppendEvent(ctx, "o", "lost-ack", "note", obj, nil)
		require(t, e)
		if n != 3 {
			t.Fatal(n)
		}
		var count int
		require(t, admin.QueryRow(ctx, "SELECT count(*) FROM kitchen.order_events WHERE order_id='o' AND dedupe_key='lost-ack'").Scan(&count))
		if count != 1 {
			t.Fatal(count)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		_, e := admin.Exec(ctx, "CREATE OR REPLACE FUNCTION kitchen.api_version() RETURNS int LANGUAGE sql AS 'SELECT 99'")
		require(t, e)
		_, e = New(ctx, s.pool)
		var v *VersionError
		if !errors.As(e, &v) || v.Got != 99 {
			t.Fatal(e)
		}
	})
}
func strptr(s string) *string { return &s }
func TestSafeErrors(t *testing.T) {
	for _, code := range []string{"K0001", "K0002", "K0003", "K0004", "K0005", "23505"} {
		e := safe(&pgconn.PgError{Code: code, Message: "SECRET", Detail: "SECRET", Where: "SECRET"})
		var typed *Error
		if !errors.As(e, &typed) || typed.Code != code || strings.Contains(e.Error(), "SECRET") {
			t.Fatal(e)
		}
	}
	if safe(errors.New("SECRET")) != ErrTransport {
		t.Fatal("transport leaked")
	}
	if !errors.Is(safe(context.DeadlineExceeded), context.DeadlineExceeded) {
		t.Fatal("context")
	}
}

type fakeResolver struct{ calls int }

func (f *fakeResolver) Resolve(context.Context, string) (string, error) {
	f.calls++
	return "SECRET", errors.New("SECRET")
}
func TestReferences(t *testing.T) {
	f := &fakeResolver{}
	_, _, e := Open(context.Background(), References{Host: "op://test/host", Password: "literal"}, f)
	if e == nil || f.calls != 0 {
		t.Fatal("must validate before resolving")
	}
	_, _, e = Open(context.Background(), References{Host: "op://test/host", Database: "op://test/db", User: "op://test/user", Password: "op://test/password"}, f)
	if e == nil || strings.Contains(e.Error(), "SECRET") {
		t.Fatal(e)
	}
}
func TestOPResolverRedacts(t *testing.T) {
	if _, e := (OPResolver{}).Resolve(context.Background(), "literal"); e == nil {
		t.Fatal("accepted literal")
	}
	dir := t.TempDir()
	require(t, os.WriteFile(filepath.Join(dir, "op"), []byte("#!/bin/sh\necho SECRET >&2\nexit 1\n"), 0700))
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, e := (OPResolver{}).Resolve(ctx, "op://test/password")
	if e == nil || strings.Contains(e.Error(), "SECRET") {
		t.Fatal(e)
	}
}

// Drop the server response to a real COMMIT on the test's Unix socket. The
// independent owner connection confirms commit before the caller sees EOF.
type lostAckConn struct {
	net.Conn
	armed, dropped, confirmed *atomic.Bool
	pending                   atomic.Bool
	confirm                   func() bool
}

func (c *lostAckConn) Write(b []byte) (int, error) {
	if bytes.Contains(bytes.ToLower(b), []byte("commit")) && c.armed.CompareAndSwap(true, false) {
		c.pending.Store(true)
	}
	return c.Conn.Write(b)
}
func (c *lostAckConn) Read(b []byte) (int, error) {
	n, e := c.Conn.Read(b)
	if c.pending.CompareAndSwap(true, false) {
		c.confirmed.Store(c.confirm())
		c.dropped.Store(true)
		_ = c.Conn.Close()
		return 0, io.EOF
	}
	return n, e
}

func TestConnectionConfig(t *testing.T) {
	t.Setenv("PGHOST", "/tmp/should-not-use")
	t.Setenv("PGPASSWORD", "ENV_SECRET")
	t.Setenv("PGUSER", "wrong")
	t.Setenv("PGSERVICE", "unexpected")
	t.Setenv("PGSERVICEFILE", "/missing-service-file")
	t.Setenv("PGSSLMODE", "disable")
	t.Setenv("PGSSLROOTCERT", "/missing-cert")
	if _, e := connectionConfig("db.example.invalid", 5432, "d", "u", "p"); e == nil {
		t.Fatal("accepted ambient service")
	}
	t.Setenv("PGSERVICE", "")
	cfg, e := connectionConfig("db.example.invalid", 5432, "synthetic-db", "synthetic-user", "synthetic-password")
	require(t, e)
	c := cfg.ConnConfig
	if c.Host != "db.example.invalid" || c.User != "synthetic-user" || c.Password != "synthetic-password" || c.TLSConfig == nil || c.TLSConfig.InsecureSkipVerify || len(c.Fallbacks) != 0 || c.TLSConfig.ServerName != c.Host {
		t.Fatal("unsafe connection configuration")
	}
	if cfg.AfterConnect == nil {
		t.Fatal("version check missing")
	}
	for _, host := range []string{"/tmp", "host1,host2", "host:bad", "", "host\n"} {
		if _, e := connectionConfig(host, 5432, "d", "u", "p"); e == nil {
			t.Fatal("accepted invalid host")
		}
	}
}

func TestControllerStaleFencesAndBounds(t *testing.T) {
	s, enroll, admin := cluster(t)
	ctx := context.Background()
	p, e := enroll.EnrollPeer(ctx, "chef", "controller", nil)
	require(t, e)
	q, e := enroll.EnrollPeer(ctx, "chef", "new controller", nil)
	require(t, e)
	_, e = s.PutOrder(ctx, "bounded", obj, nil)
	require(t, e)
	_, e = s.AppendEvent(ctx, "bounded", "input", "attention", obj, nil)
	require(t, e)
	snap, e := s.ControllerSnapshot(ctx, "bounded")
	require(t, e)
	l := chef(t, s, p, ControllerLease("bounded"))
	// Transfer invalidates the old fence before its attempted commit.
	_, e = s.ReleaseLease(ctx, l)
	require(t, e)
	newer := chef(t, s, q, l.Name)
	e = s.Transaction(ctx, func(tx *Store) error {
		_, e := tx.RecordControllerDecision(ctx, snap, 1, "stale", obj, obj, l)
		return e
	})
	if !errors.Is(e, ErrStaleFence) {
		t.Fatal(e)
	}
	// Expiry also rejects the last check even for the same holder/fence.
	_, e = admin.Exec(ctx, "UPDATE kitchen.leases SET expires_at=clock_timestamp()-interval '1 second' WHERE name=$1", newer.Name)
	require(t, e)
	e = s.Transaction(ctx, func(tx *Store) error {
		_, e := tx.RecordControllerDecision(ctx, snap, 1, "expired", obj, obj, newer)
		return e
	})
	if !errors.Is(e, ErrStaleFence) {
		t.Fatal(e)
	}
	var n int
	require(t, admin.QueryRow(ctx, "SELECT count(*) FROM kitchen.actions").Scan(&n))
	if n != 0 {
		t.Fatal(n)
	}
	// A too-large durable input is never silently truncated or consumed.
	_, e = s.AppendEvent(ctx, "bounded", "large", "attention", json.RawMessage(`{"reference":"`+strings.Repeat("x", 25000)+`"}`), nil)
	require(t, e)
	_, e = s.ControllerSnapshot(ctx, "bounded")
	if !errors.Is(e, ErrContextLimit) {
		t.Fatal(e)
	}
}

type literalDatabaseResolver struct{ refs []string }

func (r *literalDatabaseResolver) Resolve(_ context.Context, ref string) (string, error) {
	r.refs = append(r.refs, ref)
	switch ref {
	case "op://test/host":
		return "/invalid-socket-host", nil
	default:
		return "synthetic", nil
	}
}
func TestExplicitNonsecretDatabaseName(t *testing.T) {
	for _, bad := range []References{
		{Host: "literal-host", DatabaseName: "postgres", User: "op://test/user", Password: "op://test/pass"},
		{Host: "op://test/host", DatabaseName: "postgres", User: "literal-user", Password: "op://test/pass"},
		{Host: "op://test/host", DatabaseName: "postgres", User: "op://test/user", Password: "literal-password"},
		{Host: "op://test/host", DatabaseName: "postgres", Database: "op://test/db", User: "op://test/user", Password: "op://test/pass"},
		{Host: "op://test/host", DatabaseName: "op://test/db", User: "op://test/user", Password: "op://test/pass"},
	} {
		r := &literalDatabaseResolver{}
		_, _, e := Open(context.Background(), bad, r)
		if e == nil || len(r.refs) != 0 {
			t.Fatal("invalid config reached resolver", e, r.refs)
		}
	}
	r := &literalDatabaseResolver{}
	_, _, e := Open(context.Background(), References{Host: "op://test/host", DatabaseName: "postgres", User: "op://test/user", Password: "op://test/pass"}, r)
	// All three secret-bearing fields are still resolved. The deliberately invalid
	// host then stops before dialing. The literal database never reaches resolver.
	if e == nil || len(r.refs) != 3 {
		t.Fatal(e, r.refs)
	}
}
