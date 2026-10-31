package outboxrelay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// Requires explicit server and schema files; this harness creates its own Unix
// socket cluster, never reads PGHOST/DSN and never connects to a hosted database.
func localPostgres(t *testing.T) *sql.DB {
	t.Helper()
	bin := os.Getenv("OUTBOX_TEST_PG_BIN")
	schema := os.Getenv("OUTBOX_TEST_SCHEMA")
	if bin == "" || schema == "" {
		t.Skip("local PostgreSQL acceptance requires OUTBOX_TEST_PG_BIN and OUTBOX_TEST_SCHEMA (skip is not acceptance)")
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	sock := filepath.Join(root, "sock")
	if e := os.Mkdir(sock, 0700); e != nil {
		t.Fatal(e)
	}
	run := func(name string, args ...string) {
		t.Helper()
		out, e := exec.Command(filepath.Join(bin, name), args...).CombinedOutput()
		if e != nil {
			t.Fatalf("%s: %v %s", name, e, out)
		}
	}
	args := []string{"-D", data, "-U", "relaytest", "--auth=trust", "-E", "UTF8", "--no-locale"}
	if share := os.Getenv("OUTBOX_TEST_PG_SHARE"); share != "" {
		args = append(args, "-L", share)
	}
	run("initdb", args...)
	run("pg_ctl", "-D", data, "-w", "-l", filepath.Join(root, "log"), "-o", fmt.Sprintf("-k %s -c listen_addresses='' -c fsync=off", sock), "start")
	t.Cleanup(func() { exec.Command(filepath.Join(bin, "pg_ctl"), "-D", data, "-m", "immediate", "-w", "stop").Run() })
	db, e := sql.Open("postgres", fmt.Sprintf("host=%s user=relaytest dbname=postgres sslmode=disable", sock))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	raw, e := os.ReadFile(schema)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(raw)); e != nil {
		t.Fatal(e)
	}
	return db
}
func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := db.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}
func scalar(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var s string
	if e := db.QueryRow(q).Scan(&s); e != nil {
		t.Fatal(e)
	}
	return s
}
func acquire(t *testing.T, p *Postgres) {
	t.Helper()
	if e := p.DB.QueryRow("SELECT kitchen.acquire_lease($1,$2::uuid,120)", p.lease(), p.Holder).Scan(&p.Fence); e != nil {
		t.Fatal(e)
	}
}
func TestPostgresAcceptance(t *testing.T) {
	db := localPostgres(t)
	ctx := context.Background()
	p := Postgres{DB: db, Destination: "layer-test", Consumer: "relay-test", Holder: "11111111-1111-4111-8111-111111111111", TTL: 1}
	acquire(t, &p)
	mustExec(t, db, `SELECT kitchen.put_order('synthetic','{}',NULL)`)
	for i := 1; i <= 3; i++ {
		mustExec(t, db, `SELECT kitchen.append_event('synthetic',$1,'note',$2::jsonb,NULL)`, fmt.Sprint(i), fmt.Sprintf(`{"n":%d}`, i))
	}
	for i := 0; i < 3; i++ {
		ok, e := p.ProjectEvent(ctx, "synthetic")
		if !ok || e != nil {
			t.Fatal(ok, e)
		}
	}
	if ok, e := p.ProjectEvent(ctx, "synthetic"); ok || e != nil {
		t.Fatal(ok, e)
	}
	f := &fakeLayer{docs: map[string]Document{}}
	r := Relay{p, f}
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	f.fail = true
	if _, e := r.Step(ctx); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	f.fail = false
	// A newly constructed client has no local delivery state. The expired claim
	// must be inspected, while the remaining pending row can make progress.
	mustExec(t, db, `UPDATE kitchen.outbox SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE state='sending'`)
	r = Relay{p, f}
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	f.lag = true
	if _, e := r.Step(ctx); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	if f.puts != 3 {
		t.Fatal("uncertain send was replayed", f.puts)
	}
	f.lag = false
	if _, e := r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if scalar(t, db, `SELECT count(*)::text FROM kitchen.outbox WHERE state='sent'`) != "3" {
		t.Fatal("not all sent")
	}
	if scalar(t, db, `SELECT count(*)::text FROM kitchen.cursors WHERE stream LIKE 'outbox/%' AND position=1`) != "3" {
		t.Fatal("missing receipts")
	}
	// Raw outbox commit order differs from sequence order. The low ID remains
	// invisible while a higher ID is delivered; committing it later cannot skip it.
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`SELECT kitchen.enqueue_outbox('layer-test','slow','{}',NULL)`); e != nil {
		t.Fatal(e)
	}
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('layer-test','fast','{}',NULL)`)
	if _, e = r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if len(f.docs) != 5 {
		t.Fatal("late commit skipped", len(f.docs))
	}
	// Re-enqueue is idempotent and content conflicts are rejected by corrected SQL.
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('layer-test','fast','{}',NULL)`)
	if _, e = db.Exec(`SELECT kitchen.enqueue_outbox('layer-test','fast','{"changed":true}',NULL)`); e == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	// Crash immediately after durable claim: no remote document; never resend.
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('layer-test','crash-before-send','{}',NULL)`)
	d, e := p.Next(ctx)
	if e != nil || d == nil || !d.Claimed {
		t.Fatal(d, e)
	}
	mustExec(t, db, `UPDATE kitchen.outbox SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE state='sending'`)
	if _, e = r.Step(ctx); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	if f.puts != 5 {
		t.Fatal("absent uncertain document replayed")
	}
	// Stale destination owner cannot claim, finish or project after takeover.
	mustExec(t, db, `SELECT kitchen.release_lease($1,$2::uuid,$3)`, p.lease(), p.Holder, p.Fence)
	replacement := p
	replacement.Holder = "22222222-2222-4222-8222-222222222222"
	acquire(t, &replacement)
	for _, operation := range []func() error{
		func() error { _, e := p.Next(ctx); return e }, func() error { return p.Complete(ctx, *d) }, func() error { _, e := p.ProjectEvent(ctx, "synthetic"); return e },
	} {
		if e := operation(); e == nil || !strings.Contains(e.Error(), "stale_fence") {
			t.Fatal("stale owner accepted", e)
		}
	}
	// Rebuild into a new destination has separate cursors, leaving all historical
	// records (including uncertainty) intact; documents reproduce event payloads.
	rebuild := p
	rebuild.Destination = "layer-rebuild"
	rebuild.Consumer = "rebuild"
	acquire(t, &rebuild)
	for i := 0; i < 3; i++ {
		ok, e := rebuild.ProjectEvent(ctx, "synthetic")
		if !ok || e != nil {
			t.Fatal(ok, e)
		}
	}
	rf := &fakeLayer{docs: map[string]Document{}}
	rr := Relay{rebuild, rf}
	for i := 0; i < 3; i++ {
		if _, e := rr.Step(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if len(rf.docs) != 3 {
		t.Fatal(rf)
	}
	for _, doc := range rf.docs {
		var body struct {
			Seq     int `json:"seq"`
			Payload struct {
				N int `json:"n"`
			} `json:"payload"`
		}
		if e := json.Unmarshal(doc.Payload, &body); e != nil || body.Seq != body.Payload.N {
			t.Fatal(doc, e)
		}
	}
	var sourceID int64
	if e := db.QueryRow(`SELECT outbox_id FROM kitchen.outbox WHERE destination='layer-test' AND idempotency_key='fast'`).Scan(&sourceID); e != nil {
		t.Fatal(e)
	}
	if e := rebuild.RebuildOutboxRow(ctx, "layer-test", sourceID); e != nil {
		t.Fatal(e)
	}
	if e := rebuild.RebuildOutboxRow(ctx, "layer-test", sourceID); e != nil {
		t.Fatal(e)
	}
	if _, e := rr.Step(ctx); e != nil {
		t.Fatal(e)
	}
	if len(rf.docs) != 4 {
		t.Fatal("non-event outbox rebuild failed")
	}
	if e := p.RebuildOutboxRow(ctx, "layer-test", sourceID); e == nil {
		t.Fatal("in-place rebuild allowed")
	}
	if scalar(t, db, `SELECT count(*)::text FROM kitchen.outbox WHERE destination='layer-test'`) != "6" {
		t.Fatal("rebuild changed original history")
	}
}

func TestPostgresLargeEventAndTimezone(t *testing.T) {
	db := localPostgres(t)
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	p := Postgres{DB: db, Destination: "large", Consumer: "large", Holder: "11111111-1111-4111-8111-111111111111", TTL: 30}
	acquire(t, &p)
	mustExec(t, db, `SELECT kitchen.put_order('large','{}',NULL)`)
	// This event fits the source's JSONB 64 KiB bound, but a full event envelope
	// does not fit the outbox bound. A compact immutable reference must succeed.
	mustExec(t, db, `SELECT kitchen.append_event('large','max','note',jsonb_build_object('x',repeat('a',65527)),NULL)`)
	mustExec(t, db, `SET TIME ZONE 'Pacific/Kiritimati'`)
	if ok, e := p.ProjectEvent(ctx, "large"); !ok || e != nil {
		t.Fatal(ok, e)
	}
	d, e := p.Next(ctx)
	if d == nil || e != nil {
		t.Fatal(d, e)
	}
	mustExec(t, db, `SET TIME ZONE 'America/Los_Angeles'`)
	// Compare freshly hydrated completion content under another session timezone.
	if e = p.Complete(ctx, *d); e != nil {
		t.Fatal(e)
	}
	if len(d.Document.Payload) < 65536 {
		t.Fatal("fixture did not exercise envelope bound")
	}
	// Ordinary JSON objects with similarly named fields are not event references.
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('large','ordinary','{"seq":"arbitrary","projection":"order-event-v1"}',NULL)`)
	if _, e = p.Next(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresReconciliationDoesNotBlockLaterRows(t *testing.T) {
	db := localPostgres(t)
	ctx := context.Background()
	p := Postgres{DB: db, Destination: "rotate", Consumer: "rotate", Holder: "11111111-1111-4111-8111-111111111111", TTL: 1}
	acquire(t, &p)
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('rotate','absent','{}',NULL)`)
	mustExec(t, db, `SELECT kitchen.enqueue_outbox('rotate','found','{}',NULL)`)
	absent, e := p.Next(ctx)
	if e != nil {
		t.Fatal(e)
	}
	found, e := p.Next(ctx)
	if e != nil {
		t.Fatal(e)
	}
	f := &fakeLayer{docs: map[string]Document{found.Document.ID: found.Document}}
	r := Relay{p, f}
	mustExec(t, db, `UPDATE kitchen.outbox SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE state='sending'`)
	if _, e = r.Step(ctx); !errors.Is(e, ErrUncertain) {
		t.Fatal(e)
	}
	if _, e = r.Step(ctx); e != nil {
		t.Fatal("earlier absence blocked later match", e)
	}
	if f.puts != 0 {
		t.Fatal("blind retry")
	}
	if scalar(t, db, fmt.Sprintf(`SELECT state FROM kitchen.outbox WHERE outbox_id=%d`, absent.OutboxID)) != "sending" {
		t.Fatal("uncertainty lost")
	}
}
