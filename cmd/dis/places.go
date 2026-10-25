package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/1612elphi/desinformationssystem/geo"
	"github.com/1612elphi/desinformationssystem/internal/store"
)

type placeBatch struct {
	Docs []struct {
		ID     string `json:"id"`
		Places []struct {
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			N     int    `json:"n"`
			Quote string `json:"quote"`
		} `json:"places"`
	} `json:"docs"`
}

// invitations name the meeting room ("Bürgersaal, Rathaus am Marktplatz");
// that line is the venue of the meeting, not a subject of the document
var venueRE = regexp.MustCompile(`(?i)^[\s,.:;()-]*((bürger|sitzungs|rats)saal)?[\s,.:;()-]*$`)

func isVenueLine(name, quote string) bool {
	return quote != "" && venueRE.MatchString(strings.ReplaceAll(quote, name, ""))
}

func placesImport(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("places-import", flag.ExitOnError)
	dir := fs.String("dir", "", "directory of extraction batch files (*.json)")
	dupes := fs.String("dupes", "", "JSON map of file id to ids of files with identical text")
	source := fs.String("source", "haiku", "file_places.source of these rows")
	show := fs.Bool("show", false, "print every resolution")
	top := fs.Int("top", 40, "unmatched names to list")
	fs.Parse(args)
	paths, _ := filepath.Glob(filepath.Join(*dir, "*.json"))
	if *dir == "" || len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "no batch files in %q\n", *dir)
		return 2
	}
	same := map[string][]string{}
	if *dupes != "" {
		b, err := os.ReadFile(*dupes)
		if err == nil {
			err = json.Unmarshal(b, &same)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "dupes:", err)
			return 2
		}
	}

	gaz := geo.Karlsruhe()
	var links []store.PlaceLink
	unmatched := map[string]int{}
	var mentions, matched, venues, docs int
	for _, p := range paths {
		var b placeBatch
		raw, err := os.ReadFile(p)
		if err == nil {
			err = json.Unmarshal(raw, &b)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", p, err)
			return 1
		}
		for _, d := range b.Docs {
			docs++
			for _, pl := range d.Places {
				mentions++
				if isVenueLine(pl.Name, pl.Quote) {
					venues++
					continue
				}
				m, ok := gaz.Resolve(pl.Name, pl.Kind)
				if *show {
					fmt.Printf("%-6v %-8s %-40s -> %s (%s) %s\n", ok, pl.Kind, pl.Name, m.Name, m.Kind, m.How)
				}
				if !ok {
					unmatched[pl.Kind+"\t"+pl.Name]++
					continue
				}
				matched++
				for _, id := range append([]string{d.ID}, same[d.ID]...) {
					links = append(links, store.PlaceLink{
						FileID: id, Name: m.Name, Kind: m.Kind, Lat: m.Lat, Lon: m.Lon,
						OSM: m.OSM, N: pl.N, Quote: pl.Quote,
					})
				}
			}
		}
	}
	skipped, err := st.ReplacePlaces(*source, links)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("docs %d, mentions %d, matched %d, unmatched %d, venue lines dropped %d, links %d (unknown files %d)\n",
		docs, mentions, matched, mentions-matched-venues, venues, len(links)-skipped, skipped)
	names := make([]string, 0, len(unmatched))
	for k := range unmatched {
		names = append(names, k)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(unmatched[b]-unmatched[a], strings.Compare(a, b))
	})
	fmt.Printf("unmatched names: %d distinct\n", len(names))
	for _, k := range names[:min(*top, len(names))] {
		fmt.Printf("%4d  %s\n", unmatched[k], k)
	}
	return 0
}
