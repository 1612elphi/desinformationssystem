package ingest

// Who is speaking, under which TOP, second by second, from the XPanel in a
// council VOD. The panel draws a green border around the tile of the member
// who has the floor and shows the current TOP label.

const (
	// px in reference coordinates around a tile where the border sits; border
	// colour measured on 2026-09-29 (Tröndle, 53:20): median (89, 186, 89)
	borderPad = 6
	borderMin = 0.02
	// Text pixels (grey < 140) that differ between samples. Measured
	// 2026-09-29 at 720p: at most 2 within one TOP, 13 for TOP 6 -> TOP 7 (one
	// digit), 77 for 7 -> 10.
	labelChange = 6
	// re-read even without a change, in case two labels mask alike
	labelReread = 300.0
)

// SpeakerSpan is one stretch of constant (panel visible, speaker, TOP label).
// Seat, Name and Top are nil when nobody is marked or the panel is hidden.
type SpeakerSpan struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Panel bool    `json:"panel"`
	Seat  *string `json:"seat"`
	Name  *string `json:"name"`
	Top   *string `json:"top"`
}

func speakerSeat(f *Frame, lay Layout, r Rect) string {
	best, bestID := borderMin, ""
	for _, s := range lay.Seats {
		b := s.Box
		band := f.crop(pyRound(r.X0+(b[0]-borderPad)*r.S), pyRound(r.Y0+(b[1]-borderPad)*r.S),
			pyRound(r.X0+(b[2]+borderPad)*r.S), pyRound(r.Y0+(b[3]+borderPad)*r.S))
		g := band.share(func(r, g, b int) bool { return g > 140 && r < 120 && b < 120 })
		if g > best {
			best, bestID = g, s.ID
		}
	}
	return bestID
}

func labelMask(f *Frame, lay Layout, r Rect) []bool {
	g := f.box(lay.TopLabel, r, 0)
	var out []bool
	for y := g.y0; y < g.y1; y++ {
		for x := g.x0; x < g.x1; x++ {
			i := (y*f.W + x) * 3
			out = append(out, int(f.Pix[i])+int(f.Pix[i+1])+int(f.Pix[i+2]) < 3*140)
		}
	}
	return out
}

func maskDiff(a, b []bool) int {
	n := 0
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}

// SpeakerTimeline samples the video at 1 fps.
func SpeakerTimeline(video, date string) ([]SpeakerSpan, error) {
	sm, err := LoadSeatmap()
	if err != nil {
		return nil, err
	}
	lay, err := sm.LayoutFor(date)
	if err != nil {
		return nil, err
	}
	names := sm.NamesFor(date)
	w, h, err := videoSize(video)
	if err != nil {
		return nil, err
	}
	r := sm.StreamRect(w, h)
	type key struct {
		panel  bool
		seat   string
		hasTop bool
		top    string
	}
	var keys []key
	var spans []SpeakerSpan
	label, labelAt := "", -labelReread
	var mask []bool
	err = Frames(video, 1, func(t float64, f *Frame) error {
		k := key{panel: panelVisible(f, lay, r)}
		if k.panel {
			k.seat = speakerSeat(f, lay, r)
			m := labelMask(f, lay, r)
			if mask == nil || maskDiff(m, mask) > labelChange || t-labelAt >= labelReread {
				p, err := PanelReader(f, lay, r)
				if err != nil {
					return err
				}
				label, mask, labelAt = p.Top, m, t
			}
			k.hasTop, k.top = true, label
		}
		if n := len(spans); n > 0 && keys[n-1] == k {
			spans[n-1].End = t + 1
			return nil
		}
		s := SpeakerSpan{Start: t, End: t + 1, Panel: k.panel}
		if k.seat != "" {
			s.Seat = &k.seat
			if n, ok := names[k.seat]; ok {
				s.Name = &n
			}
		}
		if k.hasTop {
			s.Top = &k.top
		}
		keys, spans = append(keys, k), append(spans, s)
		return nil
	})
	return spans, err
}
