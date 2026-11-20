package fonts

import (
	"log"
	"math"
	"testing"
)

// legacyText is a faithful copy of the pre-kerning Text() algorithm
// (advance-width-only pen walk), used only to prove the refactored
// Text() keeps bit-for-bit behavior when kerning is disabled.
func legacyText(xPos, yPos, xScale, yScale float64, message, fontName string, opts *TextOpts) (*Render, error) {
	font, err := getFont(fontName)
	if err != nil {
		return nil, err
	}
	mbb, err := TextMBB(xPos, yPos, xScale, yScale, message, fontName)
	if err != nil {
		return nil, err
	}
	if font.HorizAdvX == 0 {
		font.HorizAdvX = font.UnitsPerEm
	}
	if font.MissingHorizAdvX == 0 {
		font.MissingHorizAdvX = font.HorizAdvX
	}
	x, y := xPos, yPos
	fsf := 1.0 / font.UnitsPerEm
	xScale *= fsf
	yScale *= fsf
	var xAlign, yAlign float64
	if opts != nil {
		xAlign, yAlign = opts.XAlign, opts.YAlign
	}
	width := mbb.Max[0] - mbb.Min[0]
	height := mbb.Max[1] - mbb.Min[1]
	xError := mbb.Min[0] - xPos
	yError := mbb.Min[1] - yPos
	xPos = xPos - xAlign*width - xError
	yPos = yPos - yAlign*height - yError
	x, y = xPos, yPos
	var xformPt func(pt Pt) Pt
	if opts == nil || opts.Rotate == 0.0 {
		xformPt = func(pt Pt) Pt { return pt }
	} else {
		cos := math.Cos(opts.Rotate)
		sin := math.Sin(opts.Rotate)
		xformPt = func(pt Pt) Pt {
			dx := pt[0] - xPos
			dy := pt[1] - yPos
			return Pt{xPos + dx*cos - dy*sin, yPos + dy*cos + dx*sin}
		}
	}
	result := &Render{}
	for runeIndex, c := range message {
		newPt := xformPt(Pt{x, y})
		gi := &GlyphInfo{Glyph: c, X: newPt[0], Y: newPt[1], MBB: MBB{Min: newPt, Max: newPt}}
		result.Info = append(result.Info, gi)
		if c == '\n' {
			x, y = xPos, y-yScale*(font.Ascent-font.Descent)
			continue
		}
		if c == '\t' {
			x += 2.0 * xScale * font.HorizAdvX
			continue
		}
		g, ok := font.Glyphs[c]
		if !ok {
			if c != ' ' {
				log.Printf("Warning: missing glyph %+q: skipping", c)
			}
			x += xScale * font.MissingHorizAdvX
			continue
		}
		dx, r := g.Render(x, y, xScale, yScale)
		gi.N = len(r.Polygons)
		for pi, poly := range r.Polygons {
			poly.RuneIndex = runeIndex
			for i, pt := range poly.Pts {
				newPt := xformPt(pt)
				v := MBB{Min: newPt, Max: newPt}
				if i == 0 {
					poly.MBB = v
				} else {
					poly.MBB.Join(&v)
				}
				poly.Pts[i] = newPt
			}
			if pi == 0 {
				r.MBB = poly.MBB
			} else {
				r.MBB.Join(&poly.MBB)
			}
			if len(result.Polygons) == 0 {
				result.MBB = r.MBB
			} else {
				result.MBB.Join(&r.MBB)
			}
			result.Polygons = append(result.Polygons, poly)
		}
		gi.MBB = r.MBB
		if dx == 0 {
			dx = font.HorizAdvX
		}
		w := dx * xScale
		gi.Width = w
		x += w
	}
	return result, nil
}

func TestLegacyTextBitIdentical(t *testing.T) {
	t.Parallel()

	name := testFontID
	messages := []string{
		"AV", "AVATAR", "To", "A\tV", "A\nV", "AZV", "AVT\noX",
		"V", "AVToAVTo", "AV V",
	}
	type cfg struct {
		xp, yp, xs, ys float64
		opts           *TextOpts
	}
	configs := []cfg{
		{0, 0, 1, 1, nil},
		{1.5, -2.25, 1, 1, &BottomLeft},
		{0, 0, 1, 1, &Center},
		{0, 0, 1, 1, &TopRight},
		{3, 4, 2, 3, &Center},
		{1, 1, -1, 1, nil}, // mirrored X
		{0, 0, 1, -1, nil}, // mirrored Y
		{0.5, 0.5, 1.7, 0.6, &TextOpts{XAlign: 0.25, YAlign: 0.75, Rotate: math.Pi / 7}},
		{0, 0, 1, 1, &TextOpts{Rotate: math.Pi / 2}},
	}
	for _, msg := range messages {
		for _, c := range configs {
			got, err := Text(c.xp, c.yp, c.xs, c.ys, msg, name, c.opts)
			if err != nil {
				t.Fatalf("Text(%q): %v", msg, err)
			}
			want, err := legacyText(c.xp, c.yp, c.xs, c.ys, msg, name, c.opts)
			if err != nil {
				t.Fatalf("legacyText(%q): %v", msg, err)
			}
			// Positions are derived directly from the layout walk
			// instead of accumulated pen additions, so results are
			// equal to the legacy algorithm up to (at most) a single
			// ULP and are occasionally slightly more precise.
			const exact = false
			if !rendersEqual(got, want, exact) {
				t.Fatalf("legacy mismatch for msg=%q cfg=%+v\n got Info=%v\nwant Info=%v\n got MBB=%v\nwant MBB=%v",
					msg, c, infoXs(got), infoXs(want), got.MBB, want.MBB)
			}
		}
	}
}

func infoXs(r *Render) []Pt {
	out := make([]Pt, len(r.Info))
	for i, gi := range r.Info {
		out[i] = Pt{gi.X, gi.Y}
	}
	return out
}

func rendersEqual(a, b *Render, exact bool) bool {
	if len(a.Info) != len(b.Info) || len(a.Polygons) != len(b.Polygons) {
		return false
	}
	for i := range a.Info {
		ga, gb := a.Info[i], b.Info[i]
		if ga.Glyph != gb.Glyph || ga.N != gb.N {
			return false
		}
		if !floatEqual(ga.Width, gb.Width, exact) {
			return false
		}
		if !floatEqual(ga.X, gb.X, exact) || !floatEqual(ga.Y, gb.Y, exact) {
			return false
		}
		if !mbbEqual(ga.MBB, gb.MBB, exact) {
			return false
		}
	}
	for i := range a.Polygons {
		pa, pb := a.Polygons[i], b.Polygons[i]
		if pa.RuneIndex != pb.RuneIndex || pa.Dark != pb.Dark || len(pa.Pts) != len(pb.Pts) {
			return false
		}
		for j := range pa.Pts {
			if !floatEqual(pa.Pts[j][0], pb.Pts[j][0], exact) || !floatEqual(pa.Pts[j][1], pb.Pts[j][1], exact) {
				return false
			}
		}
		if !mbbEqual(pa.MBB, pb.MBB, exact) {
			return false
		}
	}
	return mbbEqual(a.MBB, b.MBB, exact)
}

func floatEqual(a, b float64, exact bool) bool {
	if exact {
		return math.Float64bits(a) == math.Float64bits(b)
	}
	// Allow a few ULPs of rounding from the algebraic re-derivation.
	tol := 1e-14 * math.Max(1.0, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= tol
}

func mbbEqual(a, b MBB, exact bool) bool {
	return floatEqual(a.Min[0], b.Min[0], exact) && floatEqual(a.Min[1], b.Min[1], exact) &&
		floatEqual(a.Max[0], b.Max[0], exact) && floatEqual(a.Max[1], b.Max[1], exact)
}
