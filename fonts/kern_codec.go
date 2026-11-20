package fonts

import (
	"encoding/binary"
	"errors"
	"iter"
	"math"
	"sort"
)

const (
	// kernRecordSize is the on-wire size of one kerning record:
	// two int32 rune codes plus one float64 adjustment.
	kernRecordSize = 4 + 4 + 8
)

// errNilFont is returned by operations that require a non-nil font.
var errNilFont = errors.New("font must not be nil")

// kernRecordT is one entry of a kerning table, used while
// (de)serializing.
type kernRecordT struct {
	left, right rune
	adjust      float64
}

// putKernRecord writes one kerning record into buf, which must have
// at least kernRecordSize bytes of capacity.
func putKernRecord(buf *[kernRecordSize]byte, left, right rune, adjust float64) {
	binary.LittleEndian.PutUint32(buf[0:4], uint32(int32(left)))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(int32(right)))
	binary.LittleEndian.PutUint64(buf[8:16], math.Float64bits(adjust))
}

// getKernRecord reads one kerning record. ok is false when the
// adjustment is not a finite number.
func getKernRecord(raw []byte) (left, right rune, adjust float64, ok bool) {
	left = rune(int32(binary.LittleEndian.Uint32(raw[0:4])))
	right = rune(int32(binary.LittleEndian.Uint32(raw[4:8])))
	adjust = math.Float64frombits(binary.LittleEndian.Uint64(raw[8:16]))
	if math.IsNaN(adjust) || math.IsInf(adjust, 0) {
		return left, right, adjust, false
	}
	return left, right, adjust, true
}

// sortKernRecords orders records deterministically by (left, right)
// so EncodeKern is stable across map iteration orders.
func sortKernRecords(records []kernRecordT) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].left != records[j].left {
			return records[i].left < records[j].left
		}
		return records[i].right < records[j].right
	})
}

// splitLines yields the lines of s without allocating a slice.
func splitLines(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i < len(s); i++ {
			if s[i] != '\n' {
				continue
			}
			if !yield(s[start:i]) {
				return
			}
			start = i + 1
		}
		yield(s[start:])
	}
}
