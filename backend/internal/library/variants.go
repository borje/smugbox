package library

import (
	"os"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/storage"
)

// rendition is how one display variant is rendered from the original.
type rendition struct {
	variant   storage.Variant
	long      int // long-side pixels
	quality   int
	blurSigma float64 // > 0 for the blurred placeholder
}

// renditions are the variants Render produces, in that order. The blur is a
// tiny placeholder that is safe to show publicly for locked albums.
var renditions = []rendition{
	{storage.Thumb, 400, 80, 0},
	{storage.Large, 2560, 85, 0},
	{storage.Medium, 1600, 85, 0},
	{storage.Small, 800, 80, 0},
	{storage.Blur, 40, 40, 2.0},
}

// validate is rendered and thrown away while an upload request is still in
// flight. The thumb is cheap (libvips shrinks on load) yet decodes the whole
// file, so a corrupt upload is rejected before the client is told 201.
var validate = renditions[0]

// hasLarge reports whether p has a large variant. It has none when its
// original is no bigger than large; the original is served in its place.
func hasLarge(p *db.Photo) bool {
	return max(p.Width, p.Height) > renditions[1].long
}

// Open opens the file that serves variant v of p and reports which variant
// that file is: large of a photo that has none is its original.
func (l *Library) Open(p *db.Photo, v storage.Variant) (*os.File, storage.Variant, error) {
	if v == storage.Large && !hasLarge(p) {
		v = storage.Original
	}
	f, err := l.store.Open(p.AlbumID, p.ID, v)
	return f, v, err
}
