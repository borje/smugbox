package api

import (
	"errors"
	"net/http"

	"github.com/bege/smugbox/backend/internal/db"
)

type searchPhoto struct {
	photoJSON
	AlbumSlug string `json:"album_slug"`
}

// GET /api/search?q=…[&folder=<slug>] — folders, albums and photos a visitor
// may see, anywhere or beneath one folder.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) > 200 {
		q = q[:200]
	}
	out := map[string]any{}
	var folderID string
	if slug := r.URL.Query().Get("folder"); slug != "" {
		f, err := s.db.GetFolderBySlug(r.Context(), slug)
		if !validSlug(slug) || errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "folder_not_found", "")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		folderID = f.ID
		// The searched folder's path for the breadcrumb, ending with the folder.
		if out["breadcrumb"], err = s.folderBreadcrumb(r, f.ID); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	res, err := s.db.Search(r.Context(), q, folderID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	photos := make([]searchPhoto, 0, len(res.Photos))
	for _, h := range res.Photos {
		photos = append(photos, searchPhoto{toPhotoJSON(h.AlbumSlug, h.Photo), h.AlbumSlug})
	}
	out["folders"] = folderItems(res.Folders)
	out["albums"] = albumItems(res.Albums)
	out["photos"] = photos
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, out)
}
