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
	t.Setenv("LAYER_ENDPOINT", srv.URL)
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
