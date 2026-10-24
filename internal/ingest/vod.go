package ingest

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// outputs transcript.json, vod.json
var (
	WhisperBin      = envStr("WHISPER_BIN", "whisper-cli")
	WhisperModel    = envStr("WHISPER_MODEL", "data/whisper/ggml-large-v3-turbo.bin")
	WhisperVADModel = envStr("WHISPER_VAD_MODEL", "data/whisper/ggml-silero-v5.1.2.bin")
	WhisperThreads  = envInt("WHISPER_THREADS", 16)
	FFmpegBin       = envStr("FFMPEG_BIN", "ffmpeg")
)

type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

var (
	topRE  = regexp.MustCompile(`(?i)\b(?:Tagesordnungspunkt|TOP|Punkt)\s+(\d+(?:\.\d+)*)`)
	callRE = regexp.MustCompile(`(?i)(?:rufe\w*\s.{0,20}?auf|aufrufen|kommen wir zu|wir kommen zu|ich eröffne)`)
)

// TopOffsets is the earliest offset per agenda item, preferring the chair
// calling it over a bare mention. One segment can hold both ("Das war der
// Top 1 und ich rufe jetzt auf den Tagesordnungspunkt 2"), so the call
// phrase is looked for in the text before each number.
func TopOffsets(segments []Segment, anchors map[string]bool) map[string]float64 {
	type hit struct {
		score int
		at    float64
	}
	best := map[string]hit{}
	for _, s := range segments {
		for _, m := range topRE.FindAllStringSubmatchIndex(s.Text, -1) {
			a := "top" + s.Text[m[2]:m[3]]
			if !anchors[a] {
				continue
			}
			score := 0
			if callRE.MatchString(s.Text[:m[0]]) {
				score = 1
			}
			if h, ok := best[a]; !ok || score > h.score {
				best[a] = hit{score, s.Start}
			}
		}
	}
	out := map[string]float64{}
	for a, h := range best {
		out[a] = h.at
	}
	return out
}

func whisperArgs(wav, outBase string, gpu bool) []string {
	args := []string{"-m", WhisperModel, "-l", "de", "-t", strconv.Itoa(WhisperThreads),
		"-oj", "-of", outBase, "-np", "-f", wav}
	if WhisperVADModel != "" {
		args = append(args, "--vad", "-vm", WhisperVADModel)
	}
	if !gpu {
		args = append(args, "-ng")
	}
	return args
}

// Transcribe decodes media to 16 kHz mono WAV, which whisper-cli requires,
// and runs whisper-cli on it, retrying on the CPU if the GPU run fails.
func Transcribe(media string) ([]Segment, error) {
	tmp, err := os.MkdirTemp("", "dis-vod-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	wav := filepath.Join(tmp, "audio.wav")
	ff := exec.Command(FFmpegBin, "-nostdin", "-loglevel", "error", "-i", media, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	ff.Stderr = os.Stderr
	if err := ff.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	outBase := filepath.Join(tmp, "out")
	run := func(gpu bool) error {
		cmd := exec.Command(WhisperBin, whisperArgs(wav, outBase, gpu)...)
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	// keep backlog moving
	if err := run(true); err != nil {
		log.Printf("vod:   GPU run failed (%v), falling back to CPU", err)
		if err := run(false); err != nil {
			return nil, fmt.Errorf("whisper-cli: %w", err)
		}
	}
	b, err := os.ReadFile(outBase + ".json")
	if err != nil {
		return nil, err
	}
	var out struct {
		Transcription []struct {
			Offsets struct{ From, To int64 } `json:"offsets"`
			Text    string                   `json:"text"`
		} `json:"transcription"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	segs := []Segment{}
	for _, t := range out.Transcription {
		segs = append(segs, Segment{round1(float64(t.Offsets.From) / 1000), round1(float64(t.Offsets.To) / 1000), strings.TrimSpace(t.Text)})
	}
	return segs, nil
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// unique same-day gemeinderat meeting
func vodMeeting(st *store.Store, ts int64) (string, error) {
	if ts == 0 {
		return "", nil
	}
	day := time.Unix(ts, 0).In(store.Berlin).Format(time.DateOnly)
	ids, err := queryStrings(st, "SELECT id FROM meetings WHERE date = ? AND body_name LIKE 'Gemeinderat%'", day)
	if err != nil || len(ids) != 1 {
		return "", err
	}
	return ids[0], nil
}

func agendaAnchors(st *store.Store, meetingID string) (map[string]bool, error) {
	out := map[string]bool{}
	if meetingID == "" {
		return out, nil
	}
	anchors, err := queryStrings(st, "SELECT anchor FROM agenda_items WHERE meeting_id = ?", meetingID)
	for _, a := range anchors {
		out[a] = true
	}
	return out, err
}

// lists completed downloads only
func archivedIDs(dir string) (map[string]bool, error) {
	f, err := os.Open(filepath.Join(dir, "archive.txt"))
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) > 1 {
			out[fields[1]] = true
		}
	}
	return out, sc.Err()
}

type vodInfo struct {
	ID               string   `json:"id"`
	Title            *string  `json:"title"`
	Duration         *float64 `json:"duration"`
	ReleaseTimestamp *int64   `json:"release_timestamp"`
	Timestamp        *int64   `json:"timestamp"`
}

type vodRecord struct {
	VideoID   string             `json:"video_id"`
	Title     *string            `json:"title"`
	StartedAt *string            `json:"started_at"`
	Duration  *float64           `json:"duration"`
	Media     string             `json:"media"`
	MeetingID *string            `json:"meeting_id"`
	Tops      map[string]float64 `json:"tops"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", b, 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func processVOD(st *store.Store, infoPath string, info vodInfo, linkOnly bool) error {
	base := strings.TrimSuffix(infoPath, ".info.json")
	matches, err := filepath.Glob(globEscape(base) + ".*")
	if err != nil {
		return err
	}
	media := ""
	for _, p := range matches {
		if !strings.HasSuffix(p, ".json") {
			media = p
			break
		}
	}
	if media == "" {
		log.Printf("vod:   no media for %s", base)
		return nil
	}

	tpath := base + ".transcript.json"
	segments := []Segment{}
	if exists(tpath) {
		if err := readJSON(tpath, &segments); err != nil {
			return err
		}
	} else if !linkOnly {
		log.Printf("vod:   transcribing %s (%s)", filepath.Base(media), filepath.Base(WhisperModel))
		if segments, err = Transcribe(media); err != nil {
			return err
		}
		if err := writeJSONAtomic(tpath, segments); err != nil {
			return err
		}
	}

	var ts int64
	if info.ReleaseTimestamp != nil {
		ts = *info.ReleaseTimestamp
	} else if info.Timestamp != nil {
		ts = *info.Timestamp
	}
	meetingID, err := vodMeeting(st, ts)
	if err != nil {
		return err
	}
	anchors, err := agendaAnchors(st, meetingID)
	if err != nil {
		return err
	}
	rec := vodRecord{VideoID: info.ID, Title: info.Title, Duration: info.Duration,
		Media: filepath.Base(media), Tops: TopOffsets(segments, anchors)}
	if info.ReleaseTimestamp != nil {
		s := time.Unix(*info.ReleaseTimestamp, 0).In(store.Berlin).Format(time.RFC3339)
		rec.StartedAt = &s
	}
	if meetingID != "" {
		rec.MeetingID = &meetingID
	}
	if err := writeJSONAtomic(base+".vod.json", rec); err != nil {
		return err
	}
	log.Printf("vod:   %s -> %s, %d/%d TOPs located", info.ID, cmp.Or(meetingID, "no meeting"), len(rec.Tops), len(anchors))
	return nil
}

var globMeta = strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`)

func globEscape(s string) string { return globMeta.Replace(s) }

// ProcessVODs transcribes and links every fully downloaded VOD in dir.
func ProcessVODs(st *store.Store, dir string, linkOnly bool) error {
	done, err := archivedIDs(dir)
	if err != nil {
		return err
	}
	infos, err := filepath.Glob(filepath.Join(globEscape(dir), "*.info.json"))
	if err != nil {
		return err
	}
	for _, p := range infos {
		var info vodInfo
		if err := readJSON(p, &info); err != nil {
			return err
		}
		if done[info.ID] {
			if err := processVOD(st, p, info, linkOnly); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(p), err)
			}
		}
	}
	return nil
}
