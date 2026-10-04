// Package web serves the built frontend from a directory with SPA fallback,
// and the site theme under /theme/<hash>/.
package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/bege/smugbox/backend/internal/httpx"
	"github.com/bege/smugbox/backend/internal/theme"
)

// encodings are the precompressed forms the frontend build writes next to
// each asset, best ratio first — on this bundle brotli beats zstd by about
// 5%. Serving these costs no CPU per request; a file without siblings goes
// out as is, since nothing compresses responses on the fly.
var encodings = []struct{ coding, ext string }{
	{"br", ".br"},
	{"zstd", ".zst"},
	{"gzip", ".gz"},
}

// Handler serves files from dir. Paths without a file extension that do not
// match a file fall back to index.html so client-side routes work. With an
// empty dir every request is answered 404 and th may be nil.
//
// index.html is read once and rewritten with title and th (see
// rewriteIndex); that copy is served from memory, uncompressed, for "/",
// "/index.html" and the fallback. th's files are served from memory under
// /theme/<hash>/ with immutable caching.
func Handler(dir, title string, th *theme.Theme) (http.Handler, error) {
	if dir == "" {
		return http.NotFoundHandler(), nil
	}
	fsys := os.DirFS(dir)
	raw, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		return nil, fmt.Errorf("frontend: %w", err)
	}
	index, err := rewriteIndex(raw, title, th)
	if err != nil {
		return nil, err
	}
	themePrefix := "theme/" + th.Hash + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "index.html" {
			serveIndex(w, r, index)
			return
		}
		if !fs.ValidPath(rel) {
			http.NotFound(w, r)
			return
		}
		if rel == "theme" || strings.HasPrefix(rel, "theme/") {
			// Exact keys only: a directory, an old hash or an unknown file 404s.
			name, ok := strings.CutPrefix(rel, themePrefix)
			b, found := th.Files[name]
			if !ok || !found {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", theme.ContentTypes[strings.ToLower(path.Ext(name))])
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
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
		serveIndex(w, r, index)
	}), nil
}

func serveIndex(w http.ResponseWriter, r *http.Request, index []byte) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}

// rewriteIndex inserts the site title, the theme stylesheet and the site
// JSON before </head>, and class="dark" on <html> for a dark theme, so the
// first paint already has the theme. The JSON is marshalled from the
// validated manifest; json.Marshal escapes <, > and &, so no string in it
// can end the script element.
func rewriteIndex(raw []byte, title string, th *theme.Theme) ([]byte, error) {
	s := string(raw)
	if strings.Count(s, "</head>") != 1 || strings.Count(s, "<html") != 1 {
		return nil, errors.New("frontend index.html: expected exactly one <html> and one </head>")
	}
	site, err := json.Marshal(struct {
		Title string         `json:"title"`
		Theme theme.Manifest `json:"theme"`
	}{title, th.Manifest})
	if err != nil {
		return nil, err
	}
	head := "  <title>" + html.EscapeString(title) + "</title>\n" +
		`    <link rel="stylesheet" href="/theme/` + th.Hash + `/theme.css">` + "\n" +
		`    <script type="application/json" id="smugbox-site">` + string(site) + "</script>\n  "
	s = strings.Replace(s, "</head>", head+"</head>", 1)
	if th.Manifest.Dark {
		s = strings.Replace(s, "<html", `<html class="dark"`, 1)
	}
	return []byte(s), nil
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
