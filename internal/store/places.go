package store

import "fmt"

// PlaceLink is one resolved place mention in one file.
type PlaceLink struct {
	FileID     string
	Name, Kind string
	Lat, Lon   float64
	OSM        string
	N          int
	Quote      string
}

// ReplacePlaces swaps every file_places row of source for links and deletes
// places no file references any more. Links to unknown files are skipped and
// counted. Two links of one file to the same place add their counts.
func (s *Store) ReplacePlaces(source string, links []PlaceLink) (skipped int, err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM file_places WHERE source = ?", source); err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, l := range links {
		ok, seen := known[l.FileID]
		if !seen {
			var n int
			if err := tx.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", l.FileID).Scan(&n); err != nil {
				return 0, err
			}
			ok = n > 0
			known[l.FileID] = ok
		}
		if !ok {
			skipped++
			continue
		}
		var id int64
		if err := tx.QueryRow(`INSERT INTO places(name, kind, lat, lon, osm) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(name, kind) DO UPDATE SET lat = excluded.lat, lon = excluded.lon, osm = excluded.osm
			RETURNING id`, l.Name, l.Kind, l.Lat, l.Lon, l.OSM).Scan(&id); err != nil {
			return 0, fmt.Errorf("place %s: %w", l.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO file_places(file_id, place_id, n, quote, source) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(file_id, place_id) DO UPDATE SET n = file_places.n + excluded.n`,
			l.FileID, id, max(1, l.N), l.Quote, source); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec("DELETE FROM places WHERE id NOT IN (SELECT place_id FROM file_places)"); err != nil {
		return 0, err
	}
	return skipped, tx.Commit()
}

type PlaceFilter struct {
	DateFrom, DateTo, Committee, Kind string
}

const placesFrom = ` FROM file_places fp
	JOIN places p ON p.id = fp.place_id
	JOIN files f ON f.id = fp.file_id
	LEFT JOIN meetings m ON m.id = f.meeting_id`

// Places lists every place with at least one file matching the filter, with
// its distinct file count and summed mentions, plus the matching file total.
func (s *Store) Places(pf PlaceFilter) (Row, error) {
	var w where
	if pf.DateFrom != "" {
		w.add("m.date >= ?", pf.DateFrom)
	}
	if pf.DateTo != "" {
		w.add("m.date <= ?", pf.DateTo)
	}
	if pf.Committee != "" {
		w.add("(m.body_name = ? OR m.body_id = ?)", pf.Committee, pf.Committee)
	}
	if pf.Kind != "" {
		w.add("p.kind = ?", pf.Kind)
	}
	ps, err := s.rows(`SELECT p.id, p.name, p.kind, p.lat, p.lon,
		COUNT(DISTINCT fp.file_id) AS docs, SUM(fp.n) AS mentions`+placesFrom+w.sql()+
		` GROUP BY p.id ORDER BY docs DESC, p.name`, w.args...)
	if err != nil {
		return nil, err
	}
	total, err := s.count("SELECT COUNT(DISTINCT fp.file_id)"+placesFrom+w.sql(), w.args...)
	if err != nil {
		return nil, err
	}
	return Row{"places": ps, "total_docs": total}, nil
}

// GetPlace returns one place with its files, newest meeting first; nil when
// the id is unknown.
func (s *Store) GetPlace(id int64) (Row, error) {
	p, err := s.row("SELECT id, name, kind, lat, lon FROM places WHERE id = ?", id)
	if err != nil || p == nil {
		return nil, err
	}
	docs, err := s.rows(`SELECT f.id, f.label, f.doc_type, f.meeting_id, m.date AS meeting_date,
		ai.title AS agenda_title, fp.n, fp.quote`+placesFrom+agendaJoin+
		` WHERE fp.place_id = ? ORDER BY m.date DESC, f.id DESC`, id)
	if err != nil {
		return nil, err
	}
	p["documents"] = docs
	return p, nil
}
