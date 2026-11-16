package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracefixture"
)

// editJSON rewrites a run record file through a generic map.
func editJSON(t *testing.T, path string, edit func(map[string]any)) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	if err := dispatch.WriteJSON(path, m); err != nil {
		t.Fatal(err)
	}
}

// A traced run attempt given a context recipe shows its context digest and
// recipe on its run span, read from this host's records and from the
// published trace facts alike.
func TestAsksRunSpanCarriesContextDigest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	info, err := tracefixture.Write(home, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("e", 64)
	dir := filepath.Join(home, "dispatch", tracefixture.OrderNav)
	editJSON(t, filepath.Join(dir, "result.json"), func(m map[string]any) { m["context_digest"] = digest })
	editJSON(t, filepath.Join(dir, "history.json"), func(m map[string]any) {
		attempts := m["attempts"].([]any)
		last := attempts[len(attempts)-1].(map[string]any)["meta"].(map[string]any)
		last["context"], last["context_digest"] = "recipes/ok.json", digest
	})
	sessions, blocks, err := index.ReadSide(&trace.ClaudeSource{Root: info.Transcripts}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	shared := &archive{rows: map[string]trace.SpanRow{}}
	if _, err := index.RunTraceSpans(filepath.Join(home, "dispatch"), shared, &index.State{Units: map[string]string{}}, false); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	for name, h := range map[string]*Server{
		"local":   New(&MemoryReader{Sessions: sessions, Blocks: blocks}),
		"archive": New(&MemoryReader{Sessions: sessions, Blocks: blocks, Spans: shared.all()}).WithDispatchRoot(elsewhere),
	} {
		var detail struct {
			Spans []Span `json:"spans"`
		}
		getAPI(t, h.Handler(), "/api/asks/"+info.Trace, &detail)
		var with, without int
		for _, sp := range detail.Spans {
			if sp.Kind != "run" || sp.Order != tracefixture.OrderNav {
				continue
			}
			if sp.ContextDigest == digest && sp.ContextRecipe == "recipes/ok.json" {
				with++
			} else if sp.ContextDigest == "" {
				without++
			} else {
				t.Errorf("%s: span %s context %q %q", name, sp.ID, sp.ContextDigest, sp.ContextRecipe)
			}
		}
		// Only the attempt that was given the bundle carries it.
		if with != 1 || without == 0 {
			t.Fatalf("%s: %d attempts with the digest, %d without", name, with, without)
		}
	}
}
