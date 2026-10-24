package image

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	vips "github.com/cshum/vipsgen/vips816"
)

func loadDims(t *testing.T, path string) (w, h int, img *vips.Image) {
	t.Helper()
	img, err := vips.NewImageFromFile(path, &vips.LoadOptions{})
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return img.Width(), img.Height(), img
}

func TestResize(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	// Landscape pixels with orientation 6 -> displayed as 2000x3000 portrait.
	writeFixture(t, src, 3000, 2000, 6)
	before, _ := os.ReadFile(src)

	dst := filepath.Join(dir, "medium.jpg")
	if err := Resize(src, dst, 1600, 85, 0); err != nil {
		t.Fatal(err)
	}
	w, h, img := loadDims(t, dst)
	defer img.Close()
	if w != 1067 || h != 1600 {
		t.Errorf("%dx%d, want 1067x1600", w, h)
	}
	if img.Orientation() > 1 {
		t.Errorf("orientation tag %d left in output", img.Orientation())
	}
	if img.Interpretation() != vips.InterpretationSrgb {
		t.Errorf("interpretation %v, want sRGB", img.Interpretation())
	}
	if exif := img.Exif(); len(exif) != 0 {
		t.Errorf("EXIF not stripped: %v", exif)
	}
	if !img.HasICCProfile() {
		t.Error("no ICC profile attached")
	}

	blur := filepath.Join(dir, "blur.jpg")
	if err := Resize(src, blur, 40, 40, 2.0); err != nil {
		t.Fatal(err)
	}
	w, h, bimg := loadDims(t, blur)
	bimg.Close()
	if w != 27 || h != 40 {
		t.Errorf("blur %dx%d, want 27x40", w, h)
	}
	if st, _ := os.Stat(blur); st.Size() >= 2048 {
		t.Errorf("blur is %d bytes, want < 2 kB", st.Size())
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Fatal("source file modified")
	}
}

func TestResizeNeverUpscales(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	writeFixture(t, src, 1200, 800, 1)
	dst := filepath.Join(dir, "large.jpg")
	if err := Resize(src, dst, 2560, 85, 0); err != nil {
		t.Fatal(err)
	}
	w, h, img := loadDims(t, dst)
	img.Close()
	if w != 1200 || h != 800 {
		t.Fatalf("%dx%d, want 1200x800", w, h)
	}
}
