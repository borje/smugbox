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
	"sync"
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
	var once sync.Once
	e.lib.resize = func(src, dst string, long, quality int, blurSigma float64) error {
		err := image.Resize(src, dst, long, quality, blurSigma)
		once.Do(func() {
			close(rendered)
			<-release
		})
		return err
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
	e.lib.resize = image.Resize
	if err := e.lib.Render(context.Background(), e.album.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if !e.photo(p.ID).VariantsReady {
		t.Fatal("photo not ready after rendering the new original")
	}
	thumb, err := e.store.Path(e.album.ID, p.ID, storage.Thumb, "image/jpeg")
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
			if up.headLen < 3 || [3]byte(up.head[:3]) != tc.want {
				t.Fatalf("head = %x (len %d), want %x", up.head, up.headLen, tc.want)
			}
		})
	}
}

func (e *env) render(id string) {
	e.t.Helper()
	if err := e.lib.Render(context.Background(), e.album.ID, id); err != nil {
		e.t.Fatal(err)
	}
}

// open opens variant v the way a visitor gets it and returns which file
// served it and its dimensions.
func (e *env) open(id string, v storage.Variant) (storage.Variant, int, int) {
	e.t.Helper()
	f, served, err := e.lib.Open(e.photo(id), v)
	if err != nil {
		e.t.Fatalf("open %s: %v", v, err)
	}
	f.Close()
	info, err := image.Probe(f.Name())
	if err != nil {
		e.t.Fatal(err)
	}
	return served, info.Width, info.Height
}

func TestRenderProducesEveryVariant(t *testing.T) {
	e := newEnv(t)
	p, err := e.ingest("", 3000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	e.render(p.ID)
	want := map[storage.Variant][2]int{
		storage.Large:  {2560, 1707},
		storage.Medium: {1600, 1067},
		storage.Small:  {800, 533},
		storage.Thumb:  {400, 267},
		storage.Blur:   {40, 27},
	}
	for v, dims := range want {
		served, w, h := e.open(p.ID, v)
		if served != v || w != dims[0] || h != dims[1] {
			t.Errorf("%s: served %s %dx%d, want %s %dx%d", v, served, w, h, v, dims[0], dims[1])
		}
	}
}

// A photo no bigger than large has no large variant; the original serves
// it, also after a replace shrank a photo that had one.
func TestLargeFallsBackToOriginal(t *testing.T) {
	e := newEnv(t)
	p, err := e.ingest("", 3000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	e.render(p.ID)
	if _, err := e.ingest(p.ID, 1200, 800); err != nil {
		t.Fatal(err)
	}
	e.render(p.ID)
	if served, w, h := e.open(p.ID, storage.Large); served != storage.Original || w != 1200 || h != 800 {
		t.Fatalf("large served %s %dx%d, want original 1200x800", served, w, h)
	}
	if _, err := e.store.Stat(e.album.ID, p.ID, storage.Large, "image/jpeg"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale large.jpg left behind: %v", err)
	}
	if served, w, _ := e.open(p.ID, storage.Medium); served != storage.Medium || w != 1200 {
		t.Fatalf("medium served %s %d wide, want medium 1200 (never upscaled)", served, w)
	}
}

func TestVariantSize(t *testing.T) {
	big := &db.Photo{Width: 6000, Height: 4000}
	small := &db.Photo{Width: 1000, Height: 1500}
	cases := []struct {
		p    *db.Photo
		v    storage.Variant
		w, h int
	}{
		{big, storage.Thumb, 400, 267},
		{big, storage.Small, 800, 533},
		{big, storage.Medium, 1600, 1067},
		{big, storage.Large, 2560, 1707},
		{small, storage.Thumb, 267, 400},
		{small, storage.Medium, 1000, 1500},
		{small, storage.Large, 1000, 1500},
		{small, storage.Original, 1000, 1500},
	}
	for _, c := range cases {
		if w, h := VariantSize(c.p, c.v); w != c.w || h != c.h {
			t.Errorf("%dx%d %s = %dx%d, want %dx%d", c.p.Width, c.p.Height, c.v, w, h, c.w, c.h)
		}
	}
}
