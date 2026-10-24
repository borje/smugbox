package library

import (
	"bytes"
	"context"
	"errors"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/image"
	"github.com/bege/smugbox/backend/internal/storage"
)

func TestMain(m *testing.M) {
	image.Startup()
	code := m.Run()
	image.Shutdown()
	os.Exit(code)
}

type env struct {
	t     *testing.T
	lib   *Library
	db    *db.DB
	store *storage.Store
	album *db.Album
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(context.Background(), filepath.Join(dir, "smugbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	a := &db.Album{ID: "00000000-0000-0000-0000-00000000000a", Slug: "trip", Name: "Trip", IsListed: true, CreatedAt: now, UpdatedAt: now}
	if err := database.CreateAlbum(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	lib := New(Deps{DB: database, Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }})
	return &env{t: t, lib: lib, db: database, store: store, album: a}
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ingest stages a w×h JPEG and ingests it; photoID "" creates a photo.
func (e *env) ingest(photoID string, w, h int) (*db.Photo, error) {
	e.t.Helper()
	up, err := e.lib.Stage(bytes.NewReader(testJPEG(e.t, w, h)), "IMG.jpg")
	if err != nil {
		e.t.Fatal(err)
	}
	defer up.Abort()
	p, _, err := e.lib.Ingest(context.Background(), e.album, photoID, map[string]string{"lr_photo_uuid": "lr-1"}, up)
	return p, err
}

func (e *env) photo(id string) *db.Photo {
	e.t.Helper()
	p, err := e.db.GetPhoto(context.Background(), e.album.ID, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// A replace that arrives while the worker is rendering the same photo must
// wait for it. Otherwise the render can mark the photo ready between the
// replace hiding it and writing the new content hash, and visitors see
// variants of the old original under the new one, with nothing re-queuing
// the photo.
func TestReplaceWaitsForRenderInFlight(t *testing.T) {
	e := newEnv(t)
	p, err := e.ingest("", 800, 600)
	if err != nil {
		t.Fatal(err)
	}

	rendered, release := make(chan struct{}), make(chan struct{})
	e.lib.derive = func(src string, info image.Info, want []storage.Variant, dst func(storage.Variant) (string, error)) ([]storage.Variant, error) {
		produced, err := image.Derive(src, info, want, dst)
		close(rendered)
		<-release
		return produced, err
	}
	renderDone := make(chan error, 1)
	go func() { renderDone <- e.lib.Render(context.Background(), e.album.ID, p.ID) }()
	<-rendered

	replaceDone := make(chan error, 1)
	go func() {
		_, err := e.ingest(p.ID, 600, 800)
		replaceDone <- err
	}()
	select {
	case err := <-replaceDone:
		t.Fatalf("replace finished while a render of the same photo was in flight (err %v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-renderDone; err != nil {
		t.Fatal(err)
	}
	if err := <-replaceDone; err != nil {
		t.Fatal(err)
	}

	got := e.photo(p.ID)
	if got.VariantsReady || got.Width != 600 || got.ContentHash == p.ContentHash {
		t.Fatalf("after replace: ready=%v width=%d hash changed=%v, want pending 600-wide new original", got.VariantsReady, got.Width, got.ContentHash != p.ContentHash)
	}
	e.lib.derive = image.Derive
	if err := e.lib.Render(context.Background(), e.album.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if !e.photo(p.ID).VariantsReady {
		t.Fatal("photo not ready after rendering the new original")
	}
	thumb, err := e.store.Path(e.album.ID, p.ID, storage.Thumb)
	if err != nil {
		t.Fatal(err)
	}
	info, err := image.Probe(thumb)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width >= info.Height {
		t.Fatalf("thumb is %dx%d, want portrait like the new original", info.Width, info.Height)
	}
	if n := len(e.lib.locks.m); n != 0 {
		t.Fatalf("%d photo locks left behind", n)
	}
}

// A render queued before a Remove must not recreate the photo's files.
func TestRenderAfterRemoveLeavesNoFiles(t *testing.T) {
	e := newEnv(t)
	p, err := e.ingest("", 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.lib.Remove(context.Background(), e.album.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.lib.Render(context.Background(), e.album.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	dir, err := e.store.PhotoDir(e.album.ID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("photo dir after remove and render: %v, want not exist", err)
	}
	var le *Error
	if err := e.lib.Remove(context.Background(), e.album.ID, p.ID); !errors.As(err, &le) || le.Code != "photo_not_found" {
		t.Fatalf("second Remove = %v, want photo_not_found", err)
	}
}

// probe queues on deriveSem; a client that has given up must not be left
// waiting for a token, and the token must not leak when it gives up.
func TestProbeGivesUpWithTheClient(t *testing.T) {
	e := newEnv(t)
	for range cap(e.lib.deriveSem) {
		e.lib.deriveSem <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.lib.probe(ctx, "src.jpg"); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe on a cancelled request = %v, want context.Canceled", err)
	}
	if len(e.lib.deriveSem) != cap(e.lib.deriveSem) {
		t.Fatalf("deriveSem holds %d tokens, want %d", len(e.lib.deriveSem), cap(e.lib.deriveSem))
	}
}

func TestHeadCaptureAcrossShortWrites(t *testing.T) {
	cases := []struct {
		name   string
		writes [][]byte
		want   [3]byte
	}{
		{"single write", [][]byte{{0xFF, 0xD8, 0xFF, 0xE0}}, [3]byte{0xFF, 0xD8, 0xFF}},
		{"two writes", [][]byte{{0xFF, 0xD8}, {0xFF, 0xE0, 0x00, 0x10}}, [3]byte{0xFF, 0xD8, 0xFF}},
		{"byte at a time", [][]byte{{0xFF}, {0xD8}, {0xFF}, {0xE0}}, [3]byte{0xFF, 0xD8, 0xFF}},
		{"empty then full", [][]byte{{}, {0xFF, 0xD8, 0xFF}}, [3]byte{0xFF, 0xD8, 0xFF}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := &Upload{}
			for _, w := range tc.writes {
				n, err := headCapture{up}.Write(w)
				if err != nil || n != len(w) {
					t.Fatalf("Write(%x) = %d, %v", w, n, err)
				}
			}
			if up.headLen != 3 || up.head != tc.want {
				t.Fatalf("head = %x (len %d), want %x", up.head, up.headLen, tc.want)
			}
		})
	}
}
