package webfont

import (
	"encoding/xml"
	"math"
	"testing"
)

func strptr(s string) *string { return &s }

// unmarshalSVG decodes a single SVG element into v, tolerating the
// default SVG namespace.
func unmarshalSVG(data []byte, v any) error {
	return xml.Unmarshal(data, v)
}

func hkernGlyphs() []*Glyph {
	return []*Glyph{
		{Unicode: strptr("A"), GlyphName: strptr("A")},
		{Unicode: strptr("V"), GlyphName: strptr("V")},
		{Unicode: strptr("T"), GlyphName: strptr("T")},
		{Unicode: strptr("o"), GlyphName: strptr("o")},
		{Unicode: strptr("X"), GlyphName: strptr("X")},
	}
}

func TestExpandHKerns(t *testing.T) {
	t.Parallel()

	glyphs := hkernGlyphs()

	t.Run("single unicode pairs AV and To", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{
			{U1: "A", U2: "V", K: -80},
			{U1: "T", U2: "o", K: -40},
		}, glyphs)

		if got := table[[2]rune{'A', 'V'}]; got != -80 {
			t.Errorf("AV kern = %v, want -80", got)
		}
		if got := table[[2]rune{'T', 'o'}]; got != -40 {
			t.Errorf("To kern = %v, want -40", got)
		}
		if len(table) != 2 {
			t.Errorf("table size = %v, want 2: %v", len(table), table)
		}
	})

	t.Run("ordered pairs are directional", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{{U1: "A", U2: "V", K: -80}}, glyphs)
		if _, ok := table[[2]rune{'V', 'A'}]; ok {
			t.Error("reverse pair should not be generated")
		}
	})

	t.Run("comma lists expand to the cartesian product", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{
			{U1: "A,T", U2: "V,o", K: -10},
		}, glyphs)
		want := [][2]rune{{'A', 'V'}, {'A', 'o'}, {'T', 'V'}, {'T', 'o'}}
		if len(table) != len(want) {
			t.Fatalf("table size = %v, want %v: %v", len(table), len(want), table)
		}
		for _, pair := range want {
			if got, ok := table[pair]; !ok || got != -10 {
				t.Errorf("pair %v = %v (ok=%v), want -10", pair, got, ok)
			}
		}
	})

	t.Run("glyph names g1/g2 resolve through the font glyphs", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{
			{G1: "A", G2: "V", K: -80},
			{G1: "T,X", G2: "o", K: -5},
		}, glyphs)
		if got := table[[2]rune{'A', 'V'}]; got != -80 {
			t.Errorf("name-based AV kern = %v, want -80", got)
		}
		if got := table[[2]rune{'T', 'o'}]; got != -5 {
			t.Errorf("name-based To kern = %v, want -5", got)
		}
		if got := table[[2]rune{'X', 'o'}]; got != -5 {
			t.Errorf("name-based Xo kern = %v, want -5", got)
		}
	})

	t.Run("duplicate pair is last-write-wins", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{
			{U1: "A", U2: "V", K: -80},
			{U1: "A", U2: "V", K: -25},
		}, glyphs)
		if got := table[[2]rune{'A', 'V'}]; got != -25 {
			t.Errorf("duplicate pair = %v, want last value -25", got)
		}
	})

	t.Run("abnormal k values are dropped", func(t *testing.T) {
		table := ExpandHKerns([]*HKern{
			{U1: "A", U2: "V", K: math.NaN()},
			{U1: "T", U2: "o", K: math.Inf(1)},
			{U1: "X", U2: "X", K: math.Inf(-1)},
			{U1: "A", U2: "X", K: 0},
		}, glyphs)
		// Only the finite zero adjustment survives.
		if len(table) != 1 {
			t.Fatalf("table size = %v, want 1: %v", len(table), table)
		}
		if got, ok := table[[2]rune{'A', 'X'}]; !ok || got != 0 {
			t.Errorf("zero-k pair = %v (ok=%v), want 0", got, ok)
		}
	})

	t.Run("unknown names and empty sides produce nothing", func(t *testing.T) {
		for _, hk := range []*HKern{
			{G1: "A", G2: "nonexistent", K: -1},
			{U1: "A", K: -1},                  // No right side.
			{U2: "V", K: -1},                  // No left side.
			{U1: "", U2: "", K: -1},           // Both empty.
			nil,                               // Nil element.
			{U1: "A", U2: "V", K: math.NaN()}, // Abnormal.
		} {
			if got := ExpandHKerns([]*HKern{hk}, glyphs); len(got) != 0 {
				t.Errorf("hk=%+v produced pairs: %v", hk, got)
			}
		}
	})

	t.Run("nil input is safe", func(t *testing.T) {
		if got := ExpandHKerns(nil, glyphs); got == nil || len(got) != 0 {
			t.Errorf("nil hkerns = %v, want empty non-nil map", got)
		}
		if got := ExpandHKerns(nil, nil); got == nil || len(got) != 0 {
			t.Errorf("nil inputs = %v, want empty non-nil map", got)
		}
	})

	t.Run("positive adjustment is preserved", func(t *testing.T) {
		// Not every kern tightens; a positive value pushes glyphs apart.
		table := ExpandHKerns([]*HKern{{U1: "X", U2: "X", K: 30}}, glyphs)
		if got := table[[2]rune{'X', 'X'}]; got != 30 {
			t.Errorf("positive kern = %v, want 30", got)
		}
	})
}

// TestHKernXMLUnmarshal verifies the encoding/xml tags capture the
// four SVG 1.1 <hkern> attributes plus k.
func TestHKernXMLUnmarshal(t *testing.T) {
	t.Parallel()

	xmlData := []byte(`<hkern xmlns="http://www.w3.org/2000/svg" u1="A" g1="ag" u2="V" g2="vg" k="-80"/>`)
	var hk HKern
	if err := unmarshalSVG(xmlData, &hk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if hk.U1 != "A" || hk.G1 != "ag" || hk.U2 != "V" || hk.G2 != "vg" || hk.K != -80 {
		t.Errorf("unmarshaled hkern = %+v", hk)
	}
}
