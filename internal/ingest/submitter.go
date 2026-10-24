package ingest

import "regexp"

// Karlsruhe Gemeinderat fractions and groups; order is display order.
var fractions = []struct {
	code string
	res  []*regexp.Regexp
}{
	{"CDU", res(`\bCDU\b`)},
	{"B90", res(`B[üu]ndnis\s*90`, `\bGR[ÜU]NE[NR]?\b`, `\bGruene[nr]?\b`, `Die\s+Gr[üu]nen`)},
	{"SPD", res(`\bSPD\b`)},
	{"LIN", res(`\bDIE\s+LINKE\b`, `\bLINKE\b`, `\bLinke\b`)},
	{"FDP", res(`\bFDP\b`)},
	{"AFD", res(`\bAfD\b`, `\bAFD\b`)},
	{"FWV", res(`\bFWV\b`, `\bFW\b`, `Freie\s+W[äa]hler`)},
	{"KAL", res(`\bKAL\b`, `Karlsruher\s+Liste`)},
	{"VOL", res(`\bVolt\b`)},
	{"PAR", res(`Die\s+PARTEI`, `\bPARTEI\b`)},
	{"HEI", res(`Die\s+Heimat`)},
}

func res(pats ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		out[i] = pyre(p)
	}
	return out
}

var adminTypes = map[string]bool{
	"Beschlussvorlage": true, "Vorlage": true, "Niederschrift": true, "Protokoll": true,
	"Wortprotokoll": true, "Tagesordnung": true, "Einladung": true, "Abstimmungsergebnis": true,
	"Geschäftsordnung": true, "Präsentation": true, "Plan": true, "Karte": true, "Beschluss": true,
}

func scanFractions(text string) []string {
	found := []string{}
	for _, f := range fractions {
		for _, re := range f.res {
			if re.MatchString(text) {
				found = append(found, f.code)
				break
			}
		}
	}
	return found
}

// DeriveSubmitters is submitter.derive: SVK for administration documents,
// the named fraction(s) for Anträge and Anfragen. A UI heuristic only.
func DeriveSubmitters(label, fulltext, docType string) []string {
	switch {
	case docType == "Antrag" || docType == "Anfrage":
		if codes := scanFractions(label); len(codes) > 0 {
			return codes
		}
		return scanFractions(runeHead(fulltext, 800))
	case adminTypes[docType]:
		return []string{"SVK"}
	}
	return scanFractions(label)
}
