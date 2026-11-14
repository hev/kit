package archive_test

import (
	"context"
	"testing"
	"time"

	"github.com/hev/kit/pkg/archive"
)

func TestFileStoreRoundTripAndRetention(t *testing.T) {
	st, err := archive.OpenFiles(t.TempDir(), "team-traces")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).UnixMilli()
	fresh := time.Now().UnixMilli()
	sessions := []archive.SessionRow{
		{ID: "old", SessionID: "old", Host: "h", Start: old, End: old, Summary: "old session"},
		{ID: "new", SessionID: "new", Host: "h", Start: fresh, End: fresh, Summary: "new session"},
	}
	if _, err := st.WriteSessions(sessions); err != nil {
		t.Fatal(err)
	}
	if _, err := st.WriteBlocks([]archive.BlockRow{{ID: "new:1", SessionID: "new", Seq: 1, Role: "user", BlockType: "text", Text: "hello archive"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Write([]archive.Row{{ID: "new:c1", SessionID: "new", Text: "hello archive", Host: "h"}}); err != nil {
		t.Fatal(err)
	}
	// Idempotent by ID.
	if _, err := st.WriteSessions(sessions); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListSessionRows(-1, nil)
	if err != nil || len(rows) != 2 {
		t.Fatalf("sessions = %d, %v", len(rows), err)
	}
	blocks, err := st.ListBlockRows("new")
	if err != nil || len(blocks) != 1 {
		t.Fatalf("blocks = %d, %v", len(blocks), err)
	}
	hits, _, err := st.PhraseHits("hello archive", 10, nil)
	if err != nil || len(hits) != 1 {
		t.Fatalf("phrase hits = %d, %v", len(hits), err)
	}
	n, err := archive.DeleteSessionsBefore(context.Background(), st, time.Now().Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("deleted = %d, %v", n, err)
	}
	rows, _ = st.ListSessionRows(-1, nil)
	if len(rows) != 1 || rows[0].SessionID != "new" {
		t.Fatalf("after retention: %+v", rows)
	}
}

func TestOpenLayerNeedsCredentials(t *testing.T) {
	if _, err := archive.OpenLayer(archive.LayerConfig{Endpoint: "https://example.invalid"}); err == nil {
		t.Fatal("OpenLayer accepted no key")
	}
	c, err := archive.OpenLayer(archive.LayerConfig{Endpoint: "https://example.invalid", Key: "k", Namespace: "t"})
	if err != nil || c.Namespace != "t" {
		t.Fatalf("OpenLayer = %v, %v", c, err)
	}
}
