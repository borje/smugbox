package api

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bigJSON is comfortably over minCompressSize and compresses well.
var bigJSON = `{"items":[` + strings.Repeat(`{"name":"album","slug":"album"},`, 200) + `{}]}`

func runCompress(t *testing.T, accept string, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/albums", nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	rec := httptest.NewRecorder()
	compress(h).ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return string(b)
}

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, body)
	}
}

func TestCompressesJSON(t *testing.T) {
	rec := runCompress(t, "gzip, deflate", jsonHandler(bigJSON))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding %q", enc)
	}
	if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
		t.Fatalf("Vary %q", v)
	}
	if rec.Body.Len() >= len(bigJSON) {
		t.Fatalf("body grew: %d >= %d", rec.Body.Len(), len(bigJSON))
	}
	if got := gunzip(t, rec); got != bigJSON {
		t.Fatalf("round trip differs (%d vs %d bytes)", len(got), len(bigJSON))
	}
}

func TestCompressSkipsWhenClientCannotDecode(t *testing.T) {
	for _, accept := range []string{"", "gzip;q=0", "br"} {
		rec := runCompress(t, accept, jsonHandler(bigJSON))
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("accept %q: Content-Encoding %q", accept, enc)
		}
		if rec.Body.String() != bigJSON {
			t.Errorf("accept %q: body altered", accept)
		}
		// Vary still has to be there, or a cache could serve a gzipped body
		// from another client to this one.
		if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
			t.Errorf("accept %q: Vary %q", accept, v)
		}
	}
}

func TestCompressSkipsSmallAndBinaryBodies(t *testing.T) {
	small := `{"error":"not_found"}`
	rec := runCompress(t, "gzip", jsonHandler(small))
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("small body: Content-Encoding %q", enc)
	}
	if rec.Body.String() != small {
		t.Errorf("small body altered: %q", rec.Body.String())
	}

	jpeg := strings.Repeat("\xff\xd8binary", 500)
	rec = runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		io.WriteString(w, jpeg)
	})
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("jpeg: Content-Encoding %q", enc)
	}
	if v := rec.Header().Get("Vary"); v != "" {
		t.Errorf("jpeg: Vary %q, want none", v)
	}
	if rec.Body.String() != jpeg {
		t.Error("jpeg body altered")
	}
}

// Precompressed frontend assets arrive already encoded and must pass through.
func TestCompressLeavesEncodedResponsesAlone(t *testing.T) {
	rec := runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("Vary", "Accept-Encoding")
		io.WriteString(w, bigJSON)
	})
	if enc := rec.Header().Get("Content-Encoding"); enc != "zstd" {
		t.Fatalf("Content-Encoding %q", enc)
	}
	if got := rec.Header().Values("Vary"); len(got) != 1 {
		t.Fatalf("Vary %v, want one value", got)
	}
	if rec.Body.String() != bigJSON {
		t.Fatal("body altered")
	}
}

func TestCompressPreservesStatusAndEmptyBodies(t *testing.T) {
	rec := runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, bigJSON)
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding %q", enc)
	}
	if gunzip(t, rec) != bigJSON {
		t.Fatal("body differs")
	}

	rec = runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("304: code %d, %d bytes", rec.Code, rec.Body.Len())
	}
}

// A handler that flushes must not have its bytes stuck in the buffer waiting
// for minCompressSize.
func TestCompressFlushReleasesBufferedBytes(t *testing.T) {
	rec := runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		io.WriteString(w, "PK-first-chunk")
		w.(http.Flusher).Flush()
		if cw, ok := w.(*compressWriter); ok && !cw.begun {
			t.Error("Flush did not start the response")
		}
		io.WriteString(w, "-second-chunk")
	})
	if rec.Body.String() != "PK-first-chunk-second-chunk" {
		t.Fatalf("body %q", rec.Body.String())
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding %q", enc)
	}
}

func TestCompressDropsStaleContentLength(t *testing.T) {
	rec := runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprint(len(bigJSON)))
		io.WriteString(w, bigJSON)
	})
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("Content-Length %q, want none", cl)
	}
	if gunzip(t, rec) != bigJSON {
		t.Fatal("body differs")
	}
}

// End to end through the real handler chain: a visitor listing is gzipped,
// and image bytes are not.
func TestServerCompressesVisitorJSON(t *testing.T) {
	e := newEnvNoWorker(t)
	for i := range 40 {
		e.createAlbum(fmt.Sprintf("Album number %d with a reasonably long name", i))
	}
	req := httptest.NewRequest(http.MethodGet, "/api/albums", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding %q (body %d bytes)", enc, rec.Body.Len())
	}
	if !strings.Contains(gunzip(t, rec), "Album number 39") {
		t.Fatal("decompressed listing is missing an album")
	}
}

// internal/web sets Vary itself on static files; the middleware must not
// add a second copy when it passes such a response through.
func TestCompressDoesNotDuplicateVary(t *testing.T) {
	for _, existing := range []string{"Accept-Encoding", "accept-encoding", "Cookie, Accept-Encoding"} {
		rec := runCompress(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Vary", existing)
			io.WriteString(w, bigJSON)
		})
		if got := rec.Header().Values("Vary"); len(got) != 1 || got[0] != existing {
			t.Errorf("existing %q: Vary %v", existing, got)
		}
	}
}
