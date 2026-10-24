package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/image"
	"github.com/bege/smugbox/backend/internal/storage"
)

// Render renders the display variants of a pending photo from its stored
// original, commits them, and marks the photo ready. A photo that is gone
// or already ready is left alone and reports nil.
func (l *Library) Render(ctx context.Context, albumID, photoID string) error {
	unlock := l.locks.lock(photoID)
	defer unlock()
	p, err := l.db.GetPhoto(ctx, albumID, photoID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.VariantsReady {
		return nil
	}
	src, err := l.store.Path(p.AlbumID, p.ID, storage.Original)
	if err != nil {
		return err
	}
	staged := map[storage.Variant]*storage.Staged{}
	defer func() {
		for _, st := range staged {
			st.Abort()
		}
	}()
	produced, err := l.derive(src, image.Info{Width: p.Width, Height: p.Height}, image.Deferred, func(v storage.Variant) (string, error) {
		st, err := l.store.NewStaged()
		if err != nil {
			return "", err
		}
		staged[v] = st
		return st.Path(), nil
	})
	if err != nil {
		if gone, gerr := l.photoGone(ctx, p); gerr == nil && gone {
			return nil
		}
		return fmt.Errorf("derive: %w", err)
	}
	for v, st := range staged {
		if err := l.store.Commit(st, p.AlbumID, p.ID, v); err != nil {
			return err
		}
	}
	// A replacement may be smaller than the previous file: drop variants
	// that were not regenerated (large falls back to the original).
	for _, v := range image.Deferred {
		if !slices.Contains(produced, v) {
			if err := l.store.Remove(p.AlbumID, p.ID, v); err != nil {
				return err
			}
		}
	}
	ok, err := l.db.MarkVariantsReady(ctx, p.AlbumID, p.ID, p.ContentHash)
	if err != nil {
		return err
	}
	if !ok {
		// The photo lock keeps the original from changing underneath, so
		// the row is gone: its album was deleted, and Commit recreated the
		// photo's directory, which we must take back.
		if gone, err := l.photoGone(ctx, p); err == nil && gone {
			_ = l.store.RemovePhoto(p.AlbumID, p.ID)
		}
	}
	return nil
}

// photoGone reports whether p's row no longer exists. A missing original
// with the row still present is a real failure, not "gone".
func (l *Library) photoGone(ctx context.Context, p *db.Photo) (bool, error) {
	_, err := l.db.GetPhoto(ctx, p.AlbumID, p.ID)
	if errors.Is(err, db.ErrNotFound) {
		return true, nil
	}
	return false, err
}

// Remove deletes a photo's row and then its files. A file that cannot be
// removed is only logged: the row is gone, so admin gc finds the leftovers.
func (l *Library) Remove(ctx context.Context, albumID, photoID string) error {
	unlock := l.locks.lock(photoID)
	defer unlock()
	err := l.db.DeletePhoto(ctx, albumID, photoID)
	if errors.Is(err, db.ErrNotFound) {
		return clientErr("photo_not_found", "")
	}
	if err != nil {
		return err
	}
	if err := l.store.RemovePhoto(albumID, photoID); err != nil {
		l.log.Warn("remove photo files", "album", albumID, "photo", photoID, "err", err)
	}
	return nil
}
