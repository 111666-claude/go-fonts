package webfont

import (
	"math"
	"strings"
)

// HKern represents an <hkern> XML element of an SVG webfont: a
// horizontal kerning adjustment between sets of left and right
// glyphs.
//
// Per the SVG 1.1 specification, each side is described either by a
// comma-separated list of Unicode sequences (u1/u2) or by a
// comma-separated list of glyph names (g1/g2). The k attribute is
// the adjustment in native font units; negative values move the
// right glyph closer to the left one.
type HKern struct {
	U1 string  `xml:"u1,attr,omitempty"`
	G1 string  `xml:"g1,attr,omitempty"`
	U2 string  `xml:"u2,attr,omitempty"`
	G2 string  `xml:"g2,attr,omitempty"`
	K  float64 `xml:"k,attr"`
}

// ExpandHKerns expands the <hkern> elements of a font into a simple
// rune-pair kerning table. The first rune of each pair is the left
// (preceding) glyph, the second is the right (following) glyph and
// the value is the adjustment in native font units.
//
// Each <hkern> element is the Cartesian product of its left and
// right glyph sets, matching browser behavior for SVG fonts.
//
// The result is deterministic: elements are processed in document
// order and, when the same pair is produced more than once
// (duplicate elements or overlapping sets), the last well-formed
// occurrence wins. Elements with a non-finite k, empty sides,
// unknown glyph names or entries that do not decode to a rune are
// skipped. A nil input yields an empty (non-nil) map.
func ExpandHKerns(hkerns []*HKern, glyphs []*Glyph) map[[2]rune]float64 {
	result := map[[2]rune]float64{}

	// Index the font's own glyphs both by normalized Unicode string
	// and by glyph name so u1/u2 sequences (including ligatures that
	// were remapped to a single rune) and g1/g2 names resolve the
	// same way the glyph table does.
	byUnicode := map[string]rune{}
	byName := map[string]rune{}
	for _, g := range glyphs {
		if g == nil || g.Unicode == nil {
			continue
		}
		r := UTF8toRune(g.Unicode)
		if r != 0 {
			byUnicode[*g.Unicode] = r
		}
		if g.GlyphName != nil && *g.GlyphName != "" && r != 0 {
			byName[*g.GlyphName] = r
		}
	}

	resolve := func(entry string, useNames bool) (rune, bool) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return 0, false
		}
		if useNames {
			if r, ok := byName[entry]; ok {
				return r, true
			}
			return 0, false
		}
		if r, ok := byUnicode[entry]; ok {
			return r, true
		}
		// Fall back to a plain Unicode decode for entries that do
		// not correspond to a font glyph (they will simply never
		// match rendered runes, but resolving them keeps expansion
		// total and predictable).
		if r := UTF8toRune(&entry); r != 0 {
			return r, true
		}
		return 0, false
	}

	expandSide := func(list string, useNames bool) []rune {
		var runes []rune
		for _, entry := range strings.Split(list, ",") {
			if r, ok := resolve(entry, useNames); ok {
				runes = append(runes, r)
			}
		}
		return runes
	}

	for _, hk := range hkerns {
		if hk == nil {
			continue
		}
		if math.IsNaN(hk.K) || math.IsInf(hk.K, 0) {
			continue
		}
		leftSet, leftNames := expandSide(hk.U1, false), expandSide(hk.G1, true)
		rightSet, rightNames := expandSide(hk.U2, false), expandSide(hk.G2, true)
		leftSet = append(leftSet, leftNames...)
		rightSet = append(rightSet, rightNames...)
		if len(leftSet) == 0 || len(rightSet) == 0 {
			continue
		}
		for _, left := range leftSet {
			for _, right := range rightSet {
				if left == 0 || right == 0 {
					continue
				}
				result[[2]rune{left, right}] = hk.K
			}
		}
	}

	return result
}
