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
	"strings"
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

var dbPath = env("DB_PATH", filepath.Join("data", "desinformationssystem.db"))

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	role := env("ROLE", "web")
	if len(os.Args) > 1 {
		role = os.Args[1]
	}
	st, err := store.Open(dbPath)
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
			MapDir: env("MAP_DIR", filepath.Join("data", "map")),
		}
		if strings.ToLower(env("INGEST", "html")) == "oparl" {
			srv.Refresh = func(id string) (any, error) { return ingest.RefreshOne(st, id, false, false) }
		} else {
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
	case "places-import":
		code := placesImport(st, os.Args[min(2, len(os.Args)):])
		st.Close()
		os.Exit(code)
	case "data-quality":
		code := dataQuality(st)
		st.Close()
		os.Exit(code)
	case "scrape":
		os.Exit(scrapeCLI(st, os.Args[min(2, len(os.Args)):]))
	case "oparl":
		os.Exit(oparlCLI(st, os.Args[min(2, len(os.Args)):]))
	case "seed":
		os.Exit(seedCLI(st, os.Args[min(2, len(os.Args)):]))
	case "pdf-backfill":
		os.Exit(backfillCLI(st, os.Args[min(2, len(os.Args)):]))
	case "enrich-claude":
		os.Exit(enrichClaudeCLI(st, os.Args[min(2, len(os.Args)):]))
	case "vod":
		os.Exit(vodCLI(st, os.Args[min(2, len(os.Args)):]))
	case "vod-pipeline":
		os.Exit(vodPipelineCLI(st, os.Args[min(2, len(os.Args)):]))
	case "vod-votes":
		os.Exit(vodVotesCLI(st, os.Args[min(2, len(os.Args)):]))
	case "vod-speakers":
		os.Exit(vodSpeakersCLI(os.Args[min(2, len(os.Args)):]))
	case "vod-cleanup":
		os.Exit(vodCleanupCLI(st, os.Args[min(2, len(os.Args)):]))
	case "speech-judge":
		os.Exit(speechJudgeCLI(st, os.Args[min(2, len(os.Args)):]))
	case "backup":
		if len(os.Args) != 3 {
			log.Fatal("usage: dis backup <dest.db>")
		}
		// consistent live snapshot
		if _, err := st.DB.Exec("VACUUM INTO ?", os.Args[2]); err != nil {
			log.Fatal(err)
		}
	case "scraper":
		scraperLoop(st)
	case "ticker":
		ingest.RunTicker(st)
	case "init":
		fmt.Println("initialised")
	default:
		log.Fatalf("unknown role %q (want web|mcp|scrape|oparl|seed|pdf-backfill|enrich-claude|vod|vod-pipeline|vod-votes|vod-speakers|vod-cleanup|speech-judge|backup|scraper|ticker|geo-backfill|places-import|data-quality|init)", role)
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

func oparlCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("oparl", flag.ExitOnError)
	full := fs.Bool("full", false, "ignore the watermark: walk the complete meeting/paper lists")
	since := fs.String("since", "", "explicit modified_since ISO timestamp")
	noLLM := fs.Bool("no-llm", false, "skip LLM enrichment this run")
	skipMembers := fs.Bool("skip-members", false, "skip the HTML committee/roster crawl (validation runs)")
	fs.Parse(args)
	counts, err := ingest.OparlSync(st, *full, *since, ingest.EnableLLM && !*noLLM, *skipMembers)
	if err != nil {
		log.Print(err)
		return 1
	}
	fmt.Println(counts)
	return 0
}

func seedCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	dir := fs.String("dir", "", "dump root (the repo's docs/ dir)")
	limit := fs.Int("limit", 0, "only N of each kind")
	newest := fs.Bool("newest", false, "with --limit, take the NEWEST N (validation runs; the oldest entries are pre-digital stubs and unrepresentative)")
	skipMeetings := fs.Bool("skip-meetings", false, "")
	skipPapers := fs.Bool("skip-papers", false, "")
	noWatermark := fs.Bool("no-watermark", false, "don't set meta oparl_since from the dump manifest")
	fs.Parse(args)
	if fi, err := os.Stat(*dir); *dir == "" || err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "no such dump dir: %s\n", *dir)
		return 2
	}
	counts, err := ingest.Seed(st, *dir, *limit, *skipPapers, *skipMeetings, *newest)
	if err != nil {
		log.Print(err)
		return 1
	}
	if !*noWatermark && *limit == 0 {
		stamp, err := ingest.SeedWatermark(st, *dir)
		if err != nil {
			log.Print(err)
			return 1
		}
		counts.SetStr("watermark", stamp)
	}
	log.Printf("seed: done: %s", counts)
	fmt.Println(counts)
	return 0
}

func backfillCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("pdf-backfill", flag.ExitOnError)
	limit := fs.Int("limit", ingest.BackfillMaxFiles, "max files this run (0 = all pending)")
	maxSeconds := fs.Int("max-seconds", ingest.BackfillMaxSeconds, "stop after N seconds (0 = no limit)")
	count := fs.Bool("count", false, "just report how many are pending, then exit")
	fs.Parse(args)
	if *count {
		n, err := ingest.PendingPDFCount(st)
		if err != nil {
			log.Print(err)
			return 1
		}
		fmt.Printf("%d files pending a cached PDF\n", n)
		return 0
	}
	counts, err := ingest.BackfillPDFs(st, *limit, *maxSeconds)
	if err != nil {
		log.Print(err)
		return 1
	}
	log.Printf("backfill: done: %s", counts)
	fmt.Println(counts)
	return 0
}

func enrichClaudeCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("enrich-claude", flag.ExitOnError)
	batches := fs.Int("batches", ingest.ClaudeMaxBatches, "max invocations this run (0 = until drained)")
	maxSeconds := fs.Int("max-seconds", ingest.ClaudeMaxSeconds, "stop after N seconds (0 = no limit)")
	batchDocs := fs.Int("batch-docs", ingest.ClaudeBatchDocs, "documents per invocation")
	concurrency := fs.Int("concurrency", ingest.ClaudeConcurrency, "parallel invocations")
	count := fs.Bool("count", false, "report backlog and exit")
	fs.Parse(args)
	if *count {
		n, err := ingest.PendingEnrichmentCount(st)
		if err != nil {
			log.Print(err)
			return 1
		}
		fmt.Printf("%d documents awaiting enrichment\n", n)
		return 0
	}
	totals, err := ingest.EnrichClaude(st, *batches, *maxSeconds, *batchDocs, *concurrency)
	if err != nil {
		log.Print(err)
		return 2
	}
	log.Printf("enrich-claude: done: %+v", totals)
	return 0
}

func vodCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("vod", flag.ExitOnError)
	linkOnly := fs.Bool("link-only", false, "skip transcription, relink only")
	fs.Parse(args)
	if err := ingest.ProcessVODs(st, vodDir(), *linkOnly); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

// daily ingest loop
func scraperLoop(st *store.Store) {
	mode := env("INGEST", "html")
	if mode != "html" && mode != "oparl" {
		log.Fatalf("unknown INGEST: %s (want html|oparl)", mode)
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
		// keep looping on failure
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[scraper] run failed; continuing (%v)", r)
			}
		}()
		var err error
		if mode == "oparl" {
			_, err = ingest.OparlSync(st, false, "", ingest.EnableLLM, false)
		} else {
			_, err = ingest.Scrape(st, months, false, ingest.EnableLLM)
		}
		if err != nil {
			log.Printf("[scraper] run failed; continuing (%v)", err)
		}
	}
	log.Printf("[scraper] initial run (ingest=%s, months=%d)", mode, months)
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
