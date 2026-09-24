package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
)

func TestIndexCycleRetriesOfflineAndPicksUpGrowingTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".claude", "projects", "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(root, "session.jsonl")
	writeTurn := func(id, text string, appendFile bool) {
		flag := os.O_CREATE | os.O_WRONLY
		if appendFile {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}
		f, err := os.OpenFile(transcript, flag, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		fmt.Fprintf(f, `{"type":"user","uuid":%q,"sessionId":"s1","timestamp":"2026-09-05T10:00:00Z","cwd":"/work","message":{"role":"user","content":%q}}`+"\n", id, text)
	}
	writeTurn("one", "first unique phrase", false)

	offline := true
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if offline {
			http.Error(w, "unreachable", http.StatusServiceUnavailable)
			return
		}
		writes++
		io.WriteString(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	client := layer.New(srv.URL, "key", "archive", "")
	state := &index.State{Units: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	failed := runIndexCycle(client, state, logger)
	if failed.LastError == "" || len(state.Units) != 0 {
		t.Fatalf("offline cycle = %+v, state=%v", failed, state.Units)
	}
	offline = false
	caughtUp := runIndexCycle(client, state, logger)
	if caughtUp.LastError != "" || caughtUp.UnitsIndexed != 1 || writes != 3 {
		t.Fatalf("catch-up = %+v, writes=%d", caughtUp, writes)
	}
	writeTurn("two", "second phrase added while live", true)
	grown := runIndexCycle(client, state, logger)
	if grown.LastError != "" || grown.UnitsIndexed != 1 || writes != 6 {
		t.Fatalf("growth cycle = %+v, writes=%d", grown, writes)
	}
}
