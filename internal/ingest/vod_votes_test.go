package ingest

import (
	"image"
	"image/jpeg"
	"os"
	"testing"
)

func referenceFrame(t *testing.T, lay Layout) Frame {
	t.Helper()
	fh, err := os.Open("seatmaps/" + lay.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	img, err := jpeg.Decode(fh)
	if err != nil {
		t.Fatal(err)
	}
	return rgbFrame(img)
}

func rgbFrame(img image.Image) Frame {
	b := img.Bounds()
	f := Frame{b.Dx(), b.Dy(), make([]byte, 0, b.Dx()*b.Dy()*3)}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			f.Pix = append(f.Pix, byte(r>>8), byte(g>>8), byte(bl>>8))
		}
	}
	return f
}

// as vod_votes._resize
func resized(f Frame, w, h int) Frame {
	out := Frame{w, h, make([]byte, 0, w*h*3)}
	for y := range h {
		sy := y * f.H / h
		for x := range w {
			i := (sy*f.W + x*f.W/w) * 3
			out.Pix = append(out.Pix, f.Pix[i:i+3]...)
		}
	}
	return out
}

func TestSeatmap(t *testing.T) {
	sm, err := LoadSeatmap()
	if err != nil {
		t.Fatal(err)
	}
	lay, err := sm.LayoutFor("2026-09-29")
	if err != nil || lay.ValidFrom != "2026-07-28" || len(lay.Seats) != 49 {
		t.Fatalf("layout %v %v", lay.ValidFrom, err)
	}
	if _, err := sm.LayoutFor("2026-07-27"); err == nil {
		t.Error("layout before the first one")
	}
	if n := sm.NamesFor("2026-07-28"); len(n) != 49 || n["ob"] != "Dr. Mentrup" {
		t.Errorf("names %d %q", len(n), n["ob"])
	}
	if sm.FirstLayout() != "2026-07-28" {
		t.Error(sm.FirstLayout())
	}
	r := sm.StreamRect(1280, 720)
	if r.X0 < 636 || r.X0 > 638 || r.Y0 != 162 || r.S != 0.4 {
		t.Errorf("stream rect %+v", r)
	}
}

func TestTallyReference(t *testing.T) {
	sm, _ := LoadSeatmap()
	lay, _ := sm.LayoutFor("2026-07-28")
	names := sm.NamesFor("2026-07-28")
	ref := referenceFrame(t, lay)
	ident := Rect{0, 0, 1}
	check := func(f *Frame, r Rect) {
		t.Helper()
		got := TallyFrame(f, lay, r, names)
		if got.Ja != 44 || got.Nein != 0 || got.Enthaltung != 4 {
			t.Errorf("tally %d/%d/%d, want 44/0/4", got.Ja, got.Nein, got.Enthaltung)
		}
		by := map[string]string{}
		for _, m := range got.Members {
			by[m.Name] = m.Vote
		}
		if by["Schnell"] != "abwesend" || by["Winkler"] != "enthaltung" {
			t.Errorf("Schnell %q, Winkler %q", by["Schnell"], by["Winkler"])
		}
		if !CountersVisible(f, lay, r) {
			t.Error("counters not visible")
		}
	}
	check(&ref, ident)
	frame := Frame{1280, 720, make([]byte, 1280*720*3)}
	for i := range frame.Pix {
		frame.Pix[i] = 40
	}
	small := resized(ref, 640, 360)
	for y := range 360 {
		copy(frame.Pix[((180+y)*1280+640)*3:], small.Pix[y*640*3:(y+1)*640*3])
	}
	check(&frame, Rect{640, 180, 0.4})
	if CountersVisible(&frame, lay, Rect{0, 0, 0.4}) {
		t.Error("counters visible on a grey frame")
	}
}

func TestResolveAnchor(t *testing.T) {
	anchors := map[string]bool{"top6": true, "top8": true, "top8.1": true, "top14": true, "top14.1": true, "top14.2": true}
	for label, want := range map[string]string{
		"TOP 8":                       "top8",
		"TOP 8 Änderungsantrag Grüne": "top8.1",
		"TOP 6Änderungsantrag Grüne":  "",
		"TOP 14 Antrag FDP":           "",
		"TOP 14.1":                    "top14.1",
		"TOP 7":                       "",
		"Sitzungspause":               "",
	} {
		if got := ResolveAnchor(label, anchors); got != want {
			t.Errorf("%q: %q, want %q", label, got, want)
		}
	}
}

func TestParsePanel(t *testing.T) {
	p, err := parsePanel("```json\n{\"top\": \"TOP 4\", \"ja\": 47, \"nein\": 0, \"enthaltung\": 0}\n```")
	if err != nil || p != (Panel{"TOP 4", 47, 0, 0}) {
		t.Errorf("%+v %v", p, err)
	}
	if _, err := parsePanel(`{"top": "TOP 4", "ja": 47}`); err == nil {
		t.Error("incomplete reply accepted")
	}
}

func TestSpeakerSeat(t *testing.T) {
	sm, _ := LoadSeatmap()
	lay, _ := sm.LayoutFor("2026-07-28")
	f := referenceFrame(t, lay)
	// reference: chair speaking
	if got := speakerSeat(&f, lay, Rect{0, 0, 1}); got != "ob" {
		t.Errorf("speaker on the reference: %q", got)
	}
	b := lay.Seats[slicesIndex(lay, "c2r1")].Box
	for y := int(b[1]) - 4; y < int(b[3])+4; y++ {
		for x := int(b[0]) - 4; x < int(b[2])+4; x++ {
			if x < int(b[0]) || x >= int(b[2]) || y < int(b[1]) || y >= int(b[3]) {
				copy(f.Pix[(y*f.W+x)*3:], []byte{89, 186, 89})
			}
		}
	}
	if got := speakerSeat(&f, lay, Rect{0, 0, 1}); got != "c2r1" {
		t.Errorf("speaker %q, want c2r1", got)
	}
}

func slicesIndex(lay Layout, id string) int {
	for i, s := range lay.Seats {
		if s.ID == id {
			return i
		}
	}
	return -1
}
