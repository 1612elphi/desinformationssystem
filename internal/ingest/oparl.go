package ingest

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	OparlBase = envStr("OPARL_BASE", "https://web1.karlsruhe.de/ris/oparl")
	OparlBody = envStr("OPARL_BODY", "0001")
	// the API serves /ris/oparl/ but every URL it emits uses /oparl/
	oparlBroken = "https://web1.karlsruhe.de/oparl/"
	oparlFixed  = "https://web1.karlsruhe.de/ris/oparl/"
	// the server ignores elementsPerPage but honours limit
	oparlPageSize       = envInt("OPARL_PAGE_SIZE", 1000)
	oparlBackfillMonths = envInt("BACKFILL_MONTHS", 12)
	oparlOverlapHours   = envInt("OPARL_OVERLAP_HOURS", 24)
	berlin              = mustZone("Europe/Berlin")
)

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// paperType -> doc_type, used only when Classify(label) yields Sonstiges.
var paperTypeMap = map[string]string{
	"Beschlussvorlage":            "Beschlussvorlage",
	"Informationsvorlage":         "Vorlage",
	"Anfrage":                     "Anfrage",
	"Antrag":                      "Antrag",
	"Änderungs-/Ergänzungsantrag": "Antrag",
	"Offenlage":                   "Vorlage",
}

func mapPaperType(ptype string) string {
	if t, ok := paperTypeMap[ptype]; ok {
		return t
	}
	// "Haushalt THH 4100" and "Haushalt Gesamt" are budget motions
	if strings.HasPrefix(ptype, "Haushalt") {
		return "Antrag"
	}
	return ""
}

type obj = map[string]any

func str(v any) string {
	s, _ := v.(string)
	return s
}

func asObj(v any) obj {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func oparlFix(u string) string { return strings.ReplaceAll(u, oparlBroken, oparlFixed) }

// pyQuote is urllib.parse.quote(s, safe="").
func pyQuote(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// requoteSince re-appends modified_since percent-encoded. The server's own
// links.next carries a raw "+" offset, which decodes to a space, and the
// server then drops the filter without an error.
func requoteSince(u, since string) string {
	if since == "" {
		return u
	}
	rest, frag, hasFrag := strings.Cut(u, "#")
	base, query, _ := strings.Cut(rest, "?")
	var kept []string
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		k, v = pyUnquotePlus(k), pyUnquotePlus(v)
		if k != "modified_since" {
			kept = append(kept, pyQuote(k)+"="+pyQuote(v))
		}
	}
	kept = append(kept, "modified_since="+pyQuote(since))
	out := base + "?" + strings.Join(kept, "&")
	if hasFrag && frag != "" {
		out += "#" + frag
	}
	return out
}

func pyUnquotePlus(s string) string {
	if d, err := url.QueryUnescape(s); err == nil {
		return d
	}
	return strings.ReplaceAll(s, "+", " ")
}

func oparlTail(u string) (string, error) {
	u = strings.TrimRight(u, "/")
	i := strings.LastIndex(u, "/")
	if i < 0 {
		return "", fmt.Errorf("no id in url %q", u)
	}
	return u[i+1:], nil
}

// terminNum orders termin ids numerically: "termin-9999" < "termin-10007".
func terminNum(tid string) int64 {
	i := strings.LastIndex(tid, "-")
	if i < 0 {
		return -1
	}
	v, err := strconv.ParseInt(tid[i+1:], 10, 64)
	if err != nil {
		return -1
	}
	return v
}

var oparlOrgRE = regexp.MustCompile(`/organizations/([a-z]+)/(\d+)`)

// bodyIDFromOrg maps organizations/gr/N to organisation-gr-N; other kinds
// (at/ = Ämter) are not committees.
func bodyIDFromOrg(u string) string {
	if m := oparlOrgRE.FindStringSubmatch(u); m != nil && m[1] == "gr" {
		return "organisation-gr-" + m[2]
	}
	return ""
}

var isoLayouts = []string{
	"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05-07",
	"2006-01-02T15:04:05", "2006-01-02T15:04Z07:00", "2006-01-02T15:04", "2006-01-02T15", "2006-01-02",
}

// parseISO approximates datetime.fromisoformat; naive values are Berlin time.
func parseISO(s string) (time.Time, bool) {
	if len(s) > 10 && s[10] == ' ' {
		s = s[:10] + "T" + s[11:]
	}
	for _, l := range isoLayouts {
		if t, err := time.ParseInLocation(l, s, berlin); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// isoKey orders OParl timestamps as instants; unparseable values sort
// oldest, which holds the watermark back.
func isoKey(ts string) time.Time {
	t, _ := parseISO(strings.ReplaceAll(ts, "Z", "+00:00"))
	return t
}

const isoSeconds = "2006-01-02T15:04:05-07:00"

func fmtHour(t time.Time) string {
	if t.Minute() != 0 {
		return fmt.Sprintf("%d.%02d", t.Hour(), t.Minute())
	}
	return strconv.Itoa(t.Hour())
}

// germanTime rebuilds the RIS "15.30 bis 20 Uhr" string; nil when the API
// carries no time of day.
func germanTime(start, end string) *string {
	if start == "" {
		return nil
	}
	s, ok := parseISO(start)
	if !ok {
		return nil
	}
	var e time.Time
	if end != "" {
		if e, ok = parseISO(end); !ok {
			return nil
		}
	}
	if end != "" && e.After(s) && !(s.Hour() == 0 && s.Minute() == 0) {
		return strp(fmtHour(s) + " bis " + fmtHour(e) + " Uhr")
	}
	return nil
}

func locationStr(v any) *string {
	loc := asObj(v)
	if loc == nil || truthy(loc["deleted"]) {
		return nil
	}
	for _, k := range []string{"room", "description", "streetAddress"} {
		if p := pyStrip(str(loc[k])); p != "" {
			return &p
		}
	}
	return nil
}

var pdfNameRE = regexp.MustCompile(`(?i)^(\d+)\.pdf\n?$`)

// fileMeta maps an OParl File; its fileName stem ("00677201") is the id the
// HTML scraper derives from download URLs.
func fileMeta(v any, anchor *string) *FileMeta {
	f := asObj(v)
	if f == nil || truthy(f["deleted"]) {
		return nil
	}
	filename := str(f["fileName"])
	m := pdfNameRE.FindStringSubmatch(filename)
	u := str(f["downloadUrl"])
	if u == "" {
		u = str(f["accessUrl"])
	}
	u = oparlFix(u)
	if m == nil || u == "" {
		return nil
	}
	label := collapse(pyStrip(str(f["name"])))
	if label == "" {
		label = filename
	}
	return &FileMeta{ID: m[1], URL: u, Label: label, AgendaAnchor: anchor, DocType: Classify(label), Filename: filename}
}

var refBaseRE = pyre(`^\s*(20\d{2}/\d{3,5})`)

// refBase is "2026/0365/3" -> "2026/0365", the form file_vorlagen keys on.
func refBase(ref string) string {
	if m := refBaseRE.FindStringSubmatch(ref); m != nil {
		return m[1]
	}
	return ""
}

type oparlClient struct{ f *Fetcher }

func (c *oparlClient) getJSON(u string) obj {
	r := c.f.Get(oparlFix(u), false)
	if r == nil {
		return nil
	}
	var d obj
	dec := json.NewDecoder(bytes.NewReader(r.Body))
	dec.UseNumber()
	if err := dec.Decode(&d); err != nil {
		log.Printf("oparl: non-JSON response for %s", u)
		return nil
	}
	return d
}

// iterList calls fn for every object across all pages. A failed page is an
// error: stopping quietly could advance the watermark past unseen objects.
func (c *oparlClient) iterList(u, since string, fn func(obj)) error {
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	u = fmt.Sprintf("%s%slimit=%d", u, sep, oparlPageSize)
	if since != "" {
		u += "&modified_since=" + pyQuote(since)
	}
	seen := map[string]bool{}
	for pages := 0; u != "" && pages < 100000; {
		if seen[u] {
			return fmt.Errorf("pagination cycle at %s after %d page(s)", u, pages)
		}
		seen[u] = true
		d := c.getJSON(u)
		if d == nil {
			return fmt.Errorf("list page failed: %s", u)
		}
		data, ok := d["data"].([]any)
		if !ok {
			// a 200 error or maintenance page is still an incomplete crawl
			return fmt.Errorf("no data array at %s", u)
		}
		pages++
		for _, o := range data {
			fn(asObj(o))
		}
		u = ""
		if next := str(asObj(d["links"])["next"]); next != "" {
			u = requoteSince(oparlFix(next), since)
		}
	}
	return nil
}

type aiRef struct{ tid, anchor string }

type oparlIngest struct {
	st            *store.Store
	getJSON       func(u string) obj
	processFile   func(fm FileMeta, meetingID string, counts *Counts, recheck bool) (bool, error)
	recheckCutoff string
	counts        *Counts
	// modified stamps of objects whose ingest partly failed; the watermark
	// is held back to the oldest so the next run re-walks them
	failed       []string
	aimap        map[string]aiRef
	meetingDates map[string]*string
}

func newOparlIngest(st *store.Store, cli *oparlClient, recheckCutoff string, counts *Counts) (*oparlIngest, error) {
	in := &oparlIngest{
		st: st, getJSON: cli.getJSON, recheckCutoff: recheckCutoff, counts: counts,
		aimap: map[string]aiRef{}, meetingDates: map[string]*string{},
	}
	in.processFile = func(fm FileMeta, meetingID string, counts *Counts, recheck bool) (bool, error) {
		return ProcessFile(st, cli.f, fm, meetingID, counts, recheck)
	}
	rs, err := st.DB.Query("SELECT id, date FROM meetings")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var id string
		var d sql.NullString
		if err := rs.Scan(&id, &d); err != nil {
			return nil, err
		}
		in.meetingDates[id] = nil
		if d.Valid {
			in.meetingDates[id] = &d.String
		}
	}
	return in, rs.Err()
}

func (in *oparlIngest) ingestMeeting(m obj, withFiles bool) (string, error) {
	if truthy(m["deleted"]) {
		// cancelled meetings keep an existing row but never create one
		in.counts.Add("deleted_meetings", 1)
		return "", nil
	}
	num, err := oparlTail(str(m["id"]))
	if err != nil {
		return "", err
	}
	tid := "termin-" + num
	title := pyStrip(str(m["name"]))
	mdate := orNil(runeHead(str(m["start"]), 10))
	if title == "" && mdate == nil {
		in.counts.Add("empty_meetings", 1)
		return "", nil
	}
	bodyName := pyStrip(titleParenRE.ReplaceAllString(title, ""))
	// scan every entry: an Amt listed first must not orphan the meeting
	var bodyID any
	for _, o := range list(m["organization"]) {
		if b := bodyIDFromOrg(str(o)); b != "" {
			bodyID = b
			break
		}
	}

	var agenda []AgendaItem
	for _, v := range list(m["agendaItem"]) {
		ai := asObj(v)
		number := pyStrip(str(ai["number"]))
		if number == "" {
			in.counts.Add("agenda_no_number", 1)
			continue
		}
		var public *int
		if p, ok := ai["public"]; ok && p != nil {
			public = intp(0)
			if truthy(p) {
				public = intp(1)
			}
		}
		agenda = append(agenda, AgendaItem{Anchor: "top" + number, Number: strp(number), Title: pyStrip(str(ai["name"])), Public: public})
		aid, err := oparlTail(str(ai["id"]))
		if err != nil {
			return "", err
		}
		in.aimap[aid] = aiRef{tid, "top" + number}
	}

	meeting := &Meeting{
		ID: tid, Title: title, BodyName: bodyName, Date: mdate,
		Time:     germanTime(str(m["start"]), str(m["end"])),
		Location: locationStr(m["location"]),
		Public:   MeetingPublic(title),
		URL:      RIS + "/" + tid,
		Agenda:   agenda,
	}
	if bodyID != nil {
		name := bodyName
		if name == "" {
			name = bodyID.(string)
		}
		// placeholder name; the HTML /gremien sync provides the real one
		if err := in.st.Write(func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO bodies(id,name) VALUES(?,?)
				ON CONFLICT(id) DO UPDATE SET last_seen=datetime('now')`, bodyID, name)
			return err
		}); err != nil {
			return "", err
		}
	}
	if err := UpsertMeeting(in.st, meeting, bodyID); err != nil {
		return "", err
	}
	in.meetingDates[tid] = mdate
	in.counts.Add("meetings", 1)

	if withFiles {
		recheck := mdate != nil && *mdate >= in.recheckCutoff
		for _, fm := range meetingFiles(m) {
			if _, err := in.processFile(fm, tid, in.counts, recheck); err != nil {
				log.Printf("oparl: file %s (%s) failed: %v", fm.ID, tid, err)
				in.counts.Add("errors", 1)
			}
		}
	}
	return tid, nil
}

// meetingFiles lists each TOP's resolutionFile and auxiliaryFile under its
// anchor, then the meeting-level files; a file already under a TOP keeps it.
func meetingFiles(m obj) []FileMeta {
	var out []FileMeta
	idx := map[string]int{}
	add := func(v any, anchor *string) {
		fm := fileMeta(v, anchor)
		if fm == nil {
			return
		}
		i, ok := idx[fm.ID]
		switch {
		case ok && anchor == nil:
		case ok:
			out[i] = *fm
		default:
			idx[fm.ID] = len(out)
			out = append(out, *fm)
		}
	}
	for _, v := range list(m["agendaItem"]) {
		ai := asObj(v)
		var anchor *string
		if number := pyStrip(str(ai["number"])); number != "" {
			anchor = strp("top" + number)
		}
		add(ai["resolutionFile"], anchor)
		for _, fo := range list(ai["auxiliaryFile"]) {
			add(fo, anchor)
		}
	}
	add(m["invitation"], nil)
	add(m["resultsProtocol"], nil)
	add(m["verbatimProtocol"], nil)
	for _, fo := range list(m["auxiliaryFile"]) {
		add(fo, nil)
	}
	return out
}

// ensureMeeting fetches and stores a meeting a consultation points at, without
// its files.
func (in *oparlIngest) ensureMeeting(num string) (string, error) {
	tid := "termin-" + num
	if _, ok := in.meetingDates[tid]; ok {
		return tid, nil
	}
	m := in.getJSON(fmt.Sprintf("%s/bodies/%s/meetings/%s", OparlBase, OparlBody, num))
	if m == nil || truthy(m["deleted"]) {
		return "", nil
	}
	return in.ingestMeeting(m, false)
}

func (in *oparlIngest) resolveAnchor(aiNum string) (aiRef, error) {
	if r, ok := in.aimap[aiNum]; ok {
		return r, nil
	}
	ai := in.getJSON(fmt.Sprintf("%s/bodies/%s/agendaitems/%s", OparlBase, OparlBody, aiNum))
	if ai == nil || truthy(ai["deleted"]) {
		in.aimap[aiNum] = aiRef{}
		return aiRef{}, nil
	}
	var r aiRef
	if murl := str(ai["meeting"]); murl != "" {
		n, err := oparlTail(murl)
		if err != nil {
			return aiRef{}, err
		}
		r.tid = "termin-" + n
	}
	if number := pyStrip(str(ai["number"])); number != "" {
		r.anchor = "top" + number
	}
	in.aimap[aiNum] = r
	return r, nil
}

func fileExists(st *store.Store, id string) (bool, error) {
	var one int
	err := st.DB.QueryRow("SELECT 1 FROM files WHERE id=?", id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

type station struct {
	tid, anchor string
	date        *string
}

func (in *oparlIngest) ingestPaper(p obj) error {
	if truthy(p["deleted"]) {
		in.counts.Add("deleted_papers", 1)
		return nil
	}
	var ptype any
	if s, ok := p["paperType"].(string); ok {
		ptype = s
	}
	refFull := orNil(pyStrip(str(p["reference"])))
	base := refBase(str(p["reference"]))
	in.counts.Add("papers", 1)

	var stations []station
	for _, v := range list(p["consultation"]) {
		cons := asObj(v)
		murl := str(cons["meeting"])
		if murl == "" {
			continue
		}
		num, err := oparlTail(murl)
		if err != nil {
			return err
		}
		tid, err := in.ensureMeeting(num)
		if err != nil {
			return err
		}
		if tid == "" {
			continue
		}
		anchor := ""
		if aurl := str(cons["agendaItem"]); aurl != "" {
			aiNum, err := oparlTail(aurl)
			if err != nil {
				return err
			}
			r, err := in.resolveAnchor(aiNum)
			if err != nil {
				return err
			}
			anchor = r.anchor
			if r.tid != "" && r.tid != tid {
				anchor = "" // inconsistent linkage, do not guess
			}
		}
		stations = append(stations, station{tid, anchor, in.meetingDates[tid]})
	}

	var files []any
	if truthy(p["mainFile"]) {
		files = append(files, p["mainFile"])
	}
	files = append(files, list(p["auxiliaryFile"])...)
	modified := str(p["modified"])

	var fids []string
	if len(stations) > 0 {
		// attach the PDFs to the latest station, as the HTML scraper's
		// last-meeting-wins did
		tgt := stations[0]
		for _, s := range stations[1:] {
			sd, td := deref(s.date), deref(tgt.date)
			if sd > td || (sd == td && terminNum(s.tid) > terminNum(tgt.tid)) {
				tgt = s
			}
		}
		recheck := tgt.date != nil && *tgt.date >= in.recheckCutoff
		for _, fo := range files {
			fm := fileMeta(fo, orNil(tgt.anchor))
			if fm == nil {
				continue
			}
			if mapped := mapPaperType(str(ptype)); fm.DocType == "Sonstiges" && mapped != "" {
				fm.DocType = mapped
			}
			if _, err := in.processFile(*fm, tgt.tid, in.counts, recheck); err != nil {
				log.Printf("oparl: paper file %s failed: %v", fm.ID, err)
				in.counts.Add("errors", 1)
				in.failed = append(in.failed, modified)
				continue
			}
			// ProcessFile reports false for both a failed download and an
			// unchanged file, so check the row
			ok, err := fileExists(in.st, fm.ID)
			if err != nil {
				return err
			}
			if ok {
				fids = append(fids, fm.ID)
			} else {
				in.counts.Add("files_missing", 1)
				in.failed = append(in.failed, modified)
			}
		}
	} else if len(files) > 0 {
		// no station to hang new files on, but stamp paper metadata on files
		// the meeting walk already stored
		for _, fo := range files {
			if fm := fileMeta(fo, nil); fm != nil {
				ok, err := fileExists(in.st, fm.ID)
				if err != nil {
					return err
				}
				if ok {
					fids = append(fids, fm.ID)
				}
			}
		}
		if len(fids) == 0 {
			in.counts.Add("papers_unattached", 1)
		}
	}

	return in.st.Write(func(tx *sql.Tx) error {
		for _, fid := range fids {
			if _, err := tx.Exec("UPDATE files SET paper_type=?, paper_reference=? WHERE id=?", ptype, refFull, fid); err != nil {
				return err
			}
			if base == "" {
				continue
			}
			// the OParl reference is authoritative: demote own=1 rows mined
			// under another number rather than delete them, the mention is real
			if _, err := tx.Exec("UPDATE file_vorlagen SET own=0 WHERE file_id=? AND vorlage<>?", fid, base); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT OR REPLACE INTO file_vorlagen(file_id,vorlage,own) VALUES(?,?,1)", fid, base); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE files SET vorlage=? WHERE id=?", base, fid); err != nil {
				return err
			}
		}
		if base == "" {
			return nil
		}
		for _, s := range stations {
			var carrier string
			err := tx.QueryRow(`SELECT id FROM files WHERE meeting_id=?
				AND doc_type IN ('Tagesordnung','Einladung') ORDER BY id LIMIT 1`, s.tid).Scan(&carrier)
			if err == sql.ErrNoRows {
				err = tx.QueryRow(`SELECT id FROM files WHERE meeting_id=?
					AND agenda_anchor IS NULL ORDER BY id LIMIT 1`, s.tid).Scan(&carrier)
			}
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			if !contains(fids, carrier) {
				// IGNORE: never downgrade an own=1 row to a mention
				if _, err := tx.Exec("INSERT OR IGNORE INTO file_vorlagen(file_id,vorlage,own) VALUES(?,?,0)", carrier, base); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// RefreshOne is oparl.py's refresh_one, the OParl counterpart of ScrapeOne.
func RefreshOne(st *store.Store, terminID string, enableLLM, recheck bool) (map[string]any, error) {
	if !terminIDRE.MatchString(terminID) {
		return nil, errors.New("invalid termin id")
	}
	if err := os.MkdirAll(PDFDir, 0o755); err != nil {
		return nil, err
	}
	counts := NewCounts("meetings", "files_new", "files_updated", "files_seen", "text_ok", "enriched", "errors")
	num := strings.TrimPrefix(terminID, "termin-")
	cli := &oparlClient{NewFetcher()}
	defer cli.f.Close()
	m := cli.getJSON(fmt.Sprintf("%s/bodies/%s/meetings/%s", OparlBase, OparlBody, num))
	if m == nil {
		counts.Add("errors", 1)
		return map[string]any{"ok": false, "error": "Sitzung nicht erreichbar", "counts": counts.Map()}, nil
	}
	if truthy(m["deleted"]) {
		return map[string]any{"ok": false, "error": "Sitzung wurde entfernt", "counts": counts.Map()}, nil
	}
	// a cutoff in the future skips the per-file HEAD checks
	cutoff := "9999-12-31"
	if recheck {
		cutoff = time.Now().Format(time.DateOnly)
	}
	in, err := newOparlIngest(st, cli, cutoff, counts)
	if err != nil {
		return nil, err
	}
	if _, err := in.ingestMeeting(m, true); err != nil {
		return nil, err
	}
	if enableLLM {
		n, err := EnrichPending(st)
		if err != nil {
			return nil, err
		}
		counts.Add("enriched", n)
	}
	return map[string]any{"ok": true, "counts": counts.Map()}, nil
}

// nextWatermark backs the run start off by the overlap, then holds it at
// the oldest failed object's stamp when that is earlier.
func nextWatermark(runStart time.Time, watermark string, failed []string) (next string, held *string) {
	_, off := runStart.Zone()
	fixed := runStart.In(time.FixedZone("", off))
	next = fixed.Add(-time.Duration(oparlOverlapHours) * time.Hour).Format(isoSeconds)
	if len(failed) == 0 {
		return next, nil
	}
	oldest := ""
	for _, t := range failed {
		if t != "" && (oldest == "" || isoKey(t).Before(isoKey(oldest))) {
			oldest = t
		}
	}
	// a blank stamp reads back falsy and would reset to the default window
	if oldest == "" {
		oldest = watermark
		if oldest == "" {
			oldest = runStart.Format(isoSeconds)
		}
	}
	if isoKey(oldest).Before(isoKey(next)) {
		return oldest, &oldest
	}
	return next, nil
}

// OparlSync is one oparl.py run. since overrides the stored watermark; with
// full and no since it walks everything.
func OparlSync(st *store.Store, full bool, since string, enableLLM, skipMembers bool) (*Counts, error) {
	counts := NewCounts("meetings", "papers", "files_new", "files_seen", "text_ok", "enriched", "errors")
	err := oparlSync(st, full, since, enableLLM, skipMembers, counts)
	if err != nil {
		st.Write(func(tx *sql.Tx) error {
			return store.SetMeta(tx, "last_scrape_status", fmt.Sprintf("error oparl %v", err))
		})
	}
	return counts, err
}

func oparlSync(st *store.Store, full bool, since string, enableLLM, skipMembers bool, counts *Counts) error {
	if err := os.MkdirAll(PDFDir, 0o755); err != nil {
		return err
	}
	cli := &oparlClient{NewFetcher()}
	defer cli.f.Close()
	// captured before the walk so objects modified mid-run fall into the
	// next window
	runStart := time.Now().In(berlin).Truncate(time.Second)
	watermark := since
	if !full && watermark == "" {
		v, err := st.Meta("oparl_since")
		if err != nil {
			return err
		}
		if watermark = str(v); watermark == "" {
			watermark = time.Now().In(berlin).AddDate(0, 0, -oparlBackfillMonths*31).Format(isoSeconds)
		}
	}
	recheckCutoff := time.Now().AddDate(0, 0, -RecheckDays).Format(time.DateOnly)
	logSince := watermark
	if logSince == "" {
		logSince = "None"
	}
	log.Printf("oparl: sync start (since=%s, recheck>=%s)", logSince, recheckCutoff)

	// committees and rosters stay HTML: OParl carries no party data
	if !skipMembers {
		_, mcount, err := SyncCommitteesMembers(st, cli.f)
		if err != nil {
			return err
		}
		counts.Add("members", mcount)
	}
	in, err := newOparlIngest(st, cli, recheckCutoff, counts)
	if err != nil {
		return err
	}

	visited := map[string]bool{}
	err = cli.iterList(fmt.Sprintf("%s/bodies/%s/meetings", OparlBase, OparlBody), watermark, func(m obj) {
		tid, err := in.ingestMeeting(m, true)
		if err != nil {
			log.Printf("oparl: meeting %s failed: %v", str(m["id"]), err)
			counts.Add("errors", 1)
			in.failed = append(in.failed, str(m["modified"]))
		} else if tid != "" {
			visited[tid] = true
		}
	})
	if err != nil {
		return err
	}

	// recently held meetings are re-fetched even outside the modified window:
	// the vendor may not bump meeting.modified for late-added files
	recent, err := st.RecentMeetingIDs(recheckCutoff)
	if err != nil {
		return err
	}
	for _, tid := range recent {
		if visited[tid] {
			continue
		}
		m := cli.getJSON(fmt.Sprintf("%s/bodies/%s/meetings/%s", OparlBase, OparlBody, strings.TrimPrefix(tid, "termin-")))
		if m == nil {
			counts.Add("errors", 1)
			continue
		}
		// not a watermark failure: this step re-runs by date anyway
		if _, err := in.ingestMeeting(m, true); err != nil {
			log.Printf("oparl: recheck meeting %s failed: %v", tid, err)
			counts.Add("errors", 1)
		}
	}

	err = cli.iterList(fmt.Sprintf("%s/bodies/%s/papers", OparlBase, OparlBody), watermark, func(p obj) {
		if err := in.ingestPaper(p); err != nil {
			log.Printf("oparl: paper %s failed: %v", str(p["id"]), err)
			counts.Add("errors", 1)
			in.failed = append(in.failed, str(p["modified"]))
		}
	})
	if err != nil {
		return err
	}

	if enableLLM {
		n, err := EnrichPending(st)
		if err != nil {
			return err
		}
		counts.Add("enriched", n)
	}
	n, err := ParsePendingVotes(st, 1000)
	if err != nil {
		return err
	}
	counts.Add("votes", n)

	next, held := nextWatermark(runStart, watermark, in.failed)
	if held != nil {
		counts.SetStr("held_watermark", held)
		log.Printf("oparl: %d object(s) failed — holding watermark at %s", len(in.failed), next)
	}
	err = st.Write(func(tx *sql.Tx) error {
		if err := store.SetMeta(tx, "oparl_since", next); err != nil {
			return err
		}
		if err := store.SetMeta(tx, "last_scrape", time.Now().Format("2006-01-02T15:04:05")); err != nil {
			return err
		}
		return store.SetMeta(tx, "last_scrape_status", "ok oparl "+counts.String())
	})
	if err == nil {
		log.Printf("oparl: sync done: %s", counts)
	}
	return err
}
