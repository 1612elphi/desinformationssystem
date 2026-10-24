package store

import (
	"database/sql"
	"encoding/json"
)

// DocsNeedingGeo returns enriched files not yet geo-tagged (geo_status
// NULL/pending/error), newest first.
func (s *Store) DocsNeedingGeo(limit int) ([]Row, error) {
	return s.rows(`SELECT f.id, f.label, f.summary_de, ai.title AS agenda_title
		FROM files f
		LEFT JOIN agenda_items ai
			ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor
		WHERE f.enrich_status = 'ok' AND (f.geo_status IS NULL OR f.geo_status <> 'ok')
		ORDER BY f.id DESC LIMIT ?`, limit)
}

func (s *Store) FileFulltext(id string) (string, error) {
	r, err := s.row("SELECT fulltext FROM files WHERE id = ?", id)
	if err != nil || r == nil {
		return "", err
	}
	return str(r["fulltext"]), nil
}

// SetGeo replaces a file's street links and sets its district and geo_status.
func SetGeo(tx *sql.Tx, id string, streets []string, district *string, conf *float64, status string) error {
	if _, err := tx.Exec("DELETE FROM file_streets WHERE file_id = ?", id); err != nil {
		return err
	}
	for _, n := range streets {
		if _, err := tx.Exec("INSERT OR IGNORE INTO file_streets(file_id, name) VALUES (?, ?)", id, n); err != nil {
			return err
		}
	}
	_, err := tx.Exec("UPDATE files SET district = ?, district_conf = ?, geo_status = ? WHERE id = ?",
		district, conf, status, id)
	return err
}

// rollcallOK is 1 if the roll-call's ja/nein/enthaltung tally equals the
// counters, else 0; nil when there is no roll-call.
func rollcallOK(ja, nein, enth, members any) any {
	m, _ := members.(string)
	if m == "" {
		return nil
	}
	var mem []any
	if json.Unmarshal([]byte(m), &mem) != nil || len(mem) == 0 {
		return nil
	}
	tally := map[string]int64{"ja": 0, "nein": 0, "enthaltung": 0}
	for _, e := range mem {
		if d, ok := e.(map[string]any); ok {
			if v, ok := d["vote"].(string); ok {
				if _, ok := tally[v]; ok {
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

// RefreshVoteConsistency recomputes votes.members_ok from the stored roll-call
// and counters, for one vote or all when voteID is "".
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
	type vote struct {
		id                 string
		ja, nein, enth, mb any
	}
	var votes []vote
	for rs.Next() {
		var v vote
		if err := rs.Scan(&v.id, &v.ja, &v.nein, &v.enth, &v.mb); err != nil {
			rs.Close()
			return 0, err
		}
		if b, ok := v.mb.([]byte); ok {
			v.mb = string(b)
		}
		votes = append(votes, v)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return 0, err
	}
	for _, v := range votes {
		if _, err := tx.Exec("UPDATE votes SET members_ok = ? WHERE id = ?",
			rollcallOK(v.ja, v.nein, v.enth, v.mb), v.id); err != nil {
			return 0, err
		}
	}
	return len(votes), nil
}

// VoteConsistency counts how many stored roll-calls match their counters.
func (s *Store) VoteConsistency() (Row, error) {
	out := Row{}
	for _, c := range [][2]string{
		{"with_rollcall", "SELECT COUNT(*) FROM votes WHERE members IS NOT NULL"},
		{"consistent", "SELECT COUNT(*) FROM votes WHERE members_ok = 1"},
		{"mismatch", "SELECT COUNT(*) FROM votes WHERE members_ok = 0"},
	} {
		n, err := s.count(c[1])
		if err != nil {
			return nil, err
		}
		out[c[0]] = n
	}
	return out, nil
}

// OrphanFiles returns files attached to no meeting and no Vorlage.
func (s *Store) OrphanFiles(limit int) ([]Row, error) {
	return s.rows(`SELECT f.id, f.label, f.url, f.first_seen FROM files f
		WHERE (f.meeting_id IS NULL OR f.meeting_id NOT IN (SELECT id FROM meetings))
			AND NOT EXISTS (SELECT 1 FROM file_vorlagen fv WHERE fv.file_id = f.id)
		ORDER BY f.first_seen DESC LIMIT ?`, limit)
}
