// Command dis runs one role of the Desinformationssystem archive.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/1612elphi/desinformationssystem/internal/ingest"
	"github.com/1612elphi/desinformationssystem/internal/mcp"
	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	role := env("ROLE", "web")
	if len(os.Args) > 1 {
		role = os.Args[1]
	}
	st, err := store.Open(env("DB_PATH", filepath.Join("data", "desinformationssystem.db")))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	if err := st.Init(); err != nil {
		log.Fatal(err)
	}
	switch role {
	case "web", "serve":
		srv := &web.Server{
			Store:  st,
			PDFDir: env("PDF_DIR", filepath.Join("data", "pdfs")),
			Dist:   env("WEB_DIST", filepath.Join("ui", "dist")),
		}
		// INGEST=oparl keeps Refresh nil (501) until oparl.py is ported
		if env("INGEST", "html") == "html" {
			srv.Refresh = func(id string) (any, error) { return ingest.ScrapeOne(st, id, false, false) }
		}
		addr := ":" + env("WEB_PORT", "3650")
		log.Printf("web: listening on %s", addr)
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcp.Handler(st))
		mux.Handle("/", srv.Handler())
		log.Fatal(http.ListenAndServe(addr, mux))
	case "mcp":
		addr := ":" + env("MCP_PORT", "3651")
		mux := http.NewServeMux()
		mux.Handle("/mcp", mcp.Handler(st))
		log.Printf("mcp: listening on %s", addr)
		log.Fatal(http.ListenAndServe(addr, mux))
	case "geo-backfill":
		code := geoBackfill(st, os.Args[min(2, len(os.Args)):])
		st.Close()
		os.Exit(code)
	case "data-quality":
		code := dataQuality(st)
		st.Close()
		os.Exit(code)
	case "scrape":
		os.Exit(scrapeCLI(st, os.Args[min(2, len(os.Args)):]))
	case "scraper":
		scraperLoop(st)
	case "ticker":
		ingest.RunTicker(st)
	case "init":
		fmt.Println("initialised")
	default:
		log.Fatalf("unknown role %q (want web|mcp|scrape|scraper|ticker|geo-backfill|data-quality|init)", role)
	}
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}

func scrapeCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	months := fs.Int("months", envInt("BACKFILL_MONTHS", 12), "how many months back")
	backfillAll := fs.Bool("backfill-all", false, "crawl the entire archive")
	noLLM := fs.Bool("no-llm", false, "skip LLM enrichment this run")
	submitters := fs.Bool("submitters", false, "recompute submitter codes for all existing files and exit")
	votes := fs.Bool("votes", false, "parse vote tallies from Abstimmungsergebnis PDFs and exit")
	vorlagen := fs.Bool("vorlagen", false, "recompute Vorlagennummer index for all existing files and exit")
	fs.Parse(args)
	var n int
	var err error
	switch {
	case *submitters:
		if n, err = ingest.BackfillSubmitters(st); err == nil {
			fmt.Printf("{'backfilled': %d}\n", n)
		}
	case *vorlagen:
		if n, err = ingest.BackfillVorlagen(st); err == nil {
			fmt.Printf("{'vorlagen_files': %d}\n", n)
		}
	case *votes:
		if n, err = ingest.ParsePendingVotes(st, 1000); err == nil {
			fmt.Printf("{'parsed_votes': %d}\n", n)
		}
	default:
		var counts *ingest.Counts
		if counts, err = ingest.Scrape(st, *months, *backfillAll, ingest.EnableLLM && !*noLLM); err == nil {
			fmt.Println(counts)
		}
	}
	if err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

// scraperLoop is entrypoint.sh's scraper role: one run at start, then daily
// at SCRAPE_HOUR:SCRAPE_MINUTE local time.
func scraperLoop(st *store.Store) {
	if mode := env("INGEST", "html"); mode != "html" {
		log.Fatalf("[scraper] unknown INGEST: %s (the Go binary has html only)", mode)
	}
	hour, minute := envInt("SCRAPE_HOUR", 3), envInt("SCRAPE_MINUTE", 30)
	months := envInt("BACKFILL_MONTHS", 12)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Print("[scraper] SIGTERM — stopping")
		os.Exit(0)
	}()
	run := func() {
		// a failed run must not end the loop, as with the Python child process
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[scraper] run failed; continuing (%v)", r)
			}
		}()
		if _, err := ingest.Scrape(st, months, false, ingest.EnableLLM); err != nil {
			log.Printf("[scraper] run failed; continuing (%v)", err)
		}
	}
	log.Printf("[scraper] initial run (ingest=html, months=%d)", months)
	run()
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}
		wait := next.Sub(now)
		log.Printf("[scraper] sleeping %ds until next run at %d:%d", int(wait.Seconds()), hour, minute)
		time.Sleep(wait)
		log.Printf("[scraper] scheduled run %s", time.Now().Format(time.RFC3339))
		run()
	}
}
