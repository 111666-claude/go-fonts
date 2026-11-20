package fonts_test

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-fonts/fonts"
	"github.com/gmlewis/go-fonts/webfont"
)

// fixedKerningSVG is a self-contained SVG webfont fixture. It has
// square box glyphs for A, V, T and o (each 100 font units wide with
// a 200-unit advance) and two real-world kerning pairs:
//
//	<hkern u1="A" u2="V" k="-80"/>
//	<hkern u1="T" u2="o" k="-40"/>
//
// mirroring the pairs a display font would ship for an "AVATAR"
// badge.
const fixedKerningSVG = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg">
<defs>
<font id="fixturekern" horiz-adv-x="200">
<font-face units-per-em="1000" ascent="800" descent="-200"/>
<missing-glyph horiz-adv-x="200"/>
<glyph glyph-name="A" unicode="A" horiz-adv-x="200" d="M0 0 L100 0 L100 100 L0 100 Z"/>
<glyph glyph-name="V" unicode="V" horiz-adv-x="200" d="M0 0 L100 0 L100 100 L0 100 Z"/>
<glyph glyph-name="T" unicode="T" horiz-adv-x="200" d="M0 0 L100 0 L100 100 L0 100 Z"/>
<glyph glyph-name="o" unicode="o" horiz-adv-x="200" d="M0 0 L100 0 L100 100 L0 100 Z"/>
<hkern u1="A" u2="V" k="-80"/>
<hkern u1="T" u2="o" k="-40"/>
</font>
</defs>
</svg>`

// loadFixtureFont runs the same processing stages as cmd/font2go:
// XML unmarshal, hkern expansion, table serialization and runtime
// decode. The glyph outlines themselves are not needed for layout,
// so a hand-built runtime font stands in for the protobuf payload.
func loadFixtureFont(t *testing.T) *fonts.Font {
	t.Helper()

	fontData := &webfont.FontData{}
	if err := xml.Unmarshal([]byte(fixedKerningSVG), fontData); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}
	if len(fontData.Font.HKerns) != 2 {
		t.Fatalf("parsed %v hkern elements, want 2", len(fontData.Font.HKerns))
	}

	kern := webfont.ExpandHKerns(fontData.Font.HKerns, fontData.Font.Glyphs)
	if got := kern[[2]rune{'A', 'V'}]; got != -80 {
		t.Fatalf("expanded AV kern = %v, want -80", got)
	}
	if got := kern[[2]rune{'T', 'o'}]; got != -40 {
		t.Fatalf("expanded To kern = %v, want -40", got)
	}

	font := &fonts.Font{
		ID:               "fixturekern",
		HorizAdvX:        fontData.Font.HorizAdvX,
		UnitsPerEm:       fontData.Font.FontFace.UnitsPerEm,
		Ascent:           fontData.Font.FontFace.Ascent,
		Descent:          fontData.Font.FontFace.Descent,
		MissingHorizAdvX: fontData.Font.MissingGlyph.HorizAdvX,
		Glyphs:           map[rune]*fonts.Glyph{},
		Kern:             kern,
	}
	for _, g := range fontData.Font.Glyphs {
		if g.Unicode == nil {
			continue
		}
		r := []rune(*g.Unicode)[0]
		font.Glyphs[r] = &fonts.Glyph{
			HorizAdvX: g.HorizAdvX,
			Unicode:   r,
			GerberLP:  "d",
			PathSteps: []*fonts.PathStep{
				{C: 'M', P: []float64{0, 0}},
				{C: 'L', P: []float64{100, 0}},
				{C: 'L', P: []float64{100, 100}},
				{C: 'L', P: []float64{0, 100}},
				{C: 'Z'},
			},
			MBB: fonts.MBB{Min: fonts.Pt{0, 0}, Max: fonts.Pt{100, 100}},
		}
	}

	// Round-trip the table through the exact serialization used in
	// generated font packages.
	encoded := font.EncodeKern()
	font.Kern = nil
	if err := font.DecodeKern(encoded); err != nil {
		t.Fatalf("DecodeKern: %v", err)
	}

	fonts.Fonts[font.ID] = font
	t.Cleanup(func() { delete(fonts.Fonts, font.ID) })
	return font
}

// TestEndToEndKernedAVATAR renders the badge word with and without
// kerning and checks every glyph base, the total width and the
// exported vector polygons against hand-computed values.
func TestEndToEndKernedAVATAR(t *testing.T) {

	font := loadFixtureFont(t)

	// "AVATAR": pairs are A-V (-80), V-A, A-T, T-A, A-R(missing).
	// Only the first pair kerns; 'R' is absent from the fixture.
	plain, err := fonts.Text(0, 0, 1, 1, "AVATAR", font.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	kerned, err := fonts.TextKern(0, 0, 1, 1, "AVATAR", font.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if plain == nil || kerned == nil {
		t.Fatal("nil render")
	}

	// Plain bases in ems: A=0, V=.2, A=.4, T=.6, A=.8, R(missing)=1.0.
	wantPlain := []float64{0, .2, .4, .6, .8, 1.0}
	// Kerned: only V shifts left by 80 units (.08 em); everything
	// after V shifts by the same .08, including the missing R slot.
	wantKern := []float64{0, .12, .32, .52, .72, .92}
	if got := glyphBaseX(plain); !near(got, wantPlain) {
		t.Errorf("plain bases = %v, want %v", got, wantPlain)
	}
	if got := glyphBaseX(kerned); !near(got, wantKern) {
		t.Errorf("kerned bases = %v, want %v", got, wantKern)
	}

	// Total width: five present glyphs, each spanning .1 em with
	// .2 em advance; kerning removes .08 em once.
	plainWidth := 4*.2 + .1 // last present A ends at .9 em.
	kernWidth := plainWidth - .08
	if w := plain.MBB.Max[0] - plain.MBB.Min[0]; math.Abs(w-plainWidth) > 1e-12 {
		t.Errorf("plain width = %v, want %v", w, plainWidth)
	}
	if w := kerned.MBB.Max[0] - kerned.MBB.Min[0]; math.Abs(w-kernWidth) > 1e-12 {
		t.Errorf("kerned width = %v, want %v", w, kernWidth)
	}

	// The missing 'R' contributes no polygons but keeps an Info slot.
	if len(kerned.Info) != len("AVATAR") {
		t.Errorf("Info length = %v, want %v", len(kerned.Info), len("AVATAR"))
	}
	presentPolys := 0
	for _, gi := range kerned.Info {
		if gi.Glyph != 'R' {
			presentPolys += gi.N
		} else if gi.N != 0 {
			t.Errorf("missing glyph R should have 0 polygons, got %v", gi.N)
		}
	}
	if presentPolys != len(kerned.Polygons) {
		t.Errorf("polygon accounting: %v vs %v", presentPolys, len(kerned.Polygons))
	}

	// Vector outlines: identical point count, pure X translation per
	// glyph, unchanged Y.
	if len(plain.Polygons) != len(kerned.Polygons) {
		t.Fatalf("polygon count differs: %v vs %v", len(plain.Polygons), len(kerned.Polygons))
	}
	for i := range plain.Polygons {
		p, k := plain.Polygons[i], kerned.Polygons[i]
		if len(p.Pts) != len(k.Pts) {
			t.Fatalf("polygon %v point count differs", i)
		}
		dx := kerned.Info[k.RuneIndex].X - plain.Info[p.RuneIndex].X
		for j := range p.Pts {
			if math.Abs((k.Pts[j][0]-p.Pts[j][0])-dx) > 1e-12 {
				t.Errorf("poly %v pt %v: X delta %v != base shift %v", i, j, k.Pts[j][0]-p.Pts[j][0], dx)
			}
			if math.Abs(k.Pts[j][1]-p.Pts[j][1]) > 1e-12 {
				t.Errorf("poly %v pt %v: Y changed", i, j)
			}
		}
	}
}

// TestEndToEndToPair exercises the second fixture pair and confirms
// the legacy entry point ignores the table even on a fully populated
// font.
func TestEndToEndToPair(t *testing.T) {

	font := loadFixtureFont(t)
	const fsf = 1.0 / 1000.0

	kerned, err := fonts.TextKern(0, 0, 1, 1, "To", font.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0, (200 - 40) * fsf}
	if got := glyphBaseX(kerned); !near(got, want) {
		t.Errorf("To bases = %v, want %v", got, want)
	}

	legacy, err := fonts.TextMBB(0, 0, 1, 1, "To", font.ID)
	if err != nil {
		t.Fatal(err)
	}
	kernMBB, err := fonts.TextMBBKern(0, 0, 1, 1, "To", font.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs((legacy.Max[0]-legacy.Min[0])-(kernMBB.Max[0]-kernMBB.Min[0])-40*fsf) > 1e-12 {
		t.Errorf("legacy/kern MBB width difference = %v, want %v",
			(legacy.Max[0]-legacy.Min[0])-(kernMBB.Max[0]-kernMBB.Min[0]), 40*fsf)
	}
}

// TestEndToEndSVGExport verifies the exported vector file reflects
// the kerning: the SVG width follows the kerned render MBB and the
// per-glyph translate uses the kerned base positions.
func TestEndToEndSVGExport(t *testing.T) {
	font := loadFixtureFont(t)

	kerned, err := fonts.TextKern(0, 0, 1, 1, "AV", font.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "kerned.svg")
	if err := kerned.SaveSVG(font.ID, path, 1); err != nil {
		t.Fatalf("SaveSVG: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(data)
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "<path") {
		t.Fatalf("exported SVG missing structure: %q", svg[:min(120, len(svg))])
	}
	// The declared width must match the kerned render MBB width.
	wantWidth := kerned.MBB.Max[0] - kerned.MBB.Min[0]
	if !strings.Contains(svg, fmt.Sprintf("width=\"%g\"", wantWidth)) {
		t.Errorf("SVG width does not match kerned MBB width %v", wantWidth)
	}
}

func glyphBaseX(r *fonts.Render) []float64 {
	out := make([]float64, len(r.Info))
	for i, gi := range r.Info {
		out[i] = gi.X
	}
	return out
}

func near(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			return false
		}
	}
	return true
}
