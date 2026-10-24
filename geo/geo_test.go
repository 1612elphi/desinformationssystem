package geo

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

var sampleDocs = []store.Row{
	{"label": "Umgestaltung Kaiserstraße", "agenda_title": "Kaiserstraße und Marktplatz",
		"summary_de": "Die Kaiserstraße zwischen Marktplatz und Europaplatz; nochmal kaiserstraße.",
		"fulltext":   "Stadt Karlsruhe, Karl-Friedrich-Straße 10, 76133 Karlsruhe. Kaiserstraße."},
	{"label": "Durlacher Allee, Durlacher Tor und die Durlacher Straßenbahn; Am Bergwald, Am Berg.",
		"summary_de": nil, "fulltext": "XKaiserstraße Kaiserstraße_ KAISERSTRASSE Ettlinger-Tor-Platz/Ettlinger Straße"},
	{"label": "", "fulltext": strings.Repeat("Lorem ipsum dolor sit amet. ", 8) +
		"Die Kriegsstraße\n\nwird\x1c saniert. " + strings.Repeat("Ende des Textes. ", 12)},
	{"label": "Größe: Hauptstraße3 und Ettlinger Allee5 sowie (Europaplatz)"},
}

// expected hits are the output of geo.street_hits for sampleDocs
var pythonHits = [][]Hit{
	{
		{"Kaiserstraße", "label", "Umgestaltung «Kaiserstraße»"},
		{"Kaiserstraße", "agenda_title", "«Kaiserstraße» und Marktplatz"},
		{"Marktplatz", "agenda_title", "Kaiserstraße und «Marktplatz»"},
		{"Kaiserstraße", "summary_de", "Die «Kaiserstraße» zwischen Marktplatz und Europaplatz; nochmal kaiserstraße."},
		{"Marktplatz", "summary_de", "Die Kaiserstraße zwischen «Marktplatz» und Europaplatz; nochmal kaiserstraße."},
		{"Europaplatz", "summary_de", "Die Kaiserstraße zwischen Marktplatz und «Europaplatz»; nochmal kaiserstraße."},
		{"Karl-Friedrich-Straße", "fulltext", "Stadt Karlsruhe, «Karl-Friedrich-Straße» 10, 76133 Karlsruhe. Kaiserstraße."},
		{"Kaiserstraße", "fulltext", "Stadt Karlsruhe, Karl-Friedrich-Straße 10, 76133 Karlsruhe. «Kaiserstraße»."},
	},
	{
		{"Durlacher Allee", "label", "«Durlacher Allee», Durlacher Tor und die Durlacher Straßenbahn; Am Bergwald, Am Berg."},
		{"Durlacher Tor", "label", "Durlacher Allee, «Durlacher Tor» und die Durlacher Straßenbahn; Am Bergwald, Am Berg."},
		{"Am Berg", "label", "Durlacher Allee, Durlacher Tor und die Durlacher Straßenbahn; Am Bergwald, «Am Berg»."},
		{"Ettlinger-Tor-Platz", "fulltext", "XKaiserstraße Kaiserstraße_ KAISERSTRASSE «Ettlinger-Tor-Platz»/Ettlinger Straße"},
		{"Ettlinger Straße", "fulltext", "XKaiserstraße Kaiserstraße_ KAISERSTRASSE Ettlinger-Tor-Platz/«Ettlinger Straße»"},
	},
	{
		{"Kriegsstraße", "fulltext", "amet. Lorem ipsum dolor sit amet. Lorem ipsum dolor sit amet. Lorem ipsum dolor sit amet. Lorem ipsum dolor sit amet. Lorem ipsum dolor sit amet. Die «Kriegsstraße» wird saniert. Ende des Textes. Ende des Textes. Ende des Textes. Ende des Textes. Ende des Textes. Ende des Textes. Ende des Textes. Ende des Texte"},
	},
	{
		{"Europaplatz", "label", "Größe: Hauptstraße3 und Ettlinger Allee5 sowie («Europaplatz»)"},
	},
}

func TestStreetHitsMatchPython(t *testing.T) {
	for i, d := range sampleDocs {
		if got := StreetHits(d); !reflect.DeepEqual(got, pythonHits[i]) {
			t.Errorf("doc %d:\n got %q\nwant %q", i, got, pythonHits[i])
		}
	}
}

// fakeJev answers each request with answer(body) and records the raw bodies.
func fakeJev(t *testing.T, answer func(body string) map[string]typesafe.Answer) *[]string {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		json.NewEncoder(w).Encode(map[string]any{"answers": answer(string(b))})
	}))
	t.Cleanup(srv.Close)
	old := typesafe.Endpoint
	typesafe.Endpoint = srv.URL + "/v1/systemone"
	t.Cleanup(func() { typesafe.Endpoint = old })
	return &bodies
}

func TestConfirmStreets(t *testing.T) {
	bodies := fakeJev(t, func(body string) map[string]typesafe.Answer {
		switch {
		case strings.Contains(body, `contains «Karl-Friedrich-Straße»`):
			return map[string]typesafe.Answer{"ref": {Choice: "address", Confidence: 0.95}}
		case strings.Contains(body, `contains «Marktplatz»`):
			return map[string]typesafe.Answer{"ref": {Choice: "location", Confidence: 0.5}}
		}
		return map[string]typesafe.Answer{"ref": {Choice: "location", Confidence: 0.8}}
	})
	got, err := ConfirmStreets(sampleDocs[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Europaplatz", "Kaiserstraße"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(*bodies) != len(pythonHits[0]) {
		t.Errorf("%d requests, want one per hit (%d)", len(*bodies), len(pythonHits[0]))
	}
	want := `{"model":"jev-latest","state":"Umgestaltung «Kaiserstraße»","questions":{"ref":{"type":"choice",` +
		`"instructions":"The passage contains «Kaiserstraße» marked with guillemets. Judge only that occurrence: what does it refer to?",` +
		`"criteria":{"location":"refers to the Karlsruhe street, square, or place itself (the physical location)",` +
		`"person":"refers to a person (e.g. the individual a street is named after), not the place",` +
		`"address":"part of a postal address, sender/recipient block, letterhead, or contact line",` +
		`"incidental":"an ordinary word or phrase that coincides with the name but is not the place"}}}}`
	if (*bodies)[0] != want {
		t.Errorf("request body\n got %s\nwant %s", (*bodies)[0], want)
	}
}

func TestClassifyDistrict(t *testing.T) {
	choice := "keine"
	bodies := fakeJev(t, func(string) map[string]typesafe.Answer {
		return map[string]typesafe.Answer{"d": {Choice: choice, Confidence: 0.7}}
	})
	d, conf, err := ClassifyDistrict(sampleDocs[0])
	if err != nil || d != nil || conf == nil || *conf != 0.7 {
		t.Fatalf("keine: got %v %v %v", d, conf, err)
	}
	if !strings.Contains((*bodies)[0], `"state":"Umgestaltung Kaiserstraße Kaiserstraße und Marktplatz Die Kaiserstraße`) ||
		!strings.Contains((*bodies)[0], `"criteria":{"Beiertheim-Bulach":null,"Daxlanden":null,`) ||
		!strings.Contains((*bodies)[0], `"Wolfartsweier":null,"keine":"city-wide, or no specific Karlsruhe Stadtteil is the subject"}`) {
		t.Errorf("request body %s", (*bodies)[0])
	}
	choice = "Innenstadt-West"
	if d, _, err := ClassifyDistrict(sampleDocs[0]); err != nil || d == nil || *d != choice {
		t.Errorf("district: got %v %v", d, err)
	}
	if d, conf, err := ClassifyDistrict(store.Row{"label": "  "}); d != nil || conf != nil || err != nil || len(*bodies) != 2 {
		t.Errorf("empty doc: got %v %v %v after %d requests", d, conf, err, len(*bodies))
	}
}
