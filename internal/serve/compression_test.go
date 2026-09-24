package serve

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseCompressionNegotiationIsLossless(t *testing.T) {
	body := strings.Repeat("600-character previews stay intact: 界 <>&\n", 2000)
	handler := compressResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Server-Timing", "layer;dur=123.456")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, body)
	}))
	for _, tc := range []struct {
		accept string
		gzip   bool
	}{
		{"", false}, {"br", false}, {"gzip", true}, {"br, gzip;q=0.5", true},
		{"gzip;q=0, *;q=1", false}, {"*;q=1, gzip;q=0", false}, {"*", true},
		{"gzip;q=invalid", false}, {"gzip;q=-1", false}, {"gzip;q=2", false},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/sessions", nil)
			r.Header.Set("Accept-Encoding", tc.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 400 || w.Header().Get("Vary") != "Accept-Encoding" || w.Header().Get("Server-Timing") != "layer;dur=123.456" {
				t.Fatal(w.Result())
			}
			got := w.Body.Bytes()
			if tc.gzip {
				if w.Header().Get("Content-Encoding") != "gzip" || len(got) >= len(body)/2 {
					t.Fatal("response not compressed")
				}
				reader, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatal(err)
				}
				got, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				reader.Close()
			} else if w.Header().Get("Content-Encoding") != "" {
				t.Fatal("unnegotiated compression")
			}
			if string(got) != body {
				t.Fatal("representation changed")
			}
		})
	}
}

func TestDashboardCompressedResponsesKeepTimingAndPreview(t *testing.T) {
	s, f := dashboardFixture(t)
	for _, row := range f.rows["test-sessions"] {
		row["first_prompt_short"] = strings.Repeat("界", 600)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	for _, path := range []string{"/", "/api/sessions?window=all", "/api/search?q=preflight", "/api/stats?window=all", "/api/session/trace-0"} {
		for _, method := range []string{"GET", "HEAD"} {
			req, _ := http.NewRequest(method, srv.URL+path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" || !strings.HasPrefix(resp.Header.Get("Server-Timing"), "layer;dur=") {
				t.Fatal(resp)
			}
			if method == "HEAD" {
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if len(b) != 0 {
					t.Fatal("HEAD returned a body")
				}
				continue
			}
			reader, err := gzip.NewReader(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(reader)
			reader.Close()
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(path, "/api/sessions") || strings.HasPrefix(path, "/api/search") {
				var data struct {
					Sessions []struct {
						Prompt string `json:"first_prompt_short"`
					} `json:"sessions"`
				}
				if err := json.Unmarshal(b, &data); err != nil {
					t.Fatal(err)
				}
				if len(data.Sessions) == 0 {
					t.Fatal("missing rows")
				}
				for _, row := range data.Sessions {
					if row.Prompt != strings.Repeat("界", 600) {
						t.Fatal("preview changed")
					}
				}
			}
		}
	}
}
