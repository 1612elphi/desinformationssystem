package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"sync"

	"github.com/1612elphi/desinformationssystem/geo"
	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

type geoResult struct {
	id       string
	streets  []string
	district *string
	conf     *float64
	status   string
}

// tagGeo runs both geo passes for one doc; a failure marks the doc for retry
// and never aborts the batch.
func tagGeo(st *store.Store, doc store.Row) geoResult {
	id, _ := doc["id"].(string)
	r, err := func() (geoResult, error) {
		d := maps.Clone(doc)
		ft, err := st.FileFulltext(id)
		if err != nil {
			return geoResult{}, err
		}
		d["fulltext"] = ft
		streets, err := geo.ConfirmStreets(d)
		if err != nil {
			return geoResult{}, err
		}
		district, conf, err := geo.ClassifyDistrict(d)
		if err != nil {
			return geoResult{}, err
		}
		return geoResult{id, streets, district, conf, "ok"}, nil
	}()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  error %s: %v\n", id, err)
		return geoResult{id: id, streets: []string{}, status: "error"}
	}
	return r
}

func geoBackfill(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("geo-backfill", flag.ExitOnError)
	limit := fs.Int("limit", 0, "max docs this run (0 = all pending)")
	batch := fs.Int("batch", 200, "")
	workers := fs.Int("workers", 8, "")
	fs.Parse(args)

	if !typesafe.Available() {
		fmt.Fprintln(os.Stderr, "TYPESAFE_API_KEY not set")
		return 2
	}
	done := 0
	for {
		take := *batch
		if *limit != 0 {
			take = min(*batch, *limit-done)
		}
		if take <= 0 {
			break
		}
		docs, err := st.DocsNeedingGeo(take)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if len(docs) == 0 {
			break
		}
		results := make([]geoResult, len(docs))
		sem := make(chan struct{}, max(1, *workers))
		var wg sync.WaitGroup
		for i, d := range docs {
			wg.Go(func() {
				sem <- struct{}{}
				results[i] = tagGeo(st, d)
				<-sem
			})
		}
		wg.Wait()
		if err := writeGeo(st, results); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		done += len(results)
		tagged, streets := 0, 0
		for _, r := range results {
			if r.district != nil && *r.district != "" {
				tagged++
			}
			streets += len(r.streets)
		}
		fmt.Printf("processed %d: +%d with district, +%d street links (last batch %d)\n",
			done, tagged, streets, len(results))
		if len(docs) < take {
			break
		}
	}
	fmt.Printf("done: %d documents\n", done)
	return 0
}

func writeGeo(st *store.Store, results []geoResult) error {
	tx, err := st.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range results {
		if err := store.SetGeo(tx, r.id, r.streets, r.district, r.conf, r.status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func dataQuality(st *store.Store) int {
	tx, err := st.DB.Begin()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	n, err := store.RefreshVoteConsistency(tx, "")
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c, err := st.VoteConsistency()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	withRoll, mismatch := c["with_rollcall"].(int64), c["mismatch"].(int64)
	rc := max(withRoll, 1)
	fmt.Printf("vote roll-calls: %d  consistent: %d  mismatch: %d (%.0f%%)  [recomputed %d]\n",
		withRoll, c["consistent"], mismatch, float64(mismatch)/float64(rc)*100, n)

	orphans, err := st.OrphanFiles(500)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("orphan files (no meeting, no Vorlage): %d\n", len(orphans))
	for _, o := range orphans[:min(10, len(orphans))] {
		l, _ := o["label"].(string)
		label := []rune(l)
		fmt.Printf("  %v  %-40s  %v\n", o["id"], string(label[:min(40, len(label))]), o["url"])
	}
	return 0
}
