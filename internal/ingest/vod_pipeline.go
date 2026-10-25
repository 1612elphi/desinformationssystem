package ingest

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// matches vod-capture.sh floor
const pipelineMinSeconds = 600

var JobwatchURL = envStr("JOBWATCH_URL", "http://127.0.0.1:3740/api/run")

type PipelineCounts struct {
	Videos   int `json:"videos"`
	Votes    int `json:"votes"`
	Speakers int `json:"speakers"`
	Speeches int `json:"speeches"`
	Judged   int `json:"judged"`
	Failed   int `json:"failed"`
}

// vars for test stubs
var (
	pipelineVotes = func(st *store.Store, video, mid, date string) (int, error) {
		v, err := ProcessVODVotes(st, video, mid, date, true)
		return len(v), err
	}
	pipelineSpeakers = func(video, date string) ([]SpeakerSpan, error) { return SpeakerTimeline(video, date) }
	pipelineSpeeches = func(st *store.Store, video, mid string) (int, error) {
		s, err := ProcessCleanup(st, video, mid, "", true)
		return len(s), err
	}
	pipelineJudge = func(st *store.Store, mid string) (int, error) { return JudgeSpeeches(st, mid, false) }
)

func guarded(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

func hasRow(st *store.Store, q string, args ...any) (bool, error) {
	var one int
	err := st.DB.QueryRow(q, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// RunVODPipeline does everything after transcription for each council VOD
// linked to a meeting, skipping finished steps:
//
//	votes      unless the meeting has vod-source votes
//	speakers   unless <base>.speakers.json exists
//	speeches   unless the video has speeches
//	judgements incremental by text hash
//
// Videos under ten minutes and meetings before the first seat-map layout
// are skipped. A failing step is logged and the next video still runs.
func RunVODPipeline(st *store.Store, dir string, dryRun bool) (PipelineCounts, error) {
	var c PipelineCounts
	sm, err := LoadSeatmap()
	if err != nil {
		return c, err
	}
	firstLayout := sm.FirstLayout()
	paths, err := filepath.Glob(filepath.Join(globEscape(dir), "*.vod.json"))
	if err != nil {
		return c, err
	}
	slices.Sort(paths)
	var meetings []string
	for _, p := range paths {
		base := strings.TrimSuffix(p, ".vod.json")
		var rec vodRecord
		if err := readJSON(p, &rec); err != nil {
			return c, err
		}
		mid := deref(rec.MeetingID)
		if mid == "" || rec.VideoID == "" || rec.Media == "" || !exists(base+".transcript.json") {
			continue
		}
		if rec.Duration == nil || *rec.Duration < pipelineMinSeconds {
			continue
		}
		var date string
		if err := st.DB.QueryRow("SELECT date FROM meetings WHERE id = ?", mid).Scan(&date); err == sql.ErrNoRows {
			continue
		} else if err != nil {
			return c, err
		}
		if date < firstLayout {
			continue
		}
		video := filepath.Join(dir, rec.Media)
		c.Videos++
		var todo []string
		if ok, err := hasRow(st, "SELECT 1 FROM votes WHERE meeting_id=? AND source='vod'", mid); err != nil {
			return c, err
		} else if !ok {
			todo = append(todo, "votes")
		}
		if !exists(base + ".speakers.json") {
			todo = append(todo, "speakers")
		}
		if ok, err := hasRow(st, "SELECT 1 FROM speeches WHERE meeting_id=? AND video_id=?", mid, rec.VideoID); err != nil {
			return c, err
		} else if !ok {
			todo = append(todo, "speeches")
		}
		status := strings.Join(todo, ", ")
		if status == "" {
			status = "up to date"
		}
		log.Printf("vod-pipeline: %s -> %s: %s", filepath.Base(base), mid, status)
		if dryRun {
			continue
		}
		for _, step := range todo {
			err := guarded(func() error {
				switch step {
				case "votes":
					n, err := pipelineVotes(st, video, mid, date)
					c.Votes += n
					return err
				case "speakers":
					spans, err := pipelineSpeakers(video, date)
					if err != nil {
						return err
					}
					if err := WriteJSONAtomic(base+".speakers.json", spans); err != nil {
						return err
					}
					c.Speakers++
				case "speeches":
					n, err := pipelineSpeeches(st, video, mid)
					c.Speeches += n
					return err
				}
				return nil
			})
			if err != nil {
				c.Failed++
				log.Printf("vod-pipeline:   %s failed: %v", step, err)
				// later steps need this
				break
			}
		}
		if !slices.Contains(meetings, mid) {
			meetings = append(meetings, mid)
		}
	}
	if dryRun {
		return c, nil
	}
	slices.Sort(meetings)
	for _, mid := range meetings {
		err := guarded(func() error {
			n, err := pipelineJudge(st, mid)
			c.Judged += n
			return err
		})
		if err != nil {
			c.Failed++
			log.Printf("vod-pipeline:   judge %s failed: %v", mid, err)
		}
	}
	return c, nil
}

// ReportJobwatch posts the run to jobwatch; it is a display, so its outage
// only logs.
func ReportJobwatch(job string, c PipelineCounts, started time.Time) {
	status := "ok"
	if c.Failed > 0 {
		status = "error"
	}
	unix := func(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }
	body, _ := json.Marshal(map[string]any{"job": job, "status": status, "started": unix(started),
		"finished": unix(time.Now()), "counts": c})
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(JobwatchURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("jobwatch report failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("jobwatch report failed: HTTP %d", resp.StatusCode)
	}
}
