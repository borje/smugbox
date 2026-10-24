package api

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/bege/smugbox/backend/internal/db"
	"github.com/bege/smugbox/backend/internal/library"
)

const maxFieldLen = 64 << 10 // per text form field

// uploadPhoto handles POST .../photos. If the album already has a photo with
// the same lr_photo_uuid the call becomes an update (idempotent retries).
func (s *Server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	a, ok := s.albumFromPath(w, r)
	if !ok {
		return
	}
	s.ingest(w, r, a, "")
}

// replacePhoto handles PUT .../photos/{photo_id}: new file and/or metadata.
func (s *Server) replacePhoto(w http.ResponseWriter, r *http.Request) {
	a, ok := s.albumFromPath(w, r)
	if !ok {
		return
	}
	photoID := r.PathValue("photo_id")
	if _, err := uuid.Parse(photoID); err != nil {
		writeError(w, http.StatusNotFound, "photo_not_found", "")
		return
	}
	s.ingest(w, r, a, photoID)
}

// ingest parses the multipart request and hands it to the library.
// photoID is empty for POST.
func (s *Server) ingest(w http.ResponseWriter, r *http.Request, a *db.Album, photoID string) {
	fields, up, ok := s.readUpload(w, r)
	if !ok {
		return
	}
	defer up.Abort()
	photo, created, err := s.lib.Ingest(r.Context(), a, photoID, fields, up)
	if err != nil {
		if r.Context().Err() != nil {
			return // the client gave up; there is nobody to answer
		}
		s.libraryError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]string{"id": photo.ID, "url": s.photoURL(a.Slug, photo.ID)})
}

// libraryStatus maps the library's error codes to HTTP statuses.
var libraryStatus = map[string]int{
	"missing_lr_photo_uuid": http.StatusBadRequest,
	"missing_file":          http.StatusBadRequest,
	"invalid_taken_at":      http.StatusBadRequest,
	"invalid_keywords":      http.StatusBadRequest,
	"invalid_exif":          http.StatusBadRequest,
	"photo_not_found":       http.StatusNotFound,
	"not_jpeg":              http.StatusUnsupportedMediaType,
	"invalid_image":         http.StatusUnprocessableEntity,
}

// libraryError answers a *library.Error with its code, anything else 500.
func (s *Server) libraryError(w http.ResponseWriter, r *http.Request, err error) {
	var le *library.Error
	if errors.As(err, &le) {
		if status, ok := libraryStatus[le.Code]; ok {
			writeError(w, status, le.Code, le.Message)
			return
		}
	}
	s.internalError(w, r, err)
}

// readUpload reads the text fields and stages the file part, if any.
func (s *Server) readUpload(w http.ResponseWriter, r *http.Request) (map[string]string, *library.Upload, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", err.Error())
		return nil, nil, false
	}
	fields := map[string]string{}
	var up *library.Upload
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			up.Abort()
			if tooLarge(err) {
				writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_multipart", err.Error())
			}
			return nil, nil, false
		}
		if part.FormName() == "file" {
			if up != nil {
				up.Abort()
				writeError(w, http.StatusBadRequest, "invalid_multipart", "more than one file part")
				return nil, nil, false
			}
			if up, err = s.lib.Stage(part, part.FileName()); err != nil {
				if tooLarge(err) {
					writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "")
				} else {
					s.internalError(w, r, err)
				}
				return nil, nil, false
			}
			continue
		}
		value, err := readField(part)
		if err != nil {
			up.Abort()
			writeError(w, http.StatusBadRequest, "invalid_field", part.FormName()+": "+err.Error())
			return nil, nil, false
		}
		fields[part.FormName()] = value
	}
	return fields, up, true
}

func tooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func readField(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, maxFieldLen+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxFieldLen {
		return "", errors.New("field too long")
	}
	return string(b), nil
}
