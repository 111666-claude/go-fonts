package fonts

import (
	"os"
	"testing"
)

// TestMain registers the synthetic fixture fonts in the global Fonts
// map once, before any (possibly parallel) test runs. The map is
// only read afterwards, which is race-free.
func TestMain(m *testing.M) {
	// A regular kerned fixture and a kerning-less variant.
	kerned := newTestFont(nil)
	plain := newTestFont(nil)
	plain.Kern = nil
	registerTestFont(kerned)
	registerTestFont(plain)
	testFontID = kerned.ID
	testFontNoKernID = plain.ID

	os.Exit(m.Run())
}

var (
	testFontID       string
	testFontNoKernID string
)
