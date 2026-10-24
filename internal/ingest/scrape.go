package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	Site = envStr("SITE_BASE", "https://sitzungskalender.karlsruhe.de")
	RIS  = Site + "/db/ratsinformation"
	// meetings held in the last RECHECK_DAYS are always re-crawled, so late
	// results and minutes and in-place PDF updates are picked up
	RecheckDays   = envInt("RECHECK_DAYS", 21)
	ForwardMonths = envInt("FORWARD_MONTHS", 3)
	EnableLLM     = envStr("ENABLE_LLM", "1") == "1"
)

// SyncCommitteesMembers crawls /gremien and each committee page, upserting
// bodies and replacing member rosters. It returns the committees and the
// number of members parsed.
func SyncCommitteesMembers(st *store.Store, f *Fetcher) ([]Committee, int, error) {
	var committees []Committee
	if r := f.Get(RIS+"/gremien", false); r != nil {
		var err error
		if committees, err = ParseCommittees(r.Text()); err != nil {
			return nil, 0, err
		}
		if err := st.Write(func(tx *sql.Tx) error {
			for _, c := range committees {
				if _, err := tx.Exec(`INSERT INTO bodies(id,name) VALUES(?,?)
					ON CONFLICT(id) DO UPDATE SET name=excluded.name, last_seen=datetime('now')`, c.ID, c.Name); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, 0, err
		}
	}
	log.Printf("scraper: committees: %d", len(committees))

	mcount := 0
	for _, c := range committees {
		r := f.Get(RIS+"/"+c.ID, false)
		if r == nil {
			continue
		}
		mems, err := ParseMembers(r.Text())
		if err != nil {
			return nil, 0, err
		}
		if len(mems) == 0 {
			// a 200 page without a parseable roster must not wipe the stored one
			log.Printf("scraper: no members parsed for %s, keeping existing roster", c.ID)
			continue
		}
		if err := st.Write(func(tx *sql.Tx) error {
			if _, err := tx.Exec("DELETE FROM members WHERE body_id=?", c.ID); err != nil {
				return err
			}
			for _, m := range mems {
				if _, err := tx.Exec(`INSERT OR REPLACE INTO members
					(body_id,person_id,name,party,party_code,role,since)
					VALUES(?,?,?,?,?,?,?)`, c.ID, m.PersonID, m.Name, m.Party, m.PartyCode, m.Role, m.Since); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return nil, 0, err
		}
		mcount += len(mems)
	}
	log.Printf("scraper: members parsed: %d", mcount)
	return committees, mcount, nil
}

type month struct{ y, m int }

func (a month) less(b month) bool { return a.y < b.y || (a.y == b.y && a.m < b.m) }

func monthsFrom(today time.Time, n, step int) []month {
	out := []month{}
	y, m := today.Year(), int(today.Month())
	for range n + 1 {
		out = append(out, month{y, m})
		m += step
		if m == 0 {
			y, m = y-1, 12
		} else if m == 13 {
			y, m = y+1, 1
		}
	}
	return out
}

func allMonthsSince(today time.Time, startYear int) []month {
	out := []month{}
	end := month{today.Year(), int(today.Month())}
	for c := (month{startYear, 1}); !end.less(c); {
		out = append(out, c)
		if c.m++; c.m == 13 {
			c = month{c.y + 1, 1}
		}
	}
	return out
}

// Scrape is one full HTML scrape run: committees, the month window plus
// FORWARD_MONTHS ahead, recent re-checks, files, enrichment and vote PDFs.
func Scrape(st *store.Store, months int, backfillAll, enableLLM bool) (*Counts, error) {
	counts := NewCounts("meetings", "files_new", "files_seen", "text_ok", "enriched", "errors")
	err := scrape(st, months, backfillAll, enableLLM, counts)
	if err != nil {
		// record the failure so the dashboard does not keep a stale "ok"
		st.Write(func(tx *sql.Tx) error { return store.SetMeta(tx, "last_scrape_status", fmt.Sprintf("error %v", err)) })
		return counts, err
	}
	return counts, nil
}

func scrape(st *store.Store, months int, backfillAll, enableLLM bool, counts *Counts) error {
	if err := os.MkdirAll(PDFDir, 0o755); err != nil {
		return err
	}
	f := NewFetcher()
	defer f.Close()
	committees, mcount, err := SyncCommitteesMembers(st, f)
	if err != nil {
		return err
	}
	nameToID := map[string]string{}
	for _, c := range committees {
		nameToID[c.Name] = c.ID
	}
	counts.Add("members", mcount)

	today := time.Now()
	var window []month
	if backfillAll {
		window = allMonthsSince(today, 2014)
	} else {
		window = monthsFrom(today, months, -1)
	}
	set := map[month]bool{}
	for _, m := range append(window, monthsFrom(today, ForwardMonths, 1)...) {
		set[m] = true
	}
	monthList := make([]month, 0, len(set))
	for m := range set {
		monthList = append(monthList, m)
	}
	sort.Slice(monthList, func(i, j int) bool { return monthList[i].less(monthList[j]) })

	termine := map[string]bool{}
	for _, m := range monthList {
		r := f.Get(fmt.Sprintf("%s/termine?j=%d&m=%d&g=", RIS, m.y, m.m), false)
		if r == nil {
			continue
		}
		ids := ParseCalendarTermine(r.Text())
		for _, id := range ids {
			termine[id] = true
		}
		log.Printf("scraper: %04d-%02d: %d meetings", m.y, m.m, len(ids))
	}
	cutoff := today.AddDate(0, 0, -RecheckDays).Format(time.DateOnly)
	recent, err := st.RecentMeetingIDs(cutoff)
	if err != nil {
		return err
	}
	added := 0
	for _, id := range recent {
		if !termine[id] {
			termine[id] = true
			added++
		}
	}
	log.Printf("scraper: discovered %d meetings (+%d recent re-checks since %s)", len(termine), added, cutoff)

	ids := make([]string, 0, len(termine))
	for id := range termine {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, tid := range ids {
		if err := scrapeMeeting(st, f, tid, cutoff, nameToID, counts); err != nil {
			log.Printf("scraper: meeting %s failed: %v", tid, err)
			counts.Add("errors", 1)
		}
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
	err = st.Write(func(tx *sql.Tx) error {
		if err := store.SetMeta(tx, "last_scrape", time.Now().Format("2006-01-02T15:04:05")); err != nil {
			return err
		}
		return store.SetMeta(tx, "last_scrape_status", "ok "+counts.String())
	})
	if err == nil {
		log.Printf("scraper: scrape done: %s", counts)
	}
	return err
}

// one bad meeting or file must not abort the run
func scrapeMeeting(st *store.Store, f *Fetcher, tid, cutoff string, nameToID map[string]string, counts *Counts) error {
	url := RIS + "/" + tid
	r := f.Get(url, false)
	if r == nil {
		counts.Add("errors", 1)
		return nil
	}
	m, err := ParseMeeting(r.Text(), url, tid)
	if err != nil {
		return err
	}
	recheck := m.Date != nil && *m.Date >= cutoff
	counts.Add("meetings", 1)
	var bodyID any
	if id, ok := nameToID[m.BodyName]; ok {
		bodyID = id
	}
	if err := UpsertMeeting(st, m, bodyID); err != nil {
		return err
	}
	processFiles(st, f, m, counts, recheck, "file %s (%s) failed: %v")
	return nil
}

func processFiles(st *store.Store, f *Fetcher, m *Meeting, counts *Counts, recheck bool, logfmt string) {
	for _, fm := range m.Files {
		if _, err := ProcessFile(st, f, fm, m.ID, counts, recheck); err != nil {
			log.Printf("scraper: "+logfmt, fm.ID, m.ID, err)
			counts.Add("errors", 1)
		}
	}
}

// UpsertMeeting writes the meeting row and its agenda items; bodyID may be nil.
func UpsertMeeting(st *store.Store, m *Meeting, bodyID any) error {
	return st.Write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO meetings(id,body_id,body_name,title,date,time,location,public,url,scraped_at)
			VALUES(?,?,?,?,?,?,?,?,?,datetime('now'))
			ON CONFLICT(id) DO UPDATE SET body_id=excluded.body_id,
				body_name=excluded.body_name, title=excluded.title, date=excluded.date,
				time=excluded.time, location=excluded.location, public=excluded.public,
				scraped_at=datetime('now')`,
			m.ID, bodyID, m.BodyName, m.Title, m.Date, m.Time, m.Location, m.Public, m.URL); err != nil {
			return err
		}
		for _, ai := range m.Agenda {
			if _, err := tx.Exec(`INSERT INTO agenda_items(meeting_id,anchor,number,title,public)
				VALUES(?,?,?,?,?)
				ON CONFLICT(meeting_id,anchor) DO UPDATE SET number=excluded.number,
					title=excluded.title, public=excluded.public`,
				m.ID, ai.Anchor, ai.Number, ai.Title, ai.Public); err != nil {
				return err
			}
		}
		// drop TOPs the RIS removed, but only when an agenda was parsed, so a
		// markup change cannot wipe a meeting's items
		if len(m.Agenda) == 0 {
			return nil
		}
		args := []any{m.ID}
		for _, ai := range m.Agenda {
			args = append(args, ai.Anchor)
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(m.Agenda)), ",")
		_, err := tx.Exec("DELETE FROM agenda_items WHERE meeting_id=? AND anchor NOT IN ("+ph+")", args...)
		return err
	})
}

var terminIDRE = regexp.MustCompile(`^termin-\d+$`)

// ScrapeOne refreshes one meeting on demand. Without recheck it only fetches
// new files, and it skips LLM enrichment unless asked.
func ScrapeOne(st *store.Store, terminID string, enableLLM, recheck bool) (map[string]any, error) {
	if !terminIDRE.MatchString(terminID) {
		return nil, errors.New("invalid termin id")
	}
	if err := os.MkdirAll(PDFDir, 0o755); err != nil {
		return nil, err
	}
	counts := NewCounts("files_new", "files_updated", "files_seen", "text_ok", "enriched", "errors")
	f := NewFetcher()
	defer f.Close()
	url := RIS + "/" + terminID
	r := f.Get(url, false)
	if r == nil {
		counts.Add("errors", 1)
		return map[string]any{"ok": false, "error": "Sitzungsseite nicht erreichbar", "counts": counts.Map()}, nil
	}
	m, err := ParseMeeting(r.Text(), url, terminID)
	if err != nil {
		return nil, err
	}
	var bodyID any
	if m.BodyName != "" {
		var id string
		switch err := st.DB.QueryRow("SELECT id FROM bodies WHERE name=?", m.BodyName).Scan(&id); err {
		case nil:
			bodyID = id
		case sql.ErrNoRows:
		default:
			return nil, err
		}
	}
	if err := UpsertMeeting(st, m, bodyID); err != nil {
		return nil, err
	}
	processFiles(st, f, m, counts, recheck, "refresh: file %s (%s) failed: %v")
	if enableLLM {
		n, err := EnrichPending(st)
		if err != nil {
			return nil, err
		}
		counts.Add("enriched", n)
	}
	return map[string]any{"ok": true, "counts": counts.Map()}, nil
}

var labelTopRE = pyre(`TOP\s*(\d+(?:\.\d+)*)`)

// voteAnchor prefers the file's agenda anchor, else "topN"/"topN.M" from a
// "TOP n" in the label, else "".
func voteAnchor(anchor, label string) string {
	if anchor != "" {
		return anchor
	}
	if m := labelTopRE.FindStringSubmatch(label); m != nil {
		return "top" + m[1]
	}
	return ""
}

// ParseVotePDF renders page 1 of an image-only Abstimmungsergebnis PDF and
// stores its parsed tally.
func ParseVotePDF(st *store.Store, fileID string) (bool, error) {
	var meetingID, anchor, label, url, local sql.NullString
	err := st.DB.QueryRow("SELECT meeting_id, agenda_anchor, label, url, local_path FROM files WHERE id=?", fileID).
		Scan(&meetingID, &anchor, &label, &url, &local)
	if err == sql.ErrNoRows || (err == nil && (local.String == "" || !exists(local.String))) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	va := voteAnchor(anchor.String, label.String)
	if va == "" {
		log.Printf("scraper: vote pdf %s: no agenda anchor (label %q), skipping", fileID, label.String)
		return false, nil
	}
	topLabel := "TOP "
	if len(va) > 3 {
		topLabel += va[3:]
	}
	if m := labelTopRE.FindStringSubmatch(label.String); m != nil {
		topLabel = "TOP " + m[1]
	}
	td, err := os.MkdirTemp("", "votepdf")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(td)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "pdftoppm", "-jpeg", "-r", "150", "-f", "1", "-l", "1",
		local.String, filepath.Join(td, "v")).Run(); err != nil {
		log.Printf("scraper: pdftoppm failed for %s: %v", fileID, err)
		return false, nil
	}
	imgs, _ := filepath.Glob(filepath.Join(td, "*.jpg"))
	if len(imgs) == 0 {
		return false, nil
	}
	data, err := os.ReadFile(imgs[0])
	if err != nil {
		return false, err
	}
	parsed := ParseVoteImage(data, "image/jpeg")
	if parsed == nil {
		return false, nil
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	return true, st.Write(func(tx *sql.Tx) error {
		return store.UpsertVote(tx, store.Vote{
			MeetingID: meetingID.String, AgendaAnchor: va, TopLabel: topLabel,
			Ja: parsed.Ja, Nein: parsed.Nein, Enthaltung: parsed.Enthaltung, Members: parsed.Members,
			Source: "pdf", ImageURL: url.String, ImageSHA: &sha, FileID: &fileID,
		})
	})
}

// ParsePendingVotes parses Abstimmungsergebnis PDFs that have no stored PDF
// tally yet, giving up on a file after VOTE_PARSE_MAX_ATTEMPTS failures.
func ParsePendingVotes(st *store.Store, limit int) (int, error) {
	if !VoteParseEnabled() {
		return 0, nil
	}
	rs, err := st.DB.Query(`SELECT id, meeting_id, label, agenda_anchor FROM files
		WHERE doc_type='Abstimmungsergebnis' AND meeting_id IS NOT NULL
		ORDER BY downloaded_at DESC LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	type rec struct{ id, meeting, label, anchor string }
	var recs []rec
	for rs.Next() {
		var r rec
		var label, anchor sql.NullString
		if err := rs.Scan(&r.id, &r.meeting, &label, &anchor); err != nil {
			rs.Close()
			return 0, err
		}
		r.label, r.anchor = label.String, anchor.String
		recs = append(recs, r)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return 0, err
	}
	maxAttempts := envInt("VOTE_PARSE_MAX_ATTEMPTS", 3)
	done := 0
	for _, r := range recs {
		anchor := voteAnchor(r.anchor, r.label)
		if anchor == "" {
			continue
		}
		var source sql.NullString
		err := st.DB.QueryRow("SELECT source FROM votes WHERE id=?", r.meeting+":"+anchor).Scan(&source)
		if err != nil && err != sql.ErrNoRows {
			return done, err
		}
		if source.String == "pdf" {
			continue
		}
		n, err := st.VoteParseAttempts(r.id)
		if err != nil {
			return done, err
		}
		if n >= maxAttempts {
			continue
		}
		ok, err := ParseVotePDF(st, r.id)
		if err != nil {
			log.Printf("scraper: vote pdf %s failed: %v", r.id, err)
		}
		if err := st.RecordVoteParseAttempt(r.id, ok); err != nil {
			return done, err
		}
		if ok {
			done++
		}
	}
	if done > 0 {
		log.Printf("scraper: parsed %d vote PDFs", done)
	}
	return done, nil
}

type fileText struct{ id, label, docType, fulltext string }

func allFileTexts(st *store.Store) ([]fileText, error) {
	rs, err := st.DB.Query("SELECT id, label, doc_type, fulltext FROM files")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []fileText
	for rs.Next() {
		var id string
		var label, docType, fulltext sql.NullString
		if err := rs.Scan(&id, &label, &docType, &fulltext); err != nil {
			return nil, err
		}
		out = append(out, fileText{id, label.String, docType.String, fulltext.String})
	}
	return out, rs.Err()
}

// BackfillVorlagen recomputes Vorlage ownership and references for every file.
func BackfillVorlagen(st *store.Store) (int, error) {
	files, err := allFileTexts(st)
	if err != nil {
		return 0, err
	}
	n := 0
	err = st.Write(func(tx *sql.Tx) error {
		for _, f := range files {
			own, refs := ExtractVorlagen(f.label, f.fulltext, f.docType)
			if err := store.SetFileVorlagen(tx, f.id, own, refs); err != nil {
				return err
			}
			if own != "" || len(refs) > 0 {
				n++
			}
		}
		return nil
	})
	log.Printf("scraper: vorlagen backfilled: %d files carry numbers", n)
	return n, err
}

// BackfillSubmitters recomputes submitter codes for every file.
func BackfillSubmitters(st *store.Store) (int, error) {
	files, err := allFileTexts(st)
	if err != nil {
		return 0, err
	}
	err = st.Write(func(tx *sql.Tx) error {
		for _, f := range files {
			subs := store.PyJSON(DeriveSubmitters(f.label, f.fulltext, f.docType))
			if _, err := tx.Exec("UPDATE files SET submitters=? WHERE id=?", subs, f.id); err != nil {
				return err
			}
		}
		return nil
	})
	log.Printf("scraper: backfilled submitters for %d files", len(files))
	return len(files), err
}

// first column, null as empty
func queryStrings(st *store.Store, q string, args ...any) ([]string, error) {
	rows, err := st.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v.String)
	}
	return out, rows.Err()
}

// EnrichPending enriches every pending file with extracted text, with
// ENRICH_CONCURRENCY workers.
func EnrichPending(st *store.Store) (int, error) {
	ids, err := queryStrings(st, `SELECT id FROM files WHERE enrich_status='pending' AND text_status='ok'
		ORDER BY downloaded_at DESC`)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	conc := max(1, envInt("ENRICH_CONCURRENCY", 4))
	log.Printf("scraper: enriching %d documents (concurrency=%d)", len(ids), conc)
	jobs := make(chan string)
	var mu sync.Mutex
	done, finished := 0, 0
	var wg sync.WaitGroup
	for range conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				ok, err := EnrichFile(st, id)
				if err != nil {
					log.Printf("scraper: enrich failed for %s: %v", id, err)
				}
				mu.Lock()
				finished++
				if ok {
					done++
				}
				if finished%25 == 0 {
					log.Printf("scraper: enrichment progress: %d/%d", finished, len(ids))
				}
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	return done, nil
}
