package fonts

import (
	"encoding/base64"
	"math"
)

// kernPair is the key of the Font.Kern map: the first rune is the
// left (preceding) glyph, the second is the right (following) glyph.
type kernPair = [2]rune

// kernValue returns the kerning adjustment (in native font units) to
// apply between two consecutive glyphs. A missing pair, a non-finite
// stored value, or a zero adjustment all result in 0.0 so that layout
// falls back to the plain advance widths.
func (f *Font) kernValue(left, right rune) float64 {
	if f == nil || len(f.Kern) == 0 {
		return 0.0
	}
	k, ok := f.Kern[kernPair{left, right}]
	if !ok || k == 0.0 {
		return 0.0
	}
	if math.IsNaN(k) || math.IsInf(k, 0) {
		// An abnormal kerning value must not corrupt the layout:
		// treat it as if no pair had been stored.
		return 0.0
	}
	return k
}

// SetKern stores a single kerning adjustment (in native font units)
// for the ordered glyph pair (left, right). A non-finite value is
// silently ignored so that the pair keeps (or gets back) the default
// advance-only behavior.
//
// This is primarily intended for tests and for fonts constructed
// programmatically; generated font packages populate Kern directly.
func (f *Font) SetKern(left, right rune, adjustment float64) {
	if f == nil {
		return
	}
	if math.IsNaN(adjustment) || math.IsInf(adjustment, 0) {
		return
	}
	if f.Kern == nil {
		f.Kern = map[kernPair]float64{}
	}
	f.Kern[kernPair{left, right}] = adjustment
}

// EncodeKern serializes the font's kerning table into the compact,
// deterministic text format consumed by DecodeKern. Rune pairs are
// emitted in ascending (left, then right) order so identical tables
// always produce identical output. An empty table encodes to "".
//
// Each record is one base64-raw-url-encoded line:
//
//	base64( int32(left) || int32(right) || float64(adjustment) )
//
// using little-endian byte order (16 bytes per record before encoding).
func (f *Font) EncodeKern() string {
	if f == nil || len(f.Kern) == 0 {
		return ""
	}
	records := make([]kernRecordT, 0, len(f.Kern))
	for pair, adjust := range f.Kern {
		if math.IsNaN(adjust) || math.IsInf(adjust, 0) {
			continue
		}
		records = append(records, kernRecordT{pair[0], pair[1], adjust})
	}
	if len(records) == 0 {
		return ""
	}
	sortKernRecords(records)

	enc := base64.RawURLEncoding
	lineLen := enc.EncodedLen(kernRecordSize)
	out := make([]byte, 0, len(records)*(lineLen+1)-1)
	for i, rec := range records {
		if i > 0 {
			out = append(out, '\n')
		}
		var buf [kernRecordSize]byte
		putKernRecord(&buf, rec.left, rec.right, rec.adjust)
		start := len(out)
		out = append(out, make([]byte, lineLen)...)
		enc.Encode(out[start:], buf[:])
	}
	return string(out)
}

// DecodeKern parses data produced by EncodeKern (or the equivalent
// generator in cmd/font2go) into f.Kern.
//
// The result is deterministic: when the input contains a duplicate
// pair, the last well-formed occurrence wins. Malformed lines,
// records with non-finite adjustments, and pairs made of the NUL
// rune are skipped instead of aborting the decode.
func (f *Font) DecodeKern(data string) error {
	if f == nil {
		return errNilFont
	}
	if len(data) == 0 {
		return nil
	}
	if f.Kern == nil {
		f.Kern = map[kernPair]float64{}
	}
	dec := base64.RawURLEncoding
	for line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		raw := make([]byte, dec.DecodedLen(len(line)))
		n, err := dec.Decode(raw, []byte(line))
		if err != nil || n != kernRecordSize {
			continue // Malformed record: ignore.
		}
		left, right, adjust, ok := getKernRecord(raw)
		if !ok || left == 0 || right == 0 {
			continue
		}
		f.Kern[kernPair{left, right}] = adjust
	}
	return nil
}
