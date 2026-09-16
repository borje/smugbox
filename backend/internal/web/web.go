// Package web serves the built frontend from a directory with SPA fallback.
package web

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/bege/smugbox/backend/internal/httpx"
)

// encodings are the precompressed forms the frontend build writes next to
// each asset, best ratio first — on this bundle brotli beats zstd by about
// 5%. Serving these costs no CPU per request; a file without siblings goes
// out as is and is gzipped on the fly by the api middleware.
var encodings = []struct{ coding, ext string }{
	{"br", ".br"},
	{"zstd", ".zst"},
	{"gzip", ".gz"},
}

// Handler serves files from dir. Paths without a file extension that do not
// match a file fall back to index.html so client-side routes work. With an
// empty dir every request is answered 404.
func Handler(dir string) http.Handler {
	if dir == "" {
		return http.NotFoundHandler()
	}
	fsys := os.DirFS(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" {
			rel = "index.html"
		}
		if !fs.ValidPath(rel) {
			http.NotFound(w, r)
			return
		}
		if info, err := fs.Stat(fsys, rel); err == nil && !info.IsDir() {
			if strings.HasPrefix(rel, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			serveFile(w, r, fsys, rel)
			return
		}
		if path.Ext(rel) != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		serveFile(w, r, fsys, "index.html")
	})
}

// serveFile writes rel, preferring a precompressed sibling the client
// accepts. The sibling is served under rel's name so the Content-Type comes
// from the real extension rather than from .gz or .zst.
func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, rel string) {
	w.Header().Set("Vary", "Accept-Encoding")
	accept := r.Header.Get("Accept-Encoding")
	for _, enc := range encodings {
		if !httpx.Accepts(accept, enc.coding) {
			continue
		}
		f, info, ok := openSeekable(fsys, rel+enc.ext)
		if !ok {
			continue
		}
		defer f.Close()
		w.Header().Set("Content-Encoding", enc.coding)
		// ServeContent leaves Content-Length unset once Content-Encoding is
		// present, since it assumes the handler encodes as it writes. Here
		// the file on disk already is the encoded body, so its size is the
		// one to send. ServeContent overwrites this for a range request and
		// drops it on a 304.
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		http.ServeContent(w, r, rel, info.ModTime(), f)
		return
	}
	http.ServeFileFS(w, r, fsys, rel)
}

// openSeekable opens name if it is a regular, seekable file. ServeContent
// needs the seeker to answer range requests and to set Content-Length.
func openSeekable(fsys fs.FS, name string) (io.ReadSeekCloser, fs.FileInfo, bool) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, nil, false
	}
	info, err := f.Stat()
	rs, seekable := f.(io.ReadSeekCloser)
	if err != nil || !seekable || info.IsDir() {
		f.Close()
		return nil, nil, false
	}
	return rs, info, true
}
