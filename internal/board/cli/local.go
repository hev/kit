package cli

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/hev/kit/internal/board/layer"
	"github.com/hev/kit/internal/board/server"
	"github.com/hev/kit/internal/board/settings"
	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/storagemode"
)

// local is the board's own HTTP handler, run in this process over this
// host's files, when the CLI needs no server: nothing names a board server
// (board.url, $FACTORY_BOARD_URL) and the host has no Layer key. Every
// command then goes through the same handler `hev board serve` would run,
// so the two cannot disagree. It is nil when the CLI talks to a server.
var local http.Handler

// localDir is the directory local works on, for whoami.
var localDir string

// connect decides between a server and this host's files. A machine set up
// for Layer whose shell has no key (storagemode's NoKeyError) talks to its
// server, which holds the key, as before.
func connect() error {
	local, localDir = nil, ""
	if settings.Current.URLSet {
		return nil
	}
	o := layer.KeyOptions
	o.Peek = true
	r, err := storagemode.Resolve(o)
	if err != nil || r.Mode != storagemode.Filesystem {
		return nil
	}
	st, err := store.NewFiles(settings.Dir())
	if err != nil {
		return err
	}
	srv, err := server.New(st)
	if err != nil {
		return err
	}
	local, localDir = srv.Handler(), st.Info().Location
	return nil
}

// inProcess serves a request with local instead of the network.
type inProcess struct{}

func (inProcess) RoundTrip(req *http.Request) (*http.Response, error) {
	if local == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	r := req.Clone(req.Context())
	r.RemoteAddr = "127.0.0.1:0"
	r.RequestURI = req.URL.RequestURI()
	w := &recorder{header: http.Header{}}
	local.ServeHTTP(w, r)
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return &http.Response{
		Status: strconv.Itoa(w.code) + " " + http.StatusText(w.code), StatusCode: w.code,
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: w.header, Body: io.NopCloser(&w.body), ContentLength: int64(w.body.Len()), Request: req,
	}, nil
}

// recorder is the response local writes.
type recorder struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *recorder) Header() http.Header { return w.header }

func (w *recorder) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}

func (w *recorder) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(b)
}

// reachable reports whether a board server answers at the configured URL,
// over the network even when the CLI is working on files.
func reachable() bool {
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(baseURL() + "/api/storage")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 300
}
