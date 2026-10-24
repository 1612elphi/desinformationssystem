package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

var (
	PDFDir           = envStr("PDF_DIR", filepath.Join("data", "pdfs"))
	EnableOCR        = envStr("ENABLE_OCR", "0") == "1"
	PDFToTextMaxChar = envInt("PDFTOTEXT_MAXCHARS", 400000)
)

// Matching is by substring in this order, most specific first:
// "wortprotokoll" contains "protokoll", and "Protokoll mit
// Abstimmungsergebnis" must classify as Abstimmungsergebnis.
var docTypeRules = [][2]string{
	{"niederschrift", "Niederschrift"},
	{"abstimmungsergebnis", "Abstimmungsergebnis"},
	{"wortprotokoll", "Wortprotokoll"},
	{"protokoll", "Protokoll"},
	{"beschlussvorlage", "Beschlussvorlage"},
	{"beschluss", "Beschluss"},
	{"einladung", "Einladung"},
	{"tagesordnung", "Tagesordnung"},
	{"antrag", "Antrag"},
	{"anfrage", "Anfrage"},
	{"vorlage", "Vorlage"},
	{"präsentation", "Präsentation"},
	{"praesentation", "Präsentation"},
	{"geschäftsordnung", "Geschäftsordnung"},
	{"anlage", "Anlage"},
	{"plan", "Plan"},
	{"karte", "Karte"},
}

func Classify(label string) string {
	low := strings.ToLower(label)
	for _, r := range docTypeRules {
		if strings.Contains(low, r[0]) {
			return r[1]
		}
	}
	return "Sonstiges"
}

var (
	vorlageRE = pyre(`(?:Vorlage|Drucksache)(?:n-?\s?Nr\.?|nummer)?\s*[:\s]\s*(20\d{2}\s*[/-]\s*\d{3,5})`)
	allSpace  = pyre(`\s*`)
	// doc types whose header number is the document's own Vorlagennummer
	ownVorlageTypes = map[string]bool{"Beschlussvorlage": true, "Vorlage": true, "Antrag": true, "Anfrage": true, "Beschluss": true}
)

// validVorlage normalises "2026-0324" to "2026/0324"; "" for a budget-year
// range such as "2026/2027".
func validVorlage(nr string) string {
	nr = strings.ReplaceAll(allSpace.ReplaceAllString(nr, ""), "-", "/")
	year, seq, _ := strings.Cut(nr, "/")
	if len(seq) == 4 && strings.HasPrefix(seq, "20") {
		y, _ := strconv.Atoi(year)
		s, _ := strconv.Atoi(seq)
		if d := s - y; d >= -1 && d <= 1 {
			return ""
		}
	}
	return nr
}

// ExtractVorlagen returns the document's own number ("" if none) and every
// referenced number, sorted.
func ExtractVorlagen(label, text, docType string) (string, []string) {
	own := ""
	if ownVorlageTypes[docType] {
		for _, m := range vorlageRE.FindAllStringSubmatch(label+"\n"+runeHead(text, 1500), -1) {
			if own = validVorlage(m[1]); own != "" {
				break
			}
		}
	}
	set := map[string]bool{}
	for _, m := range vorlageRE.FindAllStringSubmatch(label+"\n"+runeHead(text, 200000), -1) {
		if nr := validVorlage(m[1]); nr != "" {
			set[nr] = true
		}
	}
	refs := make([]string, 0, len(set))
	for nr := range set {
		refs = append(refs, nr)
	}
	sort.Strings(refs)
	return own, refs
}

// pdftotext runs poppler's pdftotext; a non-zero exit still yields its output.
func pdftotext(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pdftotext", "-q", "-enc", "UTF-8", path, "-").Output()
	var ee *exec.ExitError
	if err != nil && (ctx.Err() != nil || !errors.As(err, &ee)) {
		return "", err
	}
	return pyStrip(strings.ToValidUTF8(string(out), "�")), nil
}

// ExtractText returns (status, text); status is ok, empty, needs_ocr or error.
func ExtractText(path string) (string, string) {
	text, err := pdftotext(path)
	if err != nil {
		log.Printf("filestore: pdftotext failed for %s: %v", path, err)
		return "error", ""
	}
	if runeLen(text) >= 20 {
		return "ok", runeHead(text, PDFToTextMaxChar)
	}
	if EnableOCR {
		text = ocr(path)
		if runeLen(text) >= 20 {
			return "ok", runeHead(text, PDFToTextMaxChar)
		}
		return "empty", text
	}
	return "needs_ocr", text
}

func ocr(path string) string {
	tmp := path + ".ocr.pdf"
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "ocrmypdf", "-l", "deu", "--skip-text", "--optimize", "0", path, tmp).Run(); err != nil {
		log.Printf("filestore: OCR failed for %s: %v", path, err)
		return ""
	}
	text, err := pdftotext(tmp)
	if err != nil {
		log.Printf("filestore: OCR failed for %s: %v", path, err)
		return ""
	}
	os.Remove(tmp)
	return text
}

// FileMeta describes one document link; AgendaAnchor nil is meeting-level.
type FileMeta struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Label        string  `json:"label"`
	AgendaAnchor *string `json:"agenda_anchor"`
	DocType      string  `json:"doc_type"`
	Filename     string  `json:"filename"`
}

// Counts is a run's counter dict; String prints it as Python's repr does,
// since last_scrape_status stores that text.
type Counts struct {
	keys []string
	m    map[string]int
	strs map[string]*string
}

func NewCounts(keys ...string) *Counts {
	c := &Counts{m: map[string]int{}}
	for _, k := range keys {
		c.Add(k, 0)
	}
	return c
}

func (c *Counts) Add(k string, n int) {
	if _, ok := c.m[k]; !ok {
		c.keys = append(c.keys, k)
	}
	c.m[k] += n
}

func (c *Counts) Get(k string) int { return c.m[k] }

// SetStr stores a string entry; nil prints as None.
func (c *Counts) SetStr(k string, v *string) {
	if c.strs == nil {
		c.strs = map[string]*string{}
	}
	if _, ok := c.strs[k]; !ok {
		c.keys = append(c.keys, k)
	}
	c.strs[k] = v
}

func (c *Counts) Map() map[string]int { return c.m }

func (c *Counts) String() string {
	parts := make([]string, len(c.keys))
	for i, k := range c.keys {
		if v, ok := c.strs[k]; !ok {
			parts[i] = fmt.Sprintf("'%s': %d", k, c.m[k])
		} else if v == nil {
			parts[i] = fmt.Sprintf("'%s': None", k)
		} else {
			parts[i] = fmt.Sprintf("'%s': '%s'", k, *v)
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// header returns a header value, or nil when the header is absent.
func header(h http.Header, key string) any {
	if len(h.Values(key)) == 0 {
		return nil
	}
	return h.Get(key)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if fi, err := in.Stat(); err == nil {
		os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}
	return nil
}

// ProcessFile inserts or refreshes a file row and downloads and extracts the
// PDF when new, or when changed on the server and recheck is set. It reports
// whether the file was (re)downloaded.
func ProcessFile(st *store.Store, f *Fetcher, fm FileMeta, meetingID string, counts *Counts, recheck bool) (bool, error) {
	fid := fm.ID
	var exSHA, exPath, exModified, exDownloaded sql.NullString
	var exSize sql.NullInt64
	err := st.DB.QueryRow("SELECT sha256, local_path, size, remote_modified, downloaded_at FROM files WHERE id=?", fid).
		Scan(&exSHA, &exPath, &exSize, &exModified, &exDownloaded)
	found := err == nil
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	refreshMeta := func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE files SET meeting_id=?, agenda_anchor=?, label=?, doc_type=? WHERE id=?",
			meetingID, fm.AgendaAnchor, fm.Label, fm.DocType, fid); err != nil {
			return err
		}
		return store.ReindexFile(tx, fid)
	}

	if found && exPath.String != "" && exists(exPath.String) {
		changed := false
		if recheck {
			// re-download only when a known Last-Modified or size changed
			if h := f.Head(fm.URL); h != nil {
				lm := h.Header.Get("Last-Modified")
				cl := h.Header.Get("Content-Length")
				if !exModified.Valid {
					if lm != "" {
						if err := st.Write(func(tx *sql.Tx) error {
							_, err := tx.Exec("UPDATE files SET remote_modified=? WHERE id=?", lm, fid)
							return err
						}); err != nil {
							return false, err
						}
					}
				} else {
					sizeChanged := false
					if n, err := strconv.ParseInt(strings.TrimSpace(cl), 10, 64); err == nil && exSize.Int64 != 0 {
						sizeChanged = n != exSize.Int64
					}
					if (lm != "" && lm != exModified.String) || sizeChanged {
						changed = true
						log.Printf("filestore: file %s changed on server (recheck), re-downloading", fid)
					}
				}
			}
		}
		if !changed {
			counts.Add("files_seen", 1)
			return false, st.Write(refreshMeta)
		}
	}

	r := f.Get(fm.URL, true)
	if r == nil {
		counts.Add("errors", 1)
		return false, nil
	}
	// a 200 that is not a PDF (maintenance or rate-limit page) must never
	// replace a known-good cached file
	if !bytes.HasPrefix(bytes.TrimLeft(r.Body, " \t\n\r\v\f"), []byte("%PDF")) {
		log.Printf("filestore: file %s: response is not a PDF (%d bytes), keeping existing", fid, len(r.Body))
		counts.Add("errors", 1)
		return false, nil
	}
	sum := sha256.Sum256(r.Body)
	sha := hex.EncodeToString(sum[:])
	lastMod := header(r.Header, "Last-Modified")
	if found && exSHA.Valid && exSHA.String == sha {
		counts.Add("files_seen", 1)
		return false, st.Write(func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE files SET remote_modified=? WHERE id=?", lastMod, fid); err != nil {
				return err
			}
			return refreshMeta(tx)
		})
	}
	if err := os.MkdirAll(PDFDir, 0o755); err != nil {
		return false, err
	}
	local := filepath.Join(PDFDir, fid+".pdf")
	// archive the superseded bytes by copy, so /api/file never sees a gap
	archived := ""
	if found && exSHA.String != "" && exPath.String != "" && exists(exPath.String) {
		vdir := filepath.Join(PDFDir, "versions")
		vpath := filepath.Join(vdir, fmt.Sprintf("%s.%s.pdf", fid, runeHead(exSHA.String, 12)))
		err := os.MkdirAll(vdir, 0o755)
		if err == nil && !exists(vpath) {
			err = copyFile(exPath.String, vpath)
		}
		if err != nil {
			log.Printf("filestore: could not archive old version of %s: %v", fid, err)
		} else {
			archived = vpath
		}
	}
	// the daily scraper and the web refresh can race on one fid, and readers
	// must never see a half-written PDF: extract from a temp file, then rename
	tmp := fmt.Sprintf("%s.tmp.%d", local, os.Getpid())
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, r.Body, 0o644); err != nil {
		return false, err
	}
	status, text := ExtractText(tmp)
	if err := os.Rename(tmp, local); err != nil {
		return false, err
	}
	if status == "ok" {
		counts.Add("text_ok", 1)
	}
	if found {
		counts.Add("files_updated", 1)
	} else {
		counts.Add("files_new", 1)
	}
	subs := store.PyJSON(DeriveSubmitters(fm.Label, text, fm.DocType))
	return true, st.Write(func(tx *sql.Tx) error {
		// new or changed: enrich_status goes back to pending
		if _, err := tx.Exec(`INSERT INTO files(id,url,meeting_id,agenda_anchor,label,doc_type,filename,
				sha256,size,remote_modified,local_path,text_status,fulltext,
				submitters,enrich_status,downloaded_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'pending',datetime('now'))
			ON CONFLICT(id) DO UPDATE SET url=excluded.url, meeting_id=excluded.meeting_id,
				agenda_anchor=excluded.agenda_anchor, label=excluded.label,
				doc_type=excluded.doc_type, sha256=excluded.sha256, size=excluded.size,
				remote_modified=excluded.remote_modified, local_path=excluded.local_path,
				text_status=excluded.text_status, fulltext=excluded.fulltext,
				submitters=excluded.submitters,
				enrich_status='pending', downloaded_at=datetime('now')`,
			fid, fm.URL, meetingID, fm.AgendaAnchor, fm.Label, fm.DocType, fm.Filename, sha, len(r.Body),
			lastMod, local, status, text, subs); err != nil {
			return err
		}
		if archived != "" {
			if err := store.AddFileVersion(tx, fid, nullStr(exSHA), nullInt(exSize), nullStr(exModified),
				archived, nullStr(exDownloaded)); err != nil {
				return err
			}
		}
		own, refs := ExtractVorlagen(fm.Label, text, fm.DocType)
		if err := store.SetFileVorlagen(tx, fid, own, refs); err != nil {
			return err
		}
		return store.ReindexFile(tx, fid)
	})
}

func nullStr(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}

func nullInt(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}
