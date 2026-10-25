package ingest

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// RosterMember is one members row of a meeting's body.
type RosterMember struct {
	Name, Party, PartyCode, Role string
}

// meetingRoster lists the members of the meeting's body. distinct and the
// fallback body follow speech_analysis.roster; without them it is
// vod_cleanup's query. Row order matters to memberFor on duplicate names.
func meetingRoster(st *store.Store, meetingID string, distinct bool) ([]RosterMember, error) {
	q := `SELECT m.name, m.party, m.party_code, m.role FROM members m JOIN meetings t ON t.body_id = m.body_id WHERE t.id = ?`
	if distinct {
		q = `SELECT DISTINCT m.name, m.party, m.party_code, m.role FROM members m
			WHERE m.body_id = COALESCE((SELECT body_id FROM meetings WHERE id = ?), 'organisation-gr-32130')`
	}
	rows, err := st.DB.Query(q, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RosterMember
	for rows.Next() {
		var n, p, c, r *string
		if err := rows.Scan(&n, &p, &c, &r); err != nil {
			return nil, err
		}
		out = append(out, RosterMember{deref(n), deref(p), deref(c), deref(r)})
	}
	return out, rows.Err()
}

func lastField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// memberFor resolves a seat-map short name ("Dr. Huber", "Schütz, K.") or a
// full name to a roster row; nil when none or several fit.
func memberFor(name string, members []RosterMember) *RosterMember {
	if name == "" {
		return nil
	}
	for i := range members {
		if members[i].Name == name {
			return &members[i]
		}
	}
	base, initial, _ := strings.Cut(name, ",")
	dr := strings.HasPrefix(strings.TrimSpace(base), "Dr.")
	sn := lastField(strings.ReplaceAll(base, "Dr.", ""))
	var hits []*RosterMember
	for i := range members {
		if lastField(members[i].Name) == sn {
			hits = append(hits, &members[i])
		}
	}
	if len(hits) > 1 {
		var drHits []*RosterMember
		for _, m := range hits {
			if strings.HasPrefix(m.Name, "Dr.") == dr {
				drHits = append(drHits, m)
			}
		}
		if len(drHits) > 0 {
			hits = drHits
		}
	}
	if ini := strings.TrimSpace(initial); len(hits) > 1 && ini != "" {
		r, _ := utf8.DecodeRuneInString(ini)
		var iniHits []*RosterMember
		for _, m := range hits {
			if strings.HasPrefix(strings.ReplaceAll(m.Name, "Dr. ", ""), string(r)) {
				iniHits = append(iniHits, m)
			}
		}
		hits = iniHits
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return nil
}

func partyOf(name string, members []RosterMember) string {
	m := memberFor(name, members)
	if m == nil {
		return ""
	}
	if strings.Contains(m.Role, "Oberbürgermeister") {
		return "OB"
	}
	return m.PartyCode
}

// wordPattern is a regexp with Python's Unicode \b at either end (RE2's \b
// is ASCII-only, so "Noé" would never end a word) and the lookaheads RE2
// lacks: notBefore must not match, and needAfter must prefix, the text
// right after the match.
type wordPattern struct {
	rx         *regexp.Regexp
	start, end bool
	notBefore  *regexp.Regexp
	needAfter  string
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (p wordPattern) findAll(text string) []string {
	var out []string
	for _, m := range p.rx.FindAllStringIndex(text, -1) {
		before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
		after, _ := utf8.DecodeRuneInString(text[m[1]:])
		rest := text[m[1]:]
		// every pattern starts and ends on a letter
		if p.start && isWordRune(before) || p.end && isWordRune(after) {
			continue
		}
		if p.notBefore != nil && p.notBefore.MatchString(rest) {
			continue
		}
		if p.needAfter != "" && !strings.HasPrefix(rest, p.needAfter) {
			continue
		}
		out = append(out, text[m[0]:m[1]])
	}
	return out
}

var lowerNext = regexp.MustCompile(`^\s+[a-zäöü]`)

func word(rx string) wordPattern {
	return wordPattern{rx: regexp.MustCompile(rx), start: true, end: true}
}

// partyOrder is the display order; also the order mentions are searched.
var partyOrder = []string{"B90", "CDU", "SPD", "AFD", "FDP", "VOL", "LIN", "KAL", "PAR", "FÜR"}

// Case-sensitive on purpose: "grüne Wiese" and "für Karlsruhe" are ordinary
// German.
var partyPatterns = map[string][]wordPattern{
	"B90": {word(`Grünen`), word(`GRÜNEN?`), {rx: regexp.MustCompile(`Grüne`), start: true, end: true, notBefore: lowerNext}},
	"CDU": {word(`CDU`)},
	"SPD": {word(`SPD`)},
	"AFD": {word(`AfD`), word(`AFD`)},
	"FDP": {word(`FDP`), word(`Freien? Wähler`), word(`FW`)},
	"VOL": {word(`Volt`)},
	"LIN": {{rx: regexp.MustCompile(`Linken?`), start: true, end: true, notBefore: lowerNext}, word(`LINKE`)},
	"KAL": {word(`KAL`), word(`Karlsruher Liste`)},
	"PAR": {word(`PARTEI`), {rx: regexp.MustCompile(`Partei`), start: true, end: true, needAfter: " Die"}},
	"FÜR": {word(`FÜR`)},
}

// member roles carry "Herr/Frau (Dr.) Stadtrat X"; a bare surname is too
// ambiguous (Müller)
const addressRx = `(?:Herr|Frau|Kollege|Kollegin)\s+(?:Stadtrat|Stadträtin|Dr\.\s+)?\s*`

// mentions maps party code -> the words that named it (fractions and
// addressed members), own excluded.
func mentions(text string, members []RosterMember, own string) map[string][]string {
	found := map[string][]string{}
	for _, code := range partyOrder {
		for _, p := range partyPatterns[code] {
			found[code] = append(found[code], p.findAll(text)...)
		}
	}
	for _, m := range members {
		sn := lastField(m.Name)
		if m.PartyCode != "" && utf8.RuneCountInString(sn) > 2 && !strings.Contains(m.Role, "Oberbürgermeister") {
			p := wordPattern{rx: regexp.MustCompile(addressRx + regexp.QuoteMeta(sn)), end: true}
			found[m.PartyCode] = append(found[m.PartyCode], p.findAll(text)...)
		}
	}
	for c, v := range found {
		if len(v) == 0 || c == own {
			delete(found, c)
		}
	}
	return found
}
