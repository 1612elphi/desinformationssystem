package ingest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

const oparlFixtures = "testdata/oparl"

// oparlServer serves the fixtures the way web1 does, with every web1 URL
// pointing at the test server. It records each list request's decoded
// modified_since.
func oparlServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var since []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, q := r.URL.Path, r.URL.Query()
		name := ""
		switch {
		case strings.HasPrefix(p, "/ris/oparl/bodies/0001/downloadfiles/") && strings.HasSuffix(p, ".pdf"):
			raw, err := os.ReadFile(filepath.Join(oparlFixtures, "fake.pdf"))
			if err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Write(raw)
			return
		case p == "/ris/oparl/bodies/0001/meetings" || p == "/ris/oparl/bodies/0001/papers":
			mu.Lock()
			since = append(since, q.Get("modified_since"))
			mu.Unlock()
			page := q.Get("page")
			if page == "" {
				page = "1"
			}
			name = p[strings.LastIndex(p, "/")+1:] + "-p" + page + ".json"
		case strings.HasPrefix(p, "/ris/oparl/bodies/0001/meetings/"):
			name = "meeting-" + p[strings.LastIndex(p, "/")+1:] + ".json"
		case strings.HasPrefix(p, "/ris/oparl/bodies/0001/agendaitems/"):
			name = "agendaitem-" + p[strings.LastIndex(p, "/")+1:] + ".json"
		}
		raw, err := os.ReadFile(filepath.Join(oparlFixtures, name))
		if name == "" || err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(strings.ReplaceAll(string(raw), "https://web1.karlsruhe.de/", srv.URL+"/")))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), since...)
	}
}

func setVar[T any](t *testing.T, p *T, v T) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func oparlTestEnv(t *testing.T, base string) string {
	pdf := filepath.Join(t.TempDir(), "pdfs")
	setVar(t, &OparlBase, base+"/ris/oparl")
	setVar(t, &oparlBroken, base+"/oparl/")
	setVar(t, &oparlFixed, base+"/ris/oparl/")
	setVar(t, &PDFDir, pdf)
	setVar(t, &RequestInterval, 0)
	setVar(t, &MaxRetries, 1)
	setVar(t, &RecheckDays, 0)
	setVar(t, &OpenRouterKey, "")
	return pdf
}

func TestOparlFixAndRequote(t *testing.T) {
	if got := oparlFix("https://web1.karlsruhe.de/oparl/bodies/0001/papers"); got != "https://web1.karlsruhe.de/ris/oparl/bodies/0001/papers" {
		t.Errorf("fix: %s", got)
	}
	next := "https://web1.karlsruhe.de/ris/oparl/bodies/0001/papers?page=2&modified_since=2026-07-25T16:46:24+02:00&limit=1000"
	want := "https://web1.karlsruhe.de/ris/oparl/bodies/0001/papers?page=2&limit=1000&modified_since=2026-07-25T16%3A46%3A24%2B02%3A00"
	if got := requoteSince(next, "2026-07-25T16:46:24+02:00"); got != want {
		t.Errorf("requote:\n got %s\nwant %s", got, want)
	}
	if got := requoteSince(next, ""); got != next {
		t.Errorf("requote without since changed the url: %s", got)
	}
}

func TestOparlIterListFailures(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/cycle" {
			fmt.Fprintf(w, `{"data":[{}],"links":{"next":%q}}`, srv.URL+"/cycle?limit=1000")
			return
		}
		w.Write([]byte(`{"error":"maintenance"}`))
	}))
	defer srv.Close()
	setVar(t, &RequestInterval, 0)
	setVar(t, &MaxRetries, 1)
	cli := &oparlClient{NewFetcher()}
	n := 0
	err := cli.iterList(srv.URL+"/cycle", "", func(obj) { n++ })
	if err == nil || !strings.HasPrefix(err.Error(), "pagination cycle at ") || n != 1 {
		t.Errorf("cycle: n=%d err=%v", n, err)
	}
	if err := cli.iterList(srv.URL+"/nodata", "", func(obj) {}); err == nil || !strings.HasPrefix(err.Error(), "no data array at ") {
		t.Errorf("no data: %v", err)
	}
}

func TestOparlNextWatermark(t *testing.T) {
	setVar(t, &oparlOverlapHours, 24)
	// after the DST change Python keeps the parsed +01:00 offset
	start := time.Date(2026, 10, 25, 12, 0, 0, 0, berlin)
	cases := []struct {
		failed     []string
		next, held string
	}{
		{nil, "2026-10-24T12:00:00+01:00", ""},
		{[]string{"2026-10-24T13:00:00+02:00"}, "2026-10-24T12:00:00+01:00", ""},
		{[]string{"2026-10-22T08:00:00+02:00", "", "2026-10-20T08:00:00+02:00"}, "2026-10-20T08:00:00+02:00", "2026-10-20T08:00:00+02:00"},
		{[]string{""}, "2026-10-01T00:00:00+02:00", "2026-10-01T00:00:00+02:00"},
		{[]string{"garbage"}, "garbage", "garbage"},
	}
	for _, c := range cases {
		next, held := nextWatermark(start, "2026-10-01T00:00:00+02:00", c.failed)
		if next != c.next || deref(held) != c.held {
			t.Errorf("%v: got %s held %v, want %s held %s", c.failed, next, deref(held), c.next, c.held)
		}
	}
}

func TestOparlMappers(t *testing.T) {
	for _, c := range [][3]string{
		{"2026-10-20T15:30:00+02:00", "2026-10-20T20:00:00+02:00", "15.30 bis 20 Uhr"},
		{"2026-10-20T00:00:00+02:00", "2026-10-20T00:00:00+02:00", ""},
		{"2026-10-20T16:00:00+02:00", "", ""},
	} {
		if got := deref(germanTime(c[0], c[1])); got != c[2] {
			t.Errorf("germanTime(%s, %s) = %q", c[0], c[1], got)
		}
	}
	if refBase(" 2026/0365/3") != "2026/0365" || refBase("DHH/2026/0001") != "" {
		t.Error("refBase")
	}
	if mapPaperType("Haushalt THH 4100") != "Antrag" || mapPaperType("Informationsvorlage") != "Vorlage" || mapPaperType("x") != "" {
		t.Error("mapPaperType")
	}
	pdf := func(id, name string) obj {
		return obj{"fileName": id + ".pdf", "name": name, "downloadUrl": oparlBroken + "f/" + id + ".pdf"}
	}
	m := obj{
		"invitation": pdf("00000001", "Einladung"),
		"agendaItem": []any{
			obj{"number": "1", "auxiliaryFile": []any{pdf("00000002", "Anlage  1"), obj{"fileName": "x.docx"}}},
			obj{"number": "2", "resolutionFile": pdf("00000002", "")},
		},
		"auxiliaryFile": []any{pdf("00000002", "dup"), obj{"fileName": "00000003.pdf", "deleted": true}},
	}
	got := meetingFiles(m)
	if len(got) != 2 || got[0].ID != "00000002" || deref(got[0].AgendaAnchor) != "top2" || got[0].Label != "00000002.pdf" ||
		got[1].ID != "00000001" || got[1].AgendaAnchor != nil || got[1].DocType != "Einladung" || !strings.HasPrefix(got[1].URL, oparlFixed) {
		t.Errorf("meetingFiles: %+v", got)
	}
}

func TestOparlResolveAnchorFetch(t *testing.T) {
	srv, _ := oparlServer(t)
	oparlTestEnv(t, srv.URL)
	st := testStore(t)
	in, err := newOparlIngest(st, &oparlClient{NewFetcher()}, "", NewCounts())
	if err != nil {
		t.Fatal(err)
	}
	if r, err := in.resolveAnchor("103314"); err != nil || r != (aiRef{"termin-10588", "top10"}) {
		t.Errorf("103314: %+v %v", r, err)
	}
	if r, err := in.resolveAnchor("1"); err != nil || r != (aiRef{}) || len(in.aimap) != 2 {
		t.Errorf("missing: %+v %v", r, err)
	}
}

var oparlVolatile = map[string]bool{"first_seen": true, "last_seen": true, "scraped_at": true, "downloaded_at": true,
	"parsed_at": true, "superseded_at": true, "last_attempt": true}

// oparlDump mirrors the Python driver's canonical table dump.
func oparlDump(t *testing.T, st *store.Store, scen, base, pdf string, result any) map[string]any {
	t.Helper()
	norm := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, base, "BASE"), pdf, "PDF") }
	raw, _ := json.Marshal(result)
	var res any
	json.Unmarshal([]byte(norm(string(raw))), &res)
	out := map[string]any{"_result": res}
	for _, tbl := range []string{"meta", "bodies", "meetings", "agenda_items", "files", "file_vorlagen", "file_versions", "files_fts"} {
		q := "SELECT * FROM " + tbl
		if tbl == "files_fts" {
			q = "SELECT rowid AS _rowid, * FROM files_fts"
		}
		rs, err := st.DB.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rs.Columns()
		rows := []any{}
		for rs.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rs.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			row := map[string]any{}
			for i, c := range cols {
				if oparlVolatile[c] {
					continue
				}
				switch v := vals[i].(type) {
				case []byte:
					row[c] = norm(string(v))
				case string:
					row[c] = norm(v)
				default:
					row[c] = v
				}
			}
			if k := row["key"]; tbl == "meta" && (k == "last_scrape" || (k == "oparl_since" && scen != "seed")) {
				row["value"] = "<now>"
			}
			rows = append(rows, row)
		}
		rs.Close()
		out[tbl] = rows
	}
	b, _ := json.Marshal(out)
	var canon map[string]any
	json.Unmarshal(b, &canon)
	return canon
}

func sortTables(d map[string]any) {
	for k, v := range d {
		if rows, ok := v.([]any); ok && k != "_result" {
			sort.Slice(rows, func(i, j int) bool {
				a, _ := json.Marshal(rows[i])
				b, _ := json.Marshal(rows[j])
				return string(a) < string(b)
			})
		}
	}
}

func oparlSeedDump(t *testing.T, base string) string {
	dump := filepath.Join(t.TempDir(), "dump")
	for _, sub := range []string{"meetings", "papers", "file-contents"} {
		os.MkdirAll(filepath.Join(dump, sub), 0o755)
	}
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(dump, name), []byte(strings.ReplaceAll(string(data), "https://web1.karlsruhe.de/", base+"/")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for src, dst := range map[string]string{"meeting-10588.json": "meetings/10588.json", "meeting-10585.json": "meetings/10585.json", "paper-54767.json": "papers/54767.json"} {
		raw, err := os.ReadFile(filepath.Join(oparlFixtures, src))
		if err != nil {
			t.Fatal(err)
		}
		write(dst, raw)
	}
	raw, _ := os.ReadFile(filepath.Join(oparlFixtures, "papers-p1.json"))
	var page struct{ Data []json.RawMessage }
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Data {
		var o struct{ ID string }
		json.Unmarshal(p, &o)
		id, _ := oparlTail(o.ID)
		write("papers/"+id+".json", p)
	}
	write("file-contents/681807.txt", []byte("Änderungsantrag\r\nVorlage Nr. 2026/0582\rEnde\n"))
	write("file-contents/681506.txt", []byte("  \n"))
	write("generation-manifest.json", []byte(`{"completedAt": "2026-10-23T03:00:00.000Z"}`))
	return dump
}

// TestOparlParity runs the scenarios of the Python driver that produced
// testdata/oparl/python-rows.json and compares every written row.
func TestOparlParity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(oparlFixtures, "python-rows.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	for _, scen := range []string{"sync", "refresh", "seed"} {
		t.Run(scen, func(t *testing.T) {
			srv, seenSince := oparlServer(t)
			pdf := oparlTestEnv(t, srv.URL)
			st := testStore(t)
			var result []any
			switch scen {
			case "sync":
				for range 2 {
					c, err := OparlSync(st, false, "2026-10-20T00:00:00+02:00", false, true)
					if err != nil {
						t.Fatal(err)
					}
					result = append(result, c.String())
				}
				result = append(result, seenSince())
			case "refresh":
				for _, id := range []string{"termin-10585", "termin-10588", "termin-99999"} {
					r, err := RefreshOne(st, id, false, false)
					if err != nil {
						t.Fatal(err)
					}
					result = append(result, r)
				}
			case "seed":
				dump := oparlSeedDump(t, srv.URL)
				c, err := Seed(st, dump, 0, false, false, false)
				if err != nil {
					t.Fatal(err)
				}
				stamp, err := SeedWatermark(st, dump)
				if err != nil {
					t.Fatal(err)
				}
				c.SetStr("watermark", stamp)
				b, err := BackfillPDFs(st, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				n, err := PendingPDFCount(st)
				if err != nil {
					t.Fatal(err)
				}
				result = append(result, c.String(), b.String(), n)
			}
			got := oparlDump(t, st, scen, srv.URL, pdf, result)
			exp := want[scen]
			sortTables(got)
			sortTables(exp)
			for k := range exp {
				if !reflect.DeepEqual(got[k], exp[k]) {
					g, _ := json.MarshalIndent(got[k], "", " ")
					e, _ := json.MarshalIndent(exp[k], "", " ")
					t.Errorf("%s differs:\n go: %s\n py: %s", k, g, e)
				}
			}
		})
	}
}
