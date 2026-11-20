package fonts

import (
	"math"
	"strings"
	"testing"
)

// glyphXs returns the base X position of every glyph in the render.
func glyphXs(r *Render) []float64 {
	out := make([]float64, len(r.Info))
	for i, gi := range r.Info {
		out[i] = gi.X
	}
	return out
}

// renderWidth is the overall polygon MBB width of a render.
func renderWidth(r *Render) float64 {
	return r.MBB.Max[0] - r.MBB.Min[0]
}

// TestTextKernPositions checks per-glyph base positions and total
// widths for the hand-computed fixture font (advance 200 units,
// kerning AV=-80, To=-40, UnitsPerEm=1000, unit scale).
func TestTextKernPositions(t *testing.T) {
	t.Parallel()

	fontName := testFontID
	const fsf = 1.0 / 1000.0

	t.Run("AV applies the pair once", func(t *testing.T) {
		plain, err := Text(0, 0, 1, 1, "AV", fontName, nil)
		if err != nil {
			t.Fatal(err)
		}
		kerned, err := TextKern(0, 0, 1, 1, "AV", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}

		// Plain layout: A at 0, V at 200 units.
		wantPlainX := []float64{0, 200 * fsf}
		if got := glyphXs(plain); !floatSlicesEqual(got, wantPlainX) {
			t.Errorf("plain AV positions = %v, want %v", got, wantPlainX)
		}
		// Kerned layout: A stays at 0, V shifts left by 80 units.
		wantKernX := []float64{0, (200 - 80) * fsf}
		if got := glyphXs(kerned); !floatSlicesEqual(got, wantKernX) {
			t.Errorf("kerned AV positions = %v, want %v", got, wantKernX)
		}

		// Total polygon width shrinks by exactly the adjustment.
		if delta := renderWidth(plain) - renderWidth(kerned); math.Abs(delta-80*fsf) > 1e-12 {
			t.Errorf("width delta = %v, want %v", delta, 80*fsf)
		}

		// TextMBBKern must agree with the render's MBB.
		mbb, err := TextMBBKern(0, 0, 1, 1, "AV", fontName, true)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs((mbb.Max[0]-mbb.Min[0])-renderWidth(kerned)) > 1e-12 {
			t.Errorf("TextMBBKern width %v != render width %v", mbb.Max[0]-mbb.Min[0], renderWidth(kerned))
		}
	})

	t.Run("each consecutive pair is applied independently", func(t *testing.T) {
		// "AVA": pairs AV (-80) then VA (no pair -> 0).
		r, err := TextKern(0, 0, 1, 1, "AVA", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		want := []float64{
			0,
			(200 - 80) * fsf,       // V: advance(A) + kern(AV)
			(200 - 80 + 200) * fsf, // second A: + advance(V), no kern
		}
		if got := glyphXs(r); !floatSlicesEqual(got, want) {
			t.Errorf("AVA positions = %v, want %v", got, want)
		}
	})

	t.Run("To pair", func(t *testing.T) {
		r, err := TextKern(0, 0, 1, 1, "To", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		want := []float64{0, (200 - 40) * fsf}
		if got := glyphXs(r); !floatSlicesEqual(got, want) {
			t.Errorf("To positions = %v, want %v", got, want)
		}
	})

	t.Run("scaling multiplies the adjustment too", func(t *testing.T) {
		r, err := TextKern(0, 0, 2, 3, "AV", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		// xScale=2 then /UnitsPerEm: V base = 2*fsf*(200-80).
		want := []float64{0, 2 * fsf * (200 - 80)}
		if got := glyphXs(r); !floatSlicesEqual(got, want) {
			t.Errorf("scaled AV positions = %v, want %v", got, want)
		}
	})
}

// TestTextKernOutlinesUnchanged proves kerning only translates whole
// glyphs: every polygon point moves by exactly the same delta as its
// glyph's base position, and glyph polygon shapes are otherwise
// identical to the unkerned render.
func TestTextKernOutlinesUnchanged(t *testing.T) {
	t.Parallel()

	fontName := testFontID

	plain, err := Text(0, 0, 1, 1, "AVT", fontName, nil)
	if err != nil {
		t.Fatal(err)
	}
	kerned, err := TextKern(0, 0, 1, 1, "AVT", fontName, nil, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plain.Polygons) != len(kerned.Polygons) {
		t.Fatalf("polygon count changed: %v vs %v", len(plain.Polygons), len(kerned.Polygons))
	}
	for i := range plain.Polygons {
		p, k := plain.Polygons[i], kerned.Polygons[i]
		if p.RuneIndex != k.RuneIndex || len(p.Pts) != len(k.Pts) {
			t.Fatalf("polygon %v structure mismatch", i)
		}
		// All points of one glyph share the glyph's X translation.
		dx := kerned.Info[k.RuneIndex].X - plain.Info[p.RuneIndex].X
		for j := range p.Pts {
			if math.Abs((k.Pts[j][0]-p.Pts[j][0])-dx) > 1e-12 {
				t.Errorf("polygon %v point %v X delta %v != glyph shift %v", i, j, k.Pts[j][0]-p.Pts[j][0], dx)
			}
			if math.Abs(k.Pts[j][1]-p.Pts[j][1]) > 1e-12 {
				t.Errorf("polygon %v point %v Y changed by kerning", i, j)
			}
		}
	}
}

// TestTextKernDisabledByDefault makes sure the legacy entry points
// and the zero-value TextOpts keep advance-width-only behavior even
// when the font has a kerning table.
func TestTextKernDisabledByDefault(t *testing.T) {
	t.Parallel()

	fontName := testFontID

	plain, err := Text(0, 0, 1, 1, "AV", fontName, nil)
	if err != nil {
		t.Fatal(err)
	}
	explicitOff, err := TextKern(0, 0, 1, 1, "AV", fontName, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	optOff, err := Text(0, 0, 1, 1, "AV", fontName, &TextOpts{})
	if err != nil {
		t.Fatal(err)
	}
	optOn, err := Text(0, 0, 1, 1, "AV", fontName, &TextOpts{EnableKerning: true})
	if err != nil {
		t.Fatal(err)
	}

	if !floatSlicesEqual(glyphXs(plain), glyphXs(explicitOff)) {
		t.Error("TextKern(useKerning=false) differs from Text")
	}
	if !floatSlicesEqual(glyphXs(plain), glyphXs(optOff)) {
		t.Error("Text with zero-value TextOpts differs from legacy Text")
	}
	if floatSlicesEqual(glyphXs(plain), glyphXs(optOn)) {
		t.Error("EnableKerning=true had no effect")
	}

	// Legacy TextMBB ignores the table entirely.
	mbbPlain, err := TextMBB(0, 0, 1, 1, "AV", fontName)
	if err != nil {
		t.Fatal(err)
	}
	mbbKern, err := TextMBBKern(0, 0, 1, 1, "AV", fontName, true)
	if err != nil {
		t.Fatal(err)
	}
	if (mbbPlain.Max[0] - mbbPlain.Min[0]) <= (mbbKern.Max[0] - mbbKern.Min[0]) {
		t.Error("kerning did not reduce the measured MBB width")
	}
	mbbOff, err := TextMBBKern(0, 0, 1, 1, "AV", fontName, false)
	if err != nil {
		t.Fatal(err)
	}
	if *mbbPlain != *mbbOff {
		t.Error("TextMBBKern(false) differs from legacy TextMBB")
	}
}

// TestTextKernNoTable ensures fonts without kerning data fall back to
// plain advance widths through every entry point.
func TestTextKernNoTable(t *testing.T) {
	t.Parallel()

	fontName := testFontNoKernID

	plain, err := Text(0, 0, 1, 1, "AVTo", fontName, nil)
	if err != nil {
		t.Fatal(err)
	}
	kerned, err := TextKern(0, 0, 1, 1, "AVTo", fontName, &TextOpts{EnableKerning: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !floatSlicesEqual(glyphXs(plain), glyphXs(kerned)) {
		t.Errorf("tableless font positions differ: %v vs %v", glyphXs(plain), glyphXs(kerned))
	}
}

// TestTextKernMissingGlyphAndSeparators verifies that missing glyphs,
// tabs and newlines break kerning pairs so an adjustment is never
// applied to the wrong following glyph.
func TestTextKernMissingGlyphAndSeparators(t *testing.T) {
	t.Parallel()

	fontName := testFontID
	const fsf = 1.0 / 1000.0

	t.Run("missing glyph breaks the pair", func(t *testing.T) {
		// 'A', unknown rune 'Z', 'V': kern(A,V) must not fire
		// across the missing glyph.
		r, err := TextKern(0, 0, 1, 1, "AZV", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		// A:0, Z advances MissingHorizAdvX (200), V at 400 units.
		want := []float64{0, 200 * fsf, 400 * fsf}
		if got := glyphXs(r); !floatSlicesEqual(got, want) {
			t.Errorf("positions = %v, want %v", got, want)
		}
	})

	t.Run("tab breaks the pair", func(t *testing.T) {
		r, err := TextKern(0, 0, 1, 1, "A\tV", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		// Tab advances 2*HorizAdvX = 400 units; V at 600.
		want := []float64{0, 200 * fsf, 600 * fsf}
		if got := glyphXs(r); !floatSlicesEqual(got, want) {
			t.Errorf("positions = %v, want %v", got, want)
		}
	})

	t.Run("newline breaks the pair and resets X", func(t *testing.T) {
		r, err := TextKern(0, 0, 1, 1, "A\nV", fontName, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if r.Info[0].X != 0 || r.Info[2].X != 0 {
			t.Errorf("line-start X positions = %v, %v; want both 0", r.Info[0].X, r.Info[2].X)
		}
		if r.Info[2].Y >= r.Info[0].Y {
			t.Errorf("second line Y=%v should be below first Y=%v", r.Info[2].Y, r.Info[0].Y)
		}
	})
}

// TestTextKernEmptyAndLong covers empty text and a long message to
// confirm linear, deterministic accumulation.
func TestTextKernEmptyAndLong(t *testing.T) {
	t.Parallel()

	fontName := testFontID

	if _, err := TextKern(0, 0, 1, 1, "", fontName, nil, true); err == nil {
		t.Error("empty message should return an error")
	}
	if _, err := TextMBBKern(0, 0, 1, 1, "\n\t", fontName, true); err == nil {
		t.Error("message without renderable glyphs should return an error")
	}

	const n = 1000
	msg := strings.Repeat("AV", n/2)
	r, err := TextKern(0, 0, 1, 1, msg, fontName, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Info) != n {
		t.Fatalf("rendered %v glyphs, want %v", len(r.Info), n)
	}
	// Each "AV" pair advances 200 (A) - 80 (kern) + 200 (V) = 320 units.
	fsf := 1.0 / 1000.0
	wantLast := float64(n/2-1) * 320 * fsf
	if got := r.Info[n-2].X; math.Abs(got-wantLast) > 1e-9 {
		t.Errorf("long-text penultimate X = %v, want %v", got, wantLast)
	}
	// Repeated rendering is deterministic.
	r2, err := TextKern(0, 0, 1, 1, msg, fontName, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !floatSlicesEqual(glyphXs(r), glyphXs(r2)) {
		t.Error("long-text render not deterministic")
	}
}

// TestTextKernNonFiniteSize guarantees a defined error instead of
// NaN propagation for non-finite dimensions.
func TestTextKernNonFiniteSize(t *testing.T) {
	t.Parallel()

	fontName := testFontID

	bad := []float64{math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, v := range bad {
		if _, err := TextKern(0, 0, v, 1, "AV", fontName, nil, true); err == nil {
			t.Errorf("TextKern with xScale=%v should fail", v)
		}
		if _, err := TextKern(0, 0, 1, v, "AV", fontName, nil, true); err == nil {
			t.Errorf("TextKern with yScale=%v should fail", v)
		}
		if _, err := TextMBBKern(0, 0, v, 1, "AV", fontName, true); err == nil {
			t.Errorf("TextMBBKern with xScale=%v should fail", v)
		}
	}
}

// TestTextKernCrossFont makes sure kerning data of one font never
// leaks into another font's layout.
func TestTextKernCrossFont(t *testing.T) {
	t.Parallel()

	kerned, err := TextKern(0, 0, 1, 1, "AV", testFontID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := TextKern(0, 0, 1, 1, "AV", testFontNoKernID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if floatSlicesEqual(glyphXs(kerned), glyphXs(plain)) {
		t.Error("kerning of one font leaked into another font")
	}
}

// TestTextKernOffsetGlyph checks a glyph whose outline overhangs the
// pen origin (non-zero, partly negative MBB): kerning must move the
// base (and therefore the whole outline) without altering its shape,
// and TextMBBKern must match the polygon MBB.
func TestTextKernOffsetGlyph(t *testing.T) {
	t.Parallel()

	fontName := testFontID
	const fsf = 1.0 / 1000.0

	plain, err := Text(0, 0, 1, 1, "AJ", fontName, nil)
	if err != nil {
		t.Fatal(err)
	}
	kerned, err := TextKern(0, 0, 1, 1, "AJ", fontName, nil, true)
	if err != nil {
		t.Fatal(err)
	}

	// J base: advance 200, kern AJ=-50 -> 150 units.
	wantBase := (200 - 50) * fsf
	if got := kerned.Info[1].X; math.Abs(got-wantBase) > 1e-12 {
		t.Errorf("offset-glyph J base = %v, want %v", got, wantBase)
	}
	// The J glyph's own outline spans xMin=-30..xMax=90 around its base.
	wantJMinX := wantBase + (-30)*fsf
	if got := kerned.Info[1].MBB.Min[0]; math.Abs(got-wantJMinX) > 1e-12 {
		t.Errorf("J glyph MBB min X = %v, want %v", got, wantJMinX)
	}
	// The whole render's left edge is still glyph A at x=0.
	if kerned.MBB.Min[0] != 0 {
		t.Errorf("render MBB min X = %v, want 0 (glyph A edge)", kerned.MBB.Min[0])
	}

	mbb, err := TextMBBKern(0, 0, 1, 1, "AJ", fontName, true)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs((mbb.Min[0])-kerned.MBB.Min[0]) > 1e-12 ||
		math.Abs((mbb.Max[0])-kerned.MBB.Max[0]) > 1e-12 {
		t.Errorf("TextMBBKern %v != render MBB %v", mbb, kerned.MBB)
	}

	// Outline points translate by exactly the base delta.
	dx := kerned.Info[1].X - plain.Info[1].X
	poly := kerned.Polygons[1]
	plainPoly := plain.Polygons[1]
	for i := range poly.Pts {
		if math.Abs((poly.Pts[i][0]-plainPoly.Pts[i][0])-dx) > 1e-12 {
			t.Errorf("offset glyph point %v did not translate with its base", i)
		}
	}
}

// TestFillBoxWithKerning makes sure the convenience FillBox helper
// works through the kerning-aware path and returns finite, usable
// values.
func TestFillBoxWithKerning(t *testing.T) {
	t.Parallel()

	fontName := testFontID
	box := MBB{Min: Pt{0, 0}, Max: Pt{10, 4}}

	// Legacy path (kerning off) still behaves as before.
	x, y, pts, err := FillBox(box, 1, 1, "AVATAR", fontName, &BottomLeft)
	if err != nil {
		t.Fatal(err)
	}
	if !finite(x) || !finite(y) || !finite(pts) || pts <= 0 {
		t.Errorf("FillBox returned bad values: x=%v y=%v pts=%v", x, y, pts)
	}

	// Kerning-aware rendering must fit inside the same target box
	// while producing a narrower text run.
	kerned, err := TextKern(0, 0, pts, pts, "AVATAR", fontName, &BottomLeft, true)
	if err != nil {
		t.Fatal(err)
	}
	w := kerned.MBB.Max[0] - kerned.MBB.Min[0]
	if w > (box.Max[0]-box.Min[0])+1e-9 {
		t.Errorf("kerned FillBox width %v exceeds box %v", w, box.Max[0]-box.Min[0])
	}
}

// TestTextKernExtremeAdjustments covers positive kerning (push apart)
// and an adjustment larger than the advance (the pen may move
// backwards, but every glyph still gets a deterministic position and
// the render remains well-formed).
func TestTextKernExtremeAdjustments(t *testing.T) {
	// Sequential: it registers an additional font in the global
	// Fonts map, which parallel tests only read.

	const fsf = 1.0 / 1000.0

	// Positive pair via a local table: use a throwaway registered
	// font instead of mutating the shared fixture.
	font := newTestFont(t)
	font.SetKern('A', 'V', 60)
	font.SetKern('V', 'T', -500) // Larger than the 200-unit advance.
	font.Kern[kernPair{'T', 'o'}] = -40
	registerTestFont(font)
	t.Cleanup(func() { delete(Fonts, font.ID) })

	// Positive adjustment: V base = advance + 60 = 260 units.
	r, err := TextKern(0, 0, 1, 1, "AV", font.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.Info[1].X, 260*fsf; math.Abs(got-want) > 1e-12 {
		t.Errorf("positive-kern V base = %v, want %v", got, want)
	}

	// Oversized negative adjustment: V base is 200+60=260; then T
	// advances 200 and kerns by -500, landing at -40 native units
	// relative to the unaligned text origin. The bottom-left anchor
	// shifts every base right by +40 units so the render MBB starts
	// at zero: T therefore reports a base of exactly 0 em.
	r, err = TextKern(0, 0, 1, 1, "AVT", font.ID, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Info[2].X; math.Abs(got) > 1e-12 {
		t.Errorf("oversized-kern T base = %v, want 0 after anchoring", got)
	}
	// A and V are shifted by the same +40-unit (.04 em) correction.
	if got, want := r.Info[0].X, 40*fsf; math.Abs(got-want) > 1e-12 {
		t.Errorf("anchored A base = %v, want %v", got, want)
	}
	if got, want := r.Info[1].X, (260+40)*fsf; math.Abs(got-want) > 1e-12 {
		t.Errorf("anchored V base = %v, want %v", got, want)
	}
	if math.IsNaN(r.Info[2].X) || math.IsInf(r.Info[2].X, 0) {
		t.Errorf("T base is not finite: %v", r.Info[2].X)
	}
}

func floatSlicesEqual(a, b []float64) bool {
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
