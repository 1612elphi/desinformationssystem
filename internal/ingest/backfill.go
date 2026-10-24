package ingest

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	BackfillMaxFiles   = envInt("BACKFILL_MAX_FILES", 0)
	BackfillMaxSeconds = envInt("BACKFILL_MAX_SECONDS", 0)
)

type pendingPDF struct {
	fm           FileMeta
	meetingID    string
	enrichStatus string
}

// PendingPDFCount is backfill_pdfs.py --count.
func PendingPDFCount(st *store.Store) (int, error) {
	rows, err := pendingPDFs(st, 0)
	return len(rows), err
}

// pendingPDFs lists rows without a cached PDF, text-less ones first since
// they gain fulltext as well.
func pendingPDFs(st *store.Store, limit int) ([]pendingPDF, error) {
	q := `SELECT id, url, label, doc_type, filename, agenda_anchor, meeting_id, enrich_status
		FROM files
		WHERE (local_path IS NULL OR local_path = '')
		  AND url LIKE 'http%'
		ORDER BY CASE WHEN text_status = 'ok' THEN 1 ELSE 0 END,
			meeting_id DESC, id DESC`
	if limit != 0 {
		q += " LIMIT " + strconv.Itoa(limit)
	}
	rs, err := st.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []pendingPDF
	for rs.Next() {
		var id, u string
		var label, docType, filename, anchor, meetingID, enrich sql.NullString
		if err := rs.Scan(&id, &u, &label, &docType, &filename, &anchor, &meetingID, &enrich); err != nil {
			return nil, err
		}
		fm := FileMeta{ID: id, URL: u, Label: label.String, DocType: docType.String, Filename: filename.String}
		if fm.DocType == "" {
			fm.DocType = "Sonstiges"
		}
		if fm.Filename == "" {
			fm.Filename = id + ".pdf"
		}
		if anchor.Valid {
			fm.AgendaAnchor = &anchor.String
		}
		out = append(out, pendingPDF{fm, meetingID.String, enrich.String})
	}
	return out, rs.Err()
}

// BackfillPDFs is backfill_pdfs.py: cache the PDFs of seeded rows at the
// normal throttle, within limit files and maxSeconds (0 = unbounded).
func BackfillPDFs(st *store.Store, limit, maxSeconds int) (*Counts, error) {
	started := time.Now()
	rows, err := pendingPDFs(st, limit)
	if err != nil {
		return nil, err
	}
	counts := NewCounts("files_new", "files_updated", "files_seen", "text_ok", "errors")
	log.Printf("backfill: %d files pending a cached PDF", len(rows))
	f := NewFetcher()
	defer f.Close()
	done := 0
	for i, r := range rows {
		if maxSeconds != 0 && time.Since(started) > time.Duration(maxSeconds)*time.Second {
			log.Printf("backfill: time budget reached after %d files", i)
			break
		}
		if _, err := ProcessFile(st, f, r.fm, r.meetingID, counts, false); err != nil {
			log.Printf("backfill: file %s failed: %v", r.fm.ID, err)
			counts.Add("errors", 1)
			continue
		}
		done++
		// ProcessFile resets enrich_status to pending; enrichment of seeded
		// rows stays a separate decision
		if r.enrichStatus == "skipped" {
			if err := st.Write(func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE files SET enrich_status='skipped' WHERE id=?", r.fm.ID)
				return err
			}); err != nil {
				return counts, err
			}
		}
		if (i+1)%100 == 0 {
			log.Printf("backfill:   %d/%d  new=%d text_ok=%d errors=%d", i+1, len(rows),
				counts.Get("files_new"), counts.Get("text_ok"), counts.Get("errors"))
		}
	}
	counts.Add("processed", done)
	counts.Add("remaining", max(0, len(rows)-done))
	return counts, nil
}
