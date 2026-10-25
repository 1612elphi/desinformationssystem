package geo

import "testing"

func TestResolve(t *testing.T) {
	g := Karlsruhe()
	for _, c := range []struct{ name, kind, want, wantKind string }{
		{"Kaiserstraße", "street", "Kaiserstraße", "street"},
		{"kaiserstr.", "street", "Kaiserstraße", "street"},
		{"Karl-Friedrich-Str.", "street", "Karl-Friedrich-Straße", "street"},
		{"Neue Anlage Straße", "street", "Neue-Anlage-Straße", "street"},
		{"Innenstadt Ost", "district", "Innenstadt-Ost", "district"},
		{"Stadtteil Durlach", "district", "Durlach", "district"},
		{"Marktplatz", "square", "Marktplatz", "square"},
		{"Kreuzstraße 13a", "building", "Kreuzstraße", "street"},
		{"Lammstraße 9", "building", "Lammstraße", "street"},
		{"Hauptfriedhof", "park", "Hauptfriedhof", "park"},
		{"Hauptfriedhof", "transit", "Hauptfriedhof", "transit"},
		{"Zündhütle", "park", "Zündhütle", "building"},
		{"Klärwerk Karlsruhe", "building", "Städtisches Klärwerk Karlsruhe", "building"},
		{"Festhalle Durlach", "building", "Festhalle", "building"},
		{"Friedhof Aue", "park", "Friedhof Durlach-Aue", "park"},
		{"Hagsfelder Friedhof", "park", "Friedhof Hagsfeld", "park"},
		{"Kaiserstraße-West", "area", "Kaiserstraße", "street"},
		{"Schickard-Straße", "street", "Wilhelm-Schickard-Straße", "street"},
		{"Citypark", "park", "Citypark Stadtpark", "park"},
		{"Wertstoffstation Am Junkertschritt", "building", "Am Junkertschritt", "street"},
		{"Starcksstraße", "street", "Starckstraße", "street"},
		{"Günter-Klotz-Anlage", "park", "Günther-Klotz-Anlage", "park"},
		{"Zoo", "building", "Zoologischer Stadtgarten", "park"},
	} {
		m, ok := g.Resolve(c.name, c.kind)
		if !ok || m.Name != c.want || m.Kind != c.wantKind {
			t.Errorf("Resolve(%q, %q) = %q %q %v (%s), want %q %q", c.name, c.kind, m.Name, m.Kind, ok, m.How, c.want, c.wantKind)
		}
	}
	for _, c := range [][2]string{
		{"Umfahrung Hagsfeld", "street"},
		{"Innenstadt", "district"},
		// two durlach cemeteries
		{"Friedhof Durlach", "park"},
		{"Grundschule Daxlanden", "school"},
		{"Hallenbad Durlach", "building"},
		{"A 5", "street"},
		{"Stachus", "square"},
		{"Verwaltungsgebäude HGW", "building"},
		{"Stuttgart", "area"},
	} {
		if m, ok := g.Resolve(c[0], c[1]); ok {
			t.Errorf("Resolve(%q, %q) = %q %q (%s), want no match", c[0], c[1], m.Name, m.Kind, m.How)
		}
	}
}
