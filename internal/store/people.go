package store

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Roll-call panels carry surnames only ("Dr. Mentrup", "Schütz, K."), while
// members has full names with titles. Matching is by surname, with the initial
// and the meeting's own committee roster as tie-breakers; an ambiguous surname
// without an initial is skipped.

var (
	titleRE     = regexp.MustCompile(`(?i)^(dr|prof|dipl|ing|med|rer|nat|phil|jur|habil|h|c)[.\-]*$`)
	rollTitleRE = regexp.MustCompile(`(?i)\b(?:dr|prof|dipl|ing|med|rer|nat|phil|jur|habil)\.\s*`)
)

func coreTokens(full string) (toks, core []string) {
	toks = strings.Fields(full)
	for _, t := range toks {
		if !strings.HasSuffix(t, ".") && !titleRE.MatchString(t) {
			core = append(core, t)
		}
	}
	return toks, core
}

func surname(full string) string {
	toks, core := coreTokens(full)
	switch {
	case len(core) > 0:
		return fold(core[len(core)-1])
	case len(toks) > 0:
		return fold(toks[len(toks)-1])
	}
	return ""
}

// firstInitial is "" when the name has no first name.
func firstInitial(full string) string {
	_, core := coreTokens(full)
	if len(core) < 2 {
		return ""
	}
	r := []rune(core[0])
	return fold(string(r[0]))
}

// parseRollName: "Schütz, K." -> ("schütz", "k"); "Dr.Huber" -> ("huber", "").
func parseRollName(name string) (sn, initial string) {
	rn := strings.TrimSpace(name)
	if before, after, ok := strings.Cut(rn, ","); ok {
		rn = before
		ini := strings.TrimRight(strings.TrimSpace(after), ".")
		if ini != "" {
			initial = fold(string([]rune(ini)[0]))
		}
	}
	toks := strings.Fields(rollTitleRE.ReplaceAllString(rn, ""))
	if len(toks) > 0 {
		sn = fold(toks[len(toks)-1])
	}
	return sn, initial
}

func (s *Store) ListPeople() ([]Row, error) {
	rows, err := s.rows(`SELECT mb.person_id, mb.name, mb.party, mb.party_code, mb.role,
			b.id AS body_id, b.name AS body_name
		FROM members mb JOIN bodies b ON b.id = mb.body_id ORDER BY mb.name`)
	if err != nil {
		return nil, err
	}
	byID := map[any]Row{}
	people := []Row{}
	for _, r := range rows {
		p, ok := byID[r["person_id"]]
		if !ok {
			p = Row{"person_id": r["person_id"], "name": r["name"], "party": r["party"],
				"party_code": r["party_code"], "memberships": []Row{}}
			byID[r["person_id"]] = p
			people = append(people, p)
		}
		if len(str(r["name"])) > len(str(p["name"])) {
			p["name"] = r["name"]
		}
		p["memberships"] = append(p["memberships"].([]Row),
			Row{"body_id": r["body_id"], "body_name": r["body_name"], "role": r["role"]})
	}
	sort.SliceStable(people, func(i, j int) bool {
		return surname(str(people[i]["name"])) < surname(str(people[j]["name"]))
	})
	return people, nil
}

func (s *Store) GetPerson(id string) (Row, error) {
	rows, err := s.rows(`SELECT mb.*, b.name AS body_name FROM members mb
		JOIN bodies b ON b.id = mb.body_id
		WHERE mb.person_id = ? ORDER BY b.name`, id)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	name := ""
	memberships := []Row{}
	for _, r := range rows {
		if n := str(r["name"]); len(n) > len(name) {
			name = n
		}
		memberships = append(memberships, Row{"body_id": r["body_id"], "body_name": r["body_name"],
			"role": r["role"], "since": r["since"]})
	}
	votes, err := s.personVotes(name)
	if err != nil {
		return nil, err
	}
	tally := map[string]int{}
	for _, v := range votes {
		tally[str(v["vote"])]++
	}
	return Row{"person_id": id, "name": name, "party": rows[0]["party"],
		"party_code": rows[0]["party_code"], "memberships": memberships,
		"votes": votes, "vote_summary": tally}, nil
}

func (s *Store) personVotes(full string) ([]Row, error) {
	sn, ini := surname(full), firstInitial(full)
	out := []Row{}
	if sn == "" {
		return out, nil
	}
	members, err := s.rows("SELECT body_id, name FROM members")
	if err != nil {
		return nil, err
	}
	counts := map[string]map[string]int{}
	total := 0
	for _, m := range members {
		b := str(m["body_id"])
		if counts[b] == nil {
			counts[b] = map[string]int{}
		}
		if n := surname(str(m["name"])); n == sn {
			counts[b][n]++
			total++
		}
	}
	rows, err := s.rows(`SELECT v.meeting_id, v.agenda_anchor, v.top_label, v.result_text, v.members,
			v.source, m.body_id, m.body_name, m.date AS meeting_date,
			ai.title AS agenda_title, ai.number AS agenda_number
		FROM votes v JOIN meetings m ON m.id = v.meeting_id
		LEFT JOIN agenda_items ai
			ON ai.meeting_id = v.meeting_id AND ai.anchor = v.agenda_anchor
		WHERE v.members IS NOT NULL
		ORDER BY m.date DESC`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		nSame := counts[str(r["body_id"])][sn]
		if nSame == 0 {
			nSame = total
		}
		for _, e := range rollCall(r["members"]) {
			esn, eini := parseRollName(str(e["name"]))
			if esn != sn || (eini != "" && ini != "" && eini != ini) || (eini == "" && nSame > 1) {
				continue
			}
			delete(r, "members")
			r["vote"] = e["vote"]
			r["roll_name"] = e["name"]
			out = append(out, r)
			break
		}
	}
	return out, nil
}

// VorlageChain is the timeline of a Vorlagennummer: every meeting where a
// document owns or mentions it, with the documents and any vote on their TOP.
func (s *Store) VorlageChain(nr string) (Row, error) {
	rows, err := s.rows(`SELECT fv.own, f.id, f.label, f.doc_type, f.meeting_id, f.agenda_anchor,
			f.submitters, m.date AS meeting_date, m.body_name, m.body_id,
			m.title AS meeting_title,
			ai.number AS agenda_number, ai.title AS agenda_title
		FROM file_vorlagen fv
		JOIN files f ON f.id = fv.file_id
		LEFT JOIN meetings m ON m.id = f.meeting_id`+agendaJoin+`
		WHERE fv.vorlage = ?
		ORDER BY m.date, f.id`, nr)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	stations := map[string]Row{}
	order := []Row{}
	own := []Row{}
	for _, d := range rows {
		decodeJSON(d)
		if num(d["own"]) != 0 {
			own = append(own, d)
		}
		delete(d, "own")
		mid := str(d["meeting_id"])
		if mid == "" {
			mid = "?"
		}
		st, ok := stations[mid]
		if !ok {
			st = Row{"meeting_id": d["meeting_id"], "date": d["meeting_date"],
				"body_name": d["body_name"], "body_id": d["body_id"],
				"meeting_title": d["meeting_title"], "documents": []Row{}, "votes": []Row{}}
			stations[mid] = st
			order = append(order, st)
		}
		st["documents"] = append(st["documents"].([]Row), d)
	}
	for _, st := range order {
		mid := str(st["meeting_id"])
		var anchors []any
		seen := map[string]bool{}
		for _, d := range st["documents"].([]Row) {
			if a := str(d["agenda_anchor"]); a != "" && !seen[a] {
				seen[a] = true
				anchors = append(anchors, a)
			}
		}
		if mid == "" || len(anchors) == 0 {
			continue
		}
		ph := strings.TrimSuffix(strings.Repeat("?,", len(anchors)), ",")
		if st["votes"], err = s.rows(`SELECT agenda_anchor, top_label, result_text, ja, nein, enthaltung, source
			FROM votes WHERE meeting_id = ? AND agenda_anchor IN (`+ph+`)`,
			append([]any{mid}, anchors...)...); err != nil {
			return nil, err
		}
	}
	key := func(st Row) string {
		d := str(st["date"])
		if d == "" {
			d = "9999"
		}
		return d + "\x00" + str(st["meeting_id"])
	}
	sort.SliceStable(order, func(i, j int) bool { return key(order[i]) < key(order[j]) })
	rank := map[string]int{"Beschlussvorlage": 0, "Vorlage": 1, "Antrag": 2, "Anfrage": 3}
	rankOf := func(d Row) int {
		if r, ok := rank[str(d["doc_type"])]; ok {
			return r
		}
		return 9
	}
	sort.SliceStable(own, func(i, j int) bool { return rankOf(own[i]) < rankOf(own[j]) })
	var title any
	if len(own) > 0 {
		title = own[0]["label"]
	}
	return Row{"vorlage": nr, "title": title, "own_documents": own, "stations": order}, nil
}

var (
	Berlin = mustZone("Europe/Berlin")
	// "15.30 bis 20 Uhr" / "16.30 bis 18.30 Uhr": dot is the minute separator
	timeRE = regexp.MustCompile(`(?i)(\d{1,2})(?:[.:](\d{1,2}))?\s*(?:bis|-|–|—)\s*(\d{1,2})(?:[.:](\d{1,2}))?\s*Uhr`)
)

func mustZone(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// ParseTimeWindow parses a meeting time string into start and end on day
// (wall-clock, in day's location). ok is false for unparseable or zero-length
// windows; an end before the start runs past midnight.
func ParseTimeWindow(s string, day time.Time) (start, end time.Time, ok bool) {
	m := timeRE.FindStringSubmatch(s)
	if m == nil {
		return
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	h1, m1, h2, m2 := n(1), n(2), n(3), n(4)
	if h1 > 23 || h2 > 23 || m1 > 59 || m2 > 59 {
		return
	}
	y, mo, d := day.Date()
	start = time.Date(y, mo, d, h1, m1, 0, 0, day.Location())
	end = time.Date(y, mo, d, h2, m2, 0, 0, day.Location())
	if end.Equal(start) {
		return start, end, false
	}
	if end.Before(start) {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, true
}

// LiveMeetings returns the meetings whose time window contains now (Berlin),
// soonest-ending first. Yesterday's meetings are included so a session past
// midnight stays live.
func (s *Store) LiveMeetings(now time.Time) ([]Row, error) {
	now = now.In(Berlin)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Berlin)
	yesterday := today.AddDate(0, 0, -1)
	rows, err := s.rows(`SELECT id, date, time FROM meetings WHERE date IN (?, ?)
		AND time IS NOT NULL AND time != ''`, today.Format(time.DateOnly), yesterday.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	type hit struct {
		end time.Time
		id  string
	}
	var hits []hit
	for _, r := range rows {
		day := yesterday
		if str(r["date"]) == today.Format(time.DateOnly) {
			day = today
		}
		if st, en, ok := ParseTimeWindow(str(r["time"]), day); ok && !now.Before(st) && !now.After(en) {
			hits = append(hits, hit{en, str(r["id"])})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].end.Before(hits[j].end) })
	out := []Row{}
	for _, h := range hits {
		m, err := s.GetMeeting(h.id)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

type partyCand struct{ initial, party string }

// partyLookup maps body_id -> surname -> candidates for roll-call attribution.
func (s *Store) partyLookup() (map[string]map[string][]partyCand, error) {
	rows, err := s.rows("SELECT body_id, name, party_code FROM members WHERE COALESCE(party_code,'') != ''")
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][]partyCand{}
	for _, r := range rows {
		b := str(r["body_id"])
		if out[b] == nil {
			out[b] = map[string][]partyCand{}
		}
		sn := surname(str(r["name"]))
		out[b][sn] = append(out[b][sn], partyCand{firstInitial(str(r["name"])), str(r["party_code"])})
	}
	return out, nil
}

func entryParty(lookup map[string]map[string][]partyCand, body, rollName string) string {
	sn, ini := parseRollName(rollName)
	cands := lookup[body][sn]
	if len(cands) == 0 {
		// fall back to any body (e.g. the OB is on no committee roster)
		for _, b := range lookup {
			cands = append(cands, b[sn]...)
		}
	}
	if ini != "" {
		var f []partyCand
		for _, c := range cands {
			if c.initial == "" || c.initial == ini {
				f = append(f, c)
			}
		}
		if len(f) > 0 {
			cands = f
		}
	}
	party := ""
	for _, c := range cands {
		if party != "" && c.party != party {
			return ""
		}
		party = c.party
	}
	return party
}

// round3 rounds like Python's round(x, 3) for the values seen here.
func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

var castKinds = []string{"ja", "nein", "enthaltung"}

// VoteAnalytics aggregates all parsed votes: party cohesion, pairwise
// agreement, dissenters. Unattributable roll-call entries are counted, not
// guessed.
func (s *Store) VoteAnalytics() (Row, error) {
	lookup, err := s.partyLookup()
	if err != nil {
		return nil, err
	}
	votes, err := s.rows(`SELECT v.id, v.ja, v.nein, v.enthaltung, v.members, v.source,
			m.body_id, m.body_name, m.date
		FROM votes v JOIN meetings m ON m.id = v.meeting_id`)
	if err != nil {
		return nil, err
	}
	totals := map[string]int{"votes": len(votes), "with_rollcall": 0, "unanimous": 0, "contested": 0}
	party := map[string]map[string]int{}
	var partyOrder []string
	pairs := map[[2]string][]bool{}
	dissent := map[string]Row{}
	var dissentOrder []string
	unknown := 0

	for _, v := range votes {
		ja, nein, enth := num(v["ja"]), num(v["nein"]), num(v["enthaltung"])
		if ja+nein+enth > 0 {
			if nein == 0 && enth == 0 {
				totals["unanimous"]++
			}
			if nein > 0 {
				totals["contested"]++
			}
		}
		if str(v["members"]) == "" {
			continue
		}
		roll := rollCall(v["members"])
		if roll == nil {
			continue
		}
		totals["with_rollcall"]++
		pv := map[string]map[string]int{}
		var pvOrder []string
		type att struct{ name, code, vote string }
		var attributed []att
		for _, e := range roll {
			name := str(e["name"])
			code := entryParty(lookup, str(v["body_id"]), name)
			if code == "" {
				unknown++
				continue
			}
			w := str(e["vote"])
			if w == "" {
				w = "abwesend"
			}
			if pv[code] == nil {
				pv[code] = map[string]int{}
				pvOrder = append(pvOrder, code)
			}
			pv[code][w]++
			attributed = append(attributed, att{name, code, w})
		}
		lines := map[string]string{}
		for _, code := range pvOrder {
			c := pv[code]
			nCast := c["ja"] + c["nein"] + c["enthaltung"]
			if nCast == 0 {
				continue
			}
			line := "ja"
			for _, k := range castKinds[1:] {
				if c[k] > c[line] {
					line = k
				}
			}
			lines[code] = line
			agg, ok := party[code]
			if !ok {
				agg = map[string]int{}
				party[code] = agg
				partyOrder = append(partyOrder, code)
			}
			agg["votes"]++
			agg["members_cast"] += nCast
			agg["with_line"] += c[line]
			for _, k := range []string{"ja", "nein", "enthaltung", "abwesend"} {
				agg[k] += c[k]
			}
		}
		for a := range lines {
			for b := range lines {
				if a < b {
					pairs[[2]string{a, b}] = append(pairs[[2]string{a, b}], lines[a] == lines[b])
				}
			}
		}
		for _, a := range attributed {
			line, ok := lines[a.code]
			if !ok || !(a.vote == "ja" || a.vote == "nein" || a.vote == "enthaltung") {
				continue
			}
			d, seen := dissent[a.name]
			if !seen {
				d = Row{"name": a.name, "party": a.code, "dissents": 0, "votes": 0}
				dissent[a.name] = d
				dissentOrder = append(dissentOrder, a.name)
			}
			d["votes"] = d["votes"].(int) + 1
			if a.vote != line {
				d["dissents"] = d["dissents"].(int) + 1
			}
		}
	}

	sort.SliceStable(partyOrder, func(i, j int) bool {
		return party[partyOrder[i]]["votes"] > party[partyOrder[j]]["votes"]
	})
	parties := []Row{}
	for _, code := range partyOrder {
		a := party[code]
		r := Row{"code": code}
		for _, k := range []string{"votes", "members_cast", "with_line", "ja", "nein", "enthaltung", "abwesend"} {
			r[k] = a[k]
		}
		cast, abw := a["members_cast"], a["abwesend"]
		r["cohesion"], r["attendance"] = nil, nil
		if cast > 0 {
			r["cohesion"] = round3(float64(a["with_line"]) / float64(cast))
		}
		if cast+abw > 0 {
			r["attendance"] = round3(1 - float64(abw)/float64(cast+abw))
		}
		parties = append(parties, r)
	}
	keys := make([][2]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	agreement := []Row{}
	for _, k := range keys {
		agree := 0
		for _, x := range pairs[k] {
			if x {
				agree++
			}
		}
		agreement = append(agreement, Row{"a": k[0], "b": k[1],
			"agree": round3(float64(agree) / float64(len(pairs[k]))), "n": len(pairs[k])})
	}
	dissenters := []Row{}
	for _, n := range dissentOrder {
		if dissent[n]["dissents"].(int) > 0 {
			dissenters = append(dissenters, dissent[n])
		}
	}
	sort.SliceStable(dissenters, func(i, j int) bool {
		di, dj := dissenters[i]["dissents"].(int), dissenters[j]["dissents"].(int)
		if di != dj {
			return di > dj
		}
		return dissenters[i]["votes"].(int) > dissenters[j]["votes"].(int)
	})
	if len(dissenters) > 20 {
		dissenters = dissenters[:20]
	}
	return Row{"totals": totals, "parties": parties, "agreement": agreement,
		"dissenters": dissenters, "unattributed_entries": unknown}, nil
}
