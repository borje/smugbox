package theme

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const manifest = `{"smugbox": 1, "name": "Test"}`

func minimal() map[string]string {
	return map[string]string{"theme.json": manifest, "theme.css": "body{}"}
}

func writeDir(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeZip(t *testing.T, p string, files map[string]string) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func keys(m map[string][]byte) []string {
	var k []string
	for p := range m {
		k = append(k, p)
	}
	slices.Sort(k)
	return k
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	files := minimal()
	files["fonts/a.woff2"] = "font"
	writeDir(t, filepath.Join(dir, "demo"), files)
	th, err := Load("demo", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(th.Files); !slices.Equal(got, []string{"fonts/a.woff2", "theme.css"}) {
		t.Fatalf("files %v", got)
	}
	if th.ID != "demo" || th.Source != filepath.Join(dir, "demo") || len(th.Hash) != 8 {
		t.Fatalf("theme %+v", th)
	}
	// Defaults.
	if th.Manifest.Dark || th.Manifest.Gallery != "rows" || th.Manifest.Name != "Test" {
		t.Fatalf("manifest %+v", th.Manifest)
	}
}

func TestLoadZip(t *testing.T) {
	dir := t.TempDir()
	files := minimal()
	files["theme.json"] = `{"smugbox": 1, "name": "Z", "version": "1.0.0", "dark": true, "gallery": "masonry"}`
	writeZip(t, filepath.Join(dir, "demo.zip"), files)
	th, err := Load("demo", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if string(th.Files["theme.css"]) != "body{}" || th.Source != filepath.Join(dir, "demo.zip") {
		t.Fatalf("theme %+v", th)
	}
	if m := th.Manifest; !m.Dark || m.Gallery != "masonry" || m.Version != "1.0.0" {
		t.Fatalf("manifest %+v", m)
	}
}

func TestNestedFolderZip(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "demo.zip"), map[string]string{
		"demo-main/theme.json":    manifest,
		"demo-main/theme.css":     "x",
		"demo-main/img/bg.webp":   "w",
		"demo-main/README.md":     "readme",
		"demo-main/LICENSE":       "license",
		"demo-main/fonts/OFL.txt": "ofl",
	})
	th, err := Load("demo", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(th.Files); !slices.Equal(got, []string{"img/bg.webp", "theme.css"}) {
		t.Fatalf("files %v", got)
	}
	slices.Sort(th.Skipped)
	if !slices.Equal(th.Skipped, []string{"LICENSE", "README.md", "fonts/OFL.txt"}) {
		t.Fatalf("skipped %v", th.Skipped)
	}
}

func TestFinderZip(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "demo.zip"), map[string]string{
		"demo/theme.json":            manifest,
		"demo/theme.css":             "x",
		"demo/.DS_Store":             "junk",
		"demo/._theme.css":           "junk",
		"__MACOSX/demo/._theme.css":  "junk",
		"__MACOSX/demo/._theme.json": "junk",
	})
	th, err := Load("demo", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(th.Files); !slices.Equal(got, []string{"theme.css"}) || len(th.Skipped) != 0 {
		t.Fatalf("files %v skipped %v", got, th.Skipped)
	}
}

func TestOverCap(t *testing.T) {
	dir := t.TempDir()
	files := minimal()
	// Zeros compress to almost nothing: the cap must count bytes read.
	files["big.png"] = strings.Repeat("\x00", maxServedBytes)
	writeZip(t, filepath.Join(dir, "demo.zip"), files)
	if _, err := Load("demo", dir, ""); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("err %v", err)
	}
}

func TestEntryNameWithDotDot(t *testing.T) {
	dir := t.TempDir()
	files := minimal()
	files["../evil.css"] = "x"
	writeZip(t, filepath.Join(dir, "demo.zip"), files)
	if _, err := Load("demo", dir, ""); err == nil || !strings.Contains(err.Error(), "invalid entry name") {
		t.Fatalf("err %v", err)
	}
}

func TestSymlinkSkipped(t *testing.T) {
	dir := t.TempDir()
	writeDir(t, filepath.Join(dir, "demo"), minimal())
	secret := filepath.Join(dir, "secret.css")
	os.WriteFile(secret, []byte("secret"), 0o644)
	if err := os.Symlink(secret, filepath.Join(dir, "demo", "leak.css")); err != nil {
		t.Fatal(err)
	}
	th, err := Load("demo", dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := th.Files["leak.css"]; ok || !slices.Contains(th.Skipped, "leak.css") {
		t.Fatalf("files %v skipped %v", keys(th.Files), th.Skipped)
	}
}

func TestInvalidThemes(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"missing manifest": {map[string]string{"theme.css": "x"}, "no theme.json"},
		"invalid json":     {map[string]string{"theme.json": "{", "theme.css": "x"}, "theme.json"},
		"unknown key":      {map[string]string{"theme.json": `{"smugbox": 1, "name": "T", "accent": "red"}`, "theme.css": "x"}, "newer smugbox"},
		"unknown gallery":  {map[string]string{"theme.json": `{"smugbox": 1, "name": "T", "gallery": "grid"}`, "theme.css": "x"}, "gallery"},
		"api mismatch":     {map[string]string{"theme.json": `{"smugbox": 2, "name": "T"}`, "theme.css": "x"}, "not supported"},
		"no api version":   {map[string]string{"theme.json": `{"name": "T"}`, "theme.css": "x"}, "smugbox"},
		"no name":          {map[string]string{"theme.json": `{"smugbox": 1}`, "theme.css": "x"}, "name"},
		"no theme.css":     {map[string]string{"theme.json": manifest}, "theme.css"},
		"two top folders":  {map[string]string{"a/theme.json": manifest, "a/theme.css": "x", "b/x.css": "x"}, "no theme.json"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeDir(t, filepath.Join(dir, "demo"), tc.files)
			if _, err := Load("demo", dir, ""); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestUserOverridesBuiltin(t *testing.T) {
	builtin, user := t.TempDir(), t.TempDir()
	writeDir(t, filepath.Join(builtin, "demo"), minimal())
	writeZip(t, filepath.Join(user, "demo.zip"), minimal())
	th, err := Load("demo", builtin, user)
	if err != nil {
		t.Fatal(err)
	}
	if th.Source != filepath.Join(user, "demo.zip") {
		t.Fatalf("source %s", th.Source)
	}
	// Only in the built-in dir: found there.
	if th, err := Load("demo", builtin, t.TempDir()); err != nil || th.Source != filepath.Join(builtin, "demo") {
		t.Fatalf("%v %v", th, err)
	}
}

func TestZipAndDirWithSameID(t *testing.T) {
	dir := t.TempDir()
	writeDir(t, filepath.Join(dir, "demo"), minimal())
	writeZip(t, filepath.Join(dir, "demo.zip"), minimal())
	if _, err := Load("demo", dir, ""); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("err %v", err)
	}
}

func TestMissingTheme(t *testing.T) {
	if _, err := Load("demo", t.TempDir(), filepath.Join(t.TempDir(), "absent")); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err %v", err)
	}
}

func TestInvalidID(t *testing.T) {
	dir := t.TempDir()
	writeDir(t, filepath.Join(dir, "Demo"), minimal())
	for _, id := range []string{"", "Demo", "../demo", "-demo", "de mo", "demo.zip"} {
		if _, err := Load(id, dir, ""); err == nil || !strings.Contains(err.Error(), "must match") {
			t.Errorf("%q: err %v", id, err)
		}
	}
}

func TestHashChanges(t *testing.T) {
	load := func(files map[string]string) string {
		dir := t.TempDir()
		writeDir(t, filepath.Join(dir, "demo"), files)
		th, err := Load("demo", dir, "")
		if err != nil {
			t.Fatal(err)
		}
		return th.Hash
	}
	base := minimal()
	base["a.css"] = "abc"
	renamed := minimal()
	renamed["b.css"] = "abc"
	edited := minimal()
	edited["a.css"] = "abd"
	h := load(base)
	if load(base) != h {
		t.Fatal("hash not stable")
	}
	if load(renamed) == h || load(edited) == h {
		t.Fatal("hash did not change with a path or a byte")
	}
}

// Every theme in the repository must load, so a contract change that breaks
// one fails here.
func TestRepoThemes(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "themes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".zip")
		if junk(e.Name()) || (!e.IsDir() && id == e.Name()) {
			continue // README.md and the like
		}
		n++
		if _, err := Load(id, dir, ""); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
	}
	if n == 0 {
		t.Fatal("no themes found")
	}
}
