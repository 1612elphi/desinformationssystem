package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestUpsertVotePrecedence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO meetings(id) VALUES('termin-1')"); err != nil {
		t.Fatal(err)
	}
	n := func(i int) *int { return &i }
	sp := func(v string) *string { return &v }
	upsert := func(v Vote) {
		t.Helper()
		if err := s.Write(func(tx *sql.Tx) error { return UpsertVote(tx, v) }); err != nil {
			t.Fatal(err)
		}
	}
	type row struct {
		source, result, members string
		ja, nein                sql.NullInt64
		ok                      sql.NullInt64
	}
	get := func() row {
		t.Helper()
		var r row
		var result, members sql.NullString
		if err := s.DB.QueryRow("SELECT source, result_text, members, ja, nein, members_ok FROM votes WHERE id='termin-1:top3'").
			Scan(&r.source, &result, &members, &r.ja, &r.nein, &r.ok); err != nil {
			t.Fatal(err)
		}
		r.result, r.members = result.String, members.String
		return r
	}

	upsert(Vote{MeetingID: "termin-1", AgendaAnchor: "top3", TopLabel: "TOP 3", ResultText: sp("Zustimmung"),
		Ja: n(2), Nein: n(0), Enthaltung: n(0), Members: []VoteMember{{"Müller", "ja"}, {"Schmidt", "ja"}},
		Source: "live", ImageURL: "img"})
	r := get()
	if r.source != "live" || r.members != `[{"name": "Müller", "vote": "ja"}, {"name": "Schmidt", "vote": "ja"}]` || r.ok.Int64 != 1 {
		t.Fatalf("live insert: %+v", r)
	}

	// a failed live parse keeps the stored tally and result text
	upsert(Vote{MeetingID: "termin-1", AgendaAnchor: "top3", TopLabel: "TOP 3", Source: "live", ImageURL: "img2"})
	if r = get(); r.ja.Int64 != 2 || r.result != "Zustimmung" || r.members == "" {
		t.Fatalf("NULLs clobbered: %+v", r)
	}

	upsert(Vote{MeetingID: "termin-1", AgendaAnchor: "top3", TopLabel: "TOP 3", Ja: n(30), Nein: n(5), Enthaltung: n(1),
		Source: "pdf", ImageURL: "pdf", FileID: sp("99")})
	if r = get(); r.source != "pdf" || r.ja.Int64 != 30 || r.ok.Int64 != 0 {
		t.Fatalf("pdf upsert: %+v", r)
	}

	upsert(Vote{MeetingID: "termin-1", AgendaAnchor: "top3", TopLabel: "TOP 3", Ja: n(1), Source: "live", ImageURL: "img3"})
	if r = get(); r.source != "pdf" || r.ja.Int64 != 30 {
		t.Fatalf("live downgraded a pdf row: %+v", r)
	}

	upsert(Vote{MeetingID: "termin-1", TopLabel: "TOP ?", Source: "live"})
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM votes").Scan(&count)
	if count != 1 {
		t.Errorf("vote without anchor stored: %d rows", count)
	}
	if sha, _ := s.VoteImageSHA("termin-1", "top3"); sha != "" {
		t.Errorf("sha %q", sha)
	}
}

func TestPyJSON(t *testing.T) {
	got := PyJSON(Obj{{"people", []string{"Ä \"x\"", "a\nb"}}, {"n", nil}, {"l", []any{1, true}}})
	want := `{"people": ["Ä \"x\"", "a\nb"], "n": null, "l": [1, true]}`
	if got != want {
		t.Errorf("got %s", got)
	}
	if PyJSON([]string{}) != "[]" {
		t.Error("empty list")
	}
}

func TestRecordVoteParseAttempt(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Init()
	s.RecordVoteParseAttempt("f", false)
	s.RecordVoteParseAttempt("f", false)
	if n, _ := s.VoteParseAttempts("f"); n != 2 {
		t.Errorf("attempts %d", n)
	}
	s.RecordVoteParseAttempt("f", true)
	if n, _ := s.VoteParseAttempts("f"); n != 0 {
		t.Errorf("success did not clear: %d", n)
	}
}

func TestListMeetingsVoteCount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO meetings(id, date) VALUES('a', '2026-10-01'), ('b', '2026-10-02');
		INSERT INTO votes(id, meeting_id, agenda_anchor, source) VALUES('a:1', 'a', '1', 'pdf'), ('a:2', 'a', '2', 'pdf')`); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListMeetings(MeetingFilter{DateFrom: "2026-10-01", DateTo: "2026-10-31"}, true, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["vote_count"] != int64(2) || rows[1]["vote_count"] != int64(0) {
		t.Fatalf("vote_count: %+v", rows)
	}
}
