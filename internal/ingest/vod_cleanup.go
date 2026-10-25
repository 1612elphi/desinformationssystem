package ingest

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// speaker by segment midpoint

var (
	CleanupModel = envStr("CLEANUP_MODEL", "TERRA")
	// thinking eats max_tokens
	CleanupChunkWords = envInt("CLEANUP_CHUNK_WORDS", 1500)
	CleanupTimeout    = envSeconds("CLEANUP_TIMEOUT", 600)
)

const (
	// seconds; caps fragment merging
	mergeGap = 30.0
	// below: summary, keep raw
	minKept   = 0.5
	fillWords = 20
)

const cleanupPrompt = `Du bereitest das Wortprotokoll einer Sitzung des Karlsruher Gemeinderats auf.
Die Reden stammen aus einer automatischen Transkription (Whisper) und sind nummeriert.
"Sprecher" kommt aus der Saalanzeige; "unbekannt" heißt, niemand war markiert, meist der
Vorsitz (Oberbürgermeister Dr. Mentrup) oder die Verwaltung.

Für jede Rede:
- Satzzeichen und Absätze korrigieren (Absätze mit einer Leerzeile trennen).
- Füllwörter (äh, ähm), Versprecher, abgebrochene Satzanfänge und Wortwiederholungen entfernen.
- Falsch verstandene Namen, Orte und Fachbegriffe NUR anhand des Glossars korrigieren.
- Den Wortlaut sonst beibehalten: nicht zusammenfassen, nicht umformulieren, nichts ergänzen.
- keep=false, wenn die Rede nur Sitzungsleitung ist: Aufrufen von Rednern oder
  Tagesordnungspunkten, Abstimmungsaufrufe, Verkünden von Ergebnissen, Hinweise zur Redezeit,
  Dank ohne Inhalt. keep=true für jede inhaltliche Aussage, auch vom Vorsitz (Antworten,
  Stellungnahmen, Erläuterungen der Verwaltung).
- Enthält eine Rede sowohl Sitzungsleitung als auch Inhalt, keep=true und nur den
  inhaltlichen Teil als text.
- speaker: bei "unbekannt" den Namen einsetzen, wenn er sich eindeutig ergibt (z. B. der
  Vorsitz ruft "Herr Bürgermeister X" auf, oder der Vorsitz spricht selbst); sonst null.
  Bei bekanntem Sprecher den Namen unverändert zurückgeben.

Antworte nur mit JSON: {"speeches": [{"i": int, "keep": bool, "speaker": str|null, "text": str}]}
`

// Speech is one speaker's stretch under one TOP. Empty strings stand for
// Python's None.
type Speech struct {
	Speaker, PanelSpeaker, Top, Anchor string
	Start, End                         float64
	Text, Clean                        string
	Keep, Failed, Shortened            bool
}

func words(s string) int { return len(strings.Fields(s)) }

func endsSentence(s string) bool {
	s = strings.TrimRight(s, " \t\n\r\f\v")
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!")
}

func joinSpeakers(segments []Segment, spans []SpeakerSpan) []Speech {
	var out []Speech
	var parts []string
	for _, seg := range segments {
		mid := (seg.Start + seg.End) / 2
		i := sort.Search(len(spans), func(i int) bool { return spans[i].Start > mid }) - 1
		name, top := "", ""
		if i >= 0 {
			name, top = deref(spans[i].Name), deref(spans[i].Top)
		}
		if n := len(out); n > 0 && out[n-1].Speaker == name && out[n-1].Top == top {
			parts = append(parts, seg.Text)
			out[n-1].End = seg.End
			out[n-1].Text = strings.Join(parts, " ")
			continue
		}
		parts = []string{seg.Text}
		out = append(out, Speech{Speaker: name, PanelSpeaker: name, Top: top, Start: seg.Start, End: seg.End, Text: seg.Text})
	}
	return out
}

// chair border drops mid-answer
func fillGaps(sp []Speech) []Speech {
	for i := 1; i < len(sp)-1; i++ {
		a, s, b := sp[i-1], &sp[i], sp[i+1]
		if s.Speaker == "" && a.Speaker != "" && a.Speaker == b.Speaker && a.Top == s.Top && s.Top == b.Top &&
			(words(s.Text) <= fillWords || !endsSentence(a.Text)) {
			s.Speaker, s.PanelSpeaker = a.Speaker, a.Speaker
		}
	}
	// jev tried: no better
	for i := 1; i < len(sp); i++ {
		a, s := sp[i-1], &sp[i]
		if s.Speaker == "" && a.Speaker != "" && a.Top == s.Top && !endsSentence(a.Text) {
			s.Speaker, s.PanelSpeaker = a.Speaker, a.Speaker
		}
	}
	var out []Speech
	for _, s := range sp {
		if n := len(out); n > 0 && out[n-1].Speaker == s.Speaker && out[n-1].Top == s.Top {
			out[n-1].Text += " " + s.Text
			out[n-1].End = s.End
		} else {
			out = append(out, s)
		}
	}
	return out
}

func cleanupGlossary(st *store.Store, members []store.Row, meetingID, anchor string) (string, error) {
	lines := []string{"Mitglieder des Gemeinderats (Name, Fraktion, Funktion):"}
	// sqlite order: null first
	sorted := slices.Clone(members)
	slices.SortStableFunc(sorted, func(a, b store.Row) int {
		if (a["name"] == nil) != (b["name"] == nil) {
			if a["name"] == nil {
				return -1
			}
			return 1
		}
		return strings.Compare(str(a["name"]), str(b["name"]))
	})
	for _, m := range sorted {
		lines = append(lines, fmt.Sprintf("- %s (%s, %s)", pyStr(m["name"]), pyStr(m["party"]), pyStr(m["role"])))
	}
	if anchor != "" {
		var num, title any
		err := st.DB.QueryRow("SELECT number, title FROM agenda_items WHERE meeting_id=? AND anchor=?", meetingID, anchor).Scan(&num, &title)
		if err == nil {
			lines = append(lines, fmt.Sprintf("\nTagesordnungspunkt %s: %s", pyStr(sqlText(num)), pyStr(sqlText(title))))
		} else if err != sql.ErrNoRows {
			return "", err
		}
	}
	var terms []string
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			terms = append(terms, t)
		}
	}
	ents, err := queryStrings(st, "SELECT entities FROM files WHERE meeting_id=? AND agenda_anchor IS ?", meetingID, orNil(anchor))
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		var ent map[string]any
		if json.Unmarshal([]byte(cmp.Or(e, "{}")), &ent) != nil {
			continue
		}
		for _, k := range []string{"people", "orgs", "locations"} {
			list, _ := ent[k].([]any)
			for _, x := range list {
				if s, ok := x.(string); ok {
					add(s)
				}
			}
		}
	}
	streets, err := queryStrings(st, `SELECT DISTINCT fs.name FROM file_streets fs JOIN files f ON f.id = fs.file_id
		WHERE f.meeting_id=? AND f.agenda_anchor IS ?`, meetingID, orNil(anchor))
	if err != nil {
		return "", err
	}
	for _, s := range streets {
		add(s)
	}
	if len(terms) > 0 {
		lines = append(lines, "\nNamen, Organisationen und Orte aus den Unterlagen dieses Punkts:", strings.Join(terms, ", "))
	}
	return strings.Join(lines, "\n"), nil
}

func sqlText(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func cleanupChunks(sp []*Speech) [][]*Speech {
	var out [][]*Speech
	var cur []*Speech
	n := 0
	for _, s := range sp {
		w := words(s.Text)
		if len(cur) > 0 && n+w > CleanupChunkWords {
			out = append(out, cur)
			cur, n = nil, 0
		}
		cur = append(cur, s)
		n += w
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

type cleanedSpeech struct {
	I       *int    `json:"i"`
	Keep    *bool   `json:"keep"`
	Speaker *string `json:"speaker"`
	Text    *string `json:"text"`
}

// CleanupCall sends one cleanup request and returns the model's reply text;
// a variable so tests and replays run without the model.
var CleanupCall = func(system, user string) (string, error) {
	return switchboardChat(map[string]any{
		"model": CleanupModel, "max_tokens": 32000,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []any{
			map[string]string{"role": "system", "content": system},
			map[string]string{"role": "user", "content": user},
		},
	}, "dis-vod-cleanup", CleanupTimeout)
}

func cleanupUser(chunk []*Speech, gloss string) string {
	parts := make([]string, len(chunk))
	for i, s := range chunk {
		parts[i] = fmt.Sprintf("[%d] Sprecher: %s\n%s", i, cmp.Or(s.Speaker, "unbekannt"), s.Text)
	}
	return "Glossar:\n" + gloss + "\n\nReden:\n\n" + strings.Join(parts, "\n\n")
}

func callCleanup(chunk []*Speech, gloss string) (map[int]cleanedSpeech, error) {
	text, err := CleanupCall(cleanupPrompt, cleanupUser(chunk, gloss))
	if err != nil {
		return nil, err
	}
	var out struct {
		Speeches []cleanedSpeech `json:"speeches"`
	}
	if json.Unmarshal([]byte(text), &out) != nil {
		return nil, nil
	}
	byI := map[int]cleanedSpeech{}
	for _, o := range out.Speeches {
		if o.I == nil || o.Keep == nil {
			return nil, nil
		}
		byI[*o.I] = o
	}
	return byI, nil
}

func cleanChunk(chunk []*Speech, gloss string) error {
	byI, err := callCleanup(chunk, gloss)
	if err != nil {
		return err
	}
	if byI == nil && len(chunk) > 1 {
		half := len(chunk) / 2
		log.Printf("vod-cleanup:     cut-off answer, splitting %d speeches", len(chunk))
		if err := cleanChunk(chunk[:half], gloss); err != nil {
			return err
		}
		return cleanChunk(chunk[half:], gloss)
	}
	for i, s := range chunk {
		o, ok := byI[i]
		s.Failed, s.Keep, s.Clean = !ok, true, s.Text
		if ok {
			s.Keep = *o.Keep
			if o.Text != nil && *o.Text != "" {
				s.Clean = *o.Text
			}
			// tile border is truth
			if s.Speaker == "" {
				s.Speaker = deref(o.Speaker)
			}
		}
		// fillers are few percent
		s.Shortened = s.Keep && float64(words(s.Clean)) < minKept*float64(words(s.Text))
		if s.Shortened {
			s.Clean = s.Text
		}
	}
	return nil
}

// chair border flickers
func mergeKept(sp []*Speech) []Speech {
	var out []Speech
	for _, s := range sp {
		if !s.Keep {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Speaker == s.Speaker && out[n-1].Anchor == s.Anchor && s.Start-out[n-1].End <= mergeGap {
			p := &out[n-1]
			p.Text += " " + s.Text
			if strings.HasSuffix(p.Clean, ".") || strings.HasSuffix(p.Clean, "?") || strings.HasSuffix(p.Clean, "!") {
				p.Clean += "\n\n" + s.Clean
			} else {
				p.Clean += " " + s.Clean
			}
			p.End = s.End
			p.Shortened = p.Shortened || s.Shortened
			p.Failed = p.Failed || s.Failed
		} else {
			out = append(out, *s)
		}
	}
	return out
}

var nameTitles = []string{"Oberbürgermeister", "Bürgermeisterin", "Bürgermeister", "Stadträtin", "Stadtrat", "Herr", "Frau"}

func bareName(n string) string {
	for _, t := range nameTitles {
		n = strings.ReplaceAll(n, t+" ", "")
	}
	return strings.TrimSpace(n)
}

// whisper truncates surnames
func canonicalNames(sp []Speech, roster []string) {
	var known []string
	for _, n := range roster {
		if !slices.Contains(known, n) {
			known = append(known, n)
		}
	}
	for _, s := range sp {
		if n := bareName(s.Speaker); s.Speaker != "" && !slices.Contains(known, n) {
			known = append(known, n)
		}
	}
	rl := utf8.RuneCountInString
	for i := range sp {
		if sp[i].Speaker == "" {
			continue
		}
		n := bareName(sp[i].Speaker)
		last := store.LastField(n)
		// exactly one fit required
		var fits []string
		for _, k := range known {
			kl := store.LastField(k)
			if k != n && rl(k) > rl(n) && strings.HasPrefix(kl, last) && (rl(kl) > rl(last) || n == last) {
				fits = append(fits, k)
			}
		}
		sp[i].Speaker = n
		if len(fits) == 1 {
			sp[i].Speaker = fits[0]
		}
	}
}

func loadSpeakerInputs(video string) ([]Segment, []SpeakerSpan, error) {
	base := strings.TrimSuffix(video, filepath.Ext(video))
	var segs []Segment
	var spans []SpeakerSpan
	if err := readJSON(base+".transcript.json", &segs); err != nil {
		return nil, nil, err
	}
	if err := readJSON(base+".speakers.json", &spans); err != nil {
		return nil, nil, err
	}
	return segs, spans, nil
}

// id from <date>_<id>.<ext>
func videoID(video string) string {
	b := filepath.Base(video)
	if _, after, ok := strings.Cut(b, "_"); ok {
		b = after
	}
	if i := strings.LastIndex(b, "."); i >= 0 {
		b = b[:i]
	}
	return b
}

// ProcessCleanup builds the speeches of one VOD and, with apply, replaces
// the video's rows (only one agenda anchor's when only is set).
func ProcessCleanup(st *store.Store, video, meetingID, only string, apply bool) ([]Speech, error) {
	segments, spans, err := loadSpeakerInputs(video)
	if err != nil {
		return nil, err
	}
	speeches := fillGaps(joinSpeakers(segments, spans))
	anchors, err := agendaAnchors(st, meetingID)
	if err != nil {
		return nil, err
	}
	// panel label beats call
	type call struct {
		t float64
		a string
	}
	var calls []call
	for a, t := range TopOffsets(segments, anchors) {
		calls = append(calls, call{t, a})
	}
	slices.SortFunc(calls, func(x, y call) int { return cmp.Or(cmp.Compare(x.t, y.t), cmp.Compare(x.a, y.a)) })
	var order []string
	groups := map[string][]*Speech{}
	for i := range speeches {
		s := &speeches[i]
		if s.Top != "" {
			s.Anchor = ResolveAnchor(s.Top, anchors)
		}
		if s.Anchor == "" {
			j := sort.Search(len(calls), func(j int) bool { return calls[j].t > s.Start }) - 1
			if j >= 0 {
				s.Anchor = calls[j].a
			}
		}
		if _, ok := groups[s.Anchor]; !ok {
			order = append(order, s.Anchor)
		}
		groups[s.Anchor] = append(groups[s.Anchor], s)
	}
	members, err := st.Roster(meetingID)
	if err != nil {
		return nil, err
	}
	var done []Speech
	for _, anchor := range order {
		group := groups[anchor]
		if only != "" && anchor != only {
			continue
		}
		n := 0
		for _, s := range group {
			n += words(s.Text)
		}
		log.Printf("vod-cleanup:   %s: %d speeches, %d words", cmp.Or(anchor, "ohne TOP"), len(group), n)
		gloss, err := cleanupGlossary(st, members, meetingID, anchor)
		if err != nil {
			return nil, err
		}
		for _, chunk := range cleanupChunks(group) {
			if err := cleanChunk(chunk, gloss); err != nil {
				return nil, err
			}
		}
		done = append(done, mergeKept(group)...)
	}
	// panel seat decides speaker
	for i := range done {
		if p := done[i].PanelSpeaker; p != "" {
			done[i].Speaker = p
			if m := store.MemberFor(p, members); m != nil {
				done[i].Speaker = str(m["name"])
			}
		}
	}
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = str(m["name"])
	}
	canonicalNames(done, names)
	if !apply {
		return done, nil
	}
	vid := videoID(video)
	// party from seat map
	sm, err := LoadSeatmap()
	if err != nil {
		return nil, err
	}
	seatParty := map[string]string{}
	for _, s := range sm.Layouts[0].Seats {
		seatParty[s.ID] = s.Party
	}
	nameParty := map[string]string{}
	for _, a := range sm.Assignments {
		nameParty[a.Name] = seatParty[a.Seat]
	}
	return done, st.Write(func(tx *sql.Tx) error {
		var err error
		if only != "" {
			_, err = tx.Exec("DELETE FROM speeches WHERE meeting_id=? AND video_id=? AND agenda_anchor IS ?", meetingID, vid, only)
		} else {
			// meetings span several recordings
			_, err = tx.Exec("DELETE FROM speeches WHERE meeting_id=? AND video_id=?", meetingID, vid)
		}
		if err != nil {
			return err
		}
		for _, s := range done {
			speaker, party := orNil(s.Speaker), ""
			if m := store.MemberFor(s.Speaker, members); m != nil {
				name := str(m["name"])
				speaker, party = &name, str(m["party"])
			}
			if party == "" {
				party = nameParty[cmp.Or(s.PanelSpeaker, s.Speaker)]
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO speeches(id,meeting_id,agenda_anchor,top_label,speaker,
				party,video_id,start_s,end_s,text_raw,text) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				fmt.Sprintf("%s:%s:%d", meetingID, vid, int(s.Start)), meetingID, orNil(s.Anchor), orNil(s.Top),
				speaker, orNil(party), vid, s.Start, s.End, s.Text, s.Clean); err != nil {
				return err
			}
		}
		return nil
	})
}
