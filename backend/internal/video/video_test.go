package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// fixture encodes a w x h test clip of two seconds; the container follows
// the extension of name.
func fixture(t *testing.T, name string, w, h int) string {
	t.Helper()
	if !Available() {
		t.Skip("ffmpeg not found in PATH")
	}
	path := filepath.Join(t.TempDir(), name)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc=size=" + strconv.Itoa(w) + "x" + strconv.Itoa(h) + ":duration=2",
		"-pix_fmt", "yuv420p", path}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	return path
}

func head(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b[:12]
}

func TestSniff(t *testing.T) {
	mp4 := fixture(t, "a.mp4", 320, 240)
	mov := fixture(t, "a.mov", 320, 240)
	if m, ok := Sniff(head(t, mp4)); !ok || m != "video/mp4" {
		t.Errorf("mp4: %q %v", m, ok)
	}
	if m, ok := Sniff(head(t, mov)); !ok || m != "video/quicktime" {
		t.Errorf("mov: %q %v", m, ok)
	}
	if _, ok := Sniff([]byte{0xFF, 0xD8, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0}); ok {
		t.Error("jpeg sniffed as video")
	}
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	info, err := Probe(ctx, fixture(t, "a.mp4", 320, 240))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 320 || info.Height != 240 || info.DurationMs < 1900 || info.DurationMs > 2100 {
		t.Errorf("got %+v", info)
	}

	plain := fixture(t, "plain.mp4", 320, 240)
	rotated := filepath.Join(t.TempDir(), "rot.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-display_rotation", "90", "-i", plain, "-c", "copy", rotated).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	if info, err := Probe(ctx, rotated); err != nil || info.Width != 240 || info.Height != 320 {
		t.Errorf("rotated: %+v %v", info, err)
	}

	junk := filepath.Join(t.TempDir(), "junk.mp4")
	os.WriteFile(junk, []byte("\x00\x00\x00\x18ftypisomnot really a video"), 0o644)
	if _, err := Probe(ctx, junk); err == nil {
		t.Error("junk probed fine")
	}
}

func TestPoster(t *testing.T) {
	src := fixture(t, "a.mp4", 320, 240)
	dst := filepath.Join(t.TempDir(), "poster.tmp")
	if err := Poster(context.Background(), src, dst, 2000); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil || len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Fatalf("poster is not a JPEG: %v", err)
	}
	// No duration known: the first frame.
	if err := Poster(context.Background(), src, dst, 0); err != nil {
		t.Fatal(err)
	}
}
