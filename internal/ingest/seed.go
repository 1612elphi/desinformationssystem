package ingest

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// seedTxtPath maps files.id "00010003" to file-contents/10003.txt, the OParl
// file id.
func seedTxtPath(dump, fid string) string {
	name := fid
	if _, err := strconv.ParseUint(fid, 10, 64); err == nil {
		if name = strings.TrimLeft(fid, "0"); name == "" {
			name = "0"
		}
	}
	return filepath.Join(dump, "file-contents", name+".txt")
}

// readPyText reads a file as Python's open(encoding="utf-8",
// errors="replace") does, including universal newlines.
func readPyText(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for len(raw) > 0 {
		r, n := utf8.DecodeRune(raw)
		b.WriteRune(r)
		raw = raw[n:]
	}
	return strings.ReplaceAll(strings.ReplaceAll(b.String(), "\r\n", "\n"), "\r", "\n"), nil
}

// seedFile writes a files row from the dump, without any download. A row
// the live pipeline already extracted keeps its text and local_path.
func seedFile(st *store.Store, dump string, fm FileMeta, meetingID string, counts *Counts) (bool, error) {
	var textStatus sql.NullString
	err := st.DB.QueryRow("SELECT text_status FROM files WHERE id=?", fm.ID).Scan(&textStatus)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err == nil && textStatus.String == "ok" {
		counts.Add("files_kept", 1)
		return false, st.Write(func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE files SET meeting_id=?, agenda_anchor=?, label=?, doc_type=? WHERE id=?",
				meetingID, fm.AgendaAnchor, fm.Label, fm.DocType, fm.ID); err != nil {
				return err
			}
			return store.ReindexFile(tx, fm.ID)
		})
	}

	text, err := readPyText(seedTxtPath(dump, fm.ID))
	status := "ok"
	switch {
	case errors.Is(err, fs.ErrNotExist):
		text, status = "", "missing"
	case err != nil:
		return false, err
	case pyStrip(text) == "":
		status = "empty"
	}
	if status == "ok" {
		counts.Add("text_ok", 1)
	} else {
		// image-only PDFs (vote tallies, scans), about 5% of the dump
		counts.Add("text_missing", 1)
	}
	subs := store.PyJSON(DeriveSubmitters(fm.Label, text, fm.DocType))
	err = st.Write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO files(id,url,meeting_id,agenda_anchor,label,doc_type,
				filename,text_status,fulltext,submitters,enrich_status,first_seen)
			VALUES(?,?,?,?,?,?,?,?,?,?,'skipped',datetime('now'))
			ON CONFLICT(id) DO UPDATE SET
				url=excluded.url, meeting_id=excluded.meeting_id,
				agenda_anchor=excluded.agenda_anchor, label=excluded.label,
				doc_type=excluded.doc_type, filename=excluded.filename,
				text_status=excluded.text_status, fulltext=excluded.fulltext,
				submitters=excluded.submitters`,
			fm.ID, fm.URL, meetingID, fm.AgendaAnchor, fm.Label, fm.DocType, fm.Filename, status, text, subs); err != nil {
			return err
		}
		own, refs := ExtractVorlagen(fm.Label, text, fm.DocType)
		if err := store.SetFileVorlagen(tx, fm.ID, own, refs); err != nil {
			return err
		}
		return store.ReindexFile(tx, fm.ID)
	})
	if err != nil {
		return false, err
	}
	counts.Add("files_new", 1)
	return true, nil
}

// seedSortedPaths lists dump/sub/*.json by numeric id, so a resumed run is
// deterministic; glob skips dot files as Python's does.
func seedSortedPaths(dump, sub string) ([]string, error) {
	all, err := filepath.Glob(filepath.Join(dump, sub, "*.json"))
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range all {
		if !strings.HasPrefix(filepath.Base(p), ".") {
			paths = append(paths, p)
		}
	}
	key := func(p string) int64 {
		if n, err := strconv.ParseUint(strings.TrimSuffix(filepath.Base(p), ".json"), 10, 63); err == nil {
			return int64(n)
		}
		return -1
	}
	sort.SliceStable(paths, func(i, j int) bool { return key(paths[i]) < key(paths[j]) })
	return paths, nil
}

// seedSlice is Python's paths[-limit:] (newest) or paths[:limit].
func seedSlice(paths []string, limit int, newest bool) []string {
	n := len(paths)
	if limit == 0 {
		return paths
	}
	at := func(i int) int {
		if i < 0 {
			i += n
		}
		return max(0, min(n, i))
	}
	if newest {
		return paths[at(-limit):]
	}
	return paths[:at(limit)]
}

func readJSONObj(path string) (obj, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var o obj
	dec := json.NewDecoder(f)
	dec.UseNumber()
	return o, dec.Decode(&o)
}

// Seed is the bulk load from a karlsruhe-oparl-syndication dump. It
// downloads nothing and leaves rows at enrich_status='skipped'.
func Seed(st *store.Store, dump string, limit int, skipPapers, skipMeetings, newest bool) (*Counts, error) {
	counts := NewCounts("meetings", "papers", "files_new", "files_seen", "text_ok", "enriched", "errors")
	misses := 0
	// only meetings are served: their embedded agenda items fill aimap
	// before the papers pass, so agenda lookups never miss
	getJSON := func(u string) obj {
		if strings.Contains(u, "/meetings/") {
			if num, err := oparlTail(u); err == nil {
				p := filepath.Join(dump, "meetings", num+".json")
				if _, err := os.Stat(p); err == nil {
					if o, err := readJSONObj(p); err == nil {
						return o
					}
				}
			}
		}
		misses++
		return nil
	}
	in, err := newOparlIngest(st, &oparlClient{}, "1900-01-01", counts)
	if err != nil {
		return counts, err
	}
	in.getJSON = getJSON
	in.processFile = func(fm FileMeta, meetingID string, counts *Counts, _ bool) (bool, error) {
		return seedFile(st, dump, fm, meetingID, counts)
	}

	if !skipMeetings {
		all, err := seedSortedPaths(dump, "meetings")
		if err != nil {
			return counts, err
		}
		paths := seedSlice(all, limit, newest)
		log.Printf("seed: seeding %d meetings", len(paths))
		for i, p := range paths {
			o, err := readJSONObj(p)
			if err == nil {
				_, err = in.ingestMeeting(o, true)
			}
			if err != nil {
				log.Printf("seed: meeting %s failed: %v", filepath.Base(p), err)
				counts.Add("errors", 1)
			}
			if (i+1)%250 == 0 {
				log.Printf("seed:   meetings %d/%d  files_new=%d", i+1, len(paths), counts.Get("files_new"))
			}
		}
	}
	if !skipPapers {
		all, err := seedSortedPaths(dump, "papers")
		if err != nil {
			return counts, err
		}
		paths := seedSlice(all, limit, newest)
		log.Printf("seed: seeding %d papers", len(paths))
		for i, p := range paths {
			o, err := readJSONObj(p)
			if err == nil {
				err = in.ingestPaper(o)
			}
			if err != nil {
				log.Printf("seed: paper %s failed: %v", filepath.Base(p), err)
				counts.Add("errors", 1)
			}
			if (i+1)%500 == 0 {
				log.Printf("seed:   papers %d/%d  files_new=%d", i+1, len(paths), counts.Get("files_new"))
			}
		}
	}
	counts.Add("client_misses", misses)
	return counts, nil
}

// SeedWatermark sets oparl_since to the dump's completedAt: deltas must
// resume from generation time, or changes made since are skipped for good.
func SeedWatermark(st *store.Store, dump string) (*string, error) {
	raw, err := os.ReadFile(filepath.Join(dump, "generation-manifest.json"))
	if err != nil {
		return nil, nil
	}
	var m struct {
		CompletedAt string `json:"completedAt"`
	}
	if json.Unmarshal(raw, &m) != nil || m.CompletedAt == "" {
		return nil, nil
	}
	return &m.CompletedAt, st.Write(func(tx *sql.Tx) error { return store.SetMeta(tx, "oparl_since", m.CompletedAt) })
}
