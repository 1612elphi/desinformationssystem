package ingest

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/PuerkitoBio/goquery"
)

var (
	TickerPoll     = envInt("TICKER_POLL", 120)
	TickerIdlePoll = envInt("TICKER_IDLE_POLL", 900)

	liveHrefRE = regexp.MustCompile(`/(\d+)/live`)
	liveCtxRE  = pyre(`(?i)(jetzt\s*live|sitzung läuft|läuft gerade)`)
	// vote images: …/{liveId}/{topId}/file/highres{n}_TOP {N}.jpg, with the space
	imgRE       = regexp.MustCompile(`(?i)https://sitzungskalender\.karlsruhe\.de/db/ratsinformation/\d+/\d+/file/highres[^"']+\.jpg`)
	imgTopRE    = pyre(`(?i)_TOP\s*([\dA-Za-z.]+)\.jpg`)
	committeRE  = pyre(`([A-ZÄÖÜ][^.<>()]{2,60}?)\s*\((?:öffentlich|nicht öffentlich|öffentlich/nicht öffentlich)\)\s*am\s+\d`)
	topStartRE  = pyre(`(?i)TOP\s*([\dA-Za-z.]+)`)
	topAheadRE  = pyre(`(?i)^TOP\s*[\dA-Za-z.]`)
	resultRE    = pyre(`(?is)^Ergebnis\s*</strong>.{0,80}?>\s*([^<]{3,80})`)
	anchorNumRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)*[a-z]?)`)
)

// CandidateLiveIDs lists ticker ids on the /live page, preferring those
// marked as running, else the first six.
func CandidateLiveIDs(page string) ([]string, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	flagged, all := map[string]bool{}, map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		m := liveHrefRE.FindStringSubmatch(a.AttrOr("href", ""))
		if m == nil {
			return
		}
		all[m[1]] = true
		ctx := ""
		anc := a
		for range 3 {
			anc = anc.Parent()
			if anc.Length() == 0 {
				break
			}
			ctx = text(anc, " ")
			if utf8.RuneCountInString(ctx) > 20 {
				break
			}
		}
		if liveCtxRE.MatchString(ctx) {
			flagged[m[1]] = true
		}
	})
	if len(flagged) > 0 {
		return sortedKeys(flagged), nil
	}
	ids := sortedKeys(all)
	return ids[:min(6, len(ids))], nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type TickerTop struct {
	TopLabel   string  `json:"top_label"`
	ImageURL   string  `json:"image_url"`
	ResultText *string `json:"result_text"`
}

type Ticker struct {
	Committee string      `json:"committee"`
	Date      string      `json:"date"`
	Tops      []TickerTop `json:"tops"`
}

// resultText finds each "TOP n … Ergebnis</strong> … >TEXT" within one TOP
// block: the gap of at most 800 characters may not cross another "TOP n".
// Python does this with a negative lookahead, which RE2 lacks.
func resultTexts(page string) [][2]string {
	var out [][2]string
	pos := 0
	for pos < len(page) {
		loc := topStartRE.FindStringSubmatchIndex(page[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		label := page[pos+loc[2] : pos+loc[3]]
		matched := false
		at := end
		for k := 0; k <= 800 && at <= len(page); k++ {
			if m := resultRE.FindStringSubmatchIndex(page[at:]); m != nil {
				out = append(out, [2]string{label, page[at+m[2] : at+m[3]]})
				pos, matched = at+m[1], true
				break
			}
			if at == len(page) || k == 800 || topAheadRE.MatchString(page[at:]) {
				break
			}
			_, size := utf8.DecodeRuneInString(page[at:])
			at += size
		}
		if !matched {
			_, size := utf8.DecodeRuneInString(page[start:])
			pos = start + size
		}
	}
	return out
}

// ParseTicker reads the header committee and date and the per-TOP vote images
// and result texts of a live-ticker page.
func ParseTicker(page string) (*Ticker, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	all := text(doc.Selection, " ")
	tk := &Ticker{Date: parseGermanDate(all), Tops: []TickerTop{}}
	// [^.] keeps the capture from running back across preceding nav text
	if cm := committeRE.FindStringSubmatch(all); cm != nil {
		tk.Committee = pyStrip(cm[1])
	}
	idx := map[string]int{}
	for _, u := range imgRE.FindAllString(page, -1) {
		mt := imgTopRE.FindStringSubmatch(u)
		if mt == nil {
			continue
		}
		label := "TOP " + mt[1]
		if _, ok := idx[label]; !ok {
			idx[label] = len(tk.Tops)
			tk.Tops = append(tk.Tops, TickerTop{TopLabel: label, ImageURL: u})
		}
	}
	for _, lr := range resultTexts(page) {
		label := "TOP " + lr[0]
		if i, ok := idx[label]; ok && (tk.Tops[i].ResultText == nil || *tk.Tops[i].ResultText == "") {
			res := pyStrip(collapse(lr[1]))
			tk.Tops[i].ResultText = &res
		}
	}
	return tk, nil
}

// matchMeeting needs a date match; several live sessions on one day are told
// apart by committee name.
func matchMeeting(tk *Ticker, live []store.Row) string {
	if tk.Date == "" {
		return ""
	}
	var same []store.Row
	for _, m := range live {
		if s, _ := m["date"].(string); s == tk.Date {
			same = append(same, m)
		}
	}
	if len(same) == 1 {
		id, _ := same[0]["id"].(string)
		return id
	}
	for _, m := range same {
		body, _ := m["body_name"].(string)
		if tk.Committee != "" && strings.HasPrefix(body, runeHead(tk.Committee, 12)) {
			id, _ := m["id"].(string)
			return id
		}
	}
	return ""
}

func processVote(st *store.Store, f *Fetcher, meetingID string, top TickerTop, counts *Counts) error {
	// keep the dotted sub-item form ("TOP 3.1" -> top3.1) the agenda anchors
	// use; dropping the dot would collide with TOP 31
	m := anchorNumRE.FindStringSubmatch(strings.ToLower(top.TopLabel))
	if m == nil {
		return nil
	}
	anchor := "top" + m[1]
	r := f.Get(strings.ReplaceAll(top.ImageURL, " ", "%20"), true)
	if r == nil {
		counts.Add("errors", 1)
		return nil
	}
	sum := sha256.Sum256(r.Body)
	sha := hex.EncodeToString(sum[:])
	prev, err := st.VoteImageSHA(meetingID, anchor)
	if err != nil {
		return err
	}
	if prev == sha {
		counts.Add("unchanged", 1)
		return nil
	}
	parsed := ParseVoteImage(r.Body, "image/jpeg")
	v := store.Vote{MeetingID: meetingID, AgendaAnchor: anchor, TopLabel: top.TopLabel,
		ResultText: top.ResultText, Source: "live", ImageURL: top.ImageURL}
	if parsed != nil {
		v.Ja, v.Nein, v.Enthaltung, v.Members = parsed.Ja, parsed.Nein, parsed.Enthaltung, parsed.Members
		// the dedupe hash is kept only on success, so a failed parse is retried
		v.ImageSHA = &sha
	}
	if err := st.Write(func(tx *sql.Tx) error { return store.UpsertVote(tx, v) }); err != nil {
		return err
	}
	if parsed == nil {
		counts.Add("errors", 1)
		return nil
	}
	counts.Add("parsed", 1)
	log.Printf("ticker: vote %s %s -> %s/%s/%s", meetingID, top.TopLabel, pyInt(parsed.Ja), pyInt(parsed.Nein), pyInt(parsed.Enthaltung))
	return nil
}

func pyInt(n *int) string {
	if n == nil {
		return "None"
	}
	return strconv.Itoa(*n)
}

// TickerOnce polls the ticker of every live meeting once.
func TickerOnce(st *store.Store) (*Counts, error) {
	counts := NewCounts("parsed", "unchanged", "errors", "tops")
	live, err := st.LiveMeetings(time.Now())
	if err != nil || len(live) == 0 {
		return counts, err
	}
	f := NewFetcher()
	defer f.Close()
	r := f.Get(RIS+"/live", false)
	if r == nil {
		return counts, nil
	}
	want := map[string]bool{}
	for _, m := range live {
		id, _ := m["id"].(string)
		want[id] = true
	}
	ids, err := CandidateLiveIDs(r.Text())
	if err != nil {
		return counts, err
	}
	for _, lid := range ids {
		if len(want) == 0 {
			break
		}
		rt := f.Get(RIS+"/"+lid+"/live", false)
		if rt == nil {
			continue
		}
		tk, err := ParseTicker(rt.Text())
		if err != nil {
			return counts, err
		}
		mid := matchMeeting(tk, live)
		if mid == "" {
			continue
		}
		delete(want, mid)
		for _, top := range tk.Tops {
			counts.Add("tops", 1)
			if err := processVote(st, f, mid, top, counts); err != nil {
				return counts, err
			}
		}
	}
	return counts, nil
}

// RunTicker polls fast while a session is live and slowly otherwise.
func RunTicker(st *store.Store) {
	log.Printf("ticker: ticker watcher start (poll=%ds idle=%ds, model=%s)", TickerPoll, TickerIdlePoll, VisionModel)
	for {
		c, err := TickerOnce(st)
		var live []store.Row
		if err == nil {
			live, err = st.LiveMeetings(time.Now())
		}
		if err != nil {
			// a transient DB or network error must not end the loop
			log.Printf("ticker: ticker run error: %v", err)
			c, live = NewCounts("parsed"), nil
		}
		nap := TickerIdlePoll
		if len(live) > 0 {
			nap = TickerPoll
		}
		if c.Get("parsed") > 0 || len(live) > 0 {
			log.Printf("ticker: ticker: %s | live=%d sleep=%ds", c, len(live), nap)
		}
		time.Sleep(time.Duration(nap) * time.Second)
	}
}
