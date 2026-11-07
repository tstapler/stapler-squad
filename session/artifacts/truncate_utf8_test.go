package artifacts

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytes_ShouldNotSplitRune_WhenCutLandsMidRune(t *testing.T) {
	t.Parallel()
	// 3-byte runes: a cut at byte 512 lands mid-rune (512 % 3 != 0).
	got := truncateBytes(strings.Repeat("日", 400), MaxResultBytes)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateBytes produced invalid UTF-8")
	}
	if len(got) > MaxResultBytes {
		t.Fatalf("len=%d exceeds %d", len(got), MaxResultBytes)
	}
}

func TestTruncate_ShouldNotSplitRune_WhenCommandOverCap(t *testing.T) {
	t.Parallel()
	got := truncate(strings.Repeat("é", 300), 201) // odd cap splits a 2-byte rune
	if !utf8.ValidString(got) {
		t.Fatalf("truncate produced invalid UTF-8")
	}
}

func TestCommandRecord_ShouldHaveValidUTF8Result_WhenOutputTruncatedMidRune(t *testing.T) {
	t.Parallel()
	call := pendingCall{cmd: &CommandArtifact{Type: "git_commit", Command: "git commit"}}
	rec := commandRecord(call, strings.Repeat("日", 400))
	if !utf8.ValidString(rec.Result) {
		t.Fatalf("Result is invalid UTF-8")
	}
}
