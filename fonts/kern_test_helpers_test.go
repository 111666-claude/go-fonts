package fonts

import (
	"strconv"
	"sync/atomic"
	"testing"
)

// testFontCounter guarantees every test font gets a unique registry
// ID.
var testFontCounter atomic.Int64

// newTestFont builds a small, fully controlled font in native units.
//
// Every glyph is a 100-unit-wide square outline sitting on the
// baseline (from (0,0) to (100,100)) with an advance of 200 units,
// so hand-computed positions and widths stay simple:
//
//	UnitsPerEm = 1000, default/missing advance = 200.
//
// The font carries two classic kerning pairs, mimicking a real
// display font (AV tightens, To tightens):
//
//	('A','V') -> -80
//	('T','o') -> -40
//
// The returned font is not registered in the global Fonts map; use
// registerTestFont for that.
func newTestFont(t *testing.T) *Font {
	font := &Font{
		ID:               "testfont" + strconv.FormatInt(testFontCounter.Add(1), 10),
		HorizAdvX:        200,
		UnitsPerEm:       1000,
		Ascent:           800,
		Descent:          -200,
		MissingHorizAdvX: 200,
		Glyphs:           map[rune]*Glyph{},
		Kern:             map[kernPair]float64{},
	}
	square := func(r rune) *Glyph {
		return &Glyph{
			HorizAdvX: 200,
			Unicode:   r,
			GerberLP:  "d",
			PathSteps: []*PathStep{
				{C: 'M', P: []float64{0, 0}},
				{C: 'L', P: []float64{100, 0}},
				{C: 'L', P: []float64{100, 100}},
				{C: 'L', P: []float64{0, 100}},
				{C: 'Z'},
			},
			MBB: MBB{Min: Pt{0, 0}, Max: Pt{100, 100}},
		}
	}
	for _, r := range "AVToX" {
		font.Glyphs[r] = square(r)
	}
	// 'J' is intentionally offset from the pen origin (negative xMin,
	// overhanging bearings) to exercise glyphs whose MBB does not start
	// at (0,0), like real-world fonts.
	font.Glyphs['J'] = &Glyph{
		HorizAdvX: 200,
		Unicode:   'J',
		GerberLP:  "d",
		PathSteps: []*PathStep{
			{C: 'M', P: []float64{-30, -20}},
			{C: 'L', P: []float64{90, -20}},
			{C: 'L', P: []float64{90, 120}},
			{C: 'L', P: []float64{-30, 120}},
			{C: 'Z'},
		},
		MBB: MBB{Min: Pt{-30, -20}, Max: Pt{90, 120}},
	}
	font.Kern[kernPair{'A', 'V'}] = -80
	font.Kern[kernPair{'T', 'o'}] = -40
	font.Kern[kernPair{'A', 'J'}] = -50
	return font
}

// registerTestFont adds f to the package-level Fonts map. All
// registration happens from TestMain before any parallel test starts
// and entries are never removed, so concurrent reads of the global
// map stay race-free.
func registerTestFont(f *Font) {
	Fonts[f.ID] = f
}
