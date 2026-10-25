package main

import (
	"cmp"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/ingest"
	"github.com/1612elphi/desinformationssystem/internal/store"
	"github.com/1612elphi/desinformationssystem/internal/typesafe"
)

func vodDir() string { return env("VOD_DIR", filepath.Join(filepath.Dir(dbPath), "vod")) }

// argparse parity: any order
func parseVideoArgs(fs *flag.FlagSet, args []string) string {
	fs.Parse(args)
	video := fs.Arg(0)
	if video != "" && len(fs.Args()) > 1 {
		fs.Parse(fs.Args()[1:])
	}
	if video == "" {
		fmt.Fprintf(os.Stderr, "usage: dis %s <video> [flags]\n", fs.Name())
		fs.PrintDefaults()
		os.Exit(2)
	}
	return video
}

func hms(s float64) string {
	t := int(s)
	return fmt.Sprintf("%d:%02d:%02d", t/3600, t%3600/60, t%60)
}

func vodPipelineCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("vod-pipeline", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "list the steps each video needs, run none")
	fs.Parse(args)
	started := time.Now()
	c, err := ingest.RunVODPipeline(st, vodDir(), *dry)
	if err != nil {
		log.Print(err)
		return 1
	}
	fmt.Printf(`{"videos": %d, "votes": %d, "speakers": %d, "speeches": %d, "judged": %d, "failed": %d}`+"\n",
		c.Videos, c.Votes, c.Speakers, c.Speeches, c.Judged, c.Failed)
	if !*dry {
		ingest.ReportJobwatch("dis-vod-pipeline", c, started)
	}
	if c.Failed > 0 {
		return 1
	}
	return 0
}

func vodVotesCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("vod-votes", flag.ExitOnError)
	meeting := fs.String("meeting", "", "meeting id, termin-N")
	date := fs.String("date", "", "meeting date, YYYY-MM-DD")
	apply := fs.Bool("apply", false, "write rows to the votes table")
	video := parseVideoArgs(fs, args)
	votes, err := ingest.ProcessVODVotes(st, video, *meeting, *date, *apply)
	if err != nil {
		log.Print(err)
		return 1
	}
	for _, v := range votes {
		flag := ""
		if !v.Complete {
			flag = fmt.Sprintf("  partial roll-call, tiles %v", v.Tiles)
		}
		fmt.Printf("%s  %-8s %2d/%2d/%2d  %s%s\n", hms(v.At), cmp.Or(v.Anchor, "?"), v.Ja, v.Nein, v.Enthaltung, v.TopLabel, flag)
	}
	return 0
}

func vodSpeakersCLI(args []string) int {
	fs := flag.NewFlagSet("vod-speakers", flag.ExitOnError)
	date := fs.String("date", "", "meeting date, YYYY-MM-DD")
	video := parseVideoArgs(fs, args)
	spans, err := ingest.SpeakerTimeline(video, *date)
	if err != nil {
		log.Print(err)
		return 1
	}
	out := strings.TrimSuffix(video, filepath.Ext(video)) + ".speakers.json"
	if err := ingest.WriteJSONAtomic(out, spans); err != nil {
		log.Print(err)
		return 1
	}
	speaking, tops := 0, map[string]bool{}
	for _, s := range spans {
		if s.Seat != nil {
			speaking++
		}
		if s.Top != nil && *s.Top != "" {
			tops[*s.Top] = true
		}
	}
	log.Printf("%d spans, %d with a speaker, %d TOP labels -> %s", len(spans), speaking, len(tops), out)
	return 0
}

func vodCleanupCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("vod-cleanup", flag.ExitOnError)
	meeting := fs.String("meeting", "", "meeting id, termin-N")
	top := fs.String("top", "", "only this agenda anchor, e.g. top6")
	apply := fs.Bool("apply", false, "replace the video's rows in the speeches table")
	video := parseVideoArgs(fs, args)
	speeches, err := ingest.ProcessCleanup(st, video, *meeting, *top, *apply)
	if err != nil {
		log.Print(err)
		return 1
	}
	for _, s := range speeches {
		raw := len(strings.Fields(s.Text))
		flag := ""
		if s.Failed {
			flag = "  NOT CLEANED, raw kept"
		} else if s.Shortened {
			flag = "  SHORTENED, raw kept"
		}
		name := []rune(cmp.Or(s.Speaker, "?"))
		fmt.Printf("%s %-8s %-20s %5dw %.2f%s\n", hms(s.Start), cmp.Or(s.Anchor, "-"), string(name[:min(len(name), 20)]),
			raw, float64(len(strings.Fields(s.Clean)))/float64(max(1, raw)), flag)
	}
	return 0
}

func speechJudgeCLI(st *store.Store, args []string) int {
	fs := flag.NewFlagSet("speech-judge", flag.ExitOnError)
	meeting := fs.String("meeting", "", "only this meeting, termin-N")
	redo := fs.Bool("redo", false, "judge every speech again")
	fs.Parse(args)
	if os.Getenv("TYPESAFE_BASE_URL") == "" {
		typesafe.BaseURL = "http://127.0.0.1:3990/typesafe"
		typesafe.Endpoint = typesafe.BaseURL + "/v1/systemone"
	}
	n, err := ingest.JudgeSpeeches(st, *meeting, *redo)
	if err != nil {
		log.Print(err)
		return 1
	}
	log.Printf("judged %d speeches", n)
	return 0
}
