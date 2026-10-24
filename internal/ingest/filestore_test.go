package ingest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	return st
}

func mustExec(t *testing.T, st *store.Store, q string, args ...any) {
	t.Helper()
	if _, err := st.DB.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func TestFetcherRetriesRateLimitPage(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write([]byte("<p>ok</p>"))
	}))
	defer srv.Close()
	old := RequestInterval
	RequestInterval = 0
	defer func() { RequestInterval = old }()
	r := NewFetcher().Get(srv.URL, false)
	if r == nil || r.Text() != "<p>ok</p>" || n.Load() != 2 {
		t.Fatalf("got %v after %d requests", r, n.Load())
	}
}

func TestRetryAfter(t *testing.T) {
	h := http.Header{}
	if retryAfter(h, 7) != 7 {
		t.Error("fallback")
	}
	h.Set("Retry-After", "9999")
	if retryAfter(h, 7).Seconds() != 300 {
		t.Error("cap")
	}
	h.Set("Retry-After", "Wed, 21 Oct 2015 07:28:00 GMT")
	if retryAfter(h, 7) != 7 {
		t.Error("http-date falls back")
	}
}

func TestProcessFile(t *testing.T) {
	st := testStore(t)
	mustExec(t, st, `INSERT INTO meetings(id, body_name, title) VALUES('termin-1','Gemeinderat','GR')`)
	body := []byte("%PDF-1.4 not really a pdf")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/html" {
			w.Write([]byte("<html>maintenance</html>"))
			return
		}
		w.Header().Set("Last-Modified", "Mon, 01 Jun 2026 10:00:00 GMT")
		w.Write(body)
	}))
	defer srv.Close()
	oldDir, oldInterval := PDFDir, RequestInterval
	PDFDir, RequestInterval = t.TempDir(), 0
	defer func() { PDFDir, RequestInterval = oldDir, oldInterval }()

	f := NewFetcher()
	anchor := "top1"
	fm := FileMeta{ID: "123", URL: srv.URL + "/123.pdf", Label: "Antrag der SPD", AgendaAnchor: &anchor, DocType: "Antrag", Filename: "123.pdf"}
	counts := NewCounts("files_new", "files_seen", "text_ok", "errors")
	if ok, err := ProcessFile(st, f, fm, "termin-1", counts, false); !ok || err != nil {
		t.Fatalf("first download: %v %v", ok, err)
	}
	var subs, status, modified, enrich, local string
	st.DB.QueryRow("SELECT submitters, text_status, remote_modified, enrich_status, local_path FROM files WHERE id='123'").
		Scan(&subs, &status, &modified, &enrich, &local)
	if subs != `["SPD"]` || modified != "Mon, 01 Jun 2026 10:00:00 GMT" || enrich != "pending" || (status != "needs_ocr" && status != "error") {
		t.Errorf("row: %q %q %q %q", subs, status, modified, enrich)
	}
	if b, _ := os.ReadFile(local); string(b) != string(body) {
		t.Errorf("local file %s", local)
	}
	if ok, _ := ProcessFile(st, f, fm, "termin-1", counts, false); ok {
		t.Error("cached file was downloaded again")
	}
	fm.URL = srv.URL + "/html"
	os.Remove(local)
	if ok, _ := ProcessFile(st, f, fm, "termin-1", counts, false); ok {
		t.Error("non-PDF response accepted")
	}
	if counts.String() != "{'files_new': 1, 'files_seen': 1, 'text_ok': 0, 'errors': 1}" {
		t.Errorf("counts %s", counts)
	}
	var fts int
	st.DB.QueryRow("SELECT COUNT(*) FROM files_fts WHERE files_fts MATCH 'SPD'").Scan(&fts)
	if fts != 1 {
		t.Errorf("fts rows %d", fts)
	}
}

func TestUpsertMeeting(t *testing.T) {
	st := testStore(t)
	m, err := ParseMeeting(readFixture(t, "termin-10443.html"), RIS+"/termin-10443", "termin-10443")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertMeeting(st, m, nil); err != nil {
		t.Fatal(err)
	}
	m.Agenda = m.Agenda[:2]
	if err := UpsertMeeting(st, m, nil); err != nil {
		t.Fatal(err)
	}
	var n int
	var date, loc string
	st.DB.QueryRow("SELECT COUNT(*) FROM agenda_items WHERE meeting_id='termin-10443'").Scan(&n)
	st.DB.QueryRow("SELECT date, location FROM meetings WHERE id='termin-10443'").Scan(&date, &loc)
	if n != 2 || date != "2026-07-15" || loc == "" {
		t.Errorf("agenda rows %d, date %q, location %q", n, date, loc)
	}
}
