package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func fileServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "board")
	old := settings.Current
	settings.Current = settings.Config{Dir: dir, CanvasDir: filepath.Join(t.TempDir(), "canvas")}
	t.Cleanup(func() { settings.Current = old })
	st, err := store.NewFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func do(s *Server, method, path, ct string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if ct != "" {
		r.Header.Set("Content-Type", ct)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestImagesOnTheThreadPageAndBlobRoute(t *testing.T) {
	s, _ := fileServer(t)
	img := pngBytes(t, 320, 200)
	// The upload says it is a JPEG; the bytes say PNG, and the bytes win.
	w := do(s, "POST", "/api/blobs", "image/jpeg", img)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var up struct {
		SHA256, Type, URL string
		Width, Height     int
	}
	json.Unmarshal(w.Body.Bytes(), &up)
	if up.Type != "image/png" || up.Width != 320 || up.Height != 200 || up.URL != "/blob/"+up.SHA256 {
		t.Fatalf("upload: %s", w.Body)
	}
	if w := do(s, "POST", "/api/blobs", "", img); w.Code != 200 {
		t.Fatalf("second upload of the same bytes: %d", w.Code)
	}

	post, _ := json.Marshal(map[string]any{"board": "research", "title": "A capture", "body": "see the image", "author": "agent-eyes",
		"images": []map[string]any{{"sha256": up.SHA256, "name": `..\evil/<b>cap.png`, "type": "text/html", "width": 1}}})
	w = do(s, "POST", "/api/threads", "application/json", post)
	if w.Code != 201 {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	var th struct {
		ID         int64 `json:"id"`
		ImageCount int   `json:"image_count"`
	}
	json.Unmarshal(w.Body.Bytes(), &th)
	if th.ImageCount != 1 {
		t.Fatalf("thread: %s", w.Body)
	}

	// The API carries the stored metadata, not what the client claimed.
	w = do(s, "GET", "/api/threads/1", "", nil)
	var read struct {
		Posts []store.Post `json:"posts"`
	}
	json.Unmarshal(w.Body.Bytes(), &read)
	im := read.Posts[0].Images[0]
	if im.Type != "image/png" || im.Width != 320 || im.Height != 200 || im.Name != "<b>cap.png" || im.URL != "/blob/"+up.SHA256 {
		t.Fatalf("image on read: %+v", im)
	}

	w = do(s, "GET", "/t/1", "", nil)
	page := w.Body.String()
	if w.Code != 200 || !strings.Contains(page, `<img src="/blob/`+up.SHA256+`"`) || !strings.Contains(page, `<a href="/blob/`+up.SHA256+`" target="_blank"`) ||
		!strings.Contains(page, `width="320" height="200"`) || strings.Contains(page, "<b>cap") {
		t.Fatalf("thread page: %d\n%s", w.Code, page)
	}
	if !strings.Contains(page, `enctype="multipart/form-data"`) || !strings.Contains(page, `type="file" name="images"`) {
		t.Error("the reply form does not take images")
	}
	if w := do(s, "GET", "/new", "", nil); !strings.Contains(w.Body.String(), `type="file" name="images"`) {
		t.Error("the new-thread form does not take images")
	}
	if w := do(s, "GET", "/recent", "", nil); !strings.Contains(w.Body.String(), "🖼 1") {
		t.Error("the thread list does not mark images")
	}

	w = do(s, "GET", "/blob/"+up.SHA256, "", nil)
	h := w.Header()
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), img) || h.Get("Content-Type") != "image/png" ||
		h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Cache-Control"), "immutable") {
		t.Fatalf("blob: %d %v", w.Code, h)
	}
	for _, p := range []string{"/blob/" + strings.Repeat("0", 64), "/blob/nothex", "/blob/..%2f..%2fthreads%2f1.json"} {
		if w := do(s, "GET", p, "", nil); w.Code != 404 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}

	// A post may only name images the board already has.
	bad, _ := json.Marshal(map[string]any{"body": "x", "author": "a", "images": []map[string]any{{"sha256": strings.Repeat("a", 64)}}})
	if w := do(s, "POST", "/api/threads/1/posts", "application/json", bad); w.Code != 400 || !strings.Contains(w.Body.String(), "upload it") {
		t.Errorf("reply naming a missing image: %d %s", w.Code, w.Body)
	}
}

func TestUploadRefusals(t *testing.T) {
	s, dir := fileServer(t)
	for name, body := range map[string][]byte{
		"svg":      []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"html":     []byte(`<html><script>alert(1)</script></html>`),
		"fake png": []byte("not a png at all"),
	} {
		if w := do(s, "POST", "/api/blobs", "image/png", body); w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	big := append(pngBytes(t, 2, 2), make([]byte, 10<<20)...)
	if w := do(s, "POST", "/api/blobs", "image/png", big); w.Code != http.StatusRequestEntityTooLarge || !strings.Contains(w.Body.String(), "10.0 MB limit") {
		t.Errorf("over the cap: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs")); !os.IsNotExist(err) {
		t.Errorf("a refused upload wrote to disk: %v", err)
	}
	var lim BlobLimits
	json.Unmarshal(do(s, "GET", "/api/blobs", "", nil).Body.Bytes(), &lim)
	if lim.MaxBytes != 10<<20 || lim.MaxPerPost != 8 || len(lim.Types) != 4 {
		t.Errorf("limits: %+v", lim)
	}
}

func multipartForm(t *testing.T, fields map[string]string, files map[string][]byte) (string, []byte) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	for name, data := range files {
		fw, _ := mw.CreateFormFile("images", name)
		fw.Write(data)
	}
	mw.Close()
	return mw.FormDataContentType(), b.Bytes()
}

func TestFormsTakeImages(t *testing.T) {
	s, dir := fileServer(t)
	ct, body := multipartForm(t, map[string]string{"board": "research", "title": "From the browser", "body": "a capture", "author": "adam"},
		map[string][]byte{"cap.png": pngBytes(t, 30, 20)})
	if w := do(s, "POST", "/new", ct, body); w.Code != http.StatusSeeOther {
		t.Fatalf("new: %d %s", w.Code, w.Body)
	}
	ct, body = multipartForm(t, map[string]string{"body": "and an svg", "author": "adam"},
		map[string][]byte{"ok.png": pngBytes(t, 5, 5), "x.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)})
	if w := do(s, "POST", "/t/1/reply", ct, body); w.Code != 400 || !strings.Contains(w.Body.String(), "SVG") {
		t.Fatalf("reply with an svg: %d %s", w.Code, w.Body)
	}
	// A plain form without files still works.
	if w := do(s, "POST", "/t/1/reply", "application/x-www-form-urlencoded", []byte("body=text+only&author=adam")); w.Code != http.StatusSeeOther {
		t.Fatalf("plain reply: %d %s", w.Code, w.Body)
	}
	var read struct {
		Posts []store.Post `json:"posts"`
	}
	json.Unmarshal(do(s, "GET", "/api/threads/1", "", nil).Body.Bytes(), &read)
	if len(read.Posts) != 2 || len(read.Posts[0].Images) != 1 || read.Posts[0].Images[0].Name != "cap.png" || len(read.Posts[1].Images) != 0 {
		t.Fatalf("posts: %+v", read.Posts)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "blobs", "sha256", "*", "*"))
	if len(entries) != 1 {
		t.Fatalf("the refused reply stored its good image too: %v", entries)
	}
}

func TestReadOnlyRefusesUploads(t *testing.T) {
	s, dir := fileServer(t)
	s.readOnly = true
	if w := do(s, "POST", "/api/blobs", "image/png", pngBytes(t, 4, 4)); w.Code != 403 || !strings.Contains(w.Body.String(), "read-only") {
		t.Fatalf("upload on a read-only board: %d %s", w.Code, w.Body)
	}
	ct, body := multipartForm(t, map[string]string{"title": "t", "body": "b", "author": "a"}, map[string][]byte{"a.png": pngBytes(t, 4, 4)})
	if w := do(s, "POST", "/new", ct, body); w.Code != 403 {
		t.Fatalf("form upload on a read-only board: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs")); !os.IsNotExist(err) {
		t.Fatalf("read-only board stored an image: %v", err)
	}
	if w := do(s, "GET", "/api/blobs", "", nil); w.Code != 200 {
		t.Fatalf("limits on a read-only board: %d", w.Code)
	}
}
