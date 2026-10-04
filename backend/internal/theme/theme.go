// Package theme loads the site theme: a zip or a folder holding theme.json,
// theme.css and assets (fonts, images). It is validated once at startup and
// held in memory, so requests never touch the theme on disk.
package theme

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// APIVersion is the only manifest "smugbox" value this core supports. It is
// bumped only when a token, slot or manifest key/value is removed or renamed.
const APIVersion = 1

const (
	maxServedBytes   = 10 << 20 // all served files together, counted on bytes read
	maxManifestBytes = 64 << 10
)

// ContentTypes is both the list of file types a theme may serve and the
// Content-Type each is served with. Other files are skipped.
var ContentTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".webp":  "image/webp",
	".avif":  "image/avif",
}

// Layouts are the gallery layouts the frontend holds props for.
var Layouts = []string{"rows", "columns", "masonry"}

// The id becomes a file name, so it is restricted to a safe alphabet.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Manifest is theme.json. It is also what the frontend receives, re-encoded
// from this struct rather than copied from the file.
type Manifest struct {
	Smugbox int    `json:"smugbox"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Dark    bool   `json:"dark"`
	Gallery string `json:"gallery"`
}

// Theme is a loaded, validated theme.
type Theme struct {
	ID       string
	Source   string // the zip or folder it was loaded from
	Manifest Manifest
	Files    map[string][]byte // served files by slash path, e.g. "fonts/x.woff2"
	Skipped  []string          // files present but not served, for the startup log
	Hash     string            // 8 hex chars; changes with any served path or byte
}

// Load finds theme id in userDir, then builtinDir (an empty dir is not
// searched), as <id>.zip or an unzipped <id>/ folder, and validates it.
func Load(id, builtinDir, userDir string) (*Theme, error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("theme id %q must match %s", id, idPattern)
	}
	var searched []string
	for _, dir := range []string{userDir, builtinDir} {
		if dir == "" {
			continue
		}
		searched = append(searched, dir)
		src, err := find(dir, id)
		if err != nil {
			return nil, err
		}
		if src != "" {
			t, err := loadFrom(src)
			if err != nil {
				return nil, fmt.Errorf("theme %s: %w", src, err)
			}
			t.ID, t.Source = id, src
			return t, nil
		}
	}
	return nil, fmt.Errorf("theme %q not found (looked for %s.zip and %s/ in %s)", id, id, id, strings.Join(searched, ", "))
}

// find returns the zip or folder for id in dir, or "" if dir has neither.
func find(dir, id string) (string, error) {
	zipPath, dirPath := filepath.Join(dir, id+".zip"), filepath.Join(dir, id)
	_, zipErr := os.Stat(zipPath)
	_, dirErr := os.Stat(dirPath)
	switch {
	case zipErr == nil && dirErr == nil:
		return "", fmt.Errorf("both %s and %s exist; remove one", zipPath, dirPath)
	case zipErr == nil:
		return zipPath, nil
	case dirErr == nil:
		return dirPath, nil
	}
	for _, err := range []error{zipErr, dirErr} {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

func loadFrom(src string) (*Theme, error) {
	if !strings.HasSuffix(src, ".zip") {
		return read(os.DirFS(src))
	}
	zr, err := zip.OpenReader(src)
	if err != nil {
		if zr != nil { // returned alongside zip.ErrInsecurePath
			zr.Close()
		}
		return nil, err
	}
	defer zr.Close()
	// The zip's fs.FS view quietly cleans names like "../x", so reject them
	// here rather than load something other than what the zip says.
	for _, f := range zr.File {
		if !fs.ValidPath(strings.TrimSuffix(f.Name, "/")) {
			return nil, fmt.Errorf("invalid entry name %q", f.Name)
		}
	}
	return read(zr)
}

// junk is what macOS Finder adds to folders and zips.
func junk(name string) bool {
	return strings.HasPrefix(name, ".") || name == "__MACOSX"
}

func read(fsys fs.FS) (*Theme, error) {
	root, err := themeRoot(fsys)
	if err != nil {
		return nil, err
	}
	m, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	t := &Theme{Manifest: m, Files: map[string][]byte{}}
	remaining := int64(maxServedBytes)
	err = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && junk(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || p == "theme.json" {
			return nil
		}
		// Symlinks and other non-regular files are never followed or served.
		if !d.Type().IsRegular() || ContentTypes[strings.ToLower(path.Ext(p))] == "" {
			t.Skipped = append(t.Skipped, p)
			return nil
		}
		b, err := readFile(root, p, remaining)
		if errors.Is(err, errTooLarge) {
			return fmt.Errorf("served files exceed %d MB", maxServedBytes>>20)
		}
		if err != nil {
			return err
		}
		remaining -= int64(len(b))
		t.Files[p] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := t.Files["theme.css"]; !ok {
		return nil, errors.New("no theme.css next to theme.json")
	}
	t.Hash = hash(t.Files)
	return t, nil
}

// themeRoot is fsys itself when theme.json is at its top level, or its one
// top-level folder (GitHub's "Download ZIP" yields name-main/...).
func themeRoot(fsys fs.FS) (fs.FS, error) {
	if _, err := fs.Stat(fsys, "theme.json"); err == nil {
		return fsys, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	entries = slices.DeleteFunc(entries, func(e fs.DirEntry) bool { return junk(e.Name()) })
	if len(entries) == 1 && entries[0].IsDir() {
		sub, err := fs.Sub(fsys, entries[0].Name())
		if err != nil {
			return nil, err
		}
		if _, err := fs.Stat(sub, "theme.json"); err == nil {
			return sub, nil
		}
	}
	return nil, errors.New("no theme.json at the top level or inside a single top-level folder")
}

func readManifest(fsys fs.FS) (Manifest, error) {
	var m Manifest
	b, err := readFile(fsys, "theme.json", maxManifestBytes)
	if err != nil {
		return m, fmt.Errorf("theme.json: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return m, fmt.Errorf("theme.json: %w (the theme may need a newer smugbox)", err)
		}
		return m, fmt.Errorf("theme.json: %w", err)
	}
	if m.Gallery == "" {
		m.Gallery = "rows"
	}
	switch {
	case m.Smugbox == 0:
		return m, errors.New(`theme.json: "smugbox" is required`)
	case m.Smugbox != APIVersion:
		return m, fmt.Errorf(`theme.json: "smugbox": %d is not supported; this smugbox supports %d`, m.Smugbox, APIVersion)
	case strings.TrimSpace(m.Name) == "":
		return m, errors.New(`theme.json: "name" is required`)
	case !slices.Contains(Layouts, m.Gallery):
		return m, fmt.Errorf(`theme.json: "gallery" must be one of %s`, strings.Join(Layouts, ", "))
	}
	return m, nil
}

var errTooLarge = errors.New("too large")

// readFile reads at most limit bytes, counting what is actually read rather
// than trusting a size from the zip header.
func readFile(fsys fs.FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errTooLarge
	}
	return b, nil
}

func hash(files map[string][]byte) string {
	h := sha256.New()
	for _, p := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(files[p]))
		h.Write(files[p])
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}
