package ingest

import (
	"crypto/sha1"
	"encoding/hex"
	"log"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var germanMonths = map[string]time.Month{
	"januar": 1, "februar": 2, "märz": 3, "maerz": 3, "april": 4, "mai": 5,
	"juni": 6, "juli": 7, "august": 8, "september": 9, "oktober": 10,
	"november": 11, "dezember": 12,
}

func parseDoc(page string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(page))
}

// Committee is one organisation-gr-NNNNN id and its name.
type Committee struct{ ID, Name string }

var orgRE = regexp.MustCompile(`organisation-gr-\d+`)

// ParseCommittees keeps first-seen order; a repeated id takes the later name.
func ParseCommittees(page string) ([]Committee, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	var out []Committee
	idx := map[string]int{}
	doc.Find(`a[href*="organisation-gr-"]`).Each(func(_ int, a *goquery.Selection) {
		id := orgRE.FindString(a.AttrOr("href", ""))
		name := collapse(text(a, " "))
		if id == "" || name == "" {
			return
		}
		if i, ok := idx[id]; ok {
			out[i].Name = name
			return
		}
		idx[id] = len(out)
		out = append(out, Committee{id, name})
	})
	return out, nil
}

var terminRE = regexp.MustCompile(`termin-\d+`)

func ParseCalendarTermine(page string) []string {
	set := map[string]bool{}
	for _, id := range terminRE.FindAllString(page, -1) {
		set[id] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

type Member struct {
	PersonID  string `json:"person_id"`
	Name      string `json:"name"`
	Party     string `json:"party"`
	PartyCode string `json:"party_code"`
	Role      string `json:"role"`
	Since     string `json:"since"`
}

var (
	peopleRE = regexp.MustCompile(`people-(\d+)`)
	yearRE   = regexp.MustCompile(`\d{4}`)
)

func firstPeopleLink(s *goquery.Selection) *goquery.Selection {
	return s.Find("a[href]").FilterFunction(func(_ int, a *goquery.Selection) bool {
		return peopleRE.MatchString(a.AttrOr("href", ""))
	}).First()
}

// ParseMembers reads the first roster table of a committee page.
func ParseMembers(page string) ([]Member, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	out := []Member{}
	seen := map[string]bool{}
	doc.Find("table.contenttable").EachWithBreak(func(_ int, tbl *goquery.Selection) bool {
		if firstPeopleLink(tbl).Length() == 0 {
			return true
		}
		tbl.Find("tr").Each(func(_ int, tr *goquery.Selection) {
			a := firstPeopleLink(tr)
			if a.Length() == 0 {
				return
			}
			pid := peopleRE.FindStringSubmatch(a.AttrOr("href", ""))[1]
			if seen[pid] {
				return
			}
			seen[pid] = true
			var cells []string
			tr.Find("td, th").Each(func(_ int, c *goquery.Selection) {
				cells = append(cells, collapse(text(c, " ")))
			})
			info := ""
			if len(cells) > 1 {
				info = cells[1]
			}
			party, role := "", info
			if p, r, ok := strings.Cut(info, ","); ok {
				party, role = pyStrip(p), pyStrip(r)
			} else if f := strings.FieldsFunc(info, isPySpace); len(f) > 0 && len(scanFractions(f[0])) > 0 {
				// "FDP/FW Stadträtin Ordentliches Mitglied": the RIS omits the comma for some
				rest := strings.TrimLeftFunc(info, isPySpace)[len(f[0]):]
				party, role = f[0], strings.TrimLeftFunc(rest, isPySpace)
			}
			since := ""
			for _, c := range cells[min(2, len(cells)):] {
				low := strings.ToLower(c)
				if strings.HasPrefix(low, "seit") || strings.HasPrefix(low, "bis") ||
					strings.HasPrefix(low, "von") || yearRE.MatchString(c) {
					since = c
					break
				}
			}
			code := ""
			if party != "" {
				if codes := scanFractions(party); len(codes) > 0 {
					code = codes[0]
				}
			}
			out = append(out, Member{pid, pyStrip(collapse(text(a, " "))), party, code, role, since})
		})
		return false
	})
	return out, nil
}

var germanDateRE = pyre(`(\d{1,2})\.\s*([A-Za-zäöüÄÖÜ]+)\s*(\d{4})`)

// parseGermanDate finds "19. Mai 2026" and returns "2026-05-19", or "".
func parseGermanDate(s string) string {
	m := germanDateRE.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	month, ok := germanMonths[strings.ToLower(m[2])]
	if !ok {
		return ""
	}
	day, _ := strconv.Atoi(m[1])
	year, _ := strconv.Atoi(m[3])
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || year < 1 {
		return ""
	}
	return t.Format(time.DateOnly)
}

func intp(n int) *int { return &n }

func publicFlag(s string) *int {
	low := strings.ToLower(s)
	if strings.Contains(low, "nicht öffentlich") || strings.Contains(low, "nichtöffentlich") {
		return intp(0)
	}
	if strings.Contains(low, "öffentlich") {
		return intp(1)
	}
	return nil
}

// MeetingPublic reads a meeting title: "(öffentlich/nicht öffentlich)" is
// mixed and gives nil.
func MeetingPublic(title string) *int {
	low := strings.ToLower(title)
	hasNon := strings.Contains(low, "nicht öffentlich") || strings.Contains(low, "nichtöffentlich")
	// "öffentlich" is a substring of "nicht öffentlich", so drop that first
	rest := strings.ReplaceAll(strings.ReplaceAll(low, "nicht öffentlich", ""), "nichtöffentlich", "")
	hasPub := strings.Contains(rest, "öffentlich")
	switch {
	case hasPub && hasNon:
		return nil
	case hasNon:
		return intp(0)
	case hasPub:
		return intp(1)
	}
	return nil
}

type AgendaItem struct {
	Anchor string  `json:"anchor"`
	Number *string `json:"number"`
	Title  string  `json:"title"`
	Public *int    `json:"public"`
}

type Meeting struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	BodyName string       `json:"body_name"`
	Date     *string      `json:"date"`
	Time     *string      `json:"time"`
	Location *string      `json:"location"`
	Public   *int         `json:"public"`
	URL      string       `json:"url"`
	Agenda   []AgendaItem `json:"agenda"`
	Files    []FileMeta   `json:"files"`
}

var (
	titlePrefixRE = pyre(`^Karlsruhe:\s*Ratsinformation\s*-\s*`)
	titleParenRE  = pyre(`\s*\(.*?\)\s*$`)
	meetTimeRE    = pyre(`(\d{1,2}[.:]\d{2})\s*(?:bis|-|–|—)\s*(\d{1,2}(?:[.:]\d{2})?)\s*Uhr`)
	pdfIDRE       = regexp.MustCompile(`(?i)/(\d+)\.pdf`)
	trailingHash  = pyre(`\s*#\s*$`)
	topNumberRE   = pyre(`^\s*([\d.]+)\s*[-–]\s*(.*)`)
)

func strp(s string) *string { return &s }

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ParseMeeting(page, pageURL, meetingID string) (*Meeting, error) {
	doc, err := parseDoc(page)
	if err != nil {
		return nil, err
	}
	title := ""
	if t := doc.Find("title").First(); t.Length() > 0 {
		title = titlePrefixRE.ReplaceAllString(text(t, ""), "")
	}
	bodyName := pyStrip(titleParenRE.ReplaceAllString(title, ""))
	if bodyName == "" {
		if h := doc.Find("h1, h2").First(); h.Length() > 0 {
			bodyName = text(h, "")
		}
	}

	// metadata rows such as "Datum: 19. Mai 2026, 15.30 bis 20 Uhr", "Ort: Bürgersaal"
	meta := map[string]string{}
	doc.Find("table.contenttable").First().Find("tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("th, td")
		if cells.Length() < 2 {
			return
		}
		label := strings.ToLower(strings.TrimRight(text(cells.Eq(0), " "), ":"))
		value := text(cells.Eq(1), " ")
		if label != "" && value != "" {
			meta[label] = value
		}
	})
	datum := meta["datum"]
	mdate := parseGermanDate(datum)
	if mdate == "" {
		mdate = parseGermanDate(text(doc.Selection, " "))
	}
	var mtime *string
	if tm := meetTimeRE.FindString(datum); tm != "" {
		mtime = strp(collapse(tm))
	}
	loc := meta["ort"]
	if loc == "" {
		loc = meta["raum"]
	}

	m := &Meeting{
		ID: meetingID, Title: title, BodyName: bodyName, Date: orNil(mdate), Time: mtime,
		Location: orNil(loc), Public: MeetingPublic(title), URL: pageURL,
		Agenda: []AgendaItem{}, Files: []FileMeta{},
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	fileIdx := map[string]int{}
	register := func(a *goquery.Selection, anchor *string) {
		href := a.AttrOr("href", "")
		if !strings.Contains(strings.ToLower(href), ".pdf") || !strings.Contains(href, "downloadfiles") {
			return
		}
		// scraped hrefs are fetched and rendered as public links, so only
		// http(s) URLs on a karlsruhe.de host are accepted
		ref, err := url.Parse(strings.TrimSpace(href))
		if err != nil {
			log.Printf("scraper: skipping off-site/odd file href %q on %s", href, pageURL)
			return
		}
		abs := base.ResolveReference(ref)
		host := strings.ToLower(abs.Hostname())
		if (abs.Scheme != "http" && abs.Scheme != "https") || !(host == "karlsruhe.de" || strings.HasSuffix(host, ".karlsruhe.de")) {
			log.Printf("scraper: skipping off-site/odd file href %q on %s", href, pageURL)
			return
		}
		absURL := abs.String()
		var fid string
		if fm := pdfIDRE.FindStringSubmatch(absURL); fm != nil {
			fid = fm[1]
		} else {
			sum := sha1.Sum([]byte(absURL))
			fid = hex.EncodeToString(sum[:])[:16]
		}
		i, seen := fileIdx[fid]
		if seen && anchor == nil {
			return
		}
		filename := path.Base(abs.EscapedPath())
		label := collapse(text(a, " "))
		if label == "" {
			label = filename
		}
		fm := FileMeta{ID: fid, URL: absURL, Label: label, AgendaAnchor: anchor, DocType: Classify(label), Filename: filename}
		if seen {
			m.Files[i] = fm
			return
		}
		fileIdx[fid] = len(m.Files)
		m.Files = append(m.Files, fm)
	}

	doc.Find("details.top").Each(func(_ int, det *goquery.Selection) {
		did := det.AttrOr("id", "")
		anchor := strings.TrimPrefix(did, "d")
		var raw string
		if strong := det.Find(".summary-title").First(); strong.Length() > 0 {
			strong.Find("a.anchor, .glyphicon").Remove()
			raw = text(strong, " ")
		}
		raw = pyStrip(trailingHash.ReplaceAllString(raw, ""))
		item := AgendaItem{Anchor: anchor, Title: raw}
		if nm := topNumberRE.FindStringSubmatch(raw); nm != nil {
			item.Number = strp(nm[1])
			item.Title = pyStrip(nm[2])
		}
		contents := det.Find(".detail-contents").First()
		ctext := ""
		scope := det
		if contents.Length() > 0 {
			ctext = text(contents, " ")
			scope = contents
		}
		item.Public = publicFlag(ctext)
		m.Agenda = append(m.Agenda, item)
		scope.Find(`a[href*="downloadfiles"]`).Each(func(_ int, a *goquery.Selection) {
			register(a, strp(anchor))
		})
	})
	// files outside any agenda block are meeting-level; register keeps the
	// anchors the agenda pass already set
	doc.Find(`a[href*="downloadfiles"]`).Each(func(_ int, a *goquery.Selection) {
		register(a, nil)
	})
	return m, nil
}
