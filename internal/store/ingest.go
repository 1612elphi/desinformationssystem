package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Write runs fn in one transaction, like db.py's write_conn.
func (s *Store) Write(fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ReindexFile refreshes the FTS row for one file, keyed on files.fts_id.
func ReindexFile(tx *sql.Tx, fileID string) error {
	var rid sql.NullInt64
	var label, fulltext, sumDE, sumEN, topics, category, body, mtitle sql.NullString
	err := tx.QueryRow(`SELECT f.fts_id, f.label, f.fulltext, f.summary_de, f.summary_en,
			f.topics, f.category, m.body_name, m.title
		FROM files f LEFT JOIN meetings m ON m.id = f.meeting_id
		WHERE f.id = ?`, fileID).Scan(&rid, &label, &fulltext, &sumDE, &sumEN, &topics, &category, &body, &mtitle)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if !rid.Valid {
		// writers are serialised, so MAX+1 inside this transaction cannot race
		if _, err := tx.Exec("UPDATE files SET fts_id = (SELECT COALESCE(MAX(fts_id),0)+1 FROM files) WHERE id = ?", fileID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT fts_id FROM files WHERE id = ?", fileID).Scan(&rid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("DELETE FROM files_fts WHERE rowid = ?", rid.Int64); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO files_fts(rowid, label, fulltext, summary_de, summary_en,
			topics, category, body_name, meeting_title)
		VALUES (?,?,?,?,?,?,?,?,?)`, rid.Int64, label.String, fulltext.String, sumDE.String,
		sumEN.String, topics.String, category.String, body.String, mtitle.String)
	return err
}

type VoteMember struct {
	Name string `json:"name"`
	Vote string `json:"vote"`
}

// Vote is one upsert_vote input; nil pointers are SQL NULL.
type Vote struct {
	MeetingID, AgendaAnchor, TopLabel string
	ResultText                        *string
	Ja, Nein, Enthaltung              *int
	Members                           []VoteMember
	Source, ImageURL                  string
	ImageSHA, FileID                  *string
}

// UpsertVote inserts or updates the vote keyed meeting_id:agenda_anchor. A
// 'pdf' row is never downgraded by a non-pdf upsert, and NULL tallies,
// members or result text never overwrite stored values.
func UpsertVote(tx *sql.Tx, v Vote) error {
	if v.AgendaAnchor == "" {
		return nil
	}
	vid := v.MeetingID + ":" + v.AgendaAnchor
	var members any
	if len(v.Members) > 0 {
		pairs := make([]any, len(v.Members))
		for i, m := range v.Members {
			pairs[i] = Obj{{"name", m.Name}, {"vote", m.Vote}}
		}
		members = PyJSON(pairs)
	}
	_, err := tx.Exec(`INSERT INTO votes(id,meeting_id,agenda_anchor,top_label,result_text,ja,nein,
			enthaltung,members,source,image_url,image_sha,file_id,parsed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'))
		ON CONFLICT(id) DO UPDATE SET top_label=excluded.top_label,
			result_text=COALESCE(excluded.result_text, votes.result_text),
			ja=COALESCE(excluded.ja, votes.ja),
			nein=COALESCE(excluded.nein, votes.nein),
			enthaltung=COALESCE(excluded.enthaltung, votes.enthaltung),
			members=COALESCE(excluded.members, votes.members),
			source=excluded.source, image_url=excluded.image_url, image_sha=excluded.image_sha,
			file_id=COALESCE(excluded.file_id, votes.file_id), parsed_at=datetime('now')
		WHERE NOT (votes.source = 'pdf' AND excluded.source IS NOT 'pdf')`,
		vid, v.MeetingID, v.AgendaAnchor, v.TopLabel, v.ResultText, v.Ja, v.Nein, v.Enthaltung,
		members, v.Source, v.ImageURL, v.ImageSHA, v.FileID)
	if err != nil {
		return err
	}
	_, err = RefreshVoteConsistency(tx, vid)
	return err
}

// rollcallOK is 1 when the roll-call tally equals the counters, 0 when not,
// nil when there is no roll-call.
func rollcallOK(ja, nein, enth, members any) any {
	s, _ := members.(string)
	if s == "" {
		return nil
	}
	var mem []any
	if json.Unmarshal([]byte(s), &mem) != nil || len(mem) == 0 {
		return nil
	}
	tally := map[string]int64{"ja": 0, "nein": 0, "enthaltung": 0}
	for _, m := range mem {
		if d, ok := m.(map[string]any); ok {
			if v, ok := d["vote"].(string); ok {
				if _, known := tally[v]; known {
					tally[v]++
				}
			}
		}
	}
	if tally["ja"] == num(ja) && tally["nein"] == num(nein) && tally["enthaltung"] == num(enth) {
		return 1
	}
	return 0
}

// RefreshVoteConsistency recomputes votes.members_ok for one vote, or for
// all votes when voteID is empty.
func RefreshVoteConsistency(tx *sql.Tx, voteID string) (int, error) {
	q := "SELECT id, ja, nein, enthaltung, members FROM votes"
	var args []any
	if voteID != "" {
		q += " WHERE id = ?"
		args = append(args, voteID)
	}
	rs, err := tx.Query(q, args...)
	if err != nil {
		return 0, err
	}
	type rec struct {
		id                   string
		ja, nein, enth, memb any
	}
	var recs []rec
	for rs.Next() {
		var r rec
		if err := rs.Scan(&r.id, &r.ja, &r.nein, &r.enth, &r.memb); err != nil {
			rs.Close()
			return 0, err
		}
		if b, ok := r.memb.([]byte); ok {
			r.memb = string(b)
		}
		recs = append(recs, r)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return 0, err
	}
	for _, r := range recs {
		if _, err := tx.Exec("UPDATE votes SET members_ok = ? WHERE id = ?",
			rollcallOK(r.ja, r.nein, r.enth, r.memb), r.id); err != nil {
			return 0, err
		}
	}
	return len(recs), nil
}

func (s *Store) VoteParseAttempts(fileID string) (int, error) {
	var n int
	err := s.DB.QueryRow("SELECT attempts FROM vote_parse_attempts WHERE file_id = ?", fileID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// RecordVoteParseAttempt counts failed parses so unreadable scans stop
// re-billing the vision model; a success clears the counter.
func (s *Store) RecordVoteParseAttempt(fileID string, success bool) error {
	return s.Write(func(tx *sql.Tx) error {
		if success {
			_, err := tx.Exec("DELETE FROM vote_parse_attempts WHERE file_id = ?", fileID)
			return err
		}
		_, err := tx.Exec(`INSERT INTO vote_parse_attempts(file_id, attempts, last_attempt)
			VALUES(?, 1, datetime('now'))
			ON CONFLICT(file_id) DO UPDATE SET attempts = attempts + 1,
				last_attempt = datetime('now')`, fileID)
		return err
	})
}

// VoteImageSHA returns the stored image hash, "" when there is none.
func (s *Store) VoteImageSHA(meetingID, anchor string) (string, error) {
	var sha sql.NullString
	err := s.DB.QueryRow("SELECT image_sha FROM votes WHERE id = ?", meetingID+":"+anchor).Scan(&sha)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sha.String, err
}

func AddFileVersion(tx *sql.Tx, fileID string, sha256, size, remoteModified any, localPath string, downloadedAt any) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO file_versions
		(file_id, sha256, size, remote_modified, local_path, downloaded_at)
		VALUES(?,?,?,?,?,?)`, fileID, sha256, size, remoteModified, localPath, downloadedAt)
	return err
}

// SetFileVorlagen replaces a file's Vorlage references; own may be "".
func SetFileVorlagen(tx *sql.Tx, fileID, own string, refs []string) error {
	if _, err := tx.Exec("DELETE FROM file_vorlagen WHERE file_id = ?", fileID); err != nil {
		return err
	}
	all := map[string]bool{}
	for _, r := range refs {
		all[r] = true
	}
	if own != "" {
		all[own] = true
	}
	for nr := range all {
		o := 0
		if nr == own {
			o = 1
		}
		if _, err := tx.Exec("INSERT OR REPLACE INTO file_vorlagen(file_id, vorlage, own) VALUES(?,?,?)", fileID, nr, o); err != nil {
			return err
		}
	}
	var ownArg any
	if own != "" {
		ownArg = own
	}
	_, err := tx.Exec("UPDATE files SET vorlage = ? WHERE id = ?", ownArg, fileID)
	return err
}

// RecentMeetingIDs lists known meetings held between sinceDate and today.
func (s *Store) RecentMeetingIDs(sinceDate string) ([]string, error) {
	rows, err := s.rows("SELECT id FROM meetings WHERE date >= ? AND date <= date('now') ORDER BY date", sinceDate)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = str(r["id"])
	}
	return out, nil
}

func SetMeta(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec("INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

// Obj is a JSON object with Python dict key order.
type Obj []struct {
	K string
	V any
}

// PyJSON encodes v as Python's json.dumps(v, ensure_ascii=False) does, so
// JSON text columns stay byte-identical to the Python writers'. It covers
// strings, ints, bools, nil, []string, []any, Obj and map[string]any.
func PyJSON(v any) string {
	var b strings.Builder
	pyJSON(&b, v)
	return b.String()
}

func pyJSON(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case string:
		b.WriteByte('"')
		for _, r := range x {
			switch r {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			default:
				if r < 0x20 {
					fmt.Fprintf(b, `\u%04x`, r)
				} else {
					b.WriteRune(r)
				}
			}
		}
		b.WriteByte('"')
	case []string:
		a := make([]any, len(x))
		for i, s := range x {
			a[i] = s
		}
		pyJSON(b, a)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			pyJSON(b, e)
		}
		b.WriteByte(']')
	case Obj:
		b.WriteByte('{')
		for i, kv := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			pyJSON(b, kv.K)
			b.WriteString(": ")
			pyJSON(b, kv.V)
		}
		b.WriteByte('}')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		o := make(Obj, len(keys))
		for i, k := range keys {
			o[i].K, o[i].V = k, x[k]
		}
		pyJSON(b, o)
	default:
		panic(fmt.Sprintf("PyJSON: unsupported %T", v))
	}
}
