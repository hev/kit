package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedEvidenceCLINeverMirrors(t *testing.T) {
	home := evidenceHome(t)
	key := "hk_" + strings.Repeat("s", 43)
	packageID := "ev_01khnj0m000000000000000000"
	source := filepath.Join(home, "fixture.txt")
	if err := os.WriteFile(source, []byte("synthetic proof "+key+" /Users/fixture/private.txt"), 0600); err != nil {
		t.Fatal(err)
	}
	var blob []byte
	var hash string
	var doc map[string]any
	pro := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			http.Error(w, "unauthorized /Users/fixture/private.txt", 401)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/evidence/blobs":
			blob, _ = io.ReadAll(r.Body)
			if strings.Contains(string(blob), key) || strings.Contains(string(blob), "/Users/") {
				t.Error("client sent unredacted text")
			}
			digest := sha256.Sum256(blob)
			hash = hex.EncodeToString(digest[:])
			_ = json.NewEncoder(w).Encode(map[string]any{"sha256": hash, "modality": "text"})
		case "POST /v1/evidence":
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["title"] != "proof [local path]" || r.Header.Get("Idempotency-Key") == "" ||
				input["items"].([]any)[0].(map[string]any)["sha256"] != hash || input["host"] != nil {
				t.Error("client wrote an invalid hosted manifest")
			}
			doc = map[string]any{"id": packageID, "schema": "hev.evidence.package/v1", "state": "sealed", "title": input["title"],
				"items": []any{map[string]any{"name": "fixture.txt", "size": len(blob), "sha256": hash}}, "claims": []any{}}
			_ = json.NewEncoder(w).Encode(doc)
		case "GET /v1/evidence/" + packageID:
			w.Header().Set("ETag", `"1"`)
			_ = json.NewEncoder(w).Encode(doc)
		case "GET /v1/evidence/blobs/" + hash:
			_, _ = w.Write(blob)
		default:
			http.Error(w, "bad route", 404)
		}
	}))
	defer pro.Close()
	t.Setenv("HEV_API_KEY", key)
	t.Setenv("HEV_KIT_SERVICE", pro.URL)
	out, _, err := runEvidence(t, "", "put", "--title", "proof /Users/fixture/private.txt", source)
	if err != nil || strings.TrimSpace(out) != packageID {
		t.Fatalf("put returned %q, %v", out, err)
	}
	out, _, err = runEvidence(t, "", "get", packageID)
	if err != nil || strings.TrimSpace(out) != pro.URL+"/v1/evidence/"+packageID {
		t.Fatalf("get returned %q, %v", out, err)
	}
	t.Setenv("HEV_API_KEY", "hk_"+strings.Repeat("a", 43))
	if _, _, err := runEvidence(t, "", "create", "--title", "unauthorized"); err == nil || strings.Contains(err.Error(), "/Users/") {
		t.Fatalf("expected sanitized hosted refusal, got %v", err)
	}
	t.Setenv("HEV_API_KEY", "badly-configured")
	if _, _, err := runEvidence(t, "", "create", "--title", "no fallback"); err == nil {
		t.Fatal("invalid credential accepted")
	}
	if _, err := os.Stat(filepath.Join(home, ".hev", "evidence")); !os.IsNotExist(err) {
		t.Fatalf("hosted runs created a local mirror: %v", err)
	}
}
