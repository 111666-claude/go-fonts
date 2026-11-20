package fonts

import (
	"math"
	"strings"
	"testing"
)

// TestKernValue covers table lookup semantics including abnormal
// values and missing pairs.
func TestKernValue(t *testing.T) {
	t.Parallel()

	font := newTestFont(t)

	tests := []struct {
		name        string
		left, right rune
		want        float64
	}{
		{"AV pair", 'A', 'V', -80},
		{"To pair", 'T', 'o', -40},
		{"unknown pair", 'A', 'X', 0},
		{"reversed pair is distinct", 'V', 'A', 0},
		{"same rune pair", 'A', 'A', 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := font.kernValue(tt.left, tt.right); got != tt.want {
				t.Errorf("kernValue(%q,%q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}

	// A nil font and an empty table always yield zero.
	var nilFont *Font
	if got := nilFont.kernValue('A', 'V'); got != 0 {
		t.Errorf("nil font kernValue = %v, want 0", got)
	}
	tableless := &Font{Glyphs: map[rune]*Glyph{}}
	if got := tableless.kernValue('A', 'V'); got != 0 {
		t.Errorf("tableless font kernValue = %v, want 0", got)
	}

	// Abnormal stored values must behave like missing pairs. Use a
	// local throwaway table so the shared fixture is untouched.
	bad := &Font{Kern: map[kernPair]float64{
		{'X', 'X'}: math.NaN(),
		{'X', 'A'}: math.Inf(1),
		{'X', 'V'}: math.Inf(-1),
	}}
	for _, pair := range []kernPair{{'X', 'X'}, {'X', 'A'}, {'X', 'V'}} {
		if got := bad.kernValue(pair[0], pair[1]); got != 0 {
			t.Errorf("abnormal kern %v = %v, want 0", pair, got)
		}
	}
}

// TestSetKern checks the programmatic setter, including non-finite
// rejection.
func TestSetKern(t *testing.T) {
	t.Parallel()

	font := &Font{}
	font.SetKern('A', 'V', -80)
	if got := font.kernValue('A', 'V'); got != -80 {
		t.Fatalf("SetKern then lookup = %v, want -80", got)
	}

	font.SetKern('A', 'V', math.NaN())
	font.SetKern('T', 'o', math.Inf(1))
	if got := font.kernValue('A', 'V'); got != -80 {
		t.Errorf("NaN SetKern overwrote pair: got %v, want -80", got)
	}
	if _, ok := font.Kern[kernPair{'T', 'o'}]; ok {
		t.Errorf("+Inf SetKern inserted a pair: %v", font.Kern)
	}

	var nilFont *Font // Must not panic.
	nilFont.SetKern('A', 'V', -10)
}

// TestKernCodecRoundTrip verifies EncodeKern/DecodeKern preserve the
// table, that encoding is deterministic across map orders and that
// malformed or duplicated input has a defined result.
func TestKernCodecRoundTrip(t *testing.T) {
	t.Parallel()

	font := newTestFont(t)
	encoded := font.EncodeKern()
	if encoded == "" {
		t.Fatal("EncodeKern returned empty string for non-empty table")
	}
	// Deterministic output: encoding again must be byte-identical.
	if again := font.EncodeKern(); again != encoded {
		t.Errorf("EncodeKern not deterministic:\n%q\nvs\n%q", encoded, again)
	}
	// Records are newline separated and fixed width.
	lines := strings.Split(encoded, "\n")
	if len(lines) != len(font.Kern) {
		t.Fatalf("encoded %v lines, want %v: %q", len(lines), len(font.Kern), encoded)
	}
	if lines[0] == lines[1] {
		t.Fatalf("encoded records are identical: %q", lines)
	}

	decoded := &Font{}
	if err := decoded.DecodeKern(encoded); err != nil {
		t.Fatalf("DecodeKern: %v", err)
	}
	if len(decoded.Kern) != len(font.Kern) {
		t.Fatalf("decoded %v pairs, want %v", len(decoded.Kern), len(font.Kern))
	}
	for pair, want := range font.Kern {
		if got, ok := decoded.Kern[pair]; !ok || got != want {
			t.Errorf("decoded pair %v = %v (ok=%v), want %v", pair, got, ok, want)
		}
	}

	// Empty and nil-ish cases.
	empty := &Font{}
	if got := empty.EncodeKern(); got != "" {
		t.Errorf("empty font EncodeKern = %q, want empty", got)
	}
	if err := empty.DecodeKern(""); err != nil {
		t.Errorf("DecodeKern(\"\") returned error: %v", err)
	}
	var nilFont *Font
	if err := nilFont.DecodeKern(encoded); err == nil {
		t.Error("nil font DecodeKern should return an error")
	}

	// Decode tolerates garbage lines and skips malformed records;
	// duplicate well-formed pairs resolve last-write-wins.
	dupFont := &Font{}
	first := (&Font{Kern: map[kernPair]float64{{'A', 'V'}: -80}}).EncodeKern()
	second := (&Font{Kern: map[kernPair]float64{{'A', 'V'}: -55}}).EncodeKern()
	mixed := "!!!not-base64!!!\n" + first + "\n\n" + second + "\n" + strings.Repeat("A", 4)
	if err := dupFont.DecodeKern(mixed); err != nil {
		t.Fatalf("DecodeKern mixed input: %v", err)
	}
	if got := dupFont.kernValue('A', 'V'); got != -55 {
		t.Errorf("duplicate pair = %v, want last-write-wins -55", got)
	}

	// Non-finite adjustments never make it into an encoded table.
	abnormal := &Font{Kern: map[kernPair]float64{
		{'A', 'V'}: math.NaN(),
		{'T', 'o'}: -40,
	}}
	if got := abnormal.EncodeKern(); strings.Count(got, "\n") != 0 {
		t.Errorf("EncodeKern emitted NaN record: %q", got)
	}
}
