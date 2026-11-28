package linediff

import (
	"testing"
)

// TestLcsLength_CapsAtMaxLines verifies both lcsLength and lcsLengthBytes
// skip the O(n*m) DP entirely once either input exceeds MaxLCSLines,
// returning 0 (which callers' len(new)-lcs / len(old)-lcs formulas turn into
// a bounded "fully replaced" approximation) instead of running the full DP.
func TestLcsLength_CapsAtMaxLines(t *testing.T) {
	origLimit := MaxLCSLines
	MaxLCSLines = 3
	defer func() { MaxLCSLines = origLimit }()

	t.Run("string lines", func(t *testing.T) {
		linesA := []string{"a", "b", "c", "d", "e"} // len=5 > cap=3, identical on both sides
		linesB := []string{"a", "b", "c", "d", "e"}
		if got := lcsLength(linesA, linesB); got != 0 {
			t.Errorf("lcsLength = %d, want 0 (over the cap, even though inputs are identical)", got)
		}
		// Sanity: at/under the cap, the real DP still runs and computes correctly.
		small := []string{"a", "b"}
		if got := lcsLength(small, small); got != 2 {
			t.Errorf("lcsLength = %d, want 2 for input at/under the cap", got)
		}
	})

	t.Run("byte lines", func(t *testing.T) {
		linesA := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d"), []byte("e")}
		linesB := [][]byte{[]byte("a"), []byte("b"), []byte("c"), []byte("d"), []byte("e")}
		if got := lcsLengthBytes(linesA, linesB); got != 0 {
			t.Errorf("lcsLengthBytes = %d, want 0 (over the cap, even though inputs are identical)", got)
		}
		small := [][]byte{[]byte("a"), []byte("b")}
		if got := lcsLengthBytes(small, small); got != 2 {
			t.Errorf("lcsLengthBytes = %d, want 2 for input at/under the cap", got)
		}
	})
}
