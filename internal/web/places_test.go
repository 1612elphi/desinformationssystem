package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

func TestPlaces(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO bodies(id, name) VALUES('gr', 'Gemeinderat')`,
		`INSERT INTO meetings(id, date, body_id, body_name) VALUES
			('termin-1', '2026-03-10', 'gr', 'Gemeinderat'),
			('termin-2', '2026-07-21', 'gr', 'Gemeinderat')`,
		`INSERT INTO agenda_items(meeting_id, anchor, title) VALUES ('termin-2', 'top1', 'Kaiserstraße')`,
		`INSERT INTO files(id, url, meeting_id, agenda_anchor, label) VALUES
			('f1', 'u', 'termin-1', NULL, 'Antrag'),
			('f2', 'u', 'termin-2', 'top1', 'Vorlage')`,
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ks := func(file string, n int) store.PlaceLink {
		return store.PlaceLink{FileID: file, Name: "Kaiserstraße", Kind: "street", Lat: 49.0098, Lon: 8.3993, OSM: "way/1", N: n, Quote: "q " + file}
	}
	links := []store.PlaceLink{
		ks("f1", 2), ks("f2", 1), ks("f2", 3),
		{FileID: "f1", Name: "Durlach", Kind: "district", Lat: 48.99, Lon: 8.47, N: 1},
		{FileID: "missing", Name: "Durlach", Kind: "district", Lat: 48.99, Lon: 8.47, N: 1},
	}
	// a second run replaces the first
	for range 2 {
		skipped, err := st.ReplacePlaces("haiku", links)
		if err != nil || skipped != 1 {
			t.Fatalf("ReplacePlaces: %d %v", skipped, err)
		}
	}

	h := (&Server{Store: st, MapDir: t.TempDir()}).Handler()
	get := func(url string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	_, v := get("/api/places")
	ps := v["places"].([]any)
	if v["total_docs"] != 2.0 || len(ps) != 2 {
		t.Fatalf("places: %v", v)
	}
	top := ps[0].(map[string]any)
	if top["name"] != "Kaiserstraße" || top["docs"] != 2.0 || top["mentions"] != 6.0 {
		t.Errorf("top place: %v", top)
	}
	if _, v = get("/api/places?from=2026-06-01&kind=street"); v["total_docs"] != 1.0 ||
		v["places"].([]any)[0].(map[string]any)["mentions"] != 4.0 {
		t.Errorf("filtered: %v", v)
	}
	if _, v = get("/api/places?committee=Bauausschuss"); len(v["places"].([]any)) != 0 {
		t.Errorf("committee filter: %v", v)
	}

	id := int(top["id"].(float64))
	code, v := get("/api/places/" + strconv.Itoa(id))
	docs := v["documents"].([]any)
	first := docs[0].(map[string]any)
	if code != 200 || len(docs) != 2 || first["id"] != "f2" || first["agenda_title"] != "Kaiserstraße" || first["n"] != 4.0 {
		t.Fatalf("place: %d %v", code, v)
	}
	if code, _ := get("/api/places/9999"); code != 404 {
		t.Errorf("unknown place: %d", code)
	}
	if code, _ := get("/api/places/x"); code != 400 {
		t.Errorf("bad id: %d", code)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/map/karlsruhe.pmtiles", nil))
	if rec.Code != 404 {
		t.Errorf("absent tiles: %d", rec.Code)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "karlsruhe.pmtiles"), []byte("PMTiles0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	h = (&Server{Store: st, MapDir: dir}).Handler()
	req := httptest.NewRequest("GET", "/map/karlsruhe.pmtiles", nil)
	req.Header.Set("Range", "bytes=0-6")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 206 || rec.Body.String() != "PMTiles" || rec.Header().Get("Cache-Control") == "" {
		t.Errorf("range: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
}
