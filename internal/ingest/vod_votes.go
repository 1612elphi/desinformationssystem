package ingest

import (
	"bytes"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/1612elphi/desinformationssystem/internal/store"
)

// names unreadable at 720p

var (
	SwitchboardBase = strings.TrimRight(envStr("SWITCHBOARD_BASE", "http://127.0.0.1:3990/v1"), "/")
	FFprobeBin      = envStr("FFPROBE_BIN", "ffprobe")
)

//go:embed seatmaps/seatmap.json
var seatmapJSON []byte

type Box [4]float64

type Seat struct {
	ID    string `json:"id"`
	Box   Box    `json:"box"`
	Party string `json:"party"`
}

type Layout struct {
	ValidFrom string         `json:"valid_from"`
	Reference string         `json:"reference"`
	Size      [2]float64     `json:"size"`
	Counters  map[string]Box `json:"counters"`
	TopLabel  Box            `json:"top_label"`
	Blank     Box            `json:"blank"`
	Seats     []Seat         `json:"seats"`
}

type Assignment struct {
	Seat string  `json:"seat"`
	Name string  `json:"name"`
	From string  `json:"from"`
	To   *string `json:"to"`
}

type Seatmap struct {
	Layouts     []Layout     `json:"layouts"`
	Assignments []Assignment `json:"assignments"`
	Stream      struct{ X, Y, W float64 }
}

func LoadSeatmap() (Seatmap, error) {
	var sm Seatmap
	err := json.Unmarshal(seatmapJSON, &sm)
	return sm, err
}

func (sm Seatmap) LayoutFor(date string) (Layout, error) {
	var best *Layout
	for i, l := range sm.Layouts {
		if l.ValidFrom <= date && (best == nil || l.ValidFrom > best.ValidFrom) {
			best = &sm.Layouts[i]
		}
	}
	if best == nil {
		return Layout{}, fmt.Errorf("no seat layout valid on %s", date)
	}
	return *best, nil
}

func (sm Seatmap) NamesFor(date string) map[string]string {
	out := map[string]string{}
	for _, a := range sm.Assignments {
		if a.From <= date && (a.To == nil || date <= *a.To) {
			out[a.Seat] = a.Name
		}
	}
	return out
}

func (sm Seatmap) FirstLayout() string {
	first := ""
	for _, l := range sm.Layouts {
		if first == "" || l.ValidFrom < first {
			first = l.ValidFrom
		}
	}
	return first
}

// Frame is one rgb24 picture.
type Frame struct {
	W, H int
	Pix  []byte
}

// Rect maps reference coordinates onto a frame: x0 + bx*S, y0 + by*S.
type Rect struct{ X0, Y0, S float64 }

// StreamRect is where the diagram sits in a w x h broadcast, from the
// fractions measured once against the reference image.
func (sm Seatmap) StreamRect(w, h int) Rect {
	return Rect{sm.Stream.X * float64(w), sm.Stream.Y * float64(h), sm.Stream.W * float64(w) / sm.Layouts[0].Size[0]}
}

type region struct {
	f              *Frame
	x0, y0, x1, y1 int
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }

// python round: half-even
func pyRound(f float64) int { return int(math.RoundToEven(f)) }

func (f *Frame) crop(xa, ya, xb, yb int) region {
	return region{f, clampInt(xa, 0, f.W), clampInt(ya, 0, f.H), clampInt(xb, 0, f.W), clampInt(yb, 0, f.H)}
}

func (f *Frame) box(b Box, r Rect, inset float64) region {
	dx, dy := (b[2]-b[0])*inset, (b[3]-b[1])*inset
	xa, xb := pyRound(r.X0+(b[0]+dx)*r.S), pyRound(r.X0+(b[2]-dx)*r.S)
	ya, yb := pyRound(r.Y0+(b[1]+dy)*r.S), pyRound(r.Y0+(b[3]-dy)*r.S)
	return f.crop(xa, ya, max(xb, xa+1), max(yb, ya+1))
}

func (g region) share(ok func(r, gr, b int) bool) float64 {
	n, hit := 0, 0
	for y := g.y0; y < g.y1; y++ {
		for x := g.x0; x < g.x1; x++ {
			i := (y*g.f.W + x) * 3
			n++
			if ok(int(g.f.Pix[i]), int(g.f.Pix[i+1]), int(g.f.Pix[i+2])) {
				hit++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return float64(hit) / float64(n)
}

func (g region) median() [3]float64 {
	var ch [3][]int
	for y := g.y0; y < g.y1; y++ {
		for x := g.x0; x < g.x1; x++ {
			i := (y*g.f.W + x) * 3
			for c := range 3 {
				ch[c] = append(ch[c], int(g.f.Pix[i+c]))
			}
		}
	}
	var out [3]float64
	for c, v := range ch {
		if len(v) == 0 {
			continue
		}
		slices.Sort(v)
		out[c] = float64(v[(len(v)-1)/2]+v[len(v)/2]) / 2
	}
	return out
}

func (g region) frame() Frame {
	w, h := g.x1-g.x0, g.y1-g.y0
	out := Frame{w, h, make([]byte, 0, w*h*3)}
	for y := g.y0; y < g.y1; y++ {
		out.Pix = append(out.Pix, g.f.Pix[(y*g.f.W+g.x0)*3:(y*g.f.W+g.x1)*3]...)
	}
	return out
}

// white enthaltung, grey absent
var tileColours = []struct {
	vote string
	rgb  [3]float64
}{
	{"ja", [3]float64{246, 248, 8}},
	{"nein", [3]float64{246, 13, 17}},
	{"enthaltung", [3]float64{251, 251, 251}},
	{"abwesend", [3]float64{227, 227, 227}},
}

// ReadSeats is seat id -> ja | nein | enthaltung | abwesend, by nearest
// tile colour, in layout order.
func ReadSeats(f *Frame, lay Layout, r Rect) []store.VoteMember {
	out := make([]store.VoteMember, len(lay.Seats))
	for i, s := range lay.Seats {
		med := f.box(s.Box, r, 0.25).median()
		best, bestD := "", math.Inf(1)
		for _, c := range tileColours {
			d := 0.0
			for k := range 3 {
				d += (c.rgb[k] - med[k]) * (c.rgb[k] - med[k])
			}
			if d < bestD {
				best, bestD = c.vote, d
			}
		}
		out[i] = store.VoteMember{Name: s.ID, Vote: best}
	}
	return out
}

func panelVisible(f *Frame, lay Layout, r Rect) bool {
	return f.box(lay.Blank, r, 0).share(func(r, g, b int) bool { return min(r, g, b) > 225 }) >= 0.9
}

// CountersVisible: the Ja/Nein/Enthaltung rings are drawn only while a vote
// is shown. The blank area under REDEZEIT must also be white, or the
// pre-session title card (city hall photo) passes the ring test.
func CountersVisible(f *Frame, lay Layout, r Rect) bool {
	if !panelVisible(f, lay, r) {
		return false
	}
	return f.box(lay.Counters["nein"], r, 0).share(func(r, g, b int) bool { return r+g+b < 3*215 }) > 0.15
}

type Tally struct {
	Ja, Nein, Enthaltung int
	Members              []store.VoteMember
}

func TallyFrame(f *Frame, lay Layout, r Rect, names map[string]string) Tally {
	var t Tally
	for _, m := range ReadSeats(f, lay, r) {
		switch m.Vote {
		case "ja":
			t.Ja++
		case "nein":
			t.Nein++
		case "enthaltung":
			t.Enthaltung++
		}
		if n, ok := names[m.Name]; ok {
			m.Name = n
		}
		t.Members = append(t.Members, m)
	}
	return t
}

func videoSize(path string) (int, int, error) {
	out, err := exec.Command(FFprobeBin, "-v", "error", "-select_streams", "v:0", "-show_entries",
		"stream=width,height", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, 0, fmt.Errorf("ffprobe %s: no video stream", path)
	}
	var w, h int
	if _, err := fmt.Sscanf(fields[0], "%d,%d", &w, &h); err != nil {
		return 0, 0, fmt.Errorf("ffprobe %s: %q", path, fields[0])
	}
	return w, h, nil
}

// ReadImage is one frame of an image, or of a video at offset ss seconds
// when ss >= 0.
func ReadImage(path string, ss float64) (Frame, error) {
	w, h, err := videoSize(path)
	if err != nil {
		return Frame{}, err
	}
	args := []string{"-v", "error"}
	if ss >= 0 {
		args = append(args, "-ss", strconv.FormatFloat(ss, 'f', -1, 64))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	raw, err := exec.Command(FFmpegBin, args...).Output()
	if err != nil {
		return Frame{}, fmt.Errorf("ffmpeg %s: %w", path, err)
	}
	if len(raw) < w*h*3 {
		return Frame{}, fmt.Errorf("ffmpeg %s: short frame at %gs", path, ss)
	}
	return Frame{w, h, raw[:w*h*3]}, nil
}

// Frames calls fn with (seconds, frame) at fps, streamed from ffmpeg.
func Frames(video string, fps float64, fn func(t float64, f *Frame) error) error {
	w, h, err := videoSize(video)
	if err != nil {
		return err
	}
	cmd := exec.Command(FFmpegBin, "-v", "error", "-i", video, "-vf", "fps="+strconv.FormatFloat(fps, 'f', -1, 64),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	f := Frame{w, h, make([]byte, w*h*3)}
	for n := 0; ; n++ {
		if _, err = io.ReadFull(out, f.Pix); err != nil {
			break
		}
		if err = fn(float64(n)/fps, &f); err != nil {
			break
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = nil
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}
	return cmd.Wait()
}

type scannedVote struct {
	Start, End float64
	Tally
}

// last frame: tiles lag
func scanVotes(video string, lay Layout, names map[string]string, r Rect) ([]scannedVote, error) {
	const minLen = 3.0
	var votes []scannedVote
	var start, lastT float64
	var last *Tally
	inside := false
	flush := func() {
		if inside && lastT-start >= minLen {
			votes = append(votes, scannedVote{start, lastT, *last})
		}
		inside = false
	}
	err := Frames(video, 1, func(t float64, f *Frame) error {
		if CountersVisible(f, lay, r) {
			if !inside {
				start, inside = t, true
			}
			// frame buffer is reused
			tl := TallyFrame(f, lay, r, names)
			lastT, last = t, &tl
			return nil
		}
		flush()
		return nil
	})
	flush()
	return votes, err
}

type Panel struct {
	Top                  string
	Ja, Nein, Enthaltung int
}

const panelPrompt = "Abstimmungspanel eines Gemeinderats. Lies die TOP-Beschriftung oben in der Mitte " +
	"und die drei Zähler JA, NEIN, ENTHALTUNG. Nur JSON: " +
	`{"top": str, "ja": int, "nein": int, "enthaltung": int}`

func (f *Frame) panelCrop(lay Layout, r Rect) Frame {
	return f.crop(pyRound(r.X0), pyRound(r.Y0), pyRound(r.X0+lay.Size[0]*r.S), pyRound(r.Y0+lay.Size[1]*r.S)).frame()
}

func pngScaled(f Frame, scale int) ([]byte, error) {
	cmd := exec.Command(FFmpegBin, "-v", "error", "-f", "rawvideo", "-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", f.W, f.H), "-i", "-",
		"-vf", fmt.Sprintf("scale=%d:%d:flags=lanczos", f.W*scale, f.H*scale),
		"-f", "image2pipe", "-c:v", "png", "-")
	cmd.Stdin = bytes.NewReader(f.Pix)
	return cmd.Output()
}

func switchboardChat(payload any, title string, timeout time.Duration) (string, error) {
	return chatCompletion(SwitchboardBase, payload, map[string]string{"Content-Type": "application/json", "X-Title": title}, timeout)
}

func parsePanel(text string) (Panel, error) {
	m, err := parseModelJSON(text)
	if err != nil {
		return Panel{}, err
	}
	// unmarshal ignores key case
	b, err := json.Marshal(m)
	if err != nil {
		return Panel{}, err
	}
	var p struct {
		Top                  *string
		Ja, Nein, Enthaltung *int
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return Panel{}, err
	}
	if p.Top == nil || p.Ja == nil || p.Nein == nil || p.Enthaltung == nil {
		return Panel{}, fmt.Errorf("panel reply incomplete: %q", text)
	}
	return Panel{*p.Top, *p.Ja, *p.Nein, *p.Enthaltung}, nil
}

func readPanelVision(f *Frame, lay Layout, r Rect) (Panel, error) {
	png, err := pngScaled(f.panelCrop(lay, r), 4)
	if err != nil {
		return Panel{}, err
	}
	text, err := switchboardChat(map[string]any{
		"model": VisionModel, "max_tokens": 200, "temperature": 0,
		"messages": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": panelPrompt},
			map[string]any{"type": "image_url", "image_url": map[string]string{
				"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}},
		}}},
	}, "dis-vod-votes", VisionTimeout)
	if err != nil {
		return Panel{}, err
	}
	return parsePanel(text)
}

var anchorLabelRE = regexp.MustCompile(`top\s*([0-9]+(?:\.[0-9]+)*)`)

// ResolveAnchor: 'TOP 8' -> top8; 'TOP 8 Änderungsantrag Grüne' -> top8.1
// when 8.1 is the only sub-item. Same dotted form as the ticker and the PDF
// backfill, so pdf/live/vod rows reconcile.
func ResolveAnchor(label string, anchors map[string]bool) string {
	low := strings.ToLower(label)
	m := anchorLabelRE.FindStringSubmatchIndex(low)
	if m == nil {
		return ""
	}
	num := low[m[2]:m[3]]
	base := "top" + num
	if strings.Contains(low[m[1]:], "antrag") && !strings.Contains(num, ".") {
		var subs []string
		for a := range anchors {
			if strings.HasPrefix(a, base+".") {
				subs = append(subs, a)
			}
		}
		if len(subs) == 1 {
			return subs[0]
		}
		return ""
	}
	if anchors[base] {
		return base
	}
	return ""
}

type VODVote struct {
	Anchor, TopLabel     string
	Ja, Nein, Enthaltung int
	Members              []store.VoteMember
	At                   float64
	Complete             bool
	Tiles                [3]int
}

// ProcessVODVotes scans a VOD, reads every vote and, with apply, stores
// each placed one as source='vod'.
func ProcessVODVotes(st *store.Store, video, meetingID, date string, apply bool) ([]VODVote, error) {
	sm, err := LoadSeatmap()
	if err != nil {
		return nil, err
	}
	lay, err := sm.LayoutFor(date)
	if err != nil {
		return nil, err
	}
	w, h, err := videoSize(video)
	if err != nil {
		return nil, err
	}
	r := sm.StreamRect(w, h)
	anchors, err := agendaAnchors(st, meetingID)
	if err != nil {
		return nil, err
	}
	scanned, err := scanVotes(video, lay, sm.NamesFor(date), r)
	if err != nil {
		return nil, err
	}
	var out []VODVote
	for _, v := range scanned {
		f, err := ReadImage(video, v.End)
		if err != nil {
			return nil, err
		}
		p, err := readPanelVision(&f, lay, r)
		if err != nil {
			return nil, err
		}
		tiles := [3]int{v.Ja, v.Nein, v.Enthaltung}
		// unrevealed tiles look absent
		complete := tiles == [3]int{p.Ja, p.Nein, p.Enthaltung}
		members := v.Members
		if !complete {
			members = nil
			for _, m := range v.Members {
				if m.Vote != "abwesend" {
					members = append(members, m)
				}
			}
		}
		out = append(out, VODVote{ResolveAnchor(p.Top, anchors), p.Top, p.Ja, p.Nein, p.Enthaltung,
			members, v.End, complete, tiles})
	}
	if !apply {
		return out, nil
	}
	return out, st.Write(func(tx *sql.Tx) error {
		for _, v := range out {
			if v.Anchor == "" {
				continue
			}
			if err := store.UpsertVote(tx, store.Vote{MeetingID: meetingID, AgendaAnchor: v.Anchor, TopLabel: v.TopLabel,
				Ja: &v.Ja, Nein: &v.Nein, Enthaltung: &v.Enthaltung, Members: v.Members, Source: "vod"}); err != nil {
				return err
			}
		}
		return nil
	})
}
