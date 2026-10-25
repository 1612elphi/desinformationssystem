package ingest

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// roundTrip turns v into the plain JSON values Python's json.load gives.
func roundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// testdata/python-parse.json is scraper.py's output on the same saved pages.
func TestParsersMatchPython(t *testing.T) {
	var want map[string]any
	if err := json.Unmarshal([]byte(readFixture(t, "python-parse.json")), &want); err != nil {
		t.Fatal(err)
	}
	cs, err := ParseCommittees(readFixture(t, "gremien.html"))
	if err != nil {
		t.Fatal(err)
	}
	pairs := [][2]string{}
	for _, c := range cs {
		pairs = append(pairs, [2]string{c.ID, c.Name})
	}
	members, err := ParseMembers(readFixture(t, "org.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{
		"committees": pairs,
		"termine":    ParseCalendarTermine(readFixture(t, "termine.html")),
		"members":    members,
	}
	for _, id := range []string{"termin-10548", "termin-10443"} {
		if got[id], err = ParseMeeting(readFixture(t, id+".html"), "https://sitzungskalender.karlsruhe.de/db/ratsinformation/"+id, id); err != nil {
			t.Fatal(err)
		}
	}
	for k, w := range want {
		if g := roundTrip(t, got[k]); !reflect.DeepEqual(g, w) {
			t.Errorf("%s differs from Python:\n got %v\nwant %v", k, g, w)
		}
	}
}

func TestParseMeetingAnchors(t *testing.T) {
	page := `<html><head><title>Karlsruhe: Ratsinformation - Bauausschuss (öffentlich)</title></head><body>
<table class="contenttable"><tr><th>Datum:</th><td>19. Mai 2026, 15.30 bis 20 Uhr</td></tr>
<tr><th>Raum:</th><td>Bürgersaal</td></tr></table>
<details class="top" id="dtop3"><summary><span class="summary-title"><a class="anchor">#</a>3 – Neubau&nbsp;Schule #</span></summary>
<div class="detail-contents">öffentlich <a href="https://web1.karlsruhe.de/ris/downloadfiles/123.pdf">Beschlussvorlage</a></div></details>
<a href="https://web1.karlsruhe.de/ris/downloadfiles/123.pdf">again</a>
<a href="/ris/downloadfiles/456.PDF">Niederschrift</a>
<a href="https://evil.example/downloadfiles/9.pdf">x</a>
</body></html>`
	m, err := ParseMeeting(page, "https://sitzungskalender.karlsruhe.de/db/ratsinformation/termin-1", "termin-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.BodyName != "Bauausschuss" || *m.Date != "2026-05-19" || *m.Time != "15.30 bis 20 Uhr" || *m.Location != "Bürgersaal" || *m.Public != 1 {
		t.Fatalf("meeting fields: %+v", m)
	}
	if len(m.Agenda) != 1 || m.Agenda[0].Anchor != "top3" || *m.Agenda[0].Number != "3" || m.Agenda[0].Title != "Neubau\u00a0Schule" || *m.Agenda[0].Public != 1 {
		t.Fatalf("agenda: %+v", m.Agenda)
	}
	if len(m.Files) != 2 {
		t.Fatalf("files: %+v", m.Files)
	}
	if f := m.Files[0]; f.ID != "123" || f.AgendaAnchor == nil || *f.AgendaAnchor != "top3" || f.DocType != "Beschlussvorlage" {
		t.Errorf("agenda file lost its anchor: %+v", f)
	}
	if f := m.Files[1]; f.ID != "456" || f.AgendaAnchor != nil || f.URL != "https://sitzungskalender.karlsruhe.de/ris/downloadfiles/456.PDF" || f.Filename != "456.PDF" {
		t.Errorf("meeting-level file: %+v", f)
	}
}

func TestParseGermanDate(t *testing.T) {
	for in, want := range map[string]string{
		"Datum: 3. März 2026":   "2026-03-03",
		"am 1.Januar2025 um":    "2025-01-01",
		"31. Februar 2026":      "",
		"19. Mai":               "",
		"7. Maerz 2024, 10 Uhr": "2024-03-07",
		"12. Dezember 2026":     "2026-12-12",
	} {
		if got := parseGermanDate(in); got != want {
			t.Errorf("parseGermanDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMeetingPublic(t *testing.T) {
	val := func(p *int) any {
		if p == nil {
			return nil
		}
		return *p
	}
	for in, want := range map[string]any{
		"Gemeinderat (öffentlich/nicht öffentlich)": nil,
		"Gemeinderat (nicht öffentlich)":            0,
		"Gemeinderat (nichtöffentlich)":             0,
		"Gemeinderat (öffentlich)":                  1,
		"Gemeinderat":                               nil,
	} {
		if got := val(MeetingPublic(in)); got != want {
			t.Errorf("MeetingPublic(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	for in, want := range map[string]string{
		"Wortprotokoll":                     "Wortprotokoll",
		"Protokoll mit Abstimmungsergebnis": "Abstimmungsergebnis",
		"Beschlussvorlage":                  "Beschlussvorlage",
		"Präsentation Verwaltung":           "Präsentation",
		"Anlage 1 Lageplan":                 "Anlage",
		"Bebauungsplan":                     "Plan",
		"Foo":                               "Sonstiges",
		"Antrag der CDU":                    "Antrag",
		"ABSTIMMUNGSERGEBNIS TOP 3":         "Abstimmungsergebnis",
		"Niederschrift öffentlicher Teil":   "Niederschrift",
	} {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractVorlagen(t *testing.T) {
	own, refs := ExtractVorlagen("Beschlussvorlage", "Vorlage: 2026/0324\nsiehe Drucksache 2025 - 112 und Vorlagen-Nr. 2026-0400; Haushalt Vorlage 2026/2027", "Beschlussvorlage")
	if own != "2026/0324" || !reflect.DeepEqual(refs, []string{"2025/112", "2026/0324", "2026/0400"}) {
		t.Errorf("got %q %v", own, refs)
	}
	nbsp := string(rune(0xa0))
	own, refs = ExtractVorlagen("Tagesordnung", "Vorlagennummer 2026/0002, Vorlagen-Nr.:"+nbsp+"2026/0001", "Tagesordnung")
	if own != "" || !reflect.DeepEqual(refs, []string{"2026/0001"}) {
		t.Errorf("mention only: got %q %v", own, refs)
	}
}

func TestDeriveSubmitters(t *testing.T) {
	cases := []struct {
		label, text, docType string
		want                 []string
	}{
		{"Antrag der CDU-Fraktion", "", "Antrag", []string{"CDU"}},
		{"Antrag", "Gemeinsamer Antrag von SPD und Bündnis 90/Die Grünen", "Antrag", []string{"B90", "SPD"}},
		{"Anfrage", "Anfrage der Freien Wähler", "Anfrage", []string{}},
		{"Anfrage", "Anfrage FW|FDP", "Anfrage", []string{"FDP", "FWV"}},
		{"Beschlussvorlage", "", "Beschlussvorlage", []string{"SVK"}},
		{"Anlage Volt", "", "Anlage", []string{"VOL"}},
		{"Anlage", "", "Sonstiges", []string{}},
		{"FÜR Ka Investitionsliste", "", "Anlage", []string{"FÜR"}},
		{"Anlage für Karlsruhe", "", "Anlage", []string{}},
	}
	for _, c := range cases {
		if got := DeriveSubmitters(c.label, c.text, c.docType); !reflect.DeepEqual(got, c.want) {
			t.Errorf("DeriveSubmitters(%q, %q, %q) = %v, want %v", c.label, c.text, c.docType, got, c.want)
		}
	}
}

func TestPyre(t *testing.T) {
	if !pyre(`a\sb`).MatchString("a b") || !pyre(`a[:\s]b`).MatchString("a b") || pyre(`a[^\s]b`).MatchString("a b") {
		t.Error("pyre does not widen \\s to Unicode whitespace")
	}
}

func TestParseMembersWithoutComma(t *testing.T) {
	page := `<table class="contenttable"><tr><td><a href="people-1">Anna Beispiel</a></td>
		<td>FDP/FW Stadträtin Ordentliches Mitglied</td></tr></table>`
	got, err := ParseMembers(page)
	if err != nil {
		t.Fatal(err)
	}
	want := []Member{{"1", "Anna Beispiel", "FDP/FW", "FDP", "Stadträtin Ordentliches Mitglied", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
