package v1

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/openapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

type boardTest struct {
	t      *testing.T
	h      http.Handler
	doc    map[string]any
	schema *jsonschema.Compiler
}

func newBoardTest(t *testing.T) boardTest {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler(st, dir, func(r *http.Request) (Caller, bool) {
		switch r.Header.Get("Authorization") {
		case "Bearer ada":
			return Caller{Author: "ada", Name: "Ada", Host: "trusted-host"}, true
		case "Bearer grace":
			return Caller{Author: "grace", Name: "Grace"}, true
		case "Bearer admin":
			return Caller{Author: "admin", Admin: true}, true
		default:
			return Caller{}, false
		}
	})
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(openapi.KitV1), &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("https://test.invalid/kit.json", doc); err != nil {
		t.Fatal(err)
	}
	return boardTest{t: t, h: h, doc: doc, schema: c}
}

type result struct {
	w    *httptest.ResponseRecorder
	json map[string]any
}

func node(doc any, keys ...string) any {
	for _, key := range keys {
		m, ok := doc.(map[string]any)
		if !ok {
			return nil
		}
		doc = m[key]
	}
	return doc
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func (f boardTest) call(method, path, pattern, key, replay string, body []byte, code int) result {
	t := f.t
	t.Helper()
	r := httptest.NewRequest(method, "/v1/board"+path, bytes.NewReader(body))
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	if replay != "" {
		r.Header.Set("Idempotency-Key", replay)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != code {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, w.Code, code, w.Body)
	}
	status := strconv.Itoa(code)
	op := strings.ToLower(method)
	pattern = "/v1/board" + pattern
	response := node(f.doc, "paths", pattern, op, "responses", status)
	if response == nil {
		t.Fatalf("%s %s does not document %d", method, pattern, code)
	}
	pointer := "#/paths/" + escapePointer(pattern) + "/" + op + "/responses/" + status
	if ref, ok := node(response, "$ref").(string); ok {
		pointer = ref
		response = node(f.doc, strings.Split(strings.TrimPrefix(ref, "#/"), "/")...)
	}
	if code == 204 {
		if w.Body.Len() != 0 {
			t.Fatal("204 returned a body")
		}
		return result{w: w}
	}
	media := w.Header().Get("Content-Type")
	if node(response, "content", media, "schema") == nil {
		t.Fatalf("%s %s does not document response media type %q", method, pattern, media)
	}
	schema, err := f.schema.Compile("https://test.invalid/kit.json" + pointer + "/content/" + escapePointer(media) + "/schema")
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if media == "application/json" {
		decoder := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
	} else {
		value = w.Body.String()
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("%s %s violates %d response schema: %v", method, path, code, err)
	}
	obj, _ := value.(map[string]any)
	return result{w: w, json: obj}
}

func TestBoardV1Contract(t *testing.T) {
	f := newBoardTest(t)
	call := f.call
	boards := call("GET", "/boards", "/boards", "ada", "", nil, 200).json["data"].([]any)
	for i, b := range boards {
		if node(b, "sort") != json.Number(strconv.Itoa(i)) {
			t.Fatalf("unsorted boards: %v", boards)
		}
	}
	opening := []byte(`{"board":"memos","title":"Parser fixed","body":"zebra parser","tags":["SEARCH","stable"],"author":"spoof","host":"spoof"}`)
	one := call("POST", "/posts", "/posts", "ada", "one", opening, 201)
	path := "/posts/" + string(one.json["id"].(json.Number))
	if one.w.Header().Get("Location") != "/v1/board"+path || one.json["author"] != "ada" || one.json["host"] != "trusted-host" {
		t.Fatalf("wrong create response: %v", one.json)
	}
	replayed := call("POST", "/posts", "/posts", "ada", "one", []byte(`{"title":"ignored","body":"different"}`), 201)
	if replayed.w.Body.String() != one.w.Body.String() {
		t.Fatal("create replay changed response")
	}
	two := call("POST", "/posts", "/posts", "grace", "", []byte(`{"title":"Unpinned","body":"another zebra"}`), 201)
	call("POST", "/posts", "/posts", "ada", "", []byte(`{"title":"Missing body"}`), 400)
	call("PATCH", path, "/posts/{id}", "grace", "", []byte(`{"pinned":true,"body":"forged"}`), 403)
	call("PATCH", path, "/posts/{id}", "grace", "", []byte(`{"pinned":true}`), 200)
	page := call("GET", "/posts?limit=1", "/posts", "ada", "", nil, 200).json
	first := page["data"].([]any)[0].(map[string]any)
	if first["id"] != one.json["id"] || first["body"] != nil || first["excerpt"] != "zebra parser" {
		t.Fatalf("wrong first page: %v", page)
	}
	last := call("GET", "/posts?limit=1&cursor="+page["next_cursor"].(string), "/posts", "grace", "", nil, 200).json
	if last["next_cursor"] != nil || node(last["data"].([]any)[0], "title") != "Unpinned" {
		t.Fatalf("wrong final page: %v", last)
	}
	filtered := call("GET", "/posts?board=memos&tag=STABLE&tag=search&since=7d", "/posts", "ada", "", nil, 200).json
	if len(filtered["data"].([]any)) != 1 {
		t.Fatalf("tag filter: %v", filtered)
	}
	call("GET", "/posts?cursor=invalid", "/posts", "ada", "", nil, 400)
	call("GET", "/posts?limit=201", "/posts", "ada", "", nil, 400)
	replyPath := path + "/replies"
	input := []byte(`{"body":"zebra followup","role":"human"}`)
	reply := call("POST", replyPath, "/posts/{id}/replies", "grace", "reply", input, 201)
	again := call("POST", replyPath, "/posts/{id}/replies", "grace", "reply", input, 201)
	if again.w.Body.String() != reply.w.Body.String() || reply.json["n"] != json.Number("2") {
		t.Fatal("reply replay changed response")
	}
	usefulPath := path + "/useful"
	for range 2 {
		vote := call("POST", usefulPath, "/posts/{id}/useful", "grace", "", nil, 200)
		if vote.json["useful"] != json.Number("1") {
			t.Fatalf("duplicate vote: %v", vote.json)
		}
	}
	before := call("GET", path+"?peek=true", "/posts/{id}", "ada", "", nil, 200).json
	if node(before, "post", "views") != json.Number("0") || len(before["replies"].([]any)) != 1 {
		t.Fatalf("unexpected peek: %v", before)
	}
	viewed := call("GET", path, "/posts/{id}", "ada", "", nil, 200).json
	if node(viewed, "post", "views") != json.Number("1") {
		t.Fatalf("view not counted: %v", viewed)
	}
	edited := call("PATCH", path, "/posts/{id}", "admin", "", []byte(`{"body":"zebra corrected","locked":true}`), 200).json
	if edited["body"] != "zebra corrected" || edited["updated"] != node(before, "post", "updated") {
		t.Fatalf("edit changed activity: %v", edited)
	}
	call("PATCH", path, "/posts/{id}", "ada", "", []byte(`{"body":null}`), 400)
	if res := call("POST", replyPath, "/posts/{id}/replies", "grace", "", input, 409); res.json["code"] != "locked" {
		t.Fatal(res.json)
	}
	found := call("GET", "/search?q=zebra&also=followup&tag=stable&author=grace&role=human&since=2020-01-01T00%3A00%3A00Z", "/search", "ada", "", nil, 200).json
	hits := found["data"].([]any)
	if len(hits) != 1 || node(hits[0], "reply", "body") != "zebra followup" || found["route"] != "lexical.bm25" {
		t.Fatalf("wrong search: %v", found)
	}
	call("GET", "/search?q=zebra&limit=1", "/search", "ada", "", nil, 200)
	call("GET", "/search?limit=1", "/search", "ada", "", nil, 400)
	call("DELETE", path, "/posts/{id}", "grace", "", nil, 403)
	call("DELETE", path, "/posts/{id}", "ada", "", nil, 204)
	call("GET", path, "/posts/{id}", "ada", "", nil, 404)
	call("PATCH", path, "/posts/{id}", "ada", "", []byte(`{"locked":false}`), 404)
	call("DELETE", path, "/posts/{id}", "ada", "", nil, 404)
	call("POST", replyPath, "/posts/{id}/replies", "grace", "", input, 404)
	call("POST", usefulPath, "/posts/{id}/useful", "grace", "", nil, 404)
	call("DELETE", "/posts/"+string(two.json["id"].(json.Number)), "/posts/{id}", "grace", "", nil, 204)
	newest := call("POST", "/posts", "/posts", "ada", "", []byte(`{"title":"Highest","body":"a post"}`), 201)
	if fmt.Sprint(newest.json["id"]) != "3" {
		t.Fatalf("unexpected assigned id: %v", newest.json)
	}
}

func TestBoardV1BlobAndAuthContract(t *testing.T) {
	f := newBoardTest(t)
	call := f.call
	limits := call("GET", "/blobs", "/blobs", "ada", "", nil, 200)
	if limits.json["max_bytes"] != json.Number("10485760") {
		t.Fatal(limits.json)
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	sha := fmt.Sprintf("%x", sha256.Sum256(pngBytes.Bytes()))
	path := "/blobs/" + sha
	upload := call("POST", "/blobs", "/blobs", "ada", "", pngBytes.Bytes(), 201)
	if upload.json["sha256"] != sha || upload.json["created"] != true {
		t.Fatal(upload.json)
	}
	again := call("POST", "/blobs", "/blobs", "ada", "", pngBytes.Bytes(), 200)
	if again.json["created"] != false {
		t.Fatal(again.json)
	}
	download := call("GET", path, "/blobs/{sha256}", "grace", "", nil, 200)
	if !bytes.Equal(download.w.Body.Bytes(), pngBytes.Bytes()) || download.w.Header().Get("ETag") != `"`+sha+`"` ||
		download.w.Header().Get("Vary") != "*" || !strings.Contains(download.w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("download headers or bytes changed: %v", download.w.Header())
	}
	ref := []byte(fmt.Sprintf(`{"title":"Image","body":"body","images":[{"sha256":%q,"name":"photo.png"}]}`, sha))
	linked := call("POST", "/posts", "/posts", "ada", "", ref, 201).json
	if url := node(linked["images"].([]any)[0], "url"); url != "/v1/board"+path {
		t.Fatalf("blob link: %v", url)
	}
	call("POST", "/blobs", "/blobs", "ada", "", []byte("<svg/>"), 415)
	call("POST", "/blobs", "/blobs", "ada", "", make([]byte, 10485761), 413)
	call("GET", "/blobs/"+strings.Repeat("0", 64), "/blobs/{sha256}", "ada", "", nil, 404)
	for _, row := range [][3]string{
		{"GET", "/boards", "/boards"},
		{"GET", "/posts", "/posts"}, {"POST", "/posts", "/posts"},
		{"GET", "/posts/1", "/posts/{id}"}, {"PATCH", "/posts/1", "/posts/{id}"}, {"DELETE", "/posts/1", "/posts/{id}"},
		{"POST", "/posts/1/replies", "/posts/{id}/replies"}, {"POST", "/posts/1/useful", "/posts/{id}/useful"},
		{"GET", "/search?q=body", "/search"},
		{"GET", "/blobs", "/blobs"}, {"POST", "/blobs", "/blobs"}, {"GET", path, "/blobs/{sha256}"},
	} {
		call(row[0], row[1], row[2], "", "", nil, 401)
	}
}

func TestBoardV1LayerRouting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inventory int
		want      string
		code      int
	}{
		{"multi-query on older gateway", 404, "layer.multi_query", 200},
		{"hybrid inventory", 200, "layer.hybrid_text", 200},
		{"upstream errors stay generic", 502, "", 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBoardTest(t)
			layerAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/capabilities") {
					w.WriteHeader(tc.inventory)
					if tc.inventory == 200 {
						fmt.Fprint(w, `{"declared":true,"hybrid_routes":[{"route":"hybrid_text","support":"supported"}],"schema_limits":{"embed":{"support":"supported"}}}`)
					} else {
						fmt.Fprint(w, "sensitive upstream endpoint and key")
					}
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if tc.want == "layer.multi_query" && body["queries"] == nil {
					t.Errorf("expected multi-query body: %v", body)
				}
				if tc.want == "layer.hybrid_text" && !strings.Contains(fmt.Sprint(body["rank_by"]), " Auto ") {
					t.Errorf("expected gateway-routed rank with an embedding: %v", body)
				}
				fmt.Fprint(w, `{"rows":[]}`)
			}))
			defer layerAPI.Close()
			f.h = Handler(store.New(layer.New(layerAPI.URL, "test-layer-key", "team-board", "")), t.TempDir(),
				func(*http.Request) (Caller, bool) { return Caller{Author: "tester"}, true })
			out := f.call("GET", "/search?q=zebra&also=another", "/search", "ada", "", nil, tc.code)
			if tc.code == 200 && out.json["route"] != tc.want {
				t.Fatalf("wrong route metadata: %v", out.json)
			}
			if strings.Contains(out.w.Body.String(), "sensitive upstream") || strings.Contains(out.w.Body.String(), layerAPI.URL) {
				t.Fatal("upstream diagnostic leaked")
			}
		})
	}
}
