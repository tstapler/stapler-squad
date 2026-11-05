package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCachedMatchesBazelSummaryLines(t *testing.T) {
	if !cached("//a:t                                   (cached) PASSED in 0.1s") {
		t.Fatal("cached PASSED")
	}
	if cached("//a:t                                   PASSED in 0.1s") {
		t.Fatal("fresh run is not cached")
	}
	if !cached("//a:t                                   (cached) FAILED in 0.1s") {
		t.Fatal("cached FAILED")
	}
}

func TestEditRestores(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	_ = os.WriteFile(p, []byte("a\n"), 0o644)
	restore := edit(p, "b\n")
	if b, _ := os.ReadFile(p); string(b) != "a\nb\n" {
		t.Fatalf("%q", b)
	}
	restore()
	if b, _ := os.ReadFile(p); string(b) != "a\n" {
		t.Fatalf("%q", b)
	}
}
