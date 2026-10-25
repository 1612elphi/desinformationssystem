package store

import (
	"path/filepath"
	"testing"
)

func TestSearchFacetCounts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO meetings(id, date) VALUES('termin-1', '2026-10-20')",
		"INSERT INTO files(id, url, meeting_id, doc_type, category) VALUES('a', 'u', 'termin-1', 'Antrag', 'Verkehr')",
		"INSERT INTO files(id, url, meeting_id, doc_type, category) VALUES('b', 'u', 'termin-1', 'Antrag', 'Bildung')",
		"INSERT INTO files(id, url, meeting_id, doc_type, category) VALUES('c', 'u', 'termin-1', 'Vorlage', 'Verkehr')",
		"INSERT INTO files(id, url, meeting_id, doc_type, category) VALUES('d', 'u', 'termin-1', 'Anlage', NULL)",
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.SearchDocuments(SearchParams{Category: "Verkehr", Facets: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res["total"] != int64(2) {
		t.Fatalf("category filter: total %v", res["total"])
	}
	fc := res["facet_counts"].(Row)
	// the category counts ignore the category filter; NULL is left out
	cats := fc["category"].([]Row)
	if len(cats) != 2 || cats[0]["value"] != "Verkehr" || cats[0]["n"] != int64(2) {
		t.Fatalf("category counts: %v", cats)
	}
	types := fc["doc_type"].([]Row)
	if len(types) != 2 || types[0]["n"] != int64(1) {
		t.Fatalf("type counts under category filter: %v", types)
	}
	if res, _ := s.SearchDocuments(SearchParams{Limit: 10}); res["facet_counts"] != nil {
		t.Fatal("facet counts without Facets")
	}
}
