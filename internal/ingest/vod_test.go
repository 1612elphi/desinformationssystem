package ingest

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTopOffsets(t *testing.T) {
	segs := []Segment{
		{Start: 10, Text: "Wie schon bei Punkt 5 gesagt, dazu später."},
		{Start: 20, Text: "Ich rufe auf Tagesordnungspunkt 3."},
		{Start: 30, Text: "Dann kommen wir zu TOP 3.1, Sanierungsbeirat."},
		{Start: 40, Text: "Das war der Top 3 und ich rufe jetzt auf den Tagesordnungspunkt 5."},
		{Start: 50, Text: "Tagesordnungspunkt 31 ist abgesetzt."},
	}
	got := TopOffsets(segs, map[string]bool{"top3": true, "top3.1": true, "top5": true, "top31": true})
	// call beats bare mention
	want := map[string]float64{"top3": 20, "top3.1": 30, "top31": 50, "top5": 40}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := TopOffsets(segs, map[string]bool{"top7": true}); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestHotwords(t *testing.T) {
	st := testStore(t)
	mustExec(t, st, `INSERT INTO bodies(id, name) VALUES('gr','Gemeinderat')`)
	mustExec(t, st, `INSERT INTO meetings(id, body_id, body_name, title, date) VALUES('termin-1','gr','Gemeinderat','GR','2026-09-29')`)
	mustExec(t, st, `INSERT INTO members(body_id, person_id, name) VALUES('gr','1','Dr. Stefan Noé'),('gr','2','Jorinda Fahringer')`)
	mustExec(t, st, `INSERT INTO files(id, url, meeting_id, district) VALUES('1','u','termin-1','Durlach'),('2','u','termin-1','Oststadt'),('3','u','termin-1','Oststadt')`)
	mustExec(t, st, `INSERT INTO file_streets(file_id, name) VALUES('1','Kaiserstraße'),('2','Durlacher Allee'),('3','Durlacher Allee')`)
	got, err := Hotwords(st, "termin-1")
	if want := "Noé, Fahringer, Oststadt, Durlach, Durlacher Allee, Kaiserstraße"; err != nil || got != want {
		t.Errorf("got %q, %v", got, err)
	}
	if got, _ := Hotwords(st, ""); got != "" {
		t.Errorf("no meeting: %q", got)
	}
	if args := whisperArgs("a.wav", "out", "", true); slices.Contains(args, "--prompt") {
		t.Error(args)
	}
	if args := whisperArgs("a.wav", "out", "Noé", true); slices.Index(args, "--prompt") != slices.Index(args, "Noé")-1 {
		t.Error(args)
	}
}

func TestEnrichClaude(t *testing.T) {
	st := testStore(t)
	mustExec(t, st, `INSERT INTO meetings(id, body_name, title, date) VALUES('termin-1','Gemeinderat','GR','2026-07-28')`)
	for _, id := range []string{"1", "2", "3"} {
		mustExec(t, st, `INSERT INTO files(id, url, meeting_id, label, fulltext, text_status, enrich_status) VALUES(?,'u','termin-1','L','Text','ok','pending')`, id)
	}
	dir := t.TempDir()
	// skips document 2
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat >> " + filepath.Join(dir, "prompts") + "\n" +
		`printf '%s' '{"is_error": false, "result": "` + "```json\\n" + `{\"0\": {\"summary_de\": \"A\", \"category\": \"Sonstiges\"}, \"1\": {\"summary_de\": \"B\"}}"}'` + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := ClaudeBin
	ClaudeBin = bin
	t.Cleanup(func() { ClaudeBin = old })

	totals, err := EnrichClaude(st, 0, 0, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := ClaudeTotals{Written: 2, Failed: 1, BatchesOK: 1, Remaining: 1}
	if totals != want {
		t.Errorf("totals %v, want %v", totals, want)
	}
	var sum, status string
	st.DB.QueryRow("SELECT summary_de, enrich_status FROM files WHERE id='3'").Scan(&sum, &status)
	if sum != "A" || status != "ok" {
		t.Errorf("file 3: %q %q", sum, status)
	}
	prompts, _ := os.ReadFile(filepath.Join(dir, "prompts"))
	if !strings.Contains(string(prompts), "--- DOKUMENT 2 ---\nGremium: Gemeinderat\nSitzung: GR (2026-07-28)\n") {
		t.Errorf("prompt: %s", prompts)
	}
}
