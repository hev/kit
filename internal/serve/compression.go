package serve

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// Compression preserves the API representation and all preview data. Clients
// that do not negotiate gzip continue to receive the same plain JSON/HTML.
func compressResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r.Header.Values("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer func() {
			if gw.gzip != nil {
				_ = gw.gzip.Close()
			}
		}()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(values []string) bool {
	wildcard := false
	for _, value := range values {
		for _, coding := range strings.Split(value, ",") {
			parts := strings.Split(coding, ";")
			name := strings.ToLower(strings.TrimSpace(parts[0]))
			quality := 1.0
			for _, param := range parts[1:] {
				key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
				if ok && strings.EqualFold(key, "q") {
					q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
					quality = 0
					if err == nil && q >= 0 && q <= 1 {
						quality = q
					}
				}
			}
			if name == "gzip" {
				return quality > 0
			}
			if name == "*" {
				wildcard = quality > 0
			}
		}
	}
	return wildcard
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gzip        *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
	w.gzip, _ = gzip.NewWriterLevel(w.ResponseWriter, gzip.BestSpeed)
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.gzip.Write(p)
}
