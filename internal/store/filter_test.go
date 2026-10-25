package store

import (
	"path/filepath"
	"testing"
)

func filterStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO bodies(id, name) VALUES('gr', 'g'), ('oa', 'o'), ('pa', 'p')`,
		`INSERT INTO meetings(id, date, title, body_id, body_name) VALUES
			('termin-1', '2026-10-20', 'Sitzung des Gemeinderates', 'gr', 'Gemeinderat'),
			('termin-2', '2026-10-21', 'Sitzung', 'oa', 'Ausschuss für Öffentliche Ordnung'),
			('termin-3', '2026-10-22', 'Straßenbau', 'pa', 'Planungsausschuss')`,
		`INSERT INTO speeches(id, meeting_id, speaker, text, start_s) VALUES
			('termin-1:v:1', 'termin-1', 'A', 'Der Radweg kommt', 1),
			('termin-2:v:1', 'termin-2', 'B', 'Radweg ja', 1)`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestMeetingQueryFolds(t *testing.T) {
	s := filterStore(t)
	for q, want := range map[string]int64{
		"öffentliche": 1, // non-ascii case
		"STRASSE":     1, // ß folds to ss
		"sitzung":     2,
		"gemeinderat": 1, // one row, two fields
		"nichts":      0,
	} {
		n, err := s.CountMeetings(MeetingFilter{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("q=%q: %d meetings, want %d", q, n, want)
		}
	}
	rows, err := s.ListMeetings(MeetingFilter{Query: "sitzung", DateFrom: "2026-10-21"}, true, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["id"] != "termin-2" {
		t.Fatalf("q with from: %v", rows)
	}
}

func TestSpeechSearchMeeting(t *testing.T) {
	s := filterStore(t)
	res, err := s.SearchSpeeches(SpeechSearch{Query: "radweg", Meeting: "termin-2", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	rows := res["results"].([]Row)
	if res["total"] != int64(1) || len(rows) != 1 || rows[0]["meeting_id"] != "termin-2" {
		t.Fatalf("meeting filter: %v", res)
	}
}
