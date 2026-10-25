package store

import (
	"reflect"
	"testing"
)

func TestMentions(t *testing.T) {
	members := []Row{
		{"name": "Katrin Schütz", "party": "CDU", "party_code": "CDU", "role": "Stadträtin"},
		{"name": "Dr. Frank Mentrup", "party": "SPD", "party_code": "SPD", "role": "Oberbürgermeister Vorsitz"},
	}
	codes, found := mentions("Wie die CDU und Frau Stadträtin Schütz sagen, Herr Oberbürgermeister Mentrup, "+
		"die Grünen sehen das anders. Eine grüne Wiese für Karlsruhe.", members, "SPD")
	if !reflect.DeepEqual(codes, []string{"B90", "CDU"}) || len(found["CDU"]) != 2 {
		t.Fatalf("got %v %v", codes, found)
	}
	if codes, _ := mentions("die Grünen", members, "B90"); len(codes) != 0 {
		t.Fatalf("own party counted: %v", codes)
	}
	if codes, _ := mentions("die Grüne Fraktion, die Grüne wiese", nil, ""); !reflect.DeepEqual(codes, []string{"B90"}) {
		t.Fatalf("got %v", codes)
	}
	noe := []Row{{"name": "Lüppo Noé", "party_code": "B90"}}
	if codes, _ := mentions("Herr Stadtrat Noé hat recht", noe, ""); !reflect.DeepEqual(codes, []string{"B90"}) {
		t.Fatalf("non-ASCII surname end: %v", codes)
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
