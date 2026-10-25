package geo

import (
	_ "embed"
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// places-ka.json is an export of OpenStreetMap data, © OpenStreetMap
// contributors, available under the Open Database License (ODbL 1.0,
// https://www.openstreetmap.org/copyright). Regenerate it with
// scripts/geo/fetch-places.mjs.
//
//go:embed places-ka.json
var placesJSON []byte

// Place is one gazetteer entry: a named feature inside Stadtkreis Karlsruhe.
type Place struct {
	Name string   `json:"name"`
	Kind string   `json:"kind"`
	Lat  float64  `json:"lat"`
	Lon  float64  `json:"lon"`
	OSM  string   `json:"osm"`
	Alt  []string `json:"alt,omitempty"`
}

// Gazetteer resolves extracted place names to Places.
type Gazetteer struct {
	places []Place
	exact  map[string][]int
	loose  map[string][]int
	spaced map[string]string
}

var (
	defaultGaz     *Gazetteer
	defaultGazOnce sync.Once
)

// Karlsruhe returns the embedded gazetteer.
func Karlsruhe() *Gazetteer {
	defaultGazOnce.Do(func() {
		var ps []Place
		if err := json.Unmarshal(placesJSON, &ps); err != nil {
			panic("geo: places-ka.json: " + err.Error())
		}
		defaultGaz = NewGazetteer(ps)
	})
	return defaultGaz
}

func NewGazetteer(ps []Place) *Gazetteer {
	g := &Gazetteer{places: ps, exact: map[string][]int{}, loose: map[string][]int{}, spaced: map[string]string{}}
	for i, p := range ps {
		for _, n := range append([]string{p.Name}, p.Alt...) {
			addIdx(g.exact, key(n), i)
			ws := looseWords(n)
			lk := strings.Join(ws, "")
			addIdx(g.loose, lk, i)
			g.spaced[lk] = strings.Join(ws, " ")
		}
	}
	return g
}

func addIdx(m map[string][]int, k string, i int) {
	if k != "" && !slices.Contains(m[k], i) {
		m[k] = append(m[k], i)
	}
}

var foldRepl = strings.NewReplacer(
	"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "è", "e", "á", "a", "à", "a",
)

var (
	strAbbrRE  = regexp.MustCompile(`(?i)str\.?(\s|-|$)`)
	platzAbbRE = regexp.MustCompile(`(?i)pl\.(\s|-|$)`)
	houseNoRE  = regexp.MustCompile(`\s+\d+\s*[a-zA-Z]?(\s*[-–/]\s*\d+\s*[a-zA-Z]?)?$`)
)

// fold case, umlauts, abbreviations
func key(s string) string {
	s = strAbbrRE.ReplaceAllString(s, "straße$1")
	s = platzAbbRE.ReplaceAllString(s, "platz$1")
	s = foldRepl.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	prefixes = []string{
		"stadtteil", "stadtbezirk", "ortsteil", "ortschaft", "karlsruhe", "ka",
		"bad", "bahnhof", "haltestelle", "station", "stadtbahnhaltestelle",
		"staedtisches", "staedtische", "alt", "im",
	}
	suffixes = []string{"karlsruhe", "ka", "bad", "haltestelle", "bahnhof"}
)

// keeps road numbers
func stripHouseNo(s string) string {
	s = strings.TrimSpace(s)
	if t := houseNoRE.ReplaceAllString(s, ""); len(key(t)) >= 4 {
		return t
	}
	return s
}

func nameWords(s string) []string {
	return strings.FieldsFunc(stripHouseNo(s), func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '(' || r == ')' || r == '/'
	})
}

func looseWords(s string) []string {
	var ws []string
	for _, w := range nameWords(strings.ReplaceAll(s, "-", " ")) {
		if k := key(w); k != "" {
			ws = append(ws, k)
		}
	}
	for len(ws) > 1 && slices.Contains(prefixes, ws[0]) {
		ws = ws[1:]
	}
	for len(ws) > 1 && slices.Contains(suffixes, ws[len(ws)-1]) {
		ws = ws[:len(ws)-1]
	}
	return ws
}

func loose(s string) string { return strings.Join(looseWords(s), "") }

var related = map[string][]string{
	"street":   {"square", "transit"},
	"square":   {"street", "transit", "area"},
	"park":     {"area", "water"},
	"area":     {"park", "district", "building", "street"},
	"district": {"area"},
	"building": {"school", "area", "park", "transit"},
	"school":   {"building"},
	"water":    {"area", "park"},
	"transit":  {"building", "street", "square"},
}

// osm kinds, most specific
var kindOrder = []string{"district", "square", "park", "area", "water", "building", "school", "street", "transit"}

var aliases = map[string]string{
	"zoo":                   "Zoologischer Stadtgarten",
	"staatstheater":         "Badisches Staatstheater",
	"westbahnhof":           "Karlsruhe West",
	"hauptbahnhof":          "Karlsruhe Hauptbahnhof",
	"hbf":                   "Karlsruhe Hauptbahnhof",
	"turmbergbahn":          "Durlach Turmberg",
	"bbbankwildparkstadion": "BBBank Wildpark",
	"hauptbahnhofsued":      "Hauptbahnhof Südseite",
	"deponieost":            "Mülldeponie Ost",
	"hallenbadneureut":      "Adolf-Ehrmann-Bad Neureut",
}

const (
	qualifierKm = 3.0
	minAffix    = 8
)

// Match is the result of resolving one extracted name.
type Match struct {
	Place
	// How names the rule that matched: exact, alias, loose, district,
	// prefix, street or edit.
	How string
}

// Resolve maps an extracted {name, kind} to a gazetteer entry. ok is false
// when no rule finds exactly one candidate; nothing is guessed.
func (g *Gazetteer) Resolve(name, kind string) (Match, bool) {
	name = strings.TrimSpace(name)
	m := func(i int, how string) (Match, bool) { return Match{g.places[i], how}, true }
	if i, ok := g.pick(g.exact[key(name)], kind); ok {
		return m(i, "exact")
	}
	// address: place on street
	if s := stripHouseNo(name); s != name {
		if i, ok := g.pick(g.exact[key(s)], "street"); ok {
			return m(i, "exact")
		}
	}
	lw := looseWords(name)
	lk := strings.Join(lw, "")
	if a, ok := aliases[lk]; ok {
		if i, ok := g.pick(g.exact[key(a)], kind); ok {
			return m(i, "alias")
		}
	}
	// kind must agree here
	if i, ok := g.unique(g.loose[lk], kind); ok {
		return m(i, "loose")
	}
	words := nameWords(name)
	if d, rest, ok := g.qualifier(words); ok {
		if i, ok := g.near(rest, kind, d); ok {
			return m(i, "district")
		}
		return Match{}, false
	}
	if i, ok := g.affix(lw, kind); ok {
		return m(i, "prefix")
	}
	for n := 1; n < len(words); n++ {
		for _, i := range g.loose[loose(strings.Join(words[n:], " "))] {
			if k := g.places[i].Kind; k == "street" || k == "square" {
				return m(i, "street")
			}
		}
	}
	// typo match keeps kind
	if i, ok := g.unique(g.nearest(lk), kind); ok && g.places[i].Kind == kind {
		return m(i, "edit")
	}
	return Match{}, false
}

func (g *Gazetteer) qualifier(words []string) (int, []string, bool) {
	if len(words) < 2 {
		return 0, nil, false
	}
	for _, c := range []struct {
		w    string
		rest []string
	}{{words[len(words)-1], words[:len(words)-1]}, {words[0], words[1:]}} {
		k := key(c.w)
		for _, k := range []string{k, strings.TrimSuffix(k, "er")} {
			for _, i := range g.exact[k] {
				if g.places[i].Kind == "district" {
					return i, c.rest, true
				}
			}
		}
	}
	return 0, nil, false
}

func (g *Gazetteer) near(rest []string, kind string, d int) (int, bool) {
	lk := loose(strings.Join(rest, " "))
	if len(lk) < 4 {
		return 0, false
	}
	dist := func(i int) float64 { return km(g.places[i], g.places[d]) }
	var exact []int
	for _, i := range append(g.exact[key(strings.Join(rest, " "))], g.loose[lk]...) {
		if i != d && dist(i) <= qualifierKm {
			exact = append(exact, i)
		}
	}
	if len(exact) > 0 {
		slices.SortFunc(exact, func(a, b int) int { return cmpFloat(dist(a), dist(b)) })
		return g.pick(exact, kind)
	}
	if len(lk) < minAffix {
		return 0, false
	}
	dk := key(g.places[d].Name)
	var cands []int
	for k, is := range g.loose {
		if !strings.Contains(k, lk) || !strings.Contains(k, dk) {
			continue
		}
		for _, i := range is {
			if i != d && dist(i) <= qualifierKm && !slices.Contains(cands, i) {
				cands = append(cands, i)
			}
		}
	}
	return g.unique(cands, kind)
}

func (g *Gazetteer) affix(lw []string, kind string) (int, bool) {
	q := strings.Join(lw, " ")
	if len(q) < minAffix {
		return 0, false
	}
	var longer, ends []int
	best := ""
	for k, is := range g.loose {
		s := g.spaced[k]
		switch {
		case strings.HasPrefix(s, q+" "):
			longer = append(longer, is...)
		case len(lw) > 1 && strings.HasSuffix(s, " "+q):
			ends = append(ends, is...)
		case len(k) >= minAffix && len(s) > len(best) && strings.HasPrefix(q, s+" "):
			best = s
		}
	}
	if i, ok := g.unique(longer, kind); ok {
		return i, true
	}
	if best != "" {
		// one-word building: category
		var named []int
		for _, i := range g.loose[strings.ReplaceAll(best, " ", "")] {
			switch g.places[i].Kind {
			case "street", "square", "park", "area", "water", "district":
				named = append(named, i)
			default:
				if strings.Contains(best, " ") {
					named = append(named, i)
				}
			}
		}
		if i, ok := g.pick(named, kind); ok {
			return i, true
		}
	}
	return g.unique(ends, kind)
}

func (g *Gazetteer) unique(cands []int, kind string) (int, bool) {
	for _, want := range append([]string{kind}, related[kind]...) {
		var hit []int
		for _, i := range cands {
			if g.places[i].Kind == want {
				hit = append(hit, i)
			}
		}
		if len(hit) > 0 {
			return hit[0], len(hit) == 1
		}
	}
	return 0, false
}

func (g *Gazetteer) pick(cands []int, kind string) (int, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	for _, want := range append(append([]string{kind}, related[kind]...), kindOrder...) {
		for _, i := range cands {
			if g.places[i].Kind == want {
				return i, true
			}
		}
	}
	return cands[0], true
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// equirectangular, under 1% error
func km(a, b Place) float64 {
	const r = 6371.0
	x := (b.Lon - a.Lon) * math.Pi / 180 * math.Cos((a.Lat+b.Lat)/2*math.Pi/180)
	y := (b.Lat - a.Lat) * math.Pi / 180
	return r * math.Hypot(x, y)
}

// ponytail: linear scan over ~6k keys per unmatched name, an index when the
// gazetteer grows past one city.
func (g *Gazetteer) nearest(k string) []int {
	n := len([]rune(k))
	max := 0
	switch {
	case n >= 12:
		max = 2
	case n >= 6:
		max = 1
	default:
		return nil
	}
	var hit string
	best := max + 1
	for c := range g.loose {
		if d := len([]rune(c)) - n; d > max || -d > max {
			continue
		}
		d := editDistance(k, c, max+1)
		switch {
		case d > max:
		case d < best:
			best, hit = d, c
		case d == best:
			hit = ""
		}
	}
	if hit == "" {
		return nil
	}
	return g.loose[hit]
}

// levenshtein, capped at limit
func editDistance(a, b string, limit int) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		low := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			low = min(low, cur[j])
		}
		if low >= limit {
			return limit
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
