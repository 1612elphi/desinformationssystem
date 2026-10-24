package ingest

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// Python's \s on str patterns, which also matches NBSP and \x1c-\x1f.
const pySpace = `\pZ\t\n\v\f\r\x1c-\x1f\x85`

// pyre compiles a Python regex, widening \s to Python's Unicode whitespace.
func pyre(p string) *regexp.Regexp {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '\\' && i+1 < len(p) {
			if p[i+1] == 's' {
				if inClass {
					b.WriteString(pySpace)
				} else {
					b.WriteString("[" + pySpace + "]")
				}
			} else {
				b.WriteByte(c)
				b.WriteByte(p[i+1])
			}
			i++
			continue
		}
		switch {
		case c == '[' && !inClass:
			inClass = true
		case c == ']' && inClass:
			inClass = false
		}
		b.WriteByte(c)
	}
	return regexp.MustCompile(b.String())
}

func isPySpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

func pyStrip(s string) string { return strings.TrimFunc(s, isPySpace) }

var spaceRunRE = pyre(`\s+`)

func collapse(s string) string { return spaceRunRE.ReplaceAllString(s, " ") }

// runeHead is Python's s[:n].
func runeHead(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// bs4 leaves these strings out of get_text unless the tag itself is one of them.
var stringContainers = map[string]bool{"script": true, "style": true, "template": true, "rt": true, "rp": true}

// text is BeautifulSoup's get_text(sep, strip=True) over the selection.
func text(sel *goquery.Selection, sep string) string {
	var parts []string
	var walk func(n *html.Node, root bool)
	walk = func(n *html.Node, root bool) {
		switch n.Type {
		case html.TextNode:
			if s := pyStrip(n.Data); s != "" {
				parts = append(parts, s)
			}
			return
		case html.ElementNode:
			if !root && stringContainers[n.Data] {
				return
			}
		case html.DocumentNode:
		default:
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, false)
		}
	}
	for _, n := range sel.Nodes {
		walk(n, true)
	}
	return strings.Join(parts, sep)
}
