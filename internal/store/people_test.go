package store

import "testing"

func TestAddPartyLine(t *testing.T) {
	lookup := map[string]map[string][]partyCand{"b": {
		"arndt":  {{"", "CDU", "1", false}},
		"berger": {{"", "CDU", "2", false}},
		"conrad": {{"", "CDU", "3", false}},
		"dorn":   {{"", "SPD", "4", false}},
	}}
	roll := []map[string]any{
		{"name": "Arndt", "vote": "nein"},
		{"name": "Berger", "vote": "ja"},
		{"name": "Conrad", "vote": "ja"},
		{"name": "Dorn", "vote": "nein"},
		{"name": "Unbekannt", "vote": "ja"},
	}
	r := Row{}
	addPartyLine(r, roll, lookup, "b", "Arndt")
	if r["party_code"] != "CDU" || r["party_line"] != "ja" {
		t.Fatalf("got %v", r)
	}
	if c := r["party_tally"].(map[string]int); c["ja"] != 2 || c["nein"] != 1 {
		t.Fatalf("tally %v", c)
	}
	r = Row{}
	addPartyLine(r, roll, lookup, "b", "Unbekannt")
	if len(r) != 0 {
		t.Fatalf("unattributed got %v", r)
	}
}

func TestEntryMatchDoctorTitle(t *testing.T) {
	lookup := map[string]map[string][]partyCand{"gr": {
		"müller": {{"d", "CDU", "10", false}, {"t", "CDU", "11", true}},
	}}
	for roll, want := range map[string]string{"Müller": "10", "Dr. Müller": "11"} {
		party, person := entryMatch(lookup, "gr", roll)
		if party != "CDU" || person != want {
			t.Errorf("%s: got %s %s, want CDU %s", roll, party, person, want)
		}
	}
}
