package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/image"
	"github.com/bege/smugbox/backend/internal/storage"
)

const (
	maxLrUUIDLen   = 200
	maxFilenameLen = 255
	maxTakenAtLen  = 64
)

// Upload is an uploaded file staged in the incoming directory. Abort it
// when done; after a successful Ingest that is a no-op.
type Upload struct {
	staged   *storage.Staged
	size     int64
	hash     string
	head     [3]byte
	headLen  int
	filename string // as the client named it
}

// Abort discards the staged file unless Ingest committed it. Safe on nil.
func (up *Upload) Abort() {
	if up != nil {
		up.staged.Abort()
	}
}

// headCapture remembers the first bytes written through it.
type headCapture struct {
	up *Upload
}

func (h headCapture) Write(p []byte) (int, error) {
	// headLen counts bytes across calls; i indexes this call's chunk.
	for i := 0; h.up.headLen < len(h.up.head) && i < len(p); i++ {
		h.up.head[h.up.headLen] = p[i]
		h.up.headLen++
	}
	return len(p), nil
}

// Stage copies r into the incoming directory, hashing it on the way. The
// file is checked later, by Ingest. A read error from r is returned wrapped,
// so a caller can still recognise its own size limit.
func (l *Library) Stage(r io.Reader, filename string) (*Upload, error) {
	st, err := l.store.NewStaged()
	if err != nil {
		return nil, err
	}
	up := &Upload{staged: st, filename: filename}
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(st, hasher, headCapture{up}), r)
	if err != nil {
		st.Abort()
		return nil, fmt.Errorf("store upload: %w", err)
	}
	up.size = n
	up.hash = hex.EncodeToString(hasher.Sum(nil))
	return up, nil
}

// Ingest stores a new photo, a replacement original, or new metadata.
//
// photoID names the photo to replace; empty means fields["lr_photo_uuid"]
// picks it, and a photo the album does not have yet is created. up is the
// new original, or nil to change metadata only. It reports whether the
// photo was created. A replaced original hides the photo until the variant
// worker has rendered it again.
func (l *Library) Ingest(ctx context.Context, a *db.Album, photoID string, fields map[string]string, up *Upload) (*db.Photo, bool, error) {
	replace := photoID != ""
	lrUUID := strings.TrimSpace(fields["lr_photo_uuid"])
	if !replace && (lrUUID == "" || len(lrUUID) > maxLrUUIDLen) {
		return nil, false, clientErr("missing_lr_photo_uuid", "")
	}
	applyMetadata, err := parseMetadata(fields)
	if err != nil {
		return nil, false, err
	}

	// Check the file before taking the lock: probing is the slow part and
	// touches nothing but the staged copy.
	var info image.Info
	if up != nil {
		if up.size == 0 || !image.IsJPEG(up.head[:up.headLen]) {
			return nil, false, clientErr("not_jpeg", "file must be a JPEG image")
		}
		if err := up.staged.Sync(); err != nil {
			return nil, false, err
		}
		if info, err = l.probe(ctx, up.staged.Path()); err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			l.log.Warn("probe upload", "album", a.ID, "photo", photoID, "lr_photo_uuid", lrUUID, "err", err)
			return nil, false, clientErr("invalid_image", err.Error())
		}
	}

	// Resolve the target row: POST with a known lr_photo_uuid is an update.
	id := photoID
	if !replace {
		p, err := l.db.GetPhotoByLrUUID(ctx, a.ID, lrUUID)
		switch {
		case err == nil:
			id = p.ID
		case errors.Is(err, db.ErrNotFound):
			id = uuid.NewString()
		default:
			return nil, false, err
		}
	}
	unlock := l.locks.lock(id)
	defer unlock()
	photo, err := l.db.GetPhoto(ctx, a.ID, id)
	switch {
	case errors.Is(err, db.ErrNotFound) && replace:
		return nil, false, clientErr("photo_not_found", "")
	case errors.Is(err, db.ErrNotFound):
		photo = nil
	case err != nil:
		return nil, false, err
	}
	isNew := photo == nil
	now := l.now()
	if isNew {
		if up == nil {
			return nil, false, clientErr("missing_file", "")
		}
		photo = &db.Photo{ID: id, AlbumID: a.ID, LrPhotoUUID: lrUUID, MimeType: "image/jpeg", CreatedAt: now}
	}
	photo.UpdatedAt = now
	applyMetadata(photo)

	if up != nil {
		photo.Width, photo.Height = info.Width, info.Height
		photo.SizeBytes = up.size
		photo.ContentHash = up.hash
		photo.MimeType = "image/jpeg"
		if photo.Filename == "" || fields["filename"] == "" {
			if fn := SanitizeFilename(up.filename); fn != "" {
				photo.Filename = fn
			}
		}
		if !isNew {
			// Hide the photo while its stored variants describe the old
			// original; the worker shows it again once they are regenerated.
			if err := l.db.MarkVariantsPending(ctx, a.ID, photo.ID); err != nil {
				return nil, false, err
			}
		}
		if err := l.store.Commit(up.staged, a.ID, photo.ID, storage.Original); err != nil {
			return nil, false, err
		}
	}
	if photo.Filename == "" {
		photo.Filename = photo.ID + ".jpg"
	}

	if isNew {
		err = l.db.InsertPhoto(ctx, photo)
	} else {
		err = l.db.UpdatePhoto(ctx, photo)
	}
	if err != nil {
		if isNew {
			_ = l.store.RemovePhoto(a.ID, photo.ID)
		}
		return nil, false, err
	}
	if up != nil {
		l.onPending()
	}
	return photo, isNew, nil
}

// probe reads the dimensions of the JPEG at src and renders a throwaway
// thumb of it, which decodes the whole file so that an unreadable JPEG is
// rejected in the request instead of by the worker after the client was
// told 201. Nothing is committed.
//
// This is the only libvips work in an upload request, and it holds
// deriveSem for its duration: otherwise every concurrent upload runs a
// pipeline of its own, which the memory of the machine this runs on does
// not allow. The token is released by defer, so a panic cannot leak it, and
// a client that gives up while queueing does not keep waiting.
func (l *Library) probe(ctx context.Context, src string) (image.Info, error) {
	select {
	case l.deriveSem <- struct{}{}:
	case <-ctx.Done():
		return image.Info{}, ctx.Err()
	}
	defer func() { <-l.deriveSem }()

	thumb, err := l.store.NewStaged()
	if err != nil {
		return image.Info{}, err
	}
	defer thumb.Abort()
	info, err := image.Probe(src)
	if err != nil {
		return info, err
	}
	_, err = image.Derive(src, info, []storage.Variant{image.Validate}, func(storage.Variant) (string, error) {
		return thumb.Path(), nil
	})
	return info, err
}

// parseMetadata validates the optional text fields and returns a function
// that copies them onto a photo.
func parseMetadata(f map[string]string) (func(*db.Photo), error) {
	var edits []func(*db.Photo)
	if v, ok := f["filename"]; ok {
		if fn := SanitizeFilename(v); fn != "" {
			edits = append(edits, func(p *db.Photo) { p.Filename = fn })
		}
	}
	if v, ok := f["title"]; ok {
		v = strings.TrimSpace(v)
		edits = append(edits, func(p *db.Photo) { p.Title = v })
	}
	if v, ok := f["caption"]; ok {
		v = strings.TrimSpace(v)
		edits = append(edits, func(p *db.Photo) { p.Caption = v })
	}
	if v, ok := f["taken_at"]; ok {
		v = strings.TrimSpace(v)
		if len(v) > maxTakenAtLen {
			return nil, clientErr("invalid_taken_at", "")
		}
		edits = append(edits, func(p *db.Photo) { p.TakenAt = v })
	}
	if v, ok := f["keywords"]; ok {
		kws, err := parseKeywords(v)
		if err != nil {
			return nil, clientErr("invalid_keywords", err.Error())
		}
		edits = append(edits, func(p *db.Photo) { p.Keywords = kws })
	}
	if v, ok := f["exif"]; ok {
		var exif json.RawMessage
		if v = strings.TrimSpace(v); v != "" {
			var obj map[string]any
			if err := json.Unmarshal([]byte(v), &obj); err != nil {
				return nil, clientErr("invalid_exif", "exif must be a JSON object")
			}
			exif, _ = json.Marshal(obj)
		}
		edits = append(edits, func(p *db.Photo) { p.Exif = exif })
	}
	return func(p *db.Photo) {
		for _, edit := range edits {
			edit(p)
		}
	}, nil
}

// parseKeywords accepts a JSON array of strings or a comma-separated list.
func parseKeywords(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var out []string
	if strings.HasPrefix(v, "[") {
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return nil, errors.New("keywords must be a JSON array of strings")
		}
	} else {
		for _, k := range strings.Split(v, ",") {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
	}
	clean := out[:0]
	for _, k := range out {
		if k = strings.TrimSpace(k); k != "" {
			clean = append(clean, k)
		}
	}
	return clean, nil
}

// SanitizeFilename keeps only the base name and strips characters that are
// unsafe in Content-Disposition or zip entries.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/', r == '\\', r == '"', r == ':', r == '*', r == '?', r == '<', r == '>', r == '|':
			b.WriteByte('_')
		case unicode.IsControl(r):
			// drop
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "." || out == ".." {
		return ""
	}
	if len(out) > maxFilenameLen {
		out = out[:maxFilenameLen]
	}
	return out
}
