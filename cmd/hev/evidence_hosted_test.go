package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestHostedEvidenceUsesSavedAPIOrigin(t *testing.T) {
	evidenceHome(t)
	key := "hk_" + strings.Repeat("s", 43)
	configPath := os.Getenv("HEV_CONFIG")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	config := "[kit]\nkey = \"" + key + "\"\napi = \"https://api.example.test\"\nservice = \"https://login.example.test\"\n"
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_KIT_SERVICE", "https://login.example.test")
	// Restore any external override after the test, and read the saved key.
	t.Setenv("HEV_API_KEY", "")
	if err := os.Unsetenv("HEV_API_KEY"); err != nil {
		t.Fatal(err)
	}
	client, err := hostedEvidence(context.Background())
	if err != nil || client == nil || client.base != "https://api.example.test" {
		t.Fatalf("saved API origin was not used: %v", err)
	}
}

func TestHostedEvidenceStagingKeepsKitBearerOffS3(t *testing.T) {
	const size = hostedEvidenceBlobCap + 1
	const uploadID = "evu_00000000000000000000000001"
	key := "hk_" + strings.Repeat("s", 43)
	src, err := os.CreateTemp(t.TempDir(), "clip-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := src.Write([]byte("\x00\x00\x00\x0cftypisom")); err != nil {
		t.Fatal(err)
	}
	if err := src.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, src); err != nil {
		t.Fatal(err)
	}
	sum := digest.Sum(nil)
	checksum := base64.StdEncoding.EncodeToString(sum)
	hash := hex.EncodeToString(sum)
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	const capability = "synthetic-signature"
	storage := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Query().Get("signature") != capability || r.ContentLength != size ||
			r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/octet-stream" ||
			r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Checksum-Sha256") != checksum {
			t.Error("incorrect signed PUT")
			w.WriteHeader(403)
			return
		}
		received := sha256.New()
		if n, err := io.Copy(received, r.Body); err != nil || n != size || hex.EncodeToString(received.Sum(nil)) != hash {
			t.Error("streamed bytes differ")
			w.WriteHeader(400)
			return
		}
		// An S3 error can arrive after bytes were committed. Finalize once.
		w.WriteHeader(500)
	}))
	defer storage.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("Kit upload request lacked its bearer")
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case hostedEvidencePath + "/blobs/uploads":
			var spec map[string]any
			if json.NewDecoder(r.Body).Decode(&spec) != nil || spec["sha256"] != hash || spec["size"] != float64(size) || spec["content_type"] != "video/mp4" {
				t.Error("incorrect grant request")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_id": uploadID, "upload_url": storage.URL + "?signature=" + capability,
				"method": "PUT", "expires_at": time.Now().Add(time.Minute),
				"headers": map[string]string{
					"content-length": strconv.Itoa(size), "content-type": "application/octet-stream",
					"if-none-match": "*", "x-amz-checksum-sha256": checksum,
				},
			})
		case hostedEvidencePath + "/blobs/uploads/" + uploadID + "/complete":
			_ = json.NewEncoder(w).Encode(map[string]string{"sha256": hash, "modality": "video"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer api.Close()

	client := &remoteEvidence{ctx: context.Background(), client: storage.Client(), base: api.URL, key: key}
	item, err := client.uploadStaged(src, size)
	if err != nil || item.Hash != hash || item.Modality != "video" {
		t.Fatalf("staged upload failed: %v", err)
	}
}
