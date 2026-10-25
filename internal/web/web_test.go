package web

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

func TestSearchEndpoints(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO bodies(id, name) VALUES('gr', 'g'), ('oa', 'o')`,
		`INSERT INTO meetings(id, date, title, body_id, body_name) VALUES
			('termin-1', '2026-10-20', 'Sitzung', 'gr', 'Gemeinderat'),
			('termin-2', '2026-10-21', 'Sitzung', 'oa', 'Ausschuss für Öffentliche Ordnung')`,
		`INSERT INTO votes(id, meeting_id, top_label, result_text, ja, nein, enthaltung, members) VALUES
			('termin-1:TOP 1', 'termin-1', 'TOP 1', 'Zustimmung', 40, 2, 1,
				'[{"name":"Müller, A.","vote":"ja"},{"name":"Noé, B.","vote":"nein"}]')`,
		`INSERT INTO speeches(id, meeting_id, speaker, text, start_s) VALUES
			('termin-1:v:1', 'termin-1', 'A', 'Der Radweg kommt', 1)`,
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	h := (&Server{Store: st}).Handler()
	get := func(url string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	for _, url := range []string{
		"/api/votes?limit=abc", "/api/votes?limit=0", "/api/speeches/search?offset=-1",
	} {
		if code, _ := get(url); code != 422 {
			t.Errorf("%s: %d, want 422", url, code)
		}
	}

	code, v := get("/api/votes?q=zustimmung&limit=9999&member=noé")
	res := v["results"].([]any)
	if code != 200 || v["total"] != 1.0 || len(res) != 1 {
		t.Fatalf("votes: %d %v", code, v)
	}
	hit := res[0].(map[string]any)
	if _, ok := hit["members"]; ok {
		t.Error("members without include_members")
	}
	if mv := hit["member_votes"].([]any); len(mv) != 1 {
		t.Errorf("member_votes: %v", mv)
	}
	_, v = get("/api/votes?include_members=1")
	if _, ok := v["results"].([]any)[0].(map[string]any)["members"]; !ok {
		t.Error("include_members=1 left out members")
	}

	code, v = get("/api/speeches/search?q=radweg&meeting=termin-1")
	res = v["results"].([]any)
	if code != 200 || v["total"] != 1.0 || res[0].(map[string]any)["snippet"] == "" {
		t.Fatalf("speeches: %d %v", code, v)
	}
	if _, v = get("/api/speeches/search?q=radweg&meeting=termin-2"); v["total"] != 0.0 {
		t.Errorf("speeches meeting filter: %v", v)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/meetings?q=%C3%96FFENTLICH", nil))
	var ms []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ms)
	if rec.Header().Get("X-Total-Count") != "1" || len(ms) != 1 || ms[0]["id"] != "termin-2" {
		t.Fatalf("meetings q: %s %s", rec.Header().Get("X-Total-Count"), rec.Body)
	}
}
