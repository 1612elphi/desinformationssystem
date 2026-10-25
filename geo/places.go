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
	// key of each name and alt name
	exact map[string][]int
	// loose key of each name and alt name
	loose map[string][]int
	// loose key to its words, space separated
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

// key folds case, umlauts and ß, expands Str./Pl. and drops everything that
// is not a letter or digit, so "Karl-Friedrich-Str." and "Karl Friedrich
// Straße" share a key.
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

// generic words that council texts and OSM put around the same name
var (
	prefixes = []string{
		"stadtteil", "stadtbezirk", "ortsteil", "ortschaft", "karlsruhe", "ka",
		"bad", "bahnhof", "haltestelle", "station", "stadtbahnhaltestelle",
		"staedtisches", "staedtische", "alt", "im",
	}
	suffixes = []string{"karlsruhe", "ka", "bad", "haltestelle", "bahnhof"}
)

// stripHouseNo drops a trailing house number ("Kreuzstraße 13a"), but keeps
// road numbers ("A 5", "L 560").
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

// looseWords are the keys of the words of s (hyphens split words), without
// a house number and without generic leading and trailing words.
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

// kinds that the extractor and OSM assign to the same feature
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

// one feature often carries several kinds in OSM (Zündhütle is a building,
// a street and a stop); this order picks the most specific one
var kindOrder = []string{"district", "square", "park", "area", "water", "building", "school", "street", "transit"}

// names the council uses for features OSM names differently
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
	// a feature named with a Stadtteil qualifier lies within this distance
	// of the Stadtteil's centre
	qualifierKm = 3.0
	// shortest loose key that prefix and suffix matching apply to
	minAffix = 8
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
	// an address names a building on a street; place it on the street
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
	// a stripped generic word may leave a different feature's name, so the
	// kind must agree
	if i, ok := g.unique(g.loose[lk], kind); ok {
		return m(i, "loose")
	}
	words := nameWords(name)
	if d, rest, ok := g.qualifier(words); ok {
		// "Festhalle Durlach": the rest of the name, near that Stadtteil
		if i, ok := g.near(rest, kind, d); ok {
			return m(i, "district")
		}
		return Match{}, false
	}
	if i, ok := g.affix(lw, kind); ok {
		return m(i, "prefix")
	}
	// "Wertstoffstation Schäferstraße": a facility named after its street
	for n := 1; n < len(words); n++ {
		for _, i := range g.loose[loose(strings.Join(words[n:], " "))] {
			if k := g.places[i].Kind; k == "street" || k == "square" {
				return m(i, "street")
			}
		}
	}
	// a typo keeps the kind; "Turmbergbahn" is one letter from Turmbergbad
	if i, ok := g.unique(g.nearest(lk), kind); ok && g.places[i].Kind == kind {
		return m(i, "edit")
	}
	return Match{}, false
}

// qualifier finds a Stadtteil, or its adjective ("Hagsfelder"), as the first
// or last word of a name of two or more words and returns its place index
// and the remaining words.
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

// near resolves rest among the entries within qualifierKm of place d: an
// exact or loose name first (nearest wins), then the one entry of the best
// kind whose name contains both rest and the Stadtteil ("Friedhof Aue" is
// Friedhof Durlach-Aue).
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

// affix matches on whole words: the one entry whose name starts with the
// query ("Citypark" is Citypark Stadtpark), else the longest entry name the
// query starts with ("Kaiserstraße-West" is Kaiserstraße), else the one
// entry whose name ends with a query of two or more words
// ("Schickard-Straße" is Wilhelm-Schickard-Straße).
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
		// a one-word building name ("Verwaltungsgebäude") is a category,
		// not a name another name can start with
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

// unique returns the only candidate of the first kind tier that has any.
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

// pick chooses among candidates sharing a name: the same kind first, then a
// related kind, then the first kind in kindOrder.
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

// km is the equirectangular distance, accurate to well under 1% at city scale.
func km(a, b Place) float64 {
	const r = 6371.0
	x := (b.Lon - a.Lon) * math.Pi / 180 * math.Cos((a.Lat+b.Lat)/2*math.Pi/180)
	y := (b.Lat - a.Lat) * math.Pi / 180
	return r * math.Hypot(x, y)
}

// nearest returns the entries of the one loose key within edit distance 1
// (2 from 12 letters) of k, or nil when none or several keys are that close.
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

// editDistance is the Levenshtein distance, or limit when it reaches limit.
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
