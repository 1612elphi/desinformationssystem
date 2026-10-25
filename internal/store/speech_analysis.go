package store

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Aggregation of the speeches table for the "Reden" tab and MCP: floor time
// vs seats, said-vs-voted, tone, who-talks-about-whom, distinctive words,
// readability. Map iteration orders, float summation orders and tie-breaks
// follow speech_analysis.py, so the output equals the Python one.

const pySpaceClass = `[\pZ\t\n\v\f\r\x1c-\x1f\x85]`

// Python's \w, which decides where \b sits; Go's \b is ASCII-only.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }

func runeBefore(s string, i int) rune {
	if i <= 0 {
		return -1
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

func runeAt(s string, i int) rune {
	if i >= len(s) {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

func boundary(s string, i int) bool {
	b, a := runeBefore(s, i), runeAt(s, i)
	return (b >= 0 && isWordRune(b)) != (a >= 0 && isWordRune(a))
}

// matcher is a Python pattern split into an RE2 body and the assertions RE2
// lacks, checked on the full text around each candidate.
type matcher struct {
	re     *regexp.Regexp
	lb, rb bool // \b before / after
	ok     func(s string, start, end int) bool
}

// findAll is re.findall: a rejected candidate is retried one rune later.
func (m matcher) findAll(s string) []string {
	var out []string
	for pos := 0; pos <= len(s); {
		loc := m.re.FindStringIndex(s[pos:])
		if loc == nil {
			break
		}
		a, b := pos+loc[0], pos+loc[1]
		if (!m.lb || boundary(s, a)) && (!m.rb || boundary(s, b)) && (m.ok == nil || m.ok(s, a, b)) {
			out = append(out, s[a:b])
			pos = max(b, a+1)
			continue
		}
		_, n := utf8.DecodeRuneInString(s[a:])
		pos = a + max(n, 1)
	}
	return out
}

func word(p string) matcher { return matcher{re: regexp.MustCompile(p), lb: true, rb: true} }

// notBeforeLower is (?!\s+[a-zäöü]): "Grüne" the fraction, not "grüne Wiese".
func notBeforeLower(p string) matcher {
	m := word(p)
	m.ok = func(s string, _, end int) bool {
		i := end
		for i < len(s) && isPySpace(runeAt(s, i)) {
			i += utf8.RuneLen(runeAt(s, i))
		}
		return i == end || !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzäöü", runeAt(s, i))
	}
	return m
}

type partyRx struct {
	code string
	rxs  []matcher
}

// Case-sensitive on purpose: "grüne Wiese" and "für Karlsruhe" are ordinary German.
var partyPatterns = []partyRx{
	{"B90", []matcher{word(`Grünen`), word(`GRÜNEN?`), notBeforeLower(`Grüne`)}},
	{"CDU", []matcher{word(`CDU`)}},
	{"SPD", []matcher{word(`SPD`)}},
	{"AFD", []matcher{word(`AfD`), word(`AFD`)}},
	{"FDP", []matcher{word(`FDP`), word(`Freien? Wähler`), word(`FW`)}},
	{"VOL", []matcher{word(`Volt`)}},
	{"LIN", []matcher{notBeforeLower(`Linken?`), word(`LINKE`)}},
	{"KAL", []matcher{word(`KAL`), word(`Karlsruher Liste`)}},
	{"PAR", []matcher{word(`PARTEI`), {re: regexp.MustCompile(`Partei`), lb: true,
		ok: func(s string, _, end int) bool { return strings.HasPrefix(s[end:], " Die") }}}},
	{"FÜR", []matcher{word(`FÜR`)}},
}

var partyOrder = []string{"B90", "CDU", "SPD", "AFD", "FDP", "VOL", "LIN", "KAL", "PAR", "FÜR"}

// member roles carry "Herr/Frau (Dr.) Stadtrat X"; a bare surname is too ambiguous (Müller)
const addressPrefix = `(?:Herr|Frau|Kollege|Kollegin)` + pySpaceClass + `+(?:Stadtrat|Stadträtin|Dr\.` +
	pySpaceClass + `+)?` + pySpaceClass + `*`

var addressCache sync.Map

func addressRx(surname string) matcher {
	if m, ok := addressCache.Load(surname); ok {
		return m.(matcher)
	}
	m := matcher{re: regexp.MustCompile(addressPrefix + regexp.QuoteMeta(surname)), rb: true}
	addressCache.Store(surname, m)
	return m
}

// ordered is a dict with Python's insertion order.
type ordered[V int | float64] struct {
	keys []string
	n    map[string]V
}

func newOrdered[V int | float64]() *ordered[V] { return &ordered[V]{n: map[string]V{}} }

func (o *ordered[V]) add(k string, v V) {
	if _, ok := o.n[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.n[k] += v
}

// pySum is Python 3.12's sum() over floats, which compensates rounding
// (Neumaier); plain += differs in the last bits.
type pySum struct{ s, c float64 }

func (p *pySum) add(x float64) {
	t := p.s + x
	if math.Abs(p.s) >= math.Abs(x) {
		p.c += (p.s - t) + x
	} else {
		p.c += (x - t) + p.s
	}
	p.s = t
}

func (p pySum) total() float64 {
	if p.c != 0 && !math.IsInf(p.c, 0) && !math.IsNaN(p.c) {
		return p.s + p.c
	}
	return p.s
}

func fsum(o *ordered[float64]) float64 {
	var p pySum
	for _, k := range o.keys {
		p.add(o.n[k])
	}
	return p.total()
}

func (o *ordered[V]) sum() V {
	var t V
	for _, k := range o.keys {
		t += o.n[k]
	}
	return t
}

// mentions maps a party code to the words that named it (fraction names and
// addressed members), the speaker's own party left out; codes in Python's order.
func mentions(text string, members []Row, own string) ([]string, map[string][]string) {
	var keys []string
	found := map[string][]string{}
	touch := func(c string, hits []string) {
		if _, ok := found[c]; !ok {
			keys = append(keys, c)
			found[c] = nil
		}
		found[c] = append(found[c], hits...)
	}
	for _, p := range partyPatterns {
		touch(p.code, nil)
		for _, m := range p.rxs {
			touch(p.code, m.findAll(text))
		}
	}
	for _, m := range members {
		code := str(m["party_code"])
		sn := lastField(str(m["name"]))
		if code != "" && utf8.RuneCountInString(sn) > 2 && !isChair(m) {
			touch(code, addressRx(sn).findAll(text))
		}
	}
	var out []string
	for _, k := range keys {
		if len(found[k]) > 0 && k != own {
			out = append(out, k)
		}
	}
	return out, found
}

func partyOf(name string, members []Row) string {
	m := MemberFor(name, members)
	if m == nil {
		return ""
	}
	if isChair(m) {
		return "OB"
	}
	return str(m["party_code"])
}

func setOf(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

var stopwords = setOf(`
aber alle allem allen aller alles als also am an ander andere anderem anderen anderer anderes
anders auch auf aus bei beim bin bis bisher bist bitte da dabei dadurch dafür dagegen daher
dahin damit danach dann daran darauf daraus darf darin darum darüber das dass daß dazu dem
den denen denn dennoch der deren des deshalb dessen die dies diese diesem diesen dieser dieses
doch dort du durch eben ein eine einem einen einer eines einfach einige einmal er es etwa etwas
euch euer eure für gab ganz gar gibt ging gegen genau gerade geht gewesen gibt habe haben hat
hatte hatten hier hin hinter ich ihm ihn ihnen ihr ihre ihrem ihren ihrer im immer in indem
ins ist ja jede jedem jeden jeder jedes jedoch jetzt kann kein keine keinem keinen keiner
können könnte man manche mehr mein meine meinem meinen meiner mich mir mit muss müssen musste
nach nachdem natürlich nein nicht nichts noch nun nur ob oder ohne sehr sein seine seinem
seinen seiner seit selbst sich sie sind so sogar solche soll sollen sollte sondern sowie
später statt um und uns unser unsere unserem unseren unserer unter viel viele vielleicht vom
von vor wann war waren warum was weil weiter weitere welche welchem welchen welcher wenn wer
werde werden wie wieder will wir wird wirklich wo wollen worden wurde wurden würde würden zu
zum zur zwar zwischen also mal eigentlich schon halt quasi insofern deswegen hierzu genauso
gesagt sagen sage sagt gehen gehe kommen komme kommt machen macht gemacht glaube denke finde
heute einfach gerne jahr jahre jahren prozent gut klar ganz letzte ersten erste zwei drei
über möchte möchten steht stehen tatsächlich wichtig ende beim sei wäre wären hätte hätten
gibt geben sehen gesehen reden sprechen dafür deutlich daher bereits wirklich richtig
irgendwie nämlich eigentlich zumindest letztendlich übrigens selbstverständlich einer
`)

// salutations and chairing vocabulary every speech shares
var boilerplate = setOf(`
oberbürgermeister oberbürgermeisterin bürgermeister bürgermeisterin kolleginnen kollegen
kollege kollegin damen herren geehrte geehrter geehrten liebe lieben vielen dank danke
dankeschön stadtrat stadträtin stadträte herr frau fraktion fraktionen antrag anträge
vorlage verwaltung gemeinderat karlsruhe stadt livestream
`)

var genericNouns = setOf(`
frage fragen thema themen punkt punkte beispiel ende zeit seite seiten sicht stelle rahmen
moment fall fälle jahr jahre jahren prozent sache dinge ding teil art weise grund problem
probleme möglichkeit möglichkeiten entscheidung vorschlag diskussion debatte ihnen sie ihr
ihre ihrer ihren ihrem ich wir
`)

var wordRE = regexp.MustCompile(`[A-Za-zÄÖÜäöüß][A-Za-zÄÖÜäöüß-]+`)

// German capitalises nouns, so a capitalised word that does not open a
// sentence is a noun or a name; counting only those drops the verbs and
// fillers that topped the per-party lists on 2026-09-29.
var nounRx = matcher{re: regexp.MustCompile(`[A-ZÄÖÜ][a-zäöüß][A-Za-zÄÖÜäöüß-]+`), lb: true,
	ok: func(s string, start, _ int) bool {
		if start == 0 {
			return false
		}
		p := runeBefore(s, start)
		return !isPySpace(p) || !strings.ContainsRune(".!?:", runeBefore(s, start-utf8.RuneLen(p)))
	}}

func tokens(text string) []string {
	var out []string
	for _, t := range nounRx.findAll(text) {
		w := strings.Trim(t, "-")
		lw := strings.ToLower(w)
		if utf8.RuneCountInString(w) > 2 && !stopwords[lw] && !boilerplate[lw] && !genericNouns[lw] {
			out = append(out, w)
		}
	}
	return out
}

var (
	vowelsRE   = regexp.MustCompile(`[aeiouäöüy]+`)
	sentenceRE = regexp.MustCompile(`[.!?]+` + pySpaceClass)
)

// readability is Amstad's German Flesch reading ease and the mean words per
// sentence.
func readability(text string) (float64, float64) {
	sentences := 0
	for _, s := range sentenceRE.Split(text, -1) {
		if strings.TrimFunc(s, isPySpace) != "" {
			sentences++
		}
	}
	words := wordRE.FindAllString(text, -1)
	if sentences == 0 || len(words) == 0 {
		return 0, 0
	}
	syl := 0
	for _, w := range words {
		syl += max(1, len(vowelsRE.FindAllString(strings.ToLower(w), -1)))
	}
	asl := float64(len(words)) / float64(sentences)
	asw := float64(syl) / float64(len(words))
	return 180 - asl - 58.5*asw, asl
}

type scoredWord struct {
	z    float64
	w    string
	n    int
	keep bool
}

// distinctive is the log-odds ratio with an informative Dirichlet prior
// (Monroe, Colaresi & Quinn 2008, "Fightin' Words"): per group the words most
// over-used against all other groups, ranked by z-score.
func distinctive(groups []string, counts map[string]*ordered[int], top, minCount int) map[string][]Row {
	prior := newOrdered[int]()
	for _, g := range groups {
		for _, w := range counts[g].keys {
			prior.add(w, counts[g].n[w])
		}
	}
	a0 := prior.sum()
	if a0 == 0 {
		a0 = 1
	}
	out := map[string][]Row{}
	for _, g := range groups {
		c := counts[g]
		nI := c.sum()
		// Counter subtraction keeps positive counts only
		nJ := 0
		for _, w := range prior.keys {
			nJ += max(0, prior.n[w]-c.n[w])
		}
		var scored []scoredWord
		for _, w := range c.keys {
			yI, aW := c.n[w], prior.n[w]
			if aW < minCount {
				continue
			}
			yJ := max(0, aW-yI)
			d := math.Log(float64(yI+aW)/float64(nI+a0-yI-aW)) -
				math.Log(float64(yJ+aW)/float64(nJ+a0-yJ-aW))
			v := 1/float64(yI+aW) + 1/float64(yJ+aW)
			scored = append(scored, scoredWord{z: d / math.Sqrt(v), w: w, n: yI})
		}
		sort.SliceStable(scored, func(i, j int) bool {
			a, b := scored[i], scored[j]
			if a.z != b.z {
				return a.z > b.z
			}
			if a.w != b.w {
				return a.w > b.w
			}
			return a.n > b.n
		})
		list := []Row{}
		for i, s := range scored {
			if i < top && s.z > 0 {
				list = append(list, Row{"word": s.w, "z": pyRound(s.z, 2), "n": s.n})
			}
		}
		out[g] = list
	}
	return out
}

// pyRound is Python's round(x, n): the correctly rounded decimal, ties to even.
func pyRound(x float64, n int) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', n, 64), 64)
	return v
}

func pyRoundInt(x float64) int64 { return int64(math.RoundToEven(x)) }

func numf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}

// probs is one question's {choice: probability} in JSON order.
type probs struct {
	keys []string
	p    map[string]float64
}

func (p probs) get(k string) float64 { return p.p[k] }

// answer is one speech judgement: question -> probs, in JSON order.
type answer struct {
	keys []string
	q    map[string]probs
}

func (a *answer) get(k string) probs {
	if a == nil {
		return probs{}
	}
	return a.q[k]
}

// objectKeys decodes a JSON object keeping its key order.
func objectKeys(raw []byte) ([]string, map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, false
	}
	var keys []string
	vals := map[string]json.RawMessage{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, nil, false
		}
		k, _ := t.(string)
		var v json.RawMessage
		if dec.Decode(&v) != nil {
			return nil, nil, false
		}
		if _, dup := vals[k]; !dup {
			keys = append(keys, k)
		}
		vals[k] = v
	}
	return keys, vals, true
}

func parseAnswer(s string) *answer {
	keys, vals, ok := objectKeys([]byte(s))
	if !ok || len(keys) == 0 {
		return nil
	}
	a := &answer{keys: keys, q: map[string]probs{}}
	for _, k := range keys {
		ck, cv, _ := objectKeys(vals[k])
		p := probs{keys: ck, p: map[string]float64{}}
		for _, c := range ck {
			var f float64
			_ = json.Unmarshal(cv[c], &f)
			p.p[c] = f
		}
		a.q[k] = p
	}
	return a
}

// stanceMin is the stance probability a said-vs-voted comparison needs to count.
const stanceMin = 0.6

const defaultBody = "organisation-gr-32130"

func (s *Store) speechRoster(meetingID any) ([]Row, error) {
	return s.rows(`SELECT DISTINCT m.name, m.party, m.party_code, m.role FROM members m
		WHERE m.body_id = COALESCE((SELECT body_id FROM meetings WHERE id = ?), '`+defaultBody+`')`, meetingID)
}

// seatCounts is seats per group from the fullest roll-call of the meeting,
// else of the latest meeting with one. The roster is no good for this: it
// keeps former members without marking all of them.
func (s *Store) seatCounts(meetingID string) (*ordered[int], error) {
	seats := newOrdered[int]()
	mid := ""
	if meetingID != "" {
		n, err := s.count("SELECT COUNT(*) FROM votes WHERE meeting_id=? AND members IS NOT NULL", meetingID)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			mid = meetingID
		}
	}
	if mid == "" {
		r, err := s.row(`SELECT v.meeting_id FROM votes v JOIN meetings m ON m.id=v.meeting_id
			WHERE v.members IS NOT NULL ORDER BY m.date DESC LIMIT 1`)
		if err != nil {
			return nil, err
		}
		if mid = str(r["meeting_id"]); mid == "" {
			return seats, nil
		}
	}
	votes, err := s.MeetingVotes(mid)
	if err != nil {
		return nil, err
	}
	var full []any
	for _, v := range votes {
		if l, ok := v["members"].([]any); ok && len(l) > len(full) {
			full = l
		}
	}
	for _, e := range full {
		m, _ := e.(map[string]any)
		if g := str(m["group"]); g != "" && g != "OB" {
			seats.add(g, 1)
		}
	}
	return seats, nil
}

type speechRow struct {
	meetingID, speaker, party, topLabel, text string
	anchor, topNumber, topTitle, date         any
	answers                                   *answer
	secs                                      float64
	words                                     int
}

type voteKey struct {
	mid    string
	anchor any
}

type voteRec struct {
	roll  []map[string]any
	tally Row
}

// SpeechAnalytics aggregates the transcribed debates, of one meeting or all.
func (s *Store) SpeechAnalytics(meetingID string) (Row, error) {
	q := `SELECT s.id, s.meeting_id, s.agenda_anchor, s.top_label, s.speaker, s.start_s, s.end_s, s.text,
			j.answers, ai.number AS top_number, ai.title AS top_title, m.date
		FROM speeches s JOIN meetings m ON m.id = s.meeting_id
		LEFT JOIN speech_judgements j ON j.speech_id = s.id
		LEFT JOIN agenda_items ai ON ai.meeting_id = s.meeting_id AND ai.anchor = s.agenda_anchor`
	var args []any
	if meetingID != "" {
		q += " WHERE s.meeting_id = ?"
		args = append(args, meetingID)
	}
	raw, err := s.rows(q+" ORDER BY s.meeting_id, s.start_s", args...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var meetings []string
	for _, r := range raw {
		if m := str(r["meeting_id"]); !seen[m] {
			seen[m] = true
			meetings = append(meetings, m)
		}
	}
	sort.Strings(meetings)
	var last any
	lastID := ""
	if len(meetings) > 0 {
		lastID = meetings[len(meetings)-1]
		last = lastID
	}
	members, err := s.speechRoster(last)
	if err != nil {
		return nil, err
	}
	rows := make([]speechRow, len(raw))
	var named []*speechRow
	for i, r := range raw {
		t := str(r["text"])
		rows[i] = speechRow{
			meetingID: str(r["meeting_id"]), speaker: str(r["speaker"]), topLabel: str(r["top_label"]),
			text: t, anchor: r["agenda_anchor"], topNumber: r["top_number"], topTitle: r["top_title"],
			date: r["date"], secs: max(0, numf(r["end_s"])-numf(r["start_s"])), words: len(pyFields(t)),
		}
		if a := str(r["answers"]); a != "" {
			rows[i].answers = parseAnswer(a)
		}
		rows[i].party = partyOf(rows[i].speaker, members)
		if rows[i].party != "" {
			named = append(named, &rows[i])
		}
	}

	seats, err := s.seatCounts(lastID)
	if err != nil {
		return nil, err
	}
	totalSeats := float64(seats.sum())
	if totalSeats == 0 {
		totalSeats = 1
	}
	var ts pySum
	for _, r := range named {
		if r.party != "OB" {
			ts.add(r.secs)
		}
	}
	totalSecs := ts.total()
	if totalSecs == 0 {
		totalSecs = 1
	}

	parties := []Row{}
	for _, code := range append([]string{"OB"}, partyOrder...) {
		var rs, judged []*speechRow
		for _, r := range named {
			if r.party == code {
				rs = append(rs, r)
				if r.answers != nil {
					judged = append(judged, r)
				}
			}
		}
		_, hasSeats := seats.n[code]
		if len(rs) == 0 && !hasSeats {
			continue
		}
		wsum := 0
		for _, r := range judged {
			wsum += r.words
		}
		if wsum == 0 {
			wsum = 1
		}
		var sentiment, toneOut any
		tone := newOrdered[float64]()
		if len(judged) > 0 {
			var acc pySum
			for _, r := range judged {
				sp := r.answers.get("sentiment")
				acc.add((sp.get("positiv") - sp.get("negativ")) * float64(r.words))
			}
			sentiment = pyRound(acc.total()/float64(wsum), 3)
			for _, r := range judged {
				tp := r.answers.get("tone")
				for _, k := range tp.keys {
					tone.add(k, tp.p[k]*float64(r.words))
				}
			}
			tsum := fsum(tone)
			if tsum == 0 {
				tsum = 1
			}
			t := Row{}
			for _, k := range tone.keys {
				t[k] = pyRound(tone.n[k]/tsum, 3)
			}
			toneOut = t
		}
		var flesch, sentenceLen any
		var ss pySum
		words := 0
		speakers := map[string]bool{}
		texts := make([]string, len(rs))
		for i, r := range rs {
			texts[i] = r.text
			ss.add(r.secs)
			words += r.words
			speakers[r.speaker] = true
		}
		if len(rs) > 0 {
			f, asl := readability(strings.Join(texts, " "))
			flesch, sentenceLen = pyRound(f, 1), pyRound(asl, 1)
		}
		secs := ss.total()
		var seatShare, timeShare any
		if code != "OB" {
			seatShare = float64(seats.n[code]) / totalSeats
			timeShare = secs / totalSecs
		}
		parties = append(parties, Row{
			"code": code, "seats": seats.n[code], "seat_share": seatShare, "time_share": timeShare,
			"secs": pyRoundInt(secs), "words": words, "speeches": len(rs), "speakers": len(speakers),
			"sentiment": sentiment, "tone": toneOut, "flesch": flesch, "sentence_len": sentenceLen,
		})
	}

	type speakerAcc struct {
		name, party  string
		secs         float64
		words, count int
	}
	bySpeaker := map[string]*speakerAcc{}
	var speakerList []*speakerAcc
	for _, r := range named {
		a := bySpeaker[r.speaker]
		if a == nil {
			a = &speakerAcc{name: r.speaker, party: r.party}
			bySpeaker[r.speaker] = a
			speakerList = append(speakerList, a)
		}
		a.secs += r.secs
		a.words += r.words
		a.count++
	}
	sort.SliceStable(speakerList, func(i, j int) bool { return speakerList[i].secs > speakerList[j].secs })
	speakers := []Row{}
	for i, a := range speakerList {
		if i < 25 {
			speakers = append(speakers, Row{"name": a.name, "party": a.party, "secs": pyRoundInt(a.secs),
				"words": a.words, "speeches": a.count})
		}
	}

	// said vs voted: each (meeting, TOP, speaker)'s word-weighted stance against their vote
	vrows, err := s.rows(`SELECT meeting_id, agenda_anchor, members, ja, nein, enthaltung
		FROM votes WHERE members IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	votes := map[voteKey]voteRec{}
	for _, v := range vrows {
		if roll := rollCall(v["members"]); roll != nil {
			votes[voteKey{str(v["meeting_id"]), v["agenda_anchor"]}] = voteRec{roll, v}
		}
	}
	type stanceKey struct {
		mid     string
		anchor  any
		speaker string
		party   string
	}
	stanceAcc := map[stanceKey]*ordered[float64]{}
	var stanceKeys []stanceKey
	amendRE := regexp.MustCompile(`(?i)antrag`)
	for _, r := range named {
		// a speech under an amendment label ("TOP 6 Änderungsantrag Grüne") argues the
		// amendment, whose vote is not stored; comparing it with the main vote is wrong
		if lr := []rune(r.topLabel); len(lr) > 5 && amendRE.MatchString(string(lr[5:])) {
			continue
		}
		if r.answers == nil {
			continue
		}
		for _, qk := range r.answers.keys {
			var anchor any
			switch {
			case qk == "stance":
				anchor = r.anchor
			case strings.HasPrefix(qk, "stance_"):
				anchor = qk[len("stance_"):]
			default:
				continue
			}
			p := r.answers.q[qk]
			if len(p.keys) == 0 {
				continue
			}
			k := stanceKey{r.meetingID, anchor, r.speaker, r.party}
			acc := stanceAcc[k]
			if acc == nil {
				acc = newOrdered[float64]()
				stanceAcc[k] = acc
				stanceKeys = append(stanceKeys, k)
			}
			for _, c := range p.keys {
				acc.add(c, p.p[c]*float64(r.words))
			}
		}
	}
	saidVoted := []Row{}
	decided, matched := 0, 0
	for _, k := range stanceKeys {
		rec, ok := votes[voteKey{k.mid, k.anchor}]
		if !ok || k.party == "OB" {
			continue
		}
		// roll-calls use the panel's short names ("Schütz, K."); resolve those to the
		// roster, since the other direction cannot tell Katrin from Nicolas Schütz
		var vote any
		for _, x := range rec.roll {
			if m := MemberFor(str(x["name"]), members); m != nil && str(m["name"]) == k.speaker {
				vote = x["vote"]
				break
			}
		}
		acc := stanceAcc[k]
		tot := fsum(acc)
		if tot == 0 {
			tot = 1
		}
		stance, best := "", math.Inf(-1)
		for _, c := range acc.keys {
			if p := acc.n[c] / tot; p > best {
				stance, best = c, p
			}
		}
		if stance == "keine" {
			continue
		}
		anchor := "None"
		if k.anchor != nil {
			anchor = str(k.anchor)
		}
		n, err := s.count(`SELECT COUNT(*) FROM agenda_items WHERE meeting_id = ?
			AND (anchor LIKE ? OR ? LIKE anchor || '.%')`, k.mid, anchor+".%", k.anchor)
		if err != nil {
			return nil, err
		}
		joint := n > 0
		// below stanceMin the model is guessing; a jointly debated TOP leaves open which
		// item a sentence argues, so those rows are shown but not counted either
		var match any
		vs, _ := vote.(string)
		if (vs == "ja" || vs == "nein") && (stance == "dafuer" || stance == "dagegen") && best >= stanceMin && !joint {
			match = (vs == "ja") == (stance == "dafuer")
			decided++
			if match == true {
				matched++
			}
		}
		saidVoted = append(saidVoted, Row{"meeting_id": k.mid, "anchor": k.anchor, "speaker": k.speaker,
			"party": k.party, "stance": stance, "p": pyRound(best, 2), "vote": vote, "match": match,
			"joint": joint})
	}
	sort.SliceStable(saidVoted, func(i, j int) bool {
		fi, fj := saidVoted[i]["match"] == false, saidVoted[j]["match"] == false
		if fi != fj {
			return fi
		}
		return saidVoted[i]["p"].(float64) > saidVoted[j]["p"].(float64)
	})

	// who talks about whom: mention counts and the judged attitude, party x party
	type edge struct {
		from, to string
		n        int
		score, w float64
	}
	edges := map[[2]string]*edge{}
	var edgeList []*edge
	for _, r := range named {
		if r.party == "OB" {
			continue
		}
		codes, _ := mentions(r.text, members, r.party)
		for _, c := range codes {
			e := edges[[2]string{r.party, c}]
			if e == nil {
				e = &edge{from: r.party, to: c}
				edges[[2]string{r.party, c}] = e
				edgeList = append(edgeList, e)
			}
			e.n++
			if p := r.answers.get("to_" + c); len(p.keys) > 0 {
				e.score += p.get("zustimmend") - p.get("ablehnend")
				e.w++
			}
		}
	}
	matrix := []Row{}
	for _, e := range edgeList {
		var att any
		if e.w > 0 {
			att = pyRound(e.score/e.w, 3)
		}
		matrix = append(matrix, Row{"from": e.from, "to": e.to, "n": e.n, "attitude": att})
	}

	// distinctive words per party, and overall frequency
	counts := map[string]*ordered[int]{}
	var groups []string
	for _, r := range named {
		if r.party == "OB" {
			continue
		}
		c := counts[r.party]
		if c == nil {
			c = newOrdered[int]()
			counts[r.party] = c
			groups = append(groups, r.party)
		}
		for _, t := range tokens(r.text) {
			c.add(t, 1)
		}
	}
	overall := newOrdered[int]()
	for _, g := range groups {
		for _, w := range counts[g].keys {
			overall.add(w, counts[g].n[w])
		}
	}
	ow := append([]string(nil), overall.keys...)
	sort.SliceStable(ow, func(i, j int) bool { return overall.n[ow[i]] > overall.n[ow[j]] })
	overallOut := []Row{}
	for i, w := range ow {
		if i < 30 {
			overallOut = append(overallOut, Row{"word": w, "n": overall.n[w]})
		}
	}

	// debate length per TOP next to its vote
	type topAcc struct {
		row     Row
		secs    float64
		parties map[string]bool
	}
	tops := map[voteKey]*topAcc{}
	var topKeys []voteKey
	for i := range rows {
		r := &rows[i]
		if str(r.anchor) == "" {
			continue
		}
		k := voteKey{r.meetingID, r.anchor}
		t := tops[k]
		if t == nil {
			t = &topAcc{row: Row{"meeting_id": r.meetingID, "anchor": r.anchor, "number": r.topNumber,
				"title": r.topTitle, "date": r.date, "speeches": 0}, parties: map[string]bool{}}
			tops[k] = t
			topKeys = append(topKeys, k)
		}
		t.secs += r.secs
		t.row["speeches"] = t.row["speeches"].(int) + 1
		if r.party != "" && r.party != "OB" {
			t.parties[r.party] = true
		}
	}
	topList := []Row{}
	for _, k := range topKeys {
		t := tops[k]
		tally := votes[k].tally
		cast := num(tally["ja"]) + num(tally["nein"]) + num(tally["enthaltung"])
		var dissent any
		if cast != 0 {
			dissent = pyRound(float64(num(tally["nein"]))/float64(cast), 3)
		}
		t.row["secs"] = pyRoundInt(t.secs)
		t.row["parties"] = len(t.parties)
		t.row["ja"], t.row["nein"], t.row["enthaltung"] = tally["ja"], tally["nein"], tally["enthaltung"]
		t.row["dissent"] = dissent
		topList = append(topList, t.row)
	}
	sort.SliceStable(topList, func(i, j int) bool { return topList[i]["secs"].(int64) > topList[j]["secs"].(int64) })

	meetingRows := []Row{}
	if len(meetings) > 0 {
		ids := make([]any, len(meetings))
		for i, m := range meetings {
			ids[i] = m
		}
		if meetingRows, err = s.rows(`SELECT id, date, body_name FROM meetings WHERE id IN (?`+
			strings.Repeat(",?", len(ids)-1)+`) ORDER BY date DESC`, ids...); err != nil {
			return nil, err
		}
	}
	var totalAll pySum
	words, judged := 0, 0
	for _, r := range rows {
		totalAll.add(r.secs)
		words += r.words
		if r.answers != nil {
			judged++
		}
	}
	return Row{
		"meetings": meetingRows,
		"totals": Row{"speeches": len(rows), "secs": pyRoundInt(totalAll.total()), "words": words,
			"speakers": len(bySpeaker), "meetings": len(meetings), "judged": judged},
		"parties":    parties,
		"speakers":   speakers,
		"said_voted": Row{"n": decided, "match": matched, "rows": saidVoted},
		"matrix":     matrix,
		"words": Row{"overall": overallOut,
			"distinctive": distinctive(groups, counts, 12, 3)},
		"tops": topList,
	}, nil
}
