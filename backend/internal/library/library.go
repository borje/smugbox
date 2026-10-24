// Package library owns the life of a photo's files: the original Lightroom
// uploaded, the display variants rendered from it, and the variants_ready
// flag that hides the photo from visitors until the two agree.
//
// Every operation on one photo (Ingest, Render, Remove) holds that photo's
// lock for the steps that touch its row or files, so none of them ever sees
// another half done. The locks live in this process; that matches the
// single-instance deployment (one SQLite connection, one variant worker).
package library

import (
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/image"
	"github.com/bege/smugbox/backend/internal/storage"
)

// Deps are the collaborators a Library needs. OnPending is called after a
// new original was committed, so the variant worker renders it.
type Deps struct {
	DB        *db.DB
	Store     *storage.Store
	Log       *slog.Logger
	Now       func() time.Time
	OnPending func()
}

// Library stores originals and renders their variants.
type Library struct {
	db        *db.DB
	store     *storage.Store
	log       *slog.Logger
	now       func() time.Time
	onPending func()

	deriveSem chan struct{} // bounds libvips work inside upload requests
	locks     photoLocks

	// derive renders the deferred variants; a seam for tests that need to
	// hold a render in flight.
	derive func(src string, info image.Info, want []storage.Variant, dst func(storage.Variant) (string, error)) ([]storage.Variant, error)
}

// New builds a Library. Log, Now and OnPending may be nil.
func New(d Deps) *Library {
	l := &Library{
		db: d.DB, store: d.Store, log: d.Log, now: d.Now, onPending: d.OnPending,
		deriveSem: make(chan struct{}, runtime.NumCPU()),
		locks:     photoLocks{m: map[string]*photoLock{}},
		derive:    image.Derive,
	}
	if l.log == nil {
		l.log = slog.Default()
	}
	if l.now == nil {
		l.now = time.Now
	}
	if l.onPending == nil {
		l.onPending = func() {}
	}
	return l
}

// Error is a client-facing failure: Code is a snake_case error code and
// Message optional detail. Anything else a Library returns is internal.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func clientErr(code, msg string) error { return &Error{Code: code, Message: msg} }

// photoLocks hands out one mutex per photo id. An entry exists only while
// someone holds or waits for it, so memory follows the photos in flight,
// not the size of the library.
type photoLocks struct {
	mu sync.Mutex
	m  map[string]*photoLock
}

type photoLock struct {
	sync.Mutex
	refs int // holders and waiters; guarded by photoLocks.mu
}

// lock blocks until the photo is free and returns the matching unlock.
func (pl *photoLocks) lock(photoID string) (unlock func()) {
	pl.mu.Lock()
	l := pl.m[photoID]
	if l == nil {
		l = &photoLock{}
		pl.m[photoID] = l
	}
	l.refs++
	pl.mu.Unlock()
	l.Lock()
	return func() {
		l.Unlock()
		pl.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(pl.m, photoID)
		}
		pl.mu.Unlock()
	}
}
