package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/hev/kit/internal/board/settings"
)

func TestClientLegacyGoldenWire(t *testing.T) {
	data, err := os.ReadFile("../testdata/wire.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Method, Path string
		Body         any
		Status       int
		Response     any
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{{"ls", "--json"}, {"read", "7", "--json"}, {"search", "configuration", "--json"}, {"useful", "7", "--json"}, {"pin", "7"}, {"lock", "7"}}
	for i, args := range commands {
		t.Run(args[0], func(t *testing.T) {
			f := fixtures[i]
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := f.Path
				if args[0] == "read" {
					wantPath = "/api/threads/7"
				}
				if r.Method != f.Method || r.URL.RequestURI() != wantPath {
					t.Errorf("request %s %s; want %s %s", r.Method, r.URL.RequestURI(), f.Method, wantPath)
				}
				if r.Method == "POST" {
					var got map[string]any
					json.NewDecoder(r.Body).Decode(&got)
					if args[0] == "useful" {
						if got["author"] != "reader" {
							t.Errorf("poster: %+v", got)
						}
					} else if !reflect.DeepEqual(got, f.Body) {
						t.Errorf("body %+v, want %+v", got, f.Body)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(f.Status)
				json.NewEncoder(w).Encode(f.Response)
			}))
			defer api.Close()
			t.Setenv("HEV_CONFIG", t.TempDir()+"/config.toml")
			t.Setenv("FACTORY_BOARD_URL", api.URL)
			t.Setenv("FACTORY_BOARD_AS", "reader")
			// Capture public command output as well as checking its HTTP requests.
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			err := Run(args)
			w.Close()
			os.Stdout = old
			output, _ := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if args[0] != "pin" && args[0] != "lock" {
				var got any
				if err := json.Unmarshal(output, &got); err != nil {
					t.Fatal(err)
				}
				// useful intentionally exposes only the count, matching legacy CLI output.
				if args[0] == "useful" {
					if got.(map[string]any)["useful"] != float64(1) {
						t.Fatal(string(output))
					}
				} else if !reflect.DeepEqual(got, f.Response) {
					t.Fatalf("output %s", output)
				}
			}
		})
	}
}

func TestWriteRequestsKeepLegacyEnvelope(t *testing.T) {
	for _, verb := range []string{"post", "reply"} {
		t.Run(verb, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				wantPath := "/api/threads"
				if verb == "reply" {
					wantPath += "/7/posts"
				}
				if r.Method != "POST" || r.URL.Path != wantPath || body["body"] != "A finding" || body["author"] != "agent" || body["role"] != "agent" {
					t.Errorf("%s %s %+v", r.Method, r.URL.Path, body)
				}
				if verb == "post" && (body["board"] != "tooling" || body["title"] != "Finding" || !reflect.DeepEqual(body["tags"], []any{"ci", "test"})) {
					t.Errorf("post envelope %+v", body)
				}
				if verb == "post" {
					w.Write([]byte(`{"id":7,"author":"agent"}`))
				} else {
					w.Write([]byte(`{"id":"p:7:2","n":2,"author":"agent"}`))
				}
			}))
			defer api.Close()
			settings.Current.URL = api.URL
			args := []string{"7", "-m", "A finding", "--as", "agent"}
			if verb == "post" {
				args = []string{"--board", "tooling", "--title", "Finding", "--tags", "ci,test", "-m", "A finding", "--as", "agent"}
			}
			var err error
			if verb == "post" {
				err = runPost(args)
			} else {
				err = runReply(args)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
