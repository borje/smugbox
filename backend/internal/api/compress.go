package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bege/smugbox/backend/internal/httpx"
)

// minCompressSize is the smallest body worth gzipping. Below it the gzip
// header and trailer cost more than the encoding saves.
const minCompressSize = 1024

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

// compressible reports whether a Content-Type is worth gzipping. JPEG
// variants and zip downloads are already compressed, so spending CPU on
// them would buy nothing.
func compressible(contentType string) bool {
	ct, _, _ := strings.Cut(contentType, ";")
	ct = strings.TrimSpace(strings.ToLower(ct))
	switch ct {
	case "application/json", "application/javascript", "application/xml", "image/svg+xml":
		return true
	}
	return strings.HasPrefix(ct, "text/")
}

// variesOnEncoding reports whether Vary already names Accept-Encoding, which
// internal/web does for the static files it may serve precompressed.
func variesOnEncoding(h http.Header) bool {
	for _, v := range h.Values("Vary") {
		for field := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(field), "Accept-Encoding") {
				return true
			}
		}
	}
	return false
}

// compress gzips text responses for clients that accept it. Responses that
// carry a Content-Encoding already — the precompressed frontend assets from
// internal/web — pass through untouched.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &compressWriter{
			ResponseWriter: w,
			gzipOK:         httpx.Accepts(r.Header.Get("Accept-Encoding"), "gzip"),
		}
		defer cw.close()
		next.ServeHTTP(cw, r)
	})
}

// compressWriter buffers the start of a response so it can see the final
// headers and the body size before choosing an encoding. Nothing reaches
// the client until begin has run.
type compressWriter struct {
	http.ResponseWriter
	gzipOK bool

	status int
	buf    []byte
	begun  bool
	gz     *gzip.Writer // nil once begin decides not to compress
}

func (w *compressWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *compressWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.begun {
		w.buf = append(w.buf, b...)
		if err := w.begin(false); err != nil {
			return 0, err
		}
		return len(b), nil
	}
	if w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// begin picks the encoding, sends the headers and flushes the buffered
// prefix. While final is false it waits for more of the body, since a
// response that stays under minCompressSize is sent uncompressed.
func (w *compressWriter) begin(final bool) error {
	if w.begun {
		return nil
	}
	if !final && len(w.buf) < minCompressSize {
		return nil
	}
	w.begun = true

	h := w.Header()
	if compressible(h.Get("Content-Type")) && h.Get("Content-Encoding") == "" {
		// Vary goes on every response that could have been compressed, not
		// only the ones that were, so a shared cache never hands a gzipped
		// body to a client that cannot read it.
		if !variesOnEncoding(h) {
			h.Add("Vary", "Accept-Encoding")
		}
		if w.gzipOK && len(w.buf) >= minCompressSize {
			h.Set("Content-Encoding", "gzip")
			h.Del("Content-Length")
			w.gz = gzipPool.Get().(*gzip.Writer)
			w.gz.Reset(w.ResponseWriter)
		}
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)

	buf := w.buf
	w.buf = nil
	if len(buf) == 0 {
		return nil
	}
	var err error
	if w.gz != nil {
		_, err = w.gz.Write(buf)
	} else {
		_, err = w.ResponseWriter.Write(buf)
	}
	return err
}

// Flush ends the buffering window: a handler that streams gets its bytes
// out rather than sitting in the buffer waiting for minCompressSize.
func (w *compressWriter) Flush() {
	_ = w.begin(true)
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *compressWriter) close() {
	_ = w.begin(true)
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(io.Discard)
	gzipPool.Put(w.gz)
	w.gz = nil
}

func (w *compressWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
