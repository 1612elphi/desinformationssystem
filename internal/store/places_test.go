package store

import (
	"path/filepath"
	"testing"
)

func TestReplacePlacesKeepsOtherSources(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO files(id, url) VALUES('a', 'u')"); err != nil {
		t.Fatal(err)
	}
	link := PlaceLink{FileID: "a", Name: "Marktplatz", Kind: "square", Lat: 49, Lon: 8.4, N: 2}
	if _, err := s.ReplacePlaces("haiku", []PlaceLink{link, link}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := s.ReplacePlaces("dis", []PlaceLink{link}); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var source string
	if err := s.DB.QueryRow("SELECT n, source FROM file_places").Scan(&n, &source); err != nil {
		t.Fatal(err)
	}
	if n != 4 || source != "haiku" {
		t.Fatalf("got n=%d source=%s, want 4 haiku", n, source)
	}
}
