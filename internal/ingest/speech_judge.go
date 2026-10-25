package ingest

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"log"
	"sort"
	"strings"

	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

// speech words sent to Jev; longer speeches are cut (cost and focus)
const jevWords = 1500

// JevSystemOne is the Jev call; a variable so tests run without TypeSafe.
var JevSystemOne = func(state any, q map[string]typesafe.Question) (map[string]typesafe.Answer, error) {
	return typesafe.SystemOne(state, q, 5)
}

var partyNames = map[string]string{"B90": "Grüne", "CDU": "CDU", "SPD": "SPD", "AFD": "AfD", "FDP": "FDP/Freie Wähler",
	"VOL": "Volt", "LIN": "Die Linke", "KAL": "Karlsruher Liste", "PAR": "Die PARTEI", "FÜR": "FÜR Karlsruhe"}

func criteria(kv ...string) typesafe.Criteria {
	var c typesafe.Criteria
	for i := 0; i+1 < len(kv); i += 2 {
		c = append(c, typesafe.Criterion{Key: kv[i], Desc: &kv[i+1]})
	}
	return c
}

type subItem struct{ anchor, title string }

func judgeQuestions(topTitle string, targets []string, subs []subItem) map[string]typesafe.Question {
	q := map[string]typesafe.Question{
		"stance": typesafe.Choice("A speech in the Karlsruhe city council under the agenda item «"+topTitle+"». "+
			"What is the speaker's position on the proposal being decided?", criteria(
			"dafuer", "supports the proposal or motion",
			"dagegen", "opposes it or asks to reject it",
			"gemischt", "supports parts and opposes parts, or demands changes",
			"keine", "takes no position (procedure, a question, information only)")),
		"tone": typesafe.Choice("What is the dominant tone of this council speech?", criteria(
			"sachlich", "factual, explanatory, neutral",
			"kooperativ", "conciliatory, seeks common ground, thanks others",
			"kritisch", "criticises firmly but on the substance",
			"polemisch", "attacks, mocks or exaggerates; personal or rhetorical jabs")),
		"sentiment": typesafe.Choice("What is the overall sentiment of this speech?", criteria(
			"positiv", "positive, approving, optimistic",
			"neutral", "neutral or balanced",
			"negativ", "negative, disapproving, alarmed")),
	}
	// A TOP debated together with its sub-items (14 with 14.1, a fraction's
	// motion) gets one stance per item: Müller spoke against motion 14.1 and
	// voted for 14 on 2026-09-29.
	for _, s := range subs {
		q["stance_"+s.anchor] = typesafe.Choice("This speech may also address the related motion «"+s.title+"». What is the "+
			"speaker's position on that motion specifically?", criteria(
			"dafuer", "supports that motion", "dagegen", "opposes that motion",
			"gemischt", "partly supports it", "keine", "does not address that motion"))
	}
	for _, code := range targets {
		name := partyNames[code]
		if name == "" {
			name = code
		}
		q["to_"+code] = typesafe.Choice("The speech mentions the council group «"+name+"». How does the "+
			"speaker relate to that group's position here?", criteria(
			"zustimmend", "agrees with them, praises or supports their position",
			"ablehnend", "disagrees with them, criticises or rejects their position",
			"neutral", "only names them, or no clear attitude"))
	}
	return q
}

type speechToJudge struct {
	id, meetingID, anchor, speaker, text, topTitle string
}

// JudgeSpeeches runs Jev over speeches whose text has no judgement yet (all
// with redo), for one meeting or all. Returns the count judged.
func JudgeSpeeches(st *store.Store, meetingID string, redo bool) (int, error) {
	rows, err := st.DB.Query(`SELECT s.id, s.meeting_id, s.agenda_anchor, s.speaker, s.text, ai.title
		FROM speeches s
		LEFT JOIN agenda_items ai ON ai.meeting_id = s.meeting_id AND ai.anchor = s.agenda_anchor
		WHERE (? IS NULL OR s.meeting_id = ?)`, nullable(meetingID), nullable(meetingID))
	if err != nil {
		return 0, err
	}
	var todo []speechToJudge
	for rows.Next() {
		var id, mid, anchor, speaker, text, title *string
		if err := rows.Scan(&id, &mid, &anchor, &speaker, &text, &title); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, speechToJudge{deref(id), deref(mid), deref(anchor), deref(speaker), deref(text), deref(title)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	done := map[string]string{}
	if rows, err = st.DB.Query("SELECT speech_id, COALESCE(text_sha, '') FROM speech_judgements"); err != nil {
		return 0, err
	}
	for rows.Next() {
		var id, sha string
		if err := rows.Scan(&id, &sha); err != nil {
			rows.Close()
			return 0, err
		}
		done[id] = sha
	}
	rows.Close()
	rosters := map[string][]RosterMember{}
	n := 0
	for _, r := range todo {
		sum := sha1.Sum([]byte(r.text))
		sha := hex.EncodeToString(sum[:])
		if !redo && done[r.id] == sha {
			continue
		}
		members, ok := rosters[r.meetingID]
		if !ok {
			if members, err = meetingRoster(st, r.meetingID, true); err != nil {
				return n, err
			}
			rosters[r.meetingID] = members
		}
		var targets []string
		for c := range mentions(r.text, members, partyOf(r.speaker, members)) {
			targets = append(targets, c)
		}
		sort.Strings(targets)
		var subs []subItem
		if r.anchor != "" {
			sr, err := st.DB.Query("SELECT anchor, title FROM agenda_items WHERE meeting_id = ? AND anchor LIKE ?", r.meetingID, r.anchor+".%")
			if err != nil {
				return n, err
			}
			for sr.Next() {
				var a, t *string
				if err := sr.Scan(&a, &t); err != nil {
					sr.Close()
					return n, err
				}
				subs = append(subs, subItem{deref(a), pyStr(stringOrNil(t))})
			}
			sr.Close()
		}
		f := strings.Fields(r.text)
		state := strings.Join(f[:min(len(f), jevWords)], " ")
		title := r.topTitle
		if title == "" {
			title = "?"
		}
		ans, err := JevSystemOne(state, judgeQuestions(title, targets, subs))
		if err != nil {
			// one failure must not lose the rest of the run
			log.Printf("speech-judge:   jev failed for %s: %v", r.id, err)
			continue
		}
		probs := map[string]json.RawMessage{}
		for k, v := range ans {
			probs[k] = v.Probabilities
			if v.Probabilities == nil {
				probs[k] = json.RawMessage("null")
			}
		}
		b, err := json.Marshal(probs)
		if err != nil {
			return n, err
		}
		if _, err := st.DB.Exec("INSERT OR REPLACE INTO speech_judgements(speech_id, text_sha, answers) VALUES(?, ?, ?)",
			r.id, sha, string(b)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func stringOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
