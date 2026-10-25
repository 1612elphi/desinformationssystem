package store

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSpeechCounts(t *testing.T) {
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
	for i, r := range [][2]any{{nil, "A"}, {nil, "A"}, {"top1", "A"}, {"top1", "A"}, {"top1", "B"}, {"top1", ""},
		{"top1", ""}, {"top1", nil}, {"top1", nil}, {"top2", "B"}, {"top1", "B"}, {nil, "A"}} {
		if _, err := s.DB.Exec(`INSERT INTO speeches(id, meeting_id, agenda_anchor, speaker, start_s, text)
			VALUES(?, 'termin-1', ?, ?, ?, 'x')`, i, r[0], r[1], i); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.SpeechCounts("termin-1")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := s.MeetingSpeeches("termin-1", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{}
	for _, r := range sp {
		want[str(r["agenda_anchor"])]++
	}
	if !reflect.DeepEqual(got, want) || got["top1"] != 7 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMentions(t *testing.T) {
	members := []Row{
		{"name": "Katrin Schütz", "party": "CDU", "party_code": "CDU", "role": "Stadträtin"},
		{"name": "Dr. Frank Mentrup", "party": "SPD", "party_code": "SPD", "role": "Oberbürgermeister Vorsitz"},
	}
	codes, found := Mentions("Wie die CDU und Frau Stadträtin Schütz sagen, Herr Oberbürgermeister Mentrup, "+
		"die Grünen sehen das anders. Eine grüne Wiese für Karlsruhe.", members, "SPD")
	if !reflect.DeepEqual(codes, []string{"B90", "CDU"}) || len(found["CDU"]) != 2 {
		t.Fatalf("got %v %v", codes, found)
	}
	if codes, _ := Mentions("die Grünen", members, "B90"); len(codes) != 0 {
		t.Fatalf("own party counted: %v", codes)
	}
	if codes, _ := Mentions("die Grüne Fraktion, die Grüne wiese", nil, ""); !reflect.DeepEqual(codes, []string{"B90"}) {
		t.Fatalf("got %v", codes)
	}
	noe := []Row{{"name": "Lüppo Noé", "party_code": "B90"}}
	if codes, _ := Mentions("Herr Stadtrat Noé hat recht", noe, ""); !reflect.DeepEqual(codes, []string{"B90"}) {
		t.Fatalf("non-ASCII surname end: %v", codes)
	}
	for text, n := range map[string]int{"Die Linke.": 1, "Die Linke will": 0, "die linke Spur": 0, "Linken": 1, "Die Grüne Jugend": 1,
		"die Grüne wiese": 0, "Partei Die PARTEI": 2, "Partei der": 0, "CDUler": 0} {
		total := 0
		_, found := Mentions(text, nil, "")
		for _, v := range found {
			total += len(v)
		}
		if total != n {
			t.Errorf("%q: %d mentions, want %d", text, total, n)
		}
	}
}

func TestTokens(t *testing.T) {
	if got := tokens("Sehr geehrter Herr Oberbürgermeister, der Radweg! Nochmal gesagt."); !reflect.DeepEqual(got, []string{"Radweg"}) {
		t.Fatalf("got %v", got)
	}
}

func TestReadability(t *testing.T) {
	// 9 words, 2 sentences
	f, asl := readability("Das ist ein Satz. Das ist noch ein Satz. ")
	if !(60 < f && f < 120) || asl != 4.5 {
		t.Fatalf("got %v %v", f, asl)
	}
}

func TestDistinctive(t *testing.T) {
	a, b := newOrdered[int](), newOrdered[int]()
	a.add("radweg", 10)
	a.add("haushalt", 2)
	b.add("radweg", 1)
	b.add("haushalt", 10)
	d := distinctive([]string{"A", "B"}, map[string]*ordered[int]{"A": a, "B": b}, 12, 1)
	if d["A"][0]["word"] != "radweg" || d["B"][0]["word"] != "haushalt" {
		t.Fatalf("got %v", d)
	}
}

func TestMemberFor(t *testing.T) {
	members := []Row{
		{"name": "Katrin Schütz"}, {"name": "Nicolas Schütz"},
		{"name": "Dr. Anton Huber"}, {"name": "Berta Huber"},
	}
	for name, want := range map[string]any{
		"Schütz, K.": "Katrin Schütz", "Schütz": nil, "Dr. Huber": "Dr. Anton Huber",
		"Huber": "Berta Huber", "Berta Huber": "Berta Huber", "Meier": nil,
	} {
		got := MemberFor(name, members)
		if (got == nil && want != nil) || (got != nil && got["name"] != want) {
			t.Errorf("MemberFor(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMemberForShortNames(t *testing.T) {
	var members []Row
	for _, n := range []string{"Dirk Müller", "Dr. Thomas Müller", "Katrin Schütz", "Nicolas Schütz", "Dr. Anton Huber", "Sonja Klingert"} {
		members = append(members, Row{"name": n})
	}
	for in, want := range map[string]string{
		"Müller": "Dirk Müller", "Dr. Müller": "Dr. Thomas Müller", "Dr. Thomas Müller": "Dr. Thomas Müller",
		"Schütz, K.": "Katrin Schütz", "Schütz, N.": "Nicolas Schütz", "Dr. Huber": "Dr. Anton Huber",
		"Dr.Huber": "Dr. Anton Huber", "Dr. Klingert": "Sonja Klingert",
		"Bürgermeister Flur": "", "Unbekannt": "", "Schütz": "",
	} {
		got := ""
		if m := MemberFor(in, members); m != nil {
			got = str(m["name"])
		}
		if got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestWithGroups(t *testing.T) {
	roster := []Row{
		{"name": "Dr. Frank Mentrup", "party_code": "SPD", "role": "Oberbürgermeister"},
		{"name": "Katrin Schütz", "party_code": "CDU"},
		{"name": "Anna Grün", "party_code": "B90"},
	}
	got := withGroups([]any{
		map[string]any{"name": "Unbekannt"}, map[string]any{"name": "Schütz, K."},
		map[string]any{"name": "Grün"}, map[string]any{"name": "Dr. Mentrup"},
	}, roster)
	var groups []any
	for _, e := range got {
		groups = append(groups, e.(map[string]any)["group"])
	}
	if !reflect.DeepEqual(groups, []any{"OB", "B90", "CDU", nil}) {
		t.Fatalf("got %v", groups)
	}
}

func TestPySum(t *testing.T) {
	var p pySum
	for _, x := range []float64{0.1, 0.2, 0.3} {
		p.add(x)
	}
	// Python 3.12: sum([0.1, 0.2, 0.3]) == 0.6; plain addition gives 0.6000000000000001
	if p.total() != 0.6 {
		t.Fatalf("got %v", p.total())
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet("Die Straße ist da", "IST"); got != "Die Straße ist da…" {
		t.Fatalf("got %q", got)
	}
}

func TestSQLLike(t *testing.T) {
	for _, c := range []struct {
		s, p string
		want bool
	}{
		{"14.1", "14.%", true}, {"14", "14.%", false}, {"14.1", "14", false}, {"top14.1", "TOP14.%", true},
		{"14x1", "14_1", true}, {"Ä.1", "ä.%", false}, {"a.b.c", "%.c", true}, {"abc", "a%c%", true},
		{"", "%", true}, {"", "_", false}, {"ü", "_", true},
	} {
		if got := sqlLike(c.s, c.p); got != c.want {
			t.Errorf("%q LIKE %q = %v", c.s, c.p, got)
		}
	}
}
