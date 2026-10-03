package index

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// Keep wire-inspection fixtures honest about source persistence and readback.
func withSessionPersistence(next http.HandlerFunc, seededIDs ...string) http.HandlerFunc {
	var mu sync.Mutex
	rows := map[string]map[string]any{}
	for _, id := range seededIDs {
		rows[id] = map[string]any{"id": id}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "-sessions") {
			next(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "GET" {
			io.WriteString(w, `{}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]any
		json.Unmarshal(raw, &body)
		if strings.HasSuffix(r.URL.Path, "/query") {
			out := []any{}
			for _, row := range rows {
				if fixtureFilter(row, body["filters"]) {
					out = append(out, row)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": out})
			return
		}
		key, cond := "upsert_rows", "upsert_condition"
		if body[key] == nil {
			key, cond = "patch_rows", "patch_condition"
		}
		if body[key] == nil {
			next(w, r)
			return
		}
		list := body[key].([]any)
		affected := 0
		for _, raw := range list {
			row := raw.(map[string]any)
			if fixtureFilter(rows[row["id"].(string)], body[cond]) {
				affected++
			}
		}
		if affected == 0 {
			io.WriteString(w, `{"rows_affected":0}`)
			return
		}
		recorder := httptest.NewRecorder()
		next(recorder, r)
		if recorder.Code != 200 {
			w.WriteHeader(recorder.Code)
			w.Write(recorder.Body.Bytes())
			return
		}
		for _, raw := range list {
			row := raw.(map[string]any)
			id := row["id"].(string)
			if key == "upsert_rows" {
				rows[id] = row
			} else {
				for k, v := range row {
					rows[id][k] = v
				}
			}
		}
		var response map[string]any
		json.Unmarshal(recorder.Body.Bytes(), &response)
		response["rows_affected"] = affected
		json.NewEncoder(w).Encode(response)
	}
}
