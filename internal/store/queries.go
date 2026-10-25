package store

import (
	"encoding/json"
	"strings"
)

type SearchParams struct {
	Query, Committee, DocType, DateFrom, DateTo string
	Public                                      *bool
	Topic, Submitter, District, Street          string
	Category                                    string
	Limit, Offset                               int
	// Facets adds per-value counts of doc_type and category to the result.
	Facets bool
}

const agendaJoin = ` LEFT JOIN agenda_items ai
	ON ai.meeting_id = f.meeting_id AND ai.anchor = f.agenda_anchor `

func searchWhere(p SearchParams) where {
	var w where
	if p.Query != "" {
		w.add("files_fts MATCH ?", FTSQuery(p.Query))
	}
	if p.Committee != "" {
		w.add("(m.body_name = ? OR m.body_id = ?)", p.Committee, p.Committee)
	}
	if p.DocType != "" {
		w.add("f.doc_type = ?", p.DocType)
	}
	if p.DateFrom != "" {
		w.add("m.date >= ?", p.DateFrom)
	}
	if p.DateTo != "" {
		w.add("m.date <= ?", p.DateTo)
	}
	if p.Public != nil {
		pub := 0
		if *p.Public {
			pub = 1
		}
		w.add("m.public = ?", pub)
	}
	if p.Topic != "" {
		w.add("f.topics LIKE ?", `%"`+p.Topic+`"%`)
	}
	if p.Submitter != "" {
		w.add("f.submitters LIKE ?", `%"`+p.Submitter+`"%`)
	}
	if p.District != "" {
		w.add("f.district = ?", p.District)
	}
	if p.Street != "" {
		w.add("f.id IN (SELECT file_id FROM file_streets WHERE name = ? COLLATE NOCASE)", p.Street)
	}
	if p.Category != "" {
		w.add("f.category = ?", p.Category)
	}
	return w
}

func searchFrom(p SearchParams, w where) string {
	if p.Query != "" {
		return " FROM files f JOIN files_fts ON files_fts.rowid = f.fts_id" +
			" LEFT JOIN meetings m ON m.id = f.meeting_id" + agendaJoin + w.sql()
	}
	return " FROM files f LEFT JOIN meetings m ON m.id = f.meeting_id" + agendaJoin + w.sql()
}

// searchFacet counts the matches per value of col, ignoring the filter on col
// itself so the other values stay selectable.
func (s *Store) searchFacet(p SearchParams, col string) ([]Row, error) {
	w := searchWhere(p)
	return s.rows("SELECT f."+col+" AS value, COUNT(*) AS n"+searchFrom(p, w)+
		" GROUP BY f."+col+" HAVING value IS NOT NULL AND value != '' ORDER BY n DESC, value LIMIT 12", w.args...)
}

// SearchDocuments is the faceted full-text search. List results never carry
// fulltext; GetDocument returns it for one document.
func (s *Store) SearchDocuments(p SearchParams) (Row, error) {
	w := searchWhere(p)
	cols := `SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
		m.title AS meeting_title, m.public AS meeting_public,
		ai.number AS agenda_number, ai.title AS agenda_title`
	var q string
	base := searchFrom(p, w)
	args := w.args
	if p.Query != "" {
		// snippet delimiters are control chars the UI splits on, so document
		// text can never inject markup
		q = cols + `, snippet(files_fts, -1, ?, ?, ' … ', 20) AS snippet,
			bm25(files_fts) AS rank` + base + " ORDER BY rank LIMIT ? OFFSET ?"
		args = append([]any{"\x01", "\x02"}, w.args...)
	} else {
		q = cols + base + " ORDER BY m.date DESC, f.id DESC LIMIT ? OFFSET ?"
	}
	total, err := s.count("SELECT COUNT(*)"+base, w.args...)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(q, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		decodeJSON(r)
		delete(r, "fulltext")
	}
	res := Row{"total": total, "results": rows}
	if p.Facets {
		tp, cp := p, p
		tp.DocType, cp.Category = "", ""
		types, err := s.searchFacet(tp, "doc_type")
		if err != nil {
			return nil, err
		}
		cats, err := s.searchFacet(cp, "category")
		if err != nil {
			return nil, err
		}
		res["facet_counts"] = Row{"doc_type": types, "category": cats}
	}
	return res, nil
}

func (s *Store) GetDocument(id string) (Row, error) {
	d, err := s.row(`SELECT f.*, m.body_name, m.body_id, m.date AS meeting_date,
			m.title AS meeting_title, m.location, m.public AS meeting_public,
			ai.number AS agenda_number, ai.title AS agenda_title
		FROM files f
		LEFT JOIN meetings m ON m.id = f.meeting_id`+agendaJoin+`
		WHERE f.id = ?`, id)
	if err != nil || d == nil {
		return nil, err
	}
	decodeJSON(d)
	if d["versions"], err = s.rows(`SELECT id, sha256, size, remote_modified, downloaded_at, superseded_at
		FROM file_versions WHERE file_id = ? ORDER BY superseded_at DESC`, id); err != nil {
		return nil, err
	}
	if d["vorlagen"], err = s.rows(
		"SELECT vorlage, own FROM file_vorlagen WHERE file_id = ? ORDER BY own DESC, vorlage", id); err != nil {
		return nil, err
	}
	streets, err := s.rows("SELECT name FROM file_streets WHERE file_id = ? ORDER BY name", id)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, r := range streets {
		names = append(names, str(r["name"]))
	}
	d["streets"] = names
	return d, nil
}

func (s *Store) GetFileVersion(id int64) (Row, error) {
	return s.row("SELECT * FROM file_versions WHERE id = ?", id)
}

func (s *Store) ListCommittees() ([]Row, error) {
	return s.rows(`SELECT b.id, b.name,
			(SELECT COUNT(*) FROM members mb WHERE mb.body_id = b.id) AS members,
			(SELECT COUNT(*) FROM meetings m WHERE m.body_id = b.id) AS meetings,
			(SELECT COUNT(*) FROM files f JOIN meetings m ON m.id=f.meeting_id
				WHERE m.body_id = b.id) AS documents
		FROM bodies b ORDER BY b.name`)
}

type MeetingFilter struct {
	Committee, DateFrom, DateTo string
}

func (f MeetingFilter) where() *where {
	w := &where{}
	if f.Committee != "" {
		w.add("(body_name = ? OR body_id = ?)", f.Committee, f.Committee)
	}
	if f.DateFrom != "" {
		w.add("date >= ?", f.DateFrom)
	}
	if f.DateTo != "" {
		w.add("date <= ?", f.DateTo)
	}
	return w
}

func (s *Store) ListMeetings(f MeetingFilter, asc bool, limit, offset int) ([]Row, error) {
	w := f.where()
	dir := "DESC"
	if asc {
		dir = "ASC"
	}
	return s.rows(`SELECT m.*,
			(SELECT COUNT(*) FROM files f WHERE f.meeting_id=m.id) AS documents,
			(SELECT COUNT(*) FROM agenda_items a WHERE a.meeting_id=m.id) AS agenda_count
		FROM meetings m`+w.sql()+" ORDER BY date "+dir+" LIMIT ? OFFSET ?",
		append(w.args, limit, offset)...)
}

func (s *Store) CountMeetings(f MeetingFilter) (int64, error) {
	w := f.where()
	return s.count("SELECT COUNT(*) FROM meetings"+w.sql(), w.args...)
}

func (s *Store) GetCommittee(id string) (Row, error) {
	b, err := s.row("SELECT * FROM bodies WHERE id = ?", id)
	if err != nil || b == nil {
		return nil, err
	}
	if b["members"], err = s.rows(`SELECT person_id, name, party, party_code, role, since
		FROM members WHERE body_id = ? ORDER BY name`, id); err != nil {
		return nil, err
	}
	if b["composition"], err = s.rows(`SELECT COALESCE(NULLIF(party_code,''), party) AS code, COUNT(*) AS n
		FROM members WHERE body_id = ? GROUP BY code ORDER BY n DESC, code`, id); err != nil {
		return nil, err
	}
	if b["document_count"], err = s.count(`SELECT COUNT(*) FROM files f JOIN meetings m ON m.id=f.meeting_id
		WHERE m.body_id = ?`, id); err != nil {
		return nil, err
	}
	if b["meetings"], err = s.ListMeetings(MeetingFilter{Committee: id}, false, 300, 0); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Store) GetMeeting(id string) (Row, error) {
	m, err := s.row("SELECT * FROM meetings WHERE id = ?", id)
	if err != nil || m == nil {
		return nil, err
	}
	if m["agenda_items"], err = s.rows(
		"SELECT * FROM agenda_items WHERE meeting_id = ? ORDER BY id", id); err != nil {
		return nil, err
	}
	files, err := s.rows(`SELECT f.id, f.label, f.doc_type, f.filename, f.agenda_anchor, f.text_status,
			f.enrich_status, f.category, f.summary_de, f.summary_en, f.topics,
			f.submitters, f.url,
			ai.number AS agenda_number, ai.title AS agenda_title
		FROM files f`+agendaJoin+`
		WHERE f.meeting_id = ? ORDER BY f.id`, id)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		decodeJSON(f)
	}
	m["files"] = files
	votes, err := s.MeetingVotes(id)
	if err != nil {
		return nil, err
	}
	if err := s.attributeRollCalls(votes, str(m["body_id"])); err != nil {
		return nil, err
	}
	m["votes"] = votes
	return m, nil
}

// attributeRollCalls adds party_code and person_id to roll-call entries
// where the members roster identifies them.
func (s *Store) attributeRollCalls(votes []Row, body string) error {
	lookup, err := s.partyLookup()
	if err != nil {
		return err
	}
	for _, v := range votes {
		members, _ := v["members"].([]any)
		for _, e := range members {
			if m, ok := e.(map[string]any); ok {
				party, person := entryMatch(lookup, body, str(m["name"]))
				m["party_code"] = nilIfEmpty(party)
				m["person_id"] = nilIfEmpty(person)
			}
		}
	}
	return nil
}

// topOrder sorts "TOP 3.1" as (3, 1), between TOP 3 and TOP 4; CAST stops at
// the first non-digit, so CAST('3.1') = 3.
func topOrder(c string) string {
	t := "REPLACE(COALESCE(" + c + ",''),'TOP ','')"
	return "CAST(" + t + " AS INTEGER), CASE WHEN instr(" + t + ", '.') > 0 THEN CAST(substr(" +
		t + ", instr(" + t + ", '.') + 1) AS INTEGER) ELSE 0 END"
}

func (s *Store) MeetingVotes(meetingID string) ([]Row, error) {
	rows, err := s.rows(`SELECT id, agenda_anchor, top_label, result_text, ja, nein, enthaltung,
			members, members_ok, source, image_url FROM votes
		WHERE meeting_id = ?
		ORDER BY `+topOrder("top_label")+`, top_label`, meetingID)
	for _, r := range rows {
		decodeJSON(r, "members")
	}
	return rows, err
}

type VoteSearch struct {
	Committee, DateFrom, DateTo, Member, Query string
	IncludeMembers                             bool
	Limit, Offset                              int
}

func (s *Store) SearchVotes(p VoteSearch) (Row, error) {
	var w where
	if p.Committee != "" {
		w.add("(m.body_name = ? OR m.body_id = ?)", p.Committee, p.Committee)
	}
	if p.DateFrom != "" {
		w.add("m.date >= ?", p.DateFrom)
	}
	if p.DateTo != "" {
		w.add("m.date <= ?", p.DateTo)
	}
	if p.Member != "" {
		w.add(`EXISTS (SELECT 1 FROM json_each(COALESCE(v.members,'[]')) je
			WHERE json_extract(je.value,'$.name') LIKE '%' || ? || '%')`, p.Member)
	}
	if p.Query != "" {
		like := "%" + p.Query + "%"
		w.add("(v.top_label LIKE ? OR v.result_text LIKE ? OR COALESCE(ai.title,'') LIKE ?)", like, like, like)
	}
	base := ` FROM votes v
		JOIN meetings m ON m.id = v.meeting_id
		LEFT JOIN agenda_items ai
			ON ai.meeting_id = v.meeting_id AND ai.anchor = v.agenda_anchor` + w.sql()
	total, err := s.count("SELECT COUNT(*)"+base, w.args...)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(`SELECT v.meeting_id, v.agenda_anchor, v.top_label, v.result_text,
			v.ja, v.nein, v.enthaltung, v.members, v.members_ok, v.source, v.file_id,
			m.body_name, m.body_id, m.date AS meeting_date,
			ai.number AS agenda_number, ai.title AS agenda_title`+base+`
		ORDER BY m.date DESC, `+topOrder("v.top_label")+` LIMIT ? OFFSET ?`,
		append(w.args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, err
	}
	needle := fold(p.Member)
	for _, r := range rows {
		roll := rollCall(r["members"])
		if needle != "" && roll != nil {
			mv := []map[string]any{}
			for _, e := range roll {
				if strings.Contains(fold(str(e["name"])), needle) {
					mv = append(mv, e)
				}
			}
			r["member_votes"] = mv
		}
		if p.IncludeMembers {
			r["members"] = roll
		} else {
			delete(r, "members")
		}
	}
	return Row{"total": total, "results": rows}, nil
}

func (s *Store) RecentDocuments(since string, limit int, committee, docType, submitter, topic string) ([]Row, error) {
	var w where
	if since != "" {
		w.add("f.downloaded_at >= ?", since)
	}
	if committee != "" {
		w.add("(m.body_name = ? OR m.body_id = ?)", committee, committee)
	}
	if docType != "" {
		w.add("f.doc_type = ?", docType)
	}
	if submitter != "" {
		w.add("f.submitters LIKE ?", `%"`+submitter+`"%`)
	}
	if topic != "" {
		w.add("f.topics LIKE ?", `%"`+topic+`"%`)
	}
	rows, err := s.rows(`SELECT f.id, f.label, f.doc_type, f.category, f.summary_de, f.summary_en,
			f.topics, f.submitters, f.downloaded_at, f.agenda_anchor, m.body_name,
			m.title AS meeting_title, m.date AS meeting_date,
			ai.number AS agenda_number, ai.title AS agenda_title
		FROM files f
		LEFT JOIN meetings m ON m.id=f.meeting_id`+agendaJoin+w.sql()+
		" ORDER BY f.downloaded_at DESC, f.id DESC LIMIT ?", append(w.args, limit)...)
	for _, r := range rows {
		decodeJSON(r)
	}
	return rows, err
}

func (s *Store) Facets() (Row, error) {
	committees, err := s.rows("SELECT id, name FROM bodies ORDER BY name")
	if err != nil {
		return nil, err
	}
	pluck := func(q, col string) ([]string, error) {
		rows, err := s.rows(q)
		out := []string{}
		for _, r := range rows {
			out = append(out, str(r[col]))
		}
		return out, err
	}
	docTypes, err := pluck("SELECT DISTINCT doc_type FROM files WHERE doc_type IS NOT NULL ORDER BY doc_type", "doc_type")
	if err != nil {
		return nil, err
	}
	submitters, err := pluck(`SELECT DISTINCT je.value AS value
		FROM files, json_each(files.submitters) je
		WHERE files.submitters IS NOT NULL AND files.submitters != '[]'
		ORDER BY je.value`, "value")
	if err != nil {
		return nil, err
	}
	drows, err := s.rows(`SELECT district, COUNT(*) AS n FROM files WHERE district IS NOT NULL AND district != ''
		GROUP BY district ORDER BY district`)
	if err != nil {
		return nil, err
	}
	districts := []string{}
	counts := map[string]any{}
	for _, r := range drows {
		districts = append(districts, str(r["district"]))
		counts[str(r["district"])] = r["n"]
	}
	return Row{"committees": committees, "doc_types": docTypes, "submitters": submitters,
		"districts": districts, "district_counts": counts}, nil
}

func (s *Store) Stats() (Row, error) {
	out := Row{}
	for _, c := range [][2]string{
		{"bodies", "SELECT COUNT(*) FROM bodies"},
		{"meetings", "SELECT COUNT(*) FROM meetings"},
		{"documents", "SELECT COUNT(*) FROM files"},
		{"with_text", "SELECT COUNT(*) FROM files WHERE text_status='ok'"},
		{"enriched", "SELECT COUNT(*) FROM files WHERE enrich_status='ok'"},
		{"votes", "SELECT COUNT(*) FROM votes"},
		{"votes_with_rollcall", "SELECT COUNT(*) FROM votes WHERE members IS NOT NULL"},
	} {
		n, err := s.count(c[1])
		if err != nil {
			return nil, err
		}
		out[c[0]] = n
	}
	for _, k := range []string{"last_scrape", "last_scrape_status"} {
		v, err := s.Meta(k)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// Meta returns the value for key, or nil when unset.
func (s *Store) Meta(key string) (any, error) {
	r, err := s.row("SELECT value FROM meta WHERE key = ?", key)
	if err != nil || r == nil {
		return nil, err
	}
	return r["value"], nil
}

// rollCall parses a votes.members value (JSON text or already decoded) into
// its entries; nil when absent or malformed.
func rollCall(v any) []map[string]any {
	if s, ok := v.(string); ok {
		var out []map[string]any
		var raw []any
		if json.Unmarshal([]byte(s), &raw) != nil {
			return nil
		}
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		if out == nil {
			out = []map[string]any{}
		}
		return out
	}
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
