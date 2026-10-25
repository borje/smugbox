package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bege/smugbox/backend/internal/video"
)

// testVideo encodes a two-second 320x240 clip; the container follows ext.
func testVideo(t *testing.T, ext string) []byte {
	t.Helper()
	if !video.Available() {
		t.Skip("ffmpeg not found in PATH")
	}
	path := filepath.Join(t.TempDir(), "clip"+ext)
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:duration=2",
		"-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUploadAndServeVideo(t *testing.T) {
	e := newEnv(t)
	a := e.createAlbum("Clips")
	ctx := context.Background()
	mp4 := testVideo(t, ".mp4")
	id, code := e.uploadPhoto(a.ID, "lr-v1", "CLIP.mp4", mp4, nil)
	if code != 201 {
		t.Fatalf("upload code %d", code)
	}
	p, _ := e.db.GetPhoto(ctx, a.ID, id)
	if p.MimeType != "video/mp4" || p.Width != 320 || p.Height != 240 || p.DurationMs < 1900 || !p.VariantsReady {
		t.Fatalf("photo row: %+v", p)
	}
	dir := filepath.Join(e.store.Root(), "photos", a.ID, id)
	if stored, err := os.ReadFile(filepath.Join(dir, "original.mp4")); err != nil || !bytes.Equal(stored, mp4) {
		t.Fatalf("original.mp4 not byte-identical: %v", err)
	}

	var album albumDetail
	decode(t, e.get("/api/albums/"+a.Slug), &album)
	if len(album.Photos) != 1 || album.Photos[0].MimeType != "video/mp4" || album.Photos[0].Duration != p.DurationMs {
		t.Fatalf("album json: %+v", album.Photos)
	}
	// A video has a large variant even when small: its original is no image.
	if s := album.Photos[0].Sizes.Large; s.Width != 320 || s.Height != 240 {
		t.Fatalf("large size: %+v", s)
	}

	base := "/api/albums/" + a.Slug + "/photos/" + id + "/"
	for _, v := range []string{"thumb", "small", "medium", "large", "blur"} {
		rec := e.get(base + v)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || !bytes.HasPrefix(rec.Body.Bytes(), []byte{0xFF, 0xD8}) {
			t.Fatalf("%s: %d %s", v, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	req := httptest.NewRequest(http.MethodGet, base+"original", nil)
	req.Header.Set("Range", "bytes=0-99")
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Type") != "video/mp4" || rec.Body.Len() != 100 {
		t.Fatalf("range: %d %s %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}

	// Replacing the video with a JPEG leaves no original.mp4 behind.
	rec = e.upload(http.MethodPut, "/api/publish/albums/"+a.ID+"/photos/"+id, nil, testJPEG(t, 40, 30, 0), "CLIP.jpg")
	if rec.Code != 200 {
		t.Fatalf("replace: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "original.mp4")); !os.IsNotExist(err) {
		t.Fatalf("original.mp4 left behind: %v", err)
	}
	if p, _ = e.db.GetPhoto(ctx, a.ID, id); p.MimeType != "image/jpeg" || p.DurationMs != 0 {
		t.Fatalf("after replace: %+v", p)
	}
}

func TestUploadQuickTimeAndBadVideo(t *testing.T) {
	e := newEnv(t)
	a := e.createAlbum("Clips")
	id, _ := e.uploadPhoto(a.ID, "lr-mov", "CLIP.mov", testVideo(t, ".mov"), nil)
	if p, _ := e.db.GetPhoto(context.Background(), a.ID, id); p.MimeType != "video/quicktime" {
		t.Fatalf("mime %q", p.MimeType)
	}
	if _, err := os.Stat(filepath.Join(e.store.Root(), "photos", a.ID, id, "original.mov")); err != nil {
		t.Fatal(err)
	}
	rec := e.upload(http.MethodPost, "/api/publish/albums/"+a.ID+"/photos", map[string]string{"lr_photo_uuid": "lr-bad"},
		[]byte("\x00\x00\x00\x18ftypisom truncated, no video here"), "bad.mp4")
	if rec.Code != 422 {
		t.Fatalf("bad video: %d %s", rec.Code, rec.Body.String())
	}

	// Headers intact, frames cut off: ffprobe passes it, decoding does not.
	full := filepath.Join(t.TempDir(), "fast.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:duration=2",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", full).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	b, _ := os.ReadFile(full)
	cut := b[:bytes.Index(b, []byte("mdat"))+64]
	rec = e.upload(http.MethodPost, "/api/publish/albums/"+a.ID+"/photos", map[string]string{"lr_photo_uuid": "lr-cut"}, cut, "cut.mp4")
	if rec.Code != 422 {
		t.Fatalf("truncated video: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUploadVideoWithoutFFprobe(t *testing.T) {
	e := newEnv(t)
	a := e.createAlbum("Clips")
	mp4 := testVideo(t, ".mp4")
	t.Setenv("PATH", "")
	rec := e.upload(http.MethodPost, "/api/publish/albums/"+a.ID+"/photos", map[string]string{"lr_photo_uuid": "lr-v"}, mp4, "a.mp4")
	if rec.Code != 500 {
		t.Fatalf("no ffprobe: %d %s, want 500", rec.Code, rec.Body.String())
	}
}
