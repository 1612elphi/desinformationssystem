package store

import "testing"

func TestAddPartyLine(t *testing.T) {
	lookup := map[string]map[string][]partyCand{"b": {
		"arndt":  {{"", "CDU", "1"}},
		"berger": {{"", "CDU", "2"}},
		"conrad": {{"", "CDU", "3"}},
		"dorn":   {{"", "SPD", "4"}},
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
