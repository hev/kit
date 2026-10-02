package layer_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
)

// Opt-in real gateway/store test. Only job-owned synthetic namespaces are
// written/read/deleted. No archive listing, operator change, or embedding setup.
func TestPhraseLiveOrderedScan(t *testing.T) {
	if os.Getenv("HEV_PHRASE_LIVE") != "1" {
		t.Skip("set HEV_PHRASE_LIVE=1 to use the existing configured gateway credential")
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	endpoint := cfg.LayerEndpoint
	if endpoint == "" {
		endpoint = layer.DefaultEndpoint
	}
	key := cfg.LayerAPIKey
	if key == "" {
		key, err = daemon.LayerKey()
		if err != nil {
			t.Fatal("configured Layer credential unavailable")
		}
	}
	namespace := fmt.Sprintf("scratch-kit-phrase-%d", time.Now().UnixNano())
	c, err := layer.New(endpoint, key, namespace, "").WithStore(layer.StoreKind(cfg.LayerStore))
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Client{Timeout: 30 * time.Second}
	request := func(method, ns string, body any) error {
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		req, err := http.NewRequest(method, endpoint+"/v2/namespaces/"+ns, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := transport.Do(req)
		if err != nil {
			return fmt.Errorf("synthetic request transport failed")
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("synthetic %s returned status %d", method, resp.StatusCode)
		}
		return nil
	}
	for _, ns := range []string{namespace, namespace + "-evals"} {
		ns := ns
		t.Cleanup(func() {
			if err := request("DELETE", ns, nil); err != nil {
				t.Error("synthetic namespace cleanup:", err)
			}
		})
	}
	rows := []map[string]any{}
	for i := 0; i < 1002; i++ {
		text := "quartz blue amber"
		if i == 999 {
			text = "amber quartz"
		}
		if i == 1000 {
			text = "lower QUARTZ\n\tAMBER exact"
		}
		if i == 1001 {
			text = "quartz amber overflow"
		}
		rows = append(rows, map[string]any{"id": fmt.Sprintf("%04d", i), "session_id": "own-session", "text": text})
	}
	rows = append(rows, map[string]any{"id": "0000-foreign", "session_id": "other-session", "text": "quartz amber foreign"})
	schema := map[string]any{"text": "string", "session_id": "string", "turn_uuid": "string", "ts": "string", "role": "string", "block_type": "string", "tool_name": "string", "is_sidechain": "bool"}
	if err := request("POST", namespace, map[string]any{"upsert_rows": rows, "schema": schema}); err != nil {
		t.Fatal(err)
	}
	if err := request("POST", namespace+"-evals", map[string]any{"upsert_rows": []map[string]any{{"id": "eval", "session_id": "own-session", "text": "evaluator QUARTZ\nAMBER"}}, "schema": schema}); err != nil {
		t.Fatal(err)
	}
	filter := []any{"session_id", "In", []string{"own-session"}}
	for _, tc := range []struct {
		limit, want int
		truncated   bool
	}{{1000, 0, true}, {1001, 1, true}, {1002, 2, false}} {
		hits, truncated, err := c.PhraseHits("quartz amber", tc.limit, filter)
		if err != nil || len(hits) != tc.want || truncated != tc.truncated {
			t.Fatalf("real scan limit=%d hits=%d truncated=%v err=%v", tc.limit, len(hits), truncated, err)
		}
		for _, hit := range hits {
			if hit.SessionID != "own-session" {
				t.Fatal("session filter failed")
			}
		}
		t.Logf("real %s ordered scan: budget=%d matches=%d truncated=%v", c.Caps.Store.Kind, tc.limit, len(hits), truncated)
	}
	evals, truncated, err := c.PhraseEvals("quartz amber", 100, []any{"id", "In", []string{"eval"}})
	if err != nil || truncated || len(evals) != 1 {
		t.Fatalf("real eval matches=%d truncated=%v err=%v", len(evals), truncated, err)
	}
	t.Log("real eval text phrase matched; plain string schema has no full-text index or embedding declaration")
}
