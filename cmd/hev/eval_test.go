package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/trace"
)

func TestEvalPutStreamsJSONRows(t *testing.T) {
	input := strings.Repeat(`{"session":"s","ts":"2026-09-07T12:00:00Z","marks":{"custom":4}}`+"\n", 31)
	batches := []int{}
	var output bytes.Buffer
	err := putEvals(strings.NewReader(input), &output, func(rows []trace.Eval) (layer.WriteResult, error) {
		batches = append(batches, len(rows))
		if rows[0].Marks["custom"] != 4 {
			t.Fatal(rows[0])
		}
		return layer.WriteResult{RowsUpserted: len(rows)}, nil
	})
	if err != nil || len(batches) != 2 || batches[0] != 30 || batches[1] != 1 || !strings.Contains(output.String(), "put 31 eval rows") {
		t.Fatalf("%v %v %s", err, batches, output.String())
	}
	if err := putEvals(strings.NewReader("{broken}"), &output, func([]trace.Eval) (layer.WriteResult, error) {
		t.Fatal("bad JSON written")
		return layer.WriteResult{}, nil
	}); err == nil {
		t.Fatal("bad input accepted")
	}
	t.Log(strings.TrimSpace(output.String()))
}

func TestEvalPutCommandFileAndStdin(t *testing.T) {
	stored := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/demo-evals" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body struct {
			Rows []struct{ ID string } `json:"upsert_rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, r := range body.Rows {
			stored[r.ID] = true
		}
		fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(config, []byte("[layer]\napi_key = \"fixture\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_CONFIG", config)
	t.Setenv("HEVLAYER_ENDPOINT", srv.URL)
	row := `{"session":"s","ts":"2026-09-07T12:00:00Z","marks":{"custom":4}}`
	file := filepath.Join(dir, "evals.jsonl")
	if err := os.WriteFile(file, []byte(row), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"put", "--namespace", "demo"}, {"put", "--namespace", "demo", file}, {"put", "--namespace", "demo", "--file", file}} {
		cmd := newEvalCmd()
		cmd.SetArgs(args)
		cmd.SetIn(strings.NewReader(row))
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		t.Log(strings.TrimSpace(out.String()))
	}
	if len(stored) != 1 {
		t.Fatalf("replay produced %d IDs", len(stored))
	}
}

// `hev eval put` scrubs evaluator prose before any store sees it: the file
// archive (no key) and hev layer both.
func TestEvalPutRedactsBeforeStoring(t *testing.T) {
	key := "sk-ant-api03-FAKEfixture0123456789abcdefXYZ" // synthetic
	row := `{"session":"s","ts":"2026-11-18T12:00:00Z","marks":{"quality":1},"summary":"saw ` + key + `","evidence":{"log":"` + key + `"},"findings":["leaked ` + key + `"]}`
	put := func(t *testing.T) {
		t.Helper()
		cmd := newEvalCmd()
		cmd.SetArgs([]string{"put"})
		cmd.SetIn(strings.NewReader(row))
		cmd.SetOut(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("files", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)
		t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
		t.Setenv("HEV_ARCHIVE_DIR", filepath.Join(dir, "archive"))
		for _, env := range []string{"HEVLAYER_API_KEY", "HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF", "LAYER_API_KEY", "LAYER_API_KEY_OP_REF", "HEVLAYER_ENDPOINT", "HEVLAYER_NAMESPACE", "HEVLAYER_STORE"} {
			t.Setenv(env, "")
		}
		put(t)
		raw, err := os.ReadFile(filepath.Join(dir, "archive", "hev-traces", "evals.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "sk-ant-api03-") || strings.Count(string(raw), "[REDACTED:") < 3 {
			t.Fatalf("evals.jsonl not redacted:\n%s", raw)
		}
	})
	t.Run("layer", func(t *testing.T) {
		var sent []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r.Body)
			sent = append(sent, buf.Bytes()...)
			fmt.Fprint(w, `{"status":"OK","rows_upserted":1}`)
		}))
		defer srv.Close()
		dir := t.TempDir()
		config := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(config, []byte("[layer]\napi_key = \"fixture\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HEV_CONFIG", config)
		t.Setenv("HEVLAYER_ENDPOINT", srv.URL)
		put(t)
		if len(sent) == 0 || strings.Contains(string(sent), "sk-ant-api03-") || !strings.Contains(string(sent), "[REDACTED:") {
			t.Fatalf("sent to layer unredacted:\n%s", sent)
		}
	})
}
