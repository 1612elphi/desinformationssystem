package store

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

func isPySpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

// pyFields is Python's str.split() with no separator.
func pyFields(s string) []string { return strings.FieldsFunc(s, isPySpace) }

func lastField(s string) string {
	f := pyFields(s)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// MemberFor resolves a seat-map short name ("Dr. Huber", "Schütz, K.") or a
// full name to its members row, nil when none or several match.
func MemberFor(name string, members []Row) Row {
	if name == "" {
		return nil
	}
	for _, m := range members {
		if str(m["name"]) == name {
			return m
		}
	}
	base, initial, _ := strings.Cut(name, ",")
	dr := strings.HasPrefix(strings.TrimFunc(base, isPySpace), "Dr.")
	sn := lastField(strings.ReplaceAll(base, "Dr.", ""))
	var hits []Row
	for _, m := range members {
		if lastField(str(m["name"])) == sn {
			hits = append(hits, m)
		}
	}
	if len(hits) > 1 {
		var f []Row
		for _, m := range hits {
			if strings.HasPrefix(str(m["name"]), "Dr.") == dr {
				f = append(f, m)
			}
		}
		if len(f) > 0 {
			hits = f
		}
	}
	if ini := []rune(strings.TrimFunc(initial, isPySpace)); len(hits) > 1 && len(ini) > 0 {
		var f []Row
		for _, m := range hits {
			if strings.HasPrefix(strings.ReplaceAll(str(m["name"]), "Dr. ", ""), string(ini[0])) {
				f = append(f, m)
			}
		}
		hits = f
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return nil
}

// groupOrder is the roll-call order on the council's display: OB first, then
// the fractions.
var groupOrder = []string{"OB", "B90", "CDU", "SPD", "AFD", "FDP", "VOL", "LIN", "KAL", "PAR", "FÜR"}

func isChair(m Row) bool { return strings.Contains(str(m["role"]), "Oberbürgermeister") }

// withGroups gives each roll-call entry a "group" (OB for the chair, else the
// member's party code) and sorts the entries by group.
func withGroups(entries []any, roster []Row) []any {
	rank := func(e any) int {
		m, _ := e.(map[string]any)
		for i, g := range groupOrder {
			if m != nil && m["group"] == g {
				return i
			}
		}
		return len(groupOrder)
	}
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		r := MemberFor(str(m["name"]), roster)
		switch {
		case r != nil && isChair(r):
			m["group"] = "OB"
		case r != nil && str(r["party_code"]) != "":
			m["group"] = r["party_code"]
		case r != nil && str(r["party"]) != "":
			m["group"] = r["party"]
		default:
			m["group"] = nil
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return rank(entries[i]) < rank(entries[j]) })
	return entries
}

const speechCols = `s.id, s.meeting_id, s.agenda_anchor, s.top_label, s.speaker, s.party,
	(SELECT party_code FROM members WHERE party = s.party LIMIT 1) AS party_code,
	s.video_id, s.start_s, s.end_s, s.text`

// MeetingSpeeches is the debate transcript of one meeting in speaking order.
// Consecutive rows by one speaker under one TOP are merged: a dropped chair
// line or a pause split them.
func (s *Store) MeetingSpeeches(meetingID, anchor, speaker string, raw bool) ([]Row, error) {
	q := "SELECT " + speechCols
	if raw {
		q += ", s.text_raw"
	}
	q += " FROM speeches s WHERE s.meeting_id = ?"
	args := []any{meetingID}
	if anchor != "" {
		q += " AND s.agenda_anchor = ?"
		args = append(args, anchor)
	}
	if speaker != "" {
		q += " AND s.speaker LIKE '%' || ? || '%'"
		args = append(args, speaker)
	}
	rows, err := s.rows(q+" ORDER BY s.start_s", args...)
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for _, d := range rows {
		if n := len(out); n > 0 {
			p := out[n-1]
			if str(d["speaker"]) != "" && p["speaker"] == d["speaker"] && p["agenda_anchor"] == d["agenda_anchor"] {
				p["end_s"] = d["end_s"]
				p["text"] = str(p["text"]) + "\n\n" + str(d["text"])
				if raw {
					p["text_raw"] = str(p["text_raw"]) + " " + str(d["text_raw"])
				}
				continue
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// SpeechCounts is the number of (merged) speeches per agenda anchor; "" holds
// the opening and closing remarks.
func (s *Store) SpeechCounts(meetingID string) (map[string]int, error) {
	sp, err := s.MeetingSpeeches(meetingID, "", "", false)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range sp {
		out[str(r["agenda_anchor"])]++
	}
	return out, nil
}

type SpeechSearch struct {
	Query, Speaker, Party, DateFrom, DateTo, Meeting string
	Limit, Offset                                    int
}

// SearchSpeeches matches Query as a case-insensitive substring of the
// cleaned text.
// ponytail: LIKE scan; fine for a few thousand speeches, FTS5 beyond that.
func (s *Store) SearchSpeeches(p SpeechSearch) (Row, error) {
	w := where{clauses: []string{"1=1"}}
	if p.Query != "" {
		w.add("s.text LIKE '%' || ? || '%'", p.Query)
	}
	if p.Speaker != "" {
		w.add("s.speaker LIKE '%' || ? || '%'", p.Speaker)
	}
	if p.Party != "" {
		w.add("(s.party LIKE ? OR s.party IN (SELECT party FROM members WHERE party_code = ?))",
			p.Party, strings.ToUpper(p.Party))
	}
	if p.DateFrom != "" {
		w.add("m.date >= ?", p.DateFrom)
	}
	if p.DateTo != "" {
		w.add("m.date <= ?", p.DateTo)
	}
	if p.Meeting != "" {
		w.add("s.meeting_id = ?", p.Meeting)
	}
	total, err := s.count("SELECT COUNT(*) FROM speeches s JOIN meetings m ON m.id = s.meeting_id"+w.sql(), w.args...)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(`SELECT `+speechCols+`, m.date AS meeting_date, m.body_name,
			ai.number AS agenda_number, ai.title AS agenda_title
		FROM speeches s JOIN meetings m ON m.id = s.meeting_id
		LEFT JOIN agenda_items ai ON ai.meeting_id = s.meeting_id AND ai.anchor = s.agenda_anchor`+
		w.sql()+` ORDER BY m.date DESC, s.start_s LIMIT ? OFFSET ?`,
		append(w.args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		text := str(r["text"])
		delete(r, "text")
		r["snippet"] = snippet(text, p.Query)
		r["words"] = len(pyFields(text))
	}
	return Row{"total": total, "results": rows}, nil
}

// snippet cuts 120 characters before and 280 from the match. The offset is
// found in the casefolded text and applied to the original, as Python does.
func snippet(text, query string) string {
	i := 0
	if query != "" {
		i = runeIndex(fold(text), fold(query))
	}
	r := []rune(text)
	lo, hi := max(0, i-120), min(len(r), i+280)
	out := ""
	if i > 120 {
		out = "…"
	}
	if lo < hi {
		out += string(r[lo:hi])
	}
	return out + "…"
}

func runeIndex(s, sub string) int {
	b := strings.Index(s, sub)
	if b < 0 {
		return -1
	}
	return len([]rune(s[:b]))
}

// rosterFor is the members of the meeting's committee, as the roll-call
// grouping reads them.
func (s *Store) rosterFor(meetingID string) ([]Row, error) {
	return s.rows(`SELECT mb.name, mb.party, mb.party_code, mb.role FROM members mb
		JOIN meetings t ON t.body_id = mb.body_id WHERE t.id = ?`, meetingID)
}

func decodeRollCall(r Row, roster []Row) {
	t, ok := r["members"].(string)
	if !ok {
		return
	}
	var v any
	if json.Unmarshal([]byte(t), &v) != nil {
		return
	}
	if l, ok := v.([]any); ok {
		v = withGroups(l, roster)
	}
	r["members"] = v
}
