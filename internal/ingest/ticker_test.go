package ingest

import (
	"reflect"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

const tickerPage = `<html><body><nav>Start. Live</nav>
<h1>Gemeinderat (öffentlich/nicht öffentlich) am 28. Juli 2026</h1>
<div><strong>TOP 1</strong> Verpflichtung <strong>Ergebnis</strong></td><td><p>einstimmige   Zustimmung</p>
<img src="https://sitzungskalender.karlsruhe.de/db/ratsinformation/777/11/file/highres1_TOP 1.jpg"></div>
<div><strong>TOP 3.1</strong> Bebauungsplan, Stopp der Planung
<strong>Ergebnis</strong><p>Mehrheitliche Zustimmung</p>
<img src='https://sitzungskalender.karlsruhe.de/db/ratsinformation/777/12/file/highres2_TOP 3.1.jpg'>
<img src='https://sitzungskalender.karlsruhe.de/db/ratsinformation/777/12/file/highres3_TOP 3.1.jpg'></div>
<div><strong>TOP 4</strong> keine Abstimmung</div>
<div><strong>TOP 5</strong> <strong>Ergebnis</strong><p>vertagt</p>
<img src="https://sitzungskalender.karlsruhe.de/db/ratsinformation/777/14/file/highres4_TOP 5.jpg"></div>
</body></html>`

func TestParseTicker(t *testing.T) {
	tk, err := ParseTicker(tickerPage)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Committee != "Live Gemeinderat" || tk.Date != "2026-07-28" {
		t.Errorf("header: %q %q", tk.Committee, tk.Date)
	}
	res := func(i int) any {
		if tk.Tops[i].ResultText == nil {
			return nil
		}
		return *tk.Tops[i].ResultText
	}
	if len(tk.Tops) != 3 || tk.Tops[0].TopLabel != "TOP 1" || tk.Tops[1].TopLabel != "TOP 3.1" || tk.Tops[2].TopLabel != "TOP 5" {
		t.Fatalf("tops: %+v", tk.Tops)
	}
	if tk.Tops[1].ImageURL != "https://sitzungskalender.karlsruhe.de/db/ratsinformation/777/12/file/highres2_TOP 3.1.jpg" {
		t.Errorf("first image per TOP wins: %s", tk.Tops[1].ImageURL)
	}
	// "Stopp" reads as a TOP marker to the Python lookahead too, so TOP 3.1
	// gets no result text
	if res(0) != "einstimmige Zustimmung" || res(1) != nil || res(2) != "vertagt" {
		t.Errorf("results: %v %v %v", res(0), res(1), res(2))
	}
}

func TestCandidateLiveIDs(t *testing.T) {
	page := `<ul><li><a href="/db/ratsinformation/12/live">Bauausschuss</a> Sitzung läuft</li>
<li><a href="/db/ratsinformation/10/live">Gemeinderat</a> am 1. Juli</li></ul>`
	ids, err := CandidateLiveIDs(page)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"12"}) {
		t.Errorf("flagged: %v", ids)
	}
	ids, _ = CandidateLiveIDs(`<a href="/9/live">a</a><a href="/10/live">b</a>`)
	if !reflect.DeepEqual(ids, []string{"10", "9"}) {
		t.Errorf("fallback: %v", ids)
	}
}

func TestMatchMeeting(t *testing.T) {
	live := []store.Row{
		{"id": "termin-1", "date": "2026-07-28", "body_name": "Gemeinderat"},
		{"id": "termin-2", "date": "2026-07-28", "body_name": "Bauausschuss"},
	}
	if got := matchMeeting(&Ticker{Date: "2026-07-28", Committee: "Bauausschuss"}, live); got != "termin-2" {
		t.Errorf("by committee: %q", got)
	}
	if got := matchMeeting(&Ticker{Date: "2026-07-28"}, live); got != "" {
		t.Errorf("ambiguous: %q", got)
	}
	if got := matchMeeting(&Ticker{Date: "2026-07-28"}, live[:1]); got != "termin-1" {
		t.Errorf("single: %q", got)
	}
	if got := matchMeeting(&Ticker{Committee: "Gemeinderat"}, live); got != "" {
		t.Errorf("no date: %q", got)
	}
}
