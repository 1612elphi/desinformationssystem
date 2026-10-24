// Command dis runs one role of the Desinformationssystem archive.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	_ "time/tzdata"

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
	case "init":
		fmt.Println("initialised")
	default:
		log.Fatalf("unknown role %q (want web|mcp|geo-backfill|data-quality|init)", role)
	}
}
