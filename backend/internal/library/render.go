package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/storage"
	"github.com/bege/smugbox/backend/internal/video"
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
	src, err := l.store.Path(p.AlbumID, p.ID, storage.Original, p.MimeType)
	if err != nil {
		return err
	}
	staged := map[storage.Variant]*storage.Staged{}
	defer func() {
		for _, st := range staged {
			st.Abort()
		}
	}()
	if isVideo(p) {
		// The variants of a video are rendered from its poster frame,
		// which is not kept.
		poster, err := l.store.NewStaged()
		if err != nil {
			return err
		}
		defer poster.Abort()
		if err := poster.Sync(); err != nil {
			return err
		}
		if err := video.Poster(ctx, src, poster.Path(), p.DurationMs); err != nil {
			if gone, gerr := l.photoGone(ctx, p); gerr == nil && gone {
				return nil
			}
			return fmt.Errorf("render poster: %w", err)
		}
		src = poster.Path()
	}
	for _, r := range renditions {
		if r.variant == storage.Large && !hasLarge(p) {
			continue
		}
		st, err := l.store.NewStaged()
		if err != nil {
			return err
		}
		staged[r.variant] = st
		if err := l.resize(src, st.Path(), r.long, r.quality, r.blurSigma); err != nil {
			if gone, gerr := l.photoGone(ctx, p); gerr == nil && gone {
				return nil
			}
			return fmt.Errorf("render %s: %w", r.variant, err)
		}
	}
	for v, st := range staged {
		if err := l.store.Commit(st, p.AlbumID, p.ID, v, p.MimeType); err != nil {
			return err
		}
	}
	// A replacement may be smaller than the previous original.
	if !hasLarge(p) {
		if err := l.store.Remove(p.AlbumID, p.ID, storage.Large, p.MimeType); err != nil {
			return err
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
