package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bege/smugbox/backend/internal/theme"
)

const indexHTML = "<!doctype html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"UTF-8\" />\n  </head>\n  <body>app</body>\n</html>\n"

func testTheme(dark bool) *theme.Theme {
	return &theme.Theme{
		ID:       "demo",
		Manifest: theme.Manifest{Smugbox: 1, Name: "Demo", Dark: dark, Gallery: "masonry"},
		Files: map[string][]byte{
			"theme.css":     []byte("body{}"),
			"fonts/a.woff2": []byte("font"),
		},
		Hash: "abcd1234",
	}
}

// handler serves dir with the title "Site" and a light test theme.
func handler(t *testing.T, dir string) http.Handler {
	t.Helper()
	h, err := Handler(dir, "Site", testTheme(false))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app-abc123.js"), []byte("console.log(1)"), 0o644)
	os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644)
	return dir
}

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestServesFilesAndSPAFallback(t *testing.T) {
	h := handler(t, setup(t))
	index := get(h, http.MethodGet, "/").Body.String()
	if !strings.Contains(index, "<title>Site</title>") {
		t.Fatalf("index not rewritten: %q", index)
	}
	for _, p := range []string{"/", "/index.html", "/a/some-album", "/a/x/y", "/nested/route"} {
		rec := get(h, http.MethodGet, p)
		if rec.Code != 200 || rec.Body.String() != index {
			t.Errorf("%s: code %d body %q", p, rec.Code, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control %q", p, cc)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: Content-Type %q", p, ct)
		}
	}
	rec := get(h, http.MethodGet, "/assets/app-abc123.js")
	if rec.Code != 200 || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("asset content type %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset cache control %q", cc)
	}
	if rec := get(h, http.MethodGet, "/favicon.svg"); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("favicon: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := get(h, http.MethodGet, "/missing.png"); rec.Code != 404 {
		t.Fatalf("missing file with extension should 404, got %d", rec.Code)
	}
	if rec := get(h, http.MethodGet, "/assets/"); rec.Code != 200 || rec.Body.String() != index {
		t.Fatalf("directory path should fall back to index, got %d", rec.Code)
	}
	if rec := get(h, http.MethodPost, "/"); rec.Code != 405 {
		t.Fatalf("POST should be 405, got %d", rec.Code)
	}
}

func TestTraversalIsNeutralised(t *testing.T) {
	dir := setup(t)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("secret"), 0o644)
	h := handler(t, dir)
	for _, p := range []string{"/../secret.txt", "/assets/../../secret.txt", "/%2e%2e/secret.txt"} {
		req := httptest.NewRequest(http.MethodGet, "http://x"+p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "secret") {
			t.Fatalf("%s leaked file outside dir", p)
		}
	}
}

func TestNoFrontendDir(t *testing.T) {
	h, err := Handler("", "Site", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/", "/a/x", "/index.html"} {
		if rec := get(h, http.MethodGet, p); rec.Code != 404 {
			t.Fatalf("%s: %d, want 404", p, rec.Code)
		}
	}
	// A configured dir without index.html is a startup error.
	if _, err := Handler(t.TempDir(), "Site", testTheme(false)); err == nil {
		t.Fatal("missing index.html accepted")
	}
}

// site returns the JSON in the index's smugbox-site script element.
func site(t *testing.T, index string) map[string]any {
	t.Helper()
	_, rest, ok := strings.Cut(index, `<script type="application/json" id="smugbox-site">`)
	body, _, ok2 := strings.Cut(rest, "</script>")
	if !ok || !ok2 {
		t.Fatalf("no site JSON in %q", index)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("site JSON %q: %v", body, err)
	}
	return v
}

func TestIndexRewrite(t *testing.T) {
	dir := setup(t)
	h, err := Handler(dir, "Site", testTheme(true))
	if err != nil {
		t.Fatal(err)
	}
	index := get(h, http.MethodGet, "/").Body.String()
	for _, want := range []string{
		"<title>Site</title>",
		`<link rel="stylesheet" href="/theme/abcd1234/theme.css">`,
		`<html class="dark" lang="en">`,
		`id="smugbox-site"`,
	} {
		if n := strings.Count(index, want); n != 1 {
			t.Errorf("%q appears %d times in %q", want, n, index)
		}
	}
	// The tags go inside <head>.
	if strings.Index(index, "id=\"smugbox-site\"") > strings.Index(index, "</head>") {
		t.Errorf("site JSON after </head>: %q", index)
	}
	v := site(t, index)
	th := v["theme"].(map[string]any)
	if v["title"] != "Site" || th["name"] != "Demo" || th["gallery"] != "masonry" || th["dark"] != true || th["smugbox"] != 1.0 {
		t.Fatalf("site JSON %v", v)
	}

	// Light theme: no class on <html>.
	index = get(handler(t, dir), http.MethodGet, "/").Body.String()
	if strings.Contains(index, "class=") {
		t.Fatalf("light theme index has a class: %q", index)
	}
}

func TestIndexEscapesTitleAndManifest(t *testing.T) {
	th := testTheme(false)
	th.Manifest.Name = "</script><script>alert(1)</script>"
	h, err := Handler(setup(t), "<b>Site</b>", th)
	if err != nil {
		t.Fatal(err)
	}
	index := get(h, http.MethodGet, "/").Body.String()
	if strings.Contains(index, "<b>") || strings.Contains(index, "alert(1)</script>") || strings.Count(index, "</script>") != 1 {
		t.Fatalf("unescaped: %q", index)
	}
	if !strings.Contains(index, "<title>&lt;b&gt;Site&lt;/b&gt;</title>") {
		t.Fatalf("title: %q", index)
	}
	v := site(t, index)
	if v["title"] != "<b>Site</b>" || v["theme"].(map[string]any)["name"] != th.Manifest.Name {
		t.Fatalf("site JSON does not round-trip: %v", v)
	}
}

func TestThemeFiles(t *testing.T) {
	h := handler(t, setup(t))
	for p, ct := range map[string]string{
		"/theme/abcd1234/theme.css":     "text/css; charset=utf-8",
		"/theme/abcd1234/fonts/a.woff2": "font/woff2",
	} {
		rec := get(h, http.MethodGet, p)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != ct {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Errorf("%s: Cache-Control %q", p, cc)
		}
	}
	for _, p := range []string{
		"/theme/abcd1234/missing.css",
		"/theme/abcd1234/fonts/",
		"/theme/abcd1234/fonts",
		"/theme/abcd1234/",
		"/theme/00000000/theme.css",
		"/theme/",
	} {
		if rec := get(h, http.MethodGet, p); rec.Code != 404 {
			t.Errorf("%s: %d, want 404", p, rec.Code)
		}
	}
}

// precompressed writes a dist-like tree where the JS bundle and index.html
// have the sibling files the frontend build produces.
func precompressed(t *testing.T) string {
	t.Helper()
	dir := setup(t)
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("assets/app-abc123.js.gz", "gzip-bytes")
	write("assets/app-abc123.js.br", "brotli-bytes")
	write("assets/app-abc123.js.zst", "zstd-bytes")
	write("index.html.gz", "gzip-html")
	return dir
}

func getEnc(h http.Handler, path, accept string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesPrecompressedSiblings(t *testing.T) {
	h := handler(t, precompressed(t))
	tests := []struct {
		accept   string
		encoding string
		body     string
	}{
		{"br, zstd, gzip", "br", "brotli-bytes"},
		{"zstd, gzip", "zstd", "zstd-bytes"},
		{"gzip, br", "br", "brotli-bytes"},
		{"gzip", "gzip", "gzip-bytes"},
		{"gzip;q=0", "", "console.log(1)"},
		{"", "", "console.log(1)"},
		{"deflate", "", "console.log(1)"},
	}
	for _, tt := range tests {
		rec := getEnc(h, "/assets/app-abc123.js", tt.accept)
		if rec.Code != 200 || rec.Body.String() != tt.body {
			t.Errorf("accept %q: code %d body %q, want %q", tt.accept, rec.Code, rec.Body.String(), tt.body)
		}
		if enc := rec.Header().Get("Content-Encoding"); enc != tt.encoding {
			t.Errorf("accept %q: Content-Encoding %q, want %q", tt.accept, enc, tt.encoding)
		}
		// The content type must come from the real extension, not from .gz.
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
			t.Errorf("accept %q: Content-Type %q", tt.accept, ct)
		}
		if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
			t.Errorf("accept %q: Vary %q", tt.accept, v)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Errorf("accept %q: Cache-Control %q", tt.accept, cc)
		}
		if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(tt.body)) {
			t.Errorf("accept %q: Content-Length %q, want %d", tt.accept, cl, len(tt.body))
		}
	}
}

// The rewritten index is served from memory; the build's precompressed
// index.html siblings hold the unrewritten file and must be ignored.
func TestIndexIgnoresPrecompressedSiblings(t *testing.T) {
	h := handler(t, precompressed(t))
	for _, p := range []string{"/", "/index.html", "/a/some-album"} {
		rec := getEnc(h, p, "gzip")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Site</title>") {
			t.Errorf("%s: code %d body %q", p, rec.Code, rec.Body.String())
		}
		if enc := rec.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("%s: Content-Encoding %q", p, enc)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: Content-Type %q", p, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: Cache-Control %q", p, cc)
		}
	}
}

func TestPrecompressedIsNotServedForItsOwnPath(t *testing.T) {
	// A request for the sibling itself must not pick up a second encoding.
	rec := getEnc(handler(t, precompressed(t)), "/assets/app-abc123.js.gz", "gzip")
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding %q, want none", enc)
	}
	if rec.Body.String() != "gzip-bytes" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestPrecompressedRevalidates(t *testing.T) {
	h := handler(t, precompressed(t))
	rec := getEnc(h, "/assets/app-abc123.js", "gzip")
	mod := rec.Header().Get("Last-Modified")
	if mod == "" {
		t.Fatal("no Last-Modified")
	}
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-Modified-Since", mod)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("code %d, want 304", rec.Code)
	}
}

// A missing sibling must fall through to the plain file rather than 404.
func TestMissingSiblingFallsBackToPlainFile(t *testing.T) {
	rec := getEnc(handler(t, setup(t)), "/assets/app-abc123.js", "zstd, br, gzip")
	if rec.Code != 200 || rec.Body.String() != "console.log(1)" {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding %q, want none", enc)
	}
	if v := rec.Header().Get("Vary"); v != "Accept-Encoding" {
		t.Fatalf("Vary %q", v)
	}
}
