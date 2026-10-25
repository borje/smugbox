package db

import (
	"context"
	"fmt"
	"strings"
)

// MaxSearchPhotos caps the photo hits of one search.
const MaxSearchPhotos = 200

// PhotoHit is a photo found by Search, with the slug of its album.
type PhotoHit struct {
	*Photo
	AlbumSlug string
}

// SearchResult is what a visitor may see that matches a search.
type SearchResult struct {
	Albums []*AlbumSummary
	Photos []PhotoHit
}

// ftsQuery turns free text into an FTS5 query: every word must match as a
// prefix. Quoting each word keeps FTS5 syntax (AND, ", *, :) inert.
func ftsQuery(q string) string {
	var terms []string
	for _, w := range strings.Fields(q) {
		w = strings.ReplaceAll(w, `"`, "")
		if w == "" {
			continue
		}
		terms = append(terms, `"`+w+`"*`)
		if len(terms) == 10 {
			break
		}
	}
	return strings.Join(terms, " ")
}

// subtree selects folderID and every folder beneath it.
const subtree = `(WITH RECURSIVE sub(id) AS (
		SELECT ?
	UNION ALL
		SELECT f.id FROM folders f JOIN sub s ON f.parent_id = s.id
	) SELECT id FROM sub)`

// appendScanner scans extra columns after the ones a scan function knows.
type appendScanner struct {
	rowScanner
	extra []any
}

func (s appendScanner) Scan(dest ...any) error { return s.rowScanner.Scan(append(dest, s.extra...)...) }

// Search finds what a visitor may see: listed albums (locked ones too, as in the listings) and ready photos
// of listed, unlocked albums. Photos of locked albums are never searched, so
// their titles and captions cannot leak. A non-empty folderID limits the
// search to what lies beneath that folder.
func (d *DB) Search(ctx context.Context, q, folderID string) (*SearchResult, error) {
	var res SearchResult
	match := ftsQuery(q)
	if match == "" {
		return &res, nil
	}
	// within(col) restricts col to the folder's subtree; it adds one argument.
	within := func(string) string { return "" }
	var scope []any
	if folderID != "" {
		within = func(col string) string { return " AND " + col + " IN " + subtree }
		scope = []any{folderID}
	}
	rows, err := d.QueryContext(ctx, summarySelect+` WHERE a.is_listed = 1
		AND a.id IN (SELECT id FROM search_fts WHERE search_fts MATCH ? AND kind = 'album')`+within("a.folder_id")+`
		ORDER BY taken_to DESC, a.created_at DESC`, append([]any{match}, scope...)...)
	if err != nil {
		return nil, fmt.Errorf("search albums: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		res.Albums = append(res.Albums, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.QueryContext(ctx, `
		WITH hits AS (SELECT id AS hit_id, rank AS hit_rank FROM search_fts WHERE search_fts MATCH ? AND kind = 'photo')
		SELECT `+photoCols+`, (SELECT slug FROM albums WHERE albums.id = photos.album_id)
		  FROM photos JOIN hits ON hits.hit_id = photos.id
		 WHERE variants_ready = 1
		   AND album_id IN (SELECT id FROM albums WHERE is_listed = 1 AND (password_hash IS NULL OR length(password_hash) = 0)`+within("folder_id")+`)
		 ORDER BY hit_rank, taken_at DESC
		 LIMIT ?`, append(append([]any{match}, scope...), MaxSearchPhotos)...)
	if err != nil {
		return nil, fmt.Errorf("search photos: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hit PhotoHit
		hit.Photo, err = scanPhoto(appendScanner{rows, []any{&hit.AlbumSlug}})
		if err != nil {
			return nil, err
		}
		res.Photos = append(res.Photos, hit)
	}
	return &res, rows.Err()
}
