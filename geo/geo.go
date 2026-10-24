// Package geo matches Karlsruhe street/place mentions in a document and
// classifies its Stadtteil. Both passes are text-only: a
// gazetteer regex finds street candidates and Jev judges each occurrence,
// then Jev picks the primary Stadtteil.
package geo

import (
	_ "embed"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

var Stadtteile = []string{
	"Beiertheim-Bulach", "Daxlanden", "Durlach", "Grötzingen", "Grünwettersbach", "Grünwinkel",
	"Hagsfeld", "Hohenwettersbach", "Innenstadt-Ost", "Innenstadt-West", "Knielingen", "Mühlburg",
	"Neureut", "Nordstadt", "Nordweststadt", "Oberreut", "Oststadt", "Palmbach", "Rintheim",
	"Rüppurr", "Stupferich", "Südstadt", "Südweststadt", "Waldstadt", "Weiherfeld-Dammerstock",
	"Weststadt", "Wolfartsweier",
}

//go:embed streets_ka.txt
var gazetteer string

var (
	streetFields = []string{"label", "agenda_title", "summary_de", "fulltext"}
	// short and relevant fields only, to keep the district prompt focused
	districtFields = []string{"label", "agenda_title", "summary_de"}
)

const (
	fulltextScan = 120_000
	keepConf     = 0.6
)

func ptr(s string) *string { return &s }

var geoCriteria = typesafe.Criteria{
	{Key: "location", Desc: ptr("refers to the Karlsruhe street, square, or place itself (the physical location)")},
	{Key: "person", Desc: ptr("refers to a person (e.g. the individual a street is named after), not the place")},
	{Key: "address", Desc: ptr("part of a postal address, sender/recipient block, letterhead, or contact line")},
	{Key: "incidental", Desc: ptr("an ordinary word or phrase that coincides with the name but is not the place")},
}

func loadStreets() []string {
	var out []string
	for _, ln := range strings.Split(gazetteer, "\n") {
		if strings.TrimSpace(ln) != "" && !strings.HasPrefix(ln, "#") {
			out = append(out, strings.TrimSpace(ln))
		}
	}
	return out
}

// buildRegex orders names longest-first so "Nordweststadt" wins over
// "Weststadt". RE2 has no lookaround, so the trailing (?!\w) of geo.py is a
// consumed non-word rune or end of text, and the leading (?<!\w) is checked
// in streetMatches.
func buildRegex(names []string) *regexp.Regexp {
	seen := map[string]bool{}
	uniq := []string{}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool {
		return utf8.RuneCountInString(uniq[i]) > utf8.RuneCountInString(uniq[j])
	})
	for i, n := range uniq {
		uniq[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)(` + strings.Join(uniq, "|") + `)(?:[^\p{L}\p{N}_]|$)`)
}

var streetRE = buildRegex(loadStreets())

// isWord is Python's Unicode \w.
func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

// streetMatches returns the byte spans of street names in text, as
// _STREET_RX.finditer does.
func streetMatches(text string) [][2]int {
	var out [][2]int
	pos := 0
	for pos < len(text) {
		m := streetRE.FindStringSubmatchIndex(text[pos:])
		if m == nil {
			break
		}
		s, e := pos+m[2], pos+m[3]
		if r, _ := utf8.DecodeLastRuneInString(text[:s]); s > 0 && isWord(r) {
			_, size := utf8.DecodeRuneInString(text[s:])
			pos = s + size
			continue
		}
		out = append(out, [2]int{s, e})
		pos = e
	}
	return out
}

// isPySpace is Python's str.isspace, which also counts U+001C..U+001F.
func isPySpace(r rune) bool {
	return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f
}

type Hit struct {
	Name, Field, Window string
}

// StreetHits returns the regex candidates with a marked context window,
// deduped per (name, field). Offsets count runes, as Python indexes str.
func StreetHits(doc store.Row) []Hit {
	var hits []Hit
	seen := map[[2]string]bool{}
	for _, field := range streetFields {
		text, _ := doc[field].(string)
		runes := []rune(text)
		if field == "fulltext" && len(runes) > fulltextScan {
			runes = runes[:fulltextScan]
			text = string(runes)
		}
		for _, m := range streetMatches(text) {
			name := text[m[0]:m[1]]
			key := [2]string{strings.ToLower(name), field}
			if seen[key] {
				continue
			}
			seen[key] = true
			s := utf8.RuneCountInString(text[:m[0]])
			e := s + utf8.RuneCountInString(name)
			off := max(0, s-150)
			w := string(runes[off:s]) + "«" + name + "»" + string(runes[e:min(len(runes), e+150)])
			hits = append(hits, Hit{name, field, strings.Join(strings.FieldsFunc(w, isPySpace), " ")})
		}
	}
	return hits
}

// ConfirmStreets returns the distinct street names Jev confirms as real place
// references in doc.
func ConfirmStreets(doc store.Row) ([]string, error) {
	// one request per hit, so one occurrence's text cannot sway another's judgment
	kept := []string{}
	for _, h := range StreetHits(doc) {
		q := typesafe.Choice("The passage contains «"+h.Name+"» marked with guillemets. Judge only that "+
			"occurrence: what does it refer to?", geoCriteria)
		ans, err := typesafe.SystemOne(h.Window, map[string]typesafe.Question{"ref": q}, 5)
		if err != nil {
			return nil, err
		}
		a, ok := ans["ref"]
		if !ok {
			return nil, errMissing("ref")
		}
		if a.Choice == "location" && a.Confidence >= keepConf && !slices.Contains(kept, h.Name) {
			kept = append(kept, h.Name)
		}
	}
	sort.Strings(kept)
	return kept, nil
}

type errMissing string

func (e errMissing) Error() string { return "typesafe: no answer for " + string(e) }

// ClassifyDistrict returns the primary Stadtteil Jev picks, or a nil district
// with the confidence for city-wide / no district; both nil without text.
func ClassifyDistrict(doc store.Row) (*string, *float64, error) {
	parts := make([]string, len(districtFields))
	for i, f := range districtFields {
		parts[i], _ = doc[f].(string)
	}
	state := []rune(strings.TrimFunc(strings.Join(parts, " "), isPySpace))
	if len(state) > 6000 {
		state = state[:6000]
	}
	if len(state) == 0 {
		return nil, nil, nil
	}
	crit := make(typesafe.Criteria, 0, len(Stadtteile)+1)
	for _, t := range Stadtteile {
		crit = append(crit, typesafe.Criterion{Key: t})
	}
	crit = append(crit, typesafe.Criterion{Key: "keine",
		Desc: ptr("city-wide, or no specific Karlsruhe Stadtteil is the subject")})
	ans, err := typesafe.SystemOne(string(state), map[string]typesafe.Question{"d": typesafe.Choice(
		"Which Karlsruhe Stadtteil (city district) is this document primarily about? "+
			"Choose 'keine' if it is city-wide or names no district.", crit)}, 5)
	if err != nil {
		return nil, nil, err
	}
	a, ok := ans["d"]
	if !ok {
		return nil, nil, errMissing("d")
	}
	if a.Choice == "keine" {
		return nil, &a.Confidence, nil
	}
	return &a.Choice, &a.Confidence, nil
}
