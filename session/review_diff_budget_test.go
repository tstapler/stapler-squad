package session

import (
	"strings"
	"testing"
)

func fileDiff(path string, size int) string {
	return "diff --git a/" + path + " b/" + path + "\n--- a/" + path + "\n+++ b/" + path + "\n@@ -1 +1 @@\n+" + strings.Repeat("x", size) + "\n"
}

func TestTruncateDiffByFile_FitsUnchanged(t *testing.T) {
	t.Parallel()
	raw := fileDiff("a.go", 100)
	got, trunc := TruncateDiffByFile(raw, 40_000)
	if trunc || got != raw {
		t.Fatalf("expected passthrough, trunc=%v", trunc)
	}
}

func TestTruncateDiffByFile_CIDoesNotStarveRealCode(t *testing.T) {
	t.Parallel()
	raw := fileDiff(".github/workflows/ci.yml", 30_000) + fileDiff(".claude/settings.local.json", 30_000) + fileDiff("session/real.go", 2_000)
	got, trunc := TruncateDiffByFile(raw, 40_000)
	if !trunc {
		t.Fatal("expected truncation")
	}
	if len(got) > 40_000 {
		t.Fatalf("over budget: %d", len(got))
	}
	if !strings.Contains(got, "diff --git a/session/real.go") || !strings.Contains(got, strings.Repeat("x", 2_000)) {
		t.Fatal("real code file must survive in full")
	}
	if !strings.Contains(got, "[diff truncated to fit review budget") {
		t.Fatal("manifest missing")
	}
	// The old byte-prefix cut would have dropped real.go entirely.
	if strings.Contains(raw[:40_000], "session/real.go") {
		t.Fatal("test premise broken: prefix cut keeps real.go")
	}
}

func TestTruncateDiffByFile_ManyFilesAllNamed(t *testing.T) {
	t.Parallel()
	var raw string
	for i := 0; i < 30; i++ {
		raw += fileDiff("pkg/f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".go", 6_000)
	}
	got, trunc := TruncateDiffByFile(raw, 40_000)
	if !trunc || len(got) > 40_000 {
		t.Fatalf("trunc=%v len=%d", trunc, len(got))
	}
	if !strings.Contains(got, "omitted:") {
		t.Fatal("manifest must list omitted files")
	}
}
