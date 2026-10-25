package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

// speechStore is testStore plus the speeches tables, verbatim from
// isosceles/main db.py until schema.sql carries them.
func speechStore(t *testing.T) *store.Store {
	st := testStore(t)
	mustExec(t, st, `CREATE TABLE IF NOT EXISTS speeches (
		id TEXT PRIMARY KEY, meeting_id TEXT REFERENCES meetings(id), agenda_anchor TEXT, top_label TEXT,
		speaker TEXT, party TEXT, video_id TEXT, start_s REAL, end_s REAL, text_raw TEXT, text TEXT,
		created_at TEXT DEFAULT (datetime('now')))`)
	mustExec(t, st, `CREATE TABLE IF NOT EXISTS speech_judgements (
		speech_id TEXT PRIMARY KEY, text_sha TEXT, answers TEXT, judged_at TEXT DEFAULT (datetime('now')))`)
	return st
}

func roster(names ...string) []RosterMember {
	out := make([]RosterMember, len(names))
	for i, n := range names {
		out[i] = RosterMember{Name: n}
	}
	return out
}

func TestMemberFor(t *testing.T) {
	ms := roster("Dirk Müller", "Dr. Thomas Müller", "Katrin Schütz", "Nicolas Schütz", "Dr. Anton Huber", "Sonja Klingert")
	for in, want := range map[string]string{
		"Müller": "Dirk Müller", "Dr. Müller": "Dr. Thomas Müller", "Dr. Thomas Müller": "Dr. Thomas Müller",
		"Schütz, K.": "Katrin Schütz", "Schütz, N.": "Nicolas Schütz", "Dr. Huber": "Dr. Anton Huber",
		"Dr.Huber": "Dr. Anton Huber", "Dr. Klingert": "Sonja Klingert",
		"Bürgermeister Flur": "", "Unbekannt": "", "Schütz": "",
	} {
		got := ""
		if m := memberFor(in, ms); m != nil {
			got = m.Name
		}
		if got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestJoinSpeakers(t *testing.T) {
	segs := []Segment{{0, 4, "a"}, {4, 8, "b"}, {8, 12, "c"}, {12, 16, "d"}}
	x, top1, top2 := "X", "TOP 1", "TOP 2"
	spans := []SpeakerSpan{{Start: 0, End: 6, Top: &top1}, {Start: 6, End: 13, Name: &x, Top: &top1}, {Start: 13, End: 20, Name: &x, Top: &top2}}
	var got [][3]string
	for _, s := range joinSpeakers(segs, spans) {
		got = append(got, [3]string{s.Speaker, s.Top, s.Text})
	}
	want := [][3]string{{"", "TOP 1", "a"}, {"X", "TOP 1", "b c"}, {"X", "TOP 2", "d"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestCanonicalNames(t *testing.T) {
	sp := []Speech{{Speaker: "Bürgermeister Flur"}, {Speaker: "Daniel Flurer"}, {Speaker: "Herr Noé"}, {}, {Speaker: "Katrin Schütz"}, {Speaker: "Dirk Müller"}}
	canonicalNames(sp, []string{"Dr. Stefan Noé", "Katrin Schütz", "Nicolas Schütz", "Dirk Müller", "Dr. Thomas Müller"})
	var got []string
	for _, s := range sp {
		got = append(got, s.Speaker)
	}
	want := []string{"Daniel Flurer", "Daniel Flurer", "Dr. Stefan Noé", "", "Katrin Schütz", "Dirk Müller"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestMergeKept(t *testing.T) {
	frag := func(t0, t1 float64) *Speech {
		return &Speech{Keep: true, Start: t0, End: t1, Text: "x", Clean: "x."}
	}
	if got := mergeKept([]*Speech{frag(0, 5), frag(10, 20)}); len(got) != 1 || got[0].Clean != "x.\n\nx." {
		t.Errorf("got %+v", got)
	}
	if got := mergeKept([]*Speech{frag(0, 5), frag(500, 520)}); len(got) != 2 {
		t.Errorf("got %d", len(got))
	}
}

func TestFillGaps(t *testing.T) {
	part := func(who, text string) Speech {
		return Speech{Speaker: who, PanelSpeaker: who, Top: "TOP 20", End: 1, Text: text}
	}
	speakers := func(sp []Speech) (out []string) {
		for _, s := range sp {
			out = append(out, s.Speaker)
		}
		return out
	}
	got := fillGaps([]Speech{part("M", "schrittweise auf fünf"), part("", strings.Repeat("Prozent hochgehen. ", 30)), part("M", "Das wird uns nicht bringen.")})
	if !reflect.DeepEqual(speakers(got), []string{"M"}) {
		t.Errorf("got %q", speakers(got))
	}
	got = fillGaps([]Speech{part("M", "Herr Bürgermeister."), part("", strings.Repeat("Vielen Dank. ", 30)), part("M", "Danke.")})
	if !reflect.DeepEqual(speakers(got), []string{"M", "", "M"}) {
		t.Errorf("got %q", speakers(got))
	}
}

func TestMentions(t *testing.T) {
	ms := []RosterMember{{Name: "Katrin Schütz", PartyCode: "CDU", Role: "Stadträtin"},
		{Name: "Dr. Stefan Noé", PartyCode: "FDP"},
		{Name: "Dr. Frank Mentrup", PartyCode: "SPD", Role: "Oberbürgermeister Vorsitz"}}
	got := mentions("Wie die CDU und Frau Stadträtin Schütz sagen, Herr Oberbürgermeister Mentrup, "+
		"die Grünen sehen das anders. Eine grüne Wiese für Karlsruhe. Herr Noé, bitte.", ms, "SPD")
	if len(got) != 3 || len(got["CDU"]) != 2 || len(got["B90"]) != 1 || len(got["FDP"]) != 1 {
		t.Errorf("got %v", got)
	}
	if got := mentions("die Grünen", ms, "B90"); len(got) != 0 {
		t.Errorf("own party: %v", got)
	}
	for text, n := range map[string]int{"Die Linke.": 1, "Die Linke will": 0, "die linke Spur": 0, "Linken": 1, "Die Grüne Jugend": 1,
		"die Grüne wiese": 0, "Partei Die PARTEI": 2, "Partei der": 0, "CDUler": 0} {
		total := 0
		for _, v := range mentions(text, nil, "") {
			total += len(v)
		}
		if total != n {
			t.Errorf("%q: %d mentions, want %d", text, total, n)
		}
	}
}

func TestProcessCleanup(t *testing.T) {
	st := speechStore(t)
	mustExec(t, st, `INSERT INTO bodies(id, name) VALUES('gr','Gemeinderat')`)
	mustExec(t, st, `INSERT INTO meetings(id, body_id, body_name, title, date) VALUES('termin-1','gr','Gemeinderat','GR','2026-09-29')`)
	mustExec(t, st, `INSERT INTO members(body_id, person_id, name, party, role) VALUES('gr','1','Jorinda Fahringer','GRÜNE','Stadträtin'),('gr','2','Petra Lorenz','','Stadträtin')`)
	mustExec(t, st, `INSERT INTO agenda_items(meeting_id, anchor, number, title) VALUES('termin-1','top6','6','Radwege')`)
	dir := t.TempDir()
	video := filepath.Join(dir, "2026-09-29_VID.mkv")
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-09-29_VID.transcript.json", []Segment{
		{10, 14, "Ich rufe auf Tagesordnungspunkt 6."},
		{20, 30, "Wir wollen mehr Radwege."},
		{30, 40, "Danke."},
		{50, 60, "Ich sehe das anders"},
	})
	fahr, lor, top6 := "Fahringer", "Lorenz", "TOP 6"
	write("2026-09-29_VID.speakers.json", []SpeakerSpan{
		{Start: 0, End: 20, Panel: true},
		{Start: 20, End: 30, Panel: true, Name: &fahr, Top: &top6},
		{Start: 30, End: 50, Panel: true, Top: &top6},
		{Start: 50, End: 60, Panel: true, Name: &lor, Top: &top6},
	})
	var users []string
	old := CleanupCall
	t.Cleanup(func() { CleanupCall = old })
	CleanupCall = func(system, user string) (string, error) {
		users = append(users, user)
		// first answer is cut off, so the chunk is split
		if len(users) == 1 {
			return `{"speeches": [`, nil
		}
		type o struct {
			I       int     `json:"i"`
			Keep    bool    `json:"keep"`
			Speaker *string `json:"speaker"`
			Text    string  `json:"text"`
		}
		var out []o
		for i, b := range strings.Split(strings.SplitN(user, "\n\nReden:\n\n", 2)[1], "\n\n") {
			head, text, _ := strings.Cut(b, "\n")
			x := o{I: i, Keep: !strings.HasPrefix(text, "Ich rufe"), Text: text}
			if strings.HasSuffix(head, "unbekannt") {
				name := "Herr Lorenz"
				x.Speaker = &name
			}
			out = append(out, x)
		}
		b, _ := json.Marshal(map[string]any{"speeches": out})
		return string(b), nil
	}
	got, err := ProcessCleanup(st, video, "termin-1", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || len(users) != 3 {
		t.Fatalf("got %+v after %d calls", got, len(users))
	}
	if !strings.Contains(users[1], "Mitglieder des Gemeinderats (Name, Fraktion, Funktion):\n- Jorinda Fahringer (GRÜNE, Stadträtin)") {
		t.Errorf("glossary: %s", users[1])
	}
	rows, err := st.DB.Query("SELECT id, agenda_anchor, speaker, COALESCE(party,''), text FROM speeches ORDER BY start_s")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][5]string
	for rows.Next() {
		var r [5]string
		rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4])
		out = append(out, r)
	}
	want := [][5]string{
		{"termin-1:VID:20", "top6", "Jorinda Fahringer", "GRÜNE", "Wir wollen mehr Radwege."},
		// named by the model, canonicalised to the roster
		{"termin-1:VID:30", "top6", "Petra Lorenz", "", "Danke."},
		// members.party is empty; the seat map gives the group
		{"termin-1:VID:50", "top6", "Petra Lorenz", "FDP/FW", "Ich sehe das anders"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("rows %q", out)
	}
}

func TestJudgeSpeeches(t *testing.T) {
	st := speechStore(t)
	mustExec(t, st, `INSERT INTO bodies(id, name) VALUES('gr','Gemeinderat')`)
	mustExec(t, st, `INSERT INTO meetings(id, body_id, body_name, title, date) VALUES('termin-1','gr','Gemeinderat','GR','2026-09-29')`)
	mustExec(t, st, `INSERT INTO members(body_id, person_id, name, party, party_code, role) VALUES('gr','1','Katrin Schütz','CDU','CDU','Stadträtin')`)
	mustExec(t, st, `INSERT INTO agenda_items(meeting_id, anchor, number, title) VALUES('termin-1','top14','14','Haushalt'),('termin-1','top14.1','14.1','Antrag FDP')`)
	mustExec(t, st, `INSERT INTO speeches(id, meeting_id, agenda_anchor, speaker, text) VALUES
		('a','termin-1','top14','Katrin Schütz','Die SPD und die CDU wollen das.'),('b','termin-1',NULL,NULL,'Danke.')`)
	var asked []map[string]typesafe.Question
	old := JevSystemOne
	t.Cleanup(func() { JevSystemOne = old })
	JevSystemOne = func(state any, q map[string]typesafe.Question) (map[string]typesafe.Answer, error) {
		asked = append(asked, q)
		out := map[string]typesafe.Answer{}
		for k := range q {
			out[k] = typesafe.Answer{Choice: "x", Probabilities: json.RawMessage(`{"x":1.0}`)}
		}
		return out, nil
	}
	n, err := JudgeSpeeches(st, "termin-1", false)
	if err != nil || n != 2 {
		t.Fatalf("judged %d, %v", n, err)
	}
	var keys []string
	for k := range asked[0] {
		keys = append(keys, k)
	}
	if len(asked[0]) != 5 || asked[0]["to_SPD"].Type != "choice" || asked[0]["stance_top14.1"].Type != "choice" {
		t.Errorf("questions %v", keys)
	}
	if !strings.Contains(asked[0]["stance"].Instructions, "«Haushalt»") || !strings.Contains(asked[1]["stance"].Instructions, "«?»") {
		t.Error("stance title")
	}
	var answers string
	st.DB.QueryRow("SELECT answers FROM speech_judgements WHERE speech_id='b'").Scan(&answers)
	if answers != `{"sentiment":{"x":1.0},"stance":{"x":1.0},"tone":{"x":1.0}}` {
		t.Errorf("answers %s", answers)
	}
	if n, _ := JudgeSpeeches(st, "termin-1", false); n != 0 {
		t.Errorf("judged unchanged speeches again: %d", n)
	}
	mustExec(t, st, `UPDATE speeches SET text='Neu.' WHERE id='b'`)
	if n, _ := JudgeSpeeches(st, "", false); n != 1 {
		t.Errorf("re-cleaned speech: %d", n)
	}
}

func TestRunVODPipeline(t *testing.T) {
	st := speechStore(t)
	mustExec(t, st, `INSERT INTO meetings(id, body_name, title, date) VALUES('termin-1','Gemeinderat','GR','2026-09-29'),
		('termin-2','Gemeinderat','GR','2026-10-20'),('termin-0','Gemeinderat','GR','2025-07-22')`)
	mustExec(t, st, `INSERT INTO votes(id, meeting_id, agenda_anchor, source) VALUES('termin-1:top4','termin-1','top4','vod')`)
	dir := t.TempDir()
	rec := func(base, mid string, dur float64, speakers bool) {
		b, _ := json.Marshal(vodRecord{VideoID: base, Media: base + ".mkv", MeetingID: &mid, Duration: &dur})
		os.WriteFile(filepath.Join(dir, base+".vod.json"), b, 0o644)
		os.WriteFile(filepath.Join(dir, base+".transcript.json"), []byte("[]"), 0o644)
		if speakers {
			os.WriteFile(filepath.Join(dir, base+".speakers.json"), []byte("[]"), 0o644)
		}
	}
	rec("a", "termin-1", 13663, true)
	rec("b", "termin-2", 11000, false)
	rec("c", "termin-0", 9000, false)
	rec("d", "termin-2", 120, false)
	var steps []string
	v, sp, sc, j := pipelineVotes, pipelineSpeakers, pipelineSpeeches, pipelineJudge
	t.Cleanup(func() { pipelineVotes, pipelineSpeakers, pipelineSpeeches, pipelineJudge = v, sp, sc, j })
	pipelineVotes = func(_ *store.Store, video, mid, date string) (int, error) {
		steps = append(steps, "votes "+filepath.Base(video)+" "+date)
		return 0, os.ErrNotExist
	}
	pipelineSpeakers = func(video, date string) ([]SpeakerSpan, error) {
		steps = append(steps, "speakers "+filepath.Base(video))
		return nil, nil
	}
	pipelineSpeeches = func(_ *store.Store, video, mid string) (int, error) {
		steps = append(steps, "speeches "+filepath.Base(video))
		return 3, nil
	}
	pipelineJudge = func(_ *store.Store, mid string) (int, error) {
		steps = append(steps, "judge "+mid)
		return 1, nil
	}
	c, err := RunVODPipeline(st, dir, true)
	if err != nil || c.Videos != 2 || len(steps) != 0 {
		t.Fatalf("dry run %+v %v %v", c, steps, err)
	}
	c, err = RunVODPipeline(st, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// a failed votes step skips the steps after it; judging still runs
	want := []string{"speeches a.mkv", "votes b.mkv 2026-10-20", "judge termin-1", "judge termin-2"}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps %q", steps)
	}
	if c != (PipelineCounts{Videos: 2, Speeches: 3, Judged: 2, Failed: 1}) {
		t.Errorf("counts %+v", c)
	}
}
