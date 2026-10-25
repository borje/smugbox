package api

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

type searchBody struct {
	Folders []folderListItem `json:"folders"`
	Albums  []albumListItem  `json:"albums"`
	Photos  []searchPhoto    `json:"photos"`
}

func (e *env) search(q string) searchBody {
	e.t.Helper()
	rec := e.get("/api/search?q=" + url.QueryEscape(q))
	if rec.Code != http.StatusOK {
		e.t.Fatalf("search %q: %d %s", q, rec.Code, rec.Body.String())
	}
	var out searchBody
	decode(e.t, rec, &out)
	return out
}

func TestSearchFindsOnlyWhatVisitorsMaySee(t *testing.T) {
	e := newEnv(t)
	jpg := testJPEG(t, 20, 10, 0)
	nordic := e.createFolder("Nordic trips", "")
	malmo := e.createAlbum("Malmö harbour")
	e.json(http.MethodPut, "/api/publish/albums/"+malmo.ID, map[string]any{"parent_id": nordic.ID, "description": "Boats at dusk"})
	id, _ := e.uploadPhoto(malmo.ID, "lr-1", "a.jpg", jpg, map[string]string{"title": "Sunset", "caption": "Over the Öresund bridge", "keywords": `["sailing","harbour"]`})
	e.uploadPhoto(malmo.ID, "lr-2", "b.jpg", jpg, map[string]string{"title": "Lunch"})

	locked := e.createLockedAlbum("Harbour wedding", "pw")
	e.uploadPhoto(locked.ID, "lr-3", "c.jpg", jpg, map[string]string{"title": "Harbour kiss"})
	hidden := e.createAlbum("Harbour drafts")
	e.json(http.MethodPut, "/api/publish/albums/"+hidden.ID, map[string]any{"is_listed": false})
	e.uploadPhoto(hidden.ID, "lr-4", "d.jpg", jpg, map[string]string{"title": "Harbour draft"})

	// Diacritics fold, words are prefixes, every word must match.
	res := e.search("oresund BRID")
	if len(res.Photos) != 1 || res.Photos[0].ID != id || res.Photos[0].AlbumSlug != "malmo-harbour" {
		t.Fatalf("photo by caption: %+v", res)
	}
	if res := e.search("oresund lunch"); len(res.Photos) != 0 {
		t.Fatalf("all words must match: %+v", res.Photos)
	}
	if res := e.search("sail"); len(res.Photos) != 1 {
		t.Fatalf("photo by keyword prefix: %+v", res.Photos)
	}
	if res := e.search("dusk"); len(res.Albums) != 1 || res.Albums[0].Slug != "malmo-harbour" {
		t.Fatalf("album by description: %+v", res.Albums)
	}
	if res := e.search("nordic"); res.Folders != nil || len(res.Albums) != 0 {
		t.Fatalf("folder names are not searched: %+v", res)
	}

	// The locked album is found by name (it is listed) but its photos are
	// not; the unlisted album is not found at all.
	res = e.search("harbour")
	if len(res.Albums) != 2 {
		t.Fatalf("albums: %+v", res.Albums)
	}
	for _, a := range res.Albums {
		if a.Slug == "harbour-drafts" {
			t.Fatal("unlisted album found")
		}
	}
	if len(res.Photos) != 1 || res.Photos[0].ID != id {
		t.Fatalf("only the public photo: %+v", res.Photos)
	}

	// Renames and deletes reach the index.
	e.json(http.MethodPut, "/api/publish/albums/"+malmo.ID, map[string]any{"name": "Göteborg"})
	if res := e.search("goteborg"); len(res.Albums) != 1 {
		t.Fatalf("renamed album: %+v", res.Albums)
	}
	if rec := e.json(http.MethodDelete, "/api/publish/albums/"+malmo.ID, nil); rec.Code >= 300 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if res := e.search("sunset"); len(res.Photos) != 0 {
		t.Fatalf("deleted photo found: %+v", res.Photos)
	}
	var n int
	if err := e.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM search_fts WHERE id = ?`, id).Scan(&n); err != nil || n != 0 {
		t.Fatalf("index row left behind: %d %v", n, err)
	}
}

func TestSearchPendingPhotoAndJunkQuery(t *testing.T) {
	e := newEnvNoWorker(t)
	a := e.createAlbum("Album")
	e.uploadPhoto(a.ID, "lr-1", "a.jpg", testJPEG(t, 20, 10, 0), map[string]string{"title": "Pending"})
	if res := e.search("pending"); len(res.Photos) != 0 {
		t.Fatalf("pending photo found: %+v", res.Photos)
	}
	for _, q := range []string{"", `"`, `AND OR NOT`, `a:b`, `*`, `(x`, `NEAR(`} {
		e.search(q) // must answer 200, not a syntax error
	}
}

func TestSearchWithinFolder(t *testing.T) {
	e := newEnv(t)
	jpg := testJPEG(t, 20, 10, 0)
	travel := e.createFolder("Travel", "")
	nordic := e.createFolder("Nordic beaches", travel.ID)
	home := e.createFolder("Home", "")
	inside := e.createAlbum("Beach day")
	e.json(http.MethodPut, "/api/publish/albums/"+inside.ID, map[string]any{"parent_id": nordic.ID})
	in, _ := e.uploadPhoto(inside.ID, "lr-1", "a.jpg", jpg, map[string]string{"title": "Beach"})
	outside := e.createAlbum("Beach at home")
	e.json(http.MethodPut, "/api/publish/albums/"+outside.ID, map[string]any{"parent_id": home.ID})
	e.uploadPhoto(outside.ID, "lr-2", "b.jpg", jpg, map[string]string{"title": "Beach"})

	rec := e.get("/api/search?q=beach&folder=travel")
	var res struct {
		searchBody
		Breadcrumb []crumb `json:"breadcrumb"`
	}
	decode(t, rec, &res)
	if rec.Code != 200 || len(res.Breadcrumb) != 1 || res.Breadcrumb[0].Name != "Travel" {
		t.Fatalf("breadcrumb: %d %+v", rec.Code, res.Breadcrumb)
	}
	if len(res.Photos) != 1 || res.Photos[0].ID != in || len(res.Albums) != 1 || res.Albums[0].Slug != "beach-day" {
		t.Fatalf("within travel: %+v", res.searchBody)
	}
	rec = e.get("/api/search?q=beach&folder=nordic-beaches")
	decode(t, rec, &res)
	if len(res.Breadcrumb) != 2 || res.Breadcrumb[0].Slug != "travel" || res.Breadcrumb[1].Slug != "nordic-beaches" || len(res.Photos) != 1 {
		t.Fatalf("nested scope: %+v %+v", res.Breadcrumb, res.Photos)
	}
	if res := e.search("beach"); len(res.Photos) != 2 || len(res.Albums) != 2 {
		t.Fatalf("unscoped: %+v", res)
	}
	if rec := e.get("/api/search?q=beach&folder=nope"); rec.Code != 404 {
		t.Fatalf("unknown folder: %d", rec.Code)
	}
}
