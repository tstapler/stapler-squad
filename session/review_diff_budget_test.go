package session

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBudget = 40_000

func fileDiff(path string, lines int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n", path, path, path, path)
	for i := 0; i < lines; i++ {
		b.WriteString("+" + strings.Repeat("x", 99) + "\n")
	}
	return b.String()
}

func TestTruncateDiffByFile_ExactBudgetBoundary(t *testing.T) {
	t.Parallel()
	raw := fileDiff("a.go", 10) + fileDiff("b.go", 10)
	for _, tc := range []struct {
		name  string
		limit int
		trunc bool
	}{{"limit==len", len(raw), false}, {"limit==len-1", len(raw) - 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, trunc := TruncateDiffByFile(raw, tc.limit)
			assert.Equal(t, tc.trunc, trunc)
			assert.LessOrEqual(t, len(got), tc.limit)
		})
	}
	got, trunc := TruncateDiffByFile("", 10)
	assert.Equal(t, "", got)
	assert.False(t, trunc)
}

func TestTruncateDiffByFile_RealCodeRankedAheadOfNoise(t *testing.T) {
	t.Parallel()
	// 3 CI files (~30KB each) + 1 real file (~7.5KB): the budget fits the real file
	// whole plus only part of the noise, so ranking decides who is cut.
	raw := fileDiff(".github/workflows/a.yml", 300) + fileDiff(".github/workflows/b.yml", 300) +
		fileDiff(".claude/settings.local.json", 300) + fileDiff("session/real.go", 70)
	limit := diffManifestReserve + 2*diffPerFileCap
	got, trunc := TruncateDiffByFile(raw, limit)
	require.True(t, trunc)
	assert.LessOrEqual(t, len(got), limit)
	assert.Contains(t, got, fileDiff("session/real.go", 70), "real file must be kept whole")
	assert.Contains(t, got, "cut: .github/workflows/a.yml")
	assert.Contains(t, got, "omitted:")
	assert.Less(t, strings.Index(got, ".github/workflows/a.yml"), strings.Index(got, "session/real.go"), "original file order kept")
}

func TestTruncateDiffByFile_LeftoverBudgetRefillsCutFiles(t *testing.T) {
	t.Parallel()
	raw := fileDiff("big.go", 300) + fileDiff("small.go", 5) + fileDiff("mid.go", 100)
	require.Greater(t, len(raw), testBudget)
	got, trunc := TruncateDiffByFile(raw, testBudget)
	require.True(t, trunc)
	assert.LessOrEqual(t, len(got), testBudget)
	assert.Greater(t, len(got), testBudget-diffManifestReserve-2*100, "per-file cap must not strand budget")
	assert.Contains(t, got, fileDiff("small.go", 5))
	assert.Contains(t, got, "diff --git a/mid.go", "every file still represented")
}

func TestTruncateDiffByFile_ManifestMatchesOutput(t *testing.T) {
	t.Parallel()
	const n = 30
	var raw string
	for i := 0; i < n; i++ {
		raw += fileDiff(fmt.Sprintf("pkg/f%02d.go", i), 60)
	}
	got, trunc := TruncateDiffByFile(raw, testBudget)
	require.True(t, trunc)
	assert.LessOrEqual(t, len(got), testBudget)
	shown := strings.Count(got, "diff --git ")
	assert.Contains(t, got, fmt.Sprintf("%d of %d files shown", shown, n))
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("pkg/f%02d.go", i)
		inBody := strings.Contains(got, "diff --git a/"+name)
		assert.True(t, inBody || strings.Contains(got, name), "%s neither shown nor named in manifest", name)
	}
}

func TestTruncateDiffByFile_NeverExceedsLimitAndStaysValidUTF8(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		raw   string
		limit int
	}{
		{"single huge file", fileDiff("a.go", 1000), testBudget / 4},
		{"non-diff text", strings.Repeat("héllo wörld ", 5000), testBudget / 4},
		{"multibyte lines", "diff --git a/a b/a\n" + strings.Repeat("+日本語日本語日本語\n", 5000) + fileDiff("b.go", 1), testBudget / 4},
		{"manifest over reserve", func() string {
			var s string
			for i := 0; i < 150; i++ {
				s += fileDiff(fmt.Sprintf("very/long/path/segment/%s/f%03d.go", strings.Repeat("d", 40), i), 40)
			}
			return s
		}(), testBudget},
		{"limit below manifest reserve", fileDiff("a.go", 50) + fileDiff("b.go", 50), 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, trunc := TruncateDiffByFile(tc.raw, tc.limit)
			assert.True(t, trunc)
			assert.LessOrEqual(t, len(got), tc.limit)
			assert.True(t, utf8.ValidString(got))
			if strings.Contains(got, "[diff truncated") {
				assert.True(t, strings.HasSuffix(got, "]\n"))
			}
		})
	}
}

func TestSplitDiffFiles_Paths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw, path string
		noise           bool
	}{
		{"plain", "diff --git a/x.go b/x.go\n+1\n", "x.go", false},
		{"rename uses b side", "diff --git a/old.go b/.github/new.yml\nrename from old.go\n", ".github/new.yml", true},
		{"quoted path with space", "diff --git \"a/dir/x y.go\" \"b/dir/x y.go\"\n+1\n", "dir/x y.go", false},
		{"quoted ci path", "diff --git \"a/.github/é.yml\" \"b/.github/é.yml\"\n+1\n", ".github/é.yml", true},
		{"lockfile", "diff --git a/Cargo.lock b/Cargo.lock\n+1\n", "Cargo.lock", true},
		{"binary", "diff --git a/img.png b/img.png\nBinary files differ\n", "img.png", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := splitDiffFiles(tc.raw)
			require.Len(t, files, 1)
			assert.Equal(t, tc.path, files[0].path)
			assert.Equal(t, tc.noise, files[0].noise)
		})
	}
	files := splitDiffFiles("preamble\n" + fileDiff("a.go", 1) + fileDiff("b.go", 1))
	require.Len(t, files, 3)
	assert.Equal(t, "", files[0].path)
}
