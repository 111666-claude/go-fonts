package main

import (
	"bytes"
	"go/format"
	"strings"
	"testing"

	"github.com/gmlewis/go-fonts/fonts"
	"github.com/gmlewis/go-fonts/webfont"
)

// TestWriteFontTemplateIncludesKernData drives the same template and
// kerning serialization as writeFont for a fixture SVG font carrying
// AV and To hkern pairs, then verifies that:
//
//   - the generated Go source is well formed (gofmt accepts it),
//   - it declares kernData and calls DecodeKern,
//   - the embedded payload decodes back to the same table.
func TestWriteFontTemplateIncludesKernData(t *testing.T) {
	t.Parallel()

	font := &webfont.Font{
		ID:        "fixturekern",
		HorizAdvX: 200,
		FontFace:  &webfont.FontFace{UnitsPerEm: 1000, Ascent: 800, Descent: -200},
		MissingGlyph: &webfont.MissingGlyph{
			HorizAdvX: 200,
		},
		Glyphs: []*webfont.Glyph{
			{Unicode: strPtr("A"), GlyphName: strPtr("A"), HorizAdvX: 200},
			{Unicode: strPtr("V"), GlyphName: strPtr("V"), HorizAdvX: 200},
			{Unicode: strPtr("T"), GlyphName: strPtr("T"), HorizAdvX: 200},
			{Unicode: strPtr("o"), GlyphName: strPtr("o"), HorizAdvX: 200},
		},
		HKerns: []*webfont.HKern{
			{U1: "A", U2: "V", K: -80},
			{U1: "T", U2: "o", K: -40},
		},
	}

	// Mirror writeFont's kerning handling exactly.
	kernFont := &fonts.Font{Kern: webfont.ExpandHKerns(font.HKerns, font.Glyphs)}
	font.KernData = kernFont.EncodeKern()
	if font.KernData == "" {
		t.Fatal("expected non-empty KernData")
	}

	var buf bytes.Buffer
	if err := outTemp.Execute(&buf, font); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	source := buf.String()

	// The emitted source must be syntactically valid Go.
	if _, err := format.Source(buf.Bytes()); err != nil {
		t.Errorf("generated source is not gofmt-clean: %v", err)
	}

	for _, want := range []string{
		"var kernData = ",
		"DecodeKern(kernData)",
		`Kern:             map[[2]rune]float64{}`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source missing %q", want)
		}
	}

	// Extract and decode the payload exactly as the generated init
	// would, using a fresh runtime font.
	start := strings.Index(source, "var kernData = `")
	if start < 0 {
		t.Fatal("kernData declaration not found")
	}
	payloadStart := start + len("var kernData = `")
	payloadEnd := strings.Index(source[payloadStart:], "`")
	if payloadEnd < 0 {
		t.Fatal("unterminated kernData literal")
	}
	payload := source[payloadStart : payloadStart+payloadEnd]

	decoded := &fonts.Font{}
	if err := decoded.DecodeKern(payload); err != nil {
		t.Fatalf("DecodeKern: %v", err)
	}
	if got := decoded.Kern[[2]rune{'A', 'V'}]; got != -80 {
		t.Errorf("decoded AV = %v, want -80", got)
	}
	if got := decoded.Kern[[2]rune{'T', 'o'}]; got != -40 {
		t.Errorf("decoded To = %v, want -40", got)
	}
}

// TestWriteFontTemplateWithoutKern makes sure a font without any
// hkern elements still generates valid code with an empty payload.
func TestWriteFontTemplateWithoutKern(t *testing.T) {
	t.Parallel()

	font := &webfont.Font{
		ID:           "plainfont",
		HorizAdvX:    200,
		FontFace:     &webfont.FontFace{UnitsPerEm: 1000, Ascent: 800, Descent: -200},
		MissingGlyph: &webfont.MissingGlyph{HorizAdvX: 200},
		Glyphs:       []*webfont.Glyph{{Unicode: strPtr("A"), HorizAdvX: 200}},
	}
	kernFont := &fonts.Font{Kern: webfont.ExpandHKerns(font.HKerns, font.Glyphs)}
	font.KernData = kernFont.EncodeKern()

	var buf bytes.Buffer
	if err := outTemp.Execute(&buf, font); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	if _, err := format.Source(buf.Bytes()); err != nil {
		t.Errorf("generated source is not gofmt-clean: %v", err)
	}
	if !strings.Contains(buf.String(), "var kernData = `") {
		t.Error("kernData declaration missing even for kerning-less fonts")
	}
	decoded := &fonts.Font{}
	if err := decoded.DecodeKern(font.KernData); err != nil {
		t.Errorf("DecodeKern on empty payload: %v", err)
	}
	if len(decoded.Kern) != 0 {
		t.Errorf("expected empty table, got %v", decoded.Kern)
	}
}

func strPtr(s string) *string { return &s }
