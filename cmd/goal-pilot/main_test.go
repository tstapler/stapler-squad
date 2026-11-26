package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRun_should_RefuseWithExit2_When_FlagOff(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := run(context.Background(), []string{"--criteria", "x", "--dir", t.TempDir()}, false, &o, &e); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if o.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", o.String())
	}
	if !strings.Contains(e.String(), "feature flag") {
		t.Fatalf("stderr should name the feature flag: %q", e.String())
	}
}

func TestRun_should_RequireCriteria_When_FlagOn(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := run(context.Background(), []string{"--dir", t.TempDir()}, true, &o, &e); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(e.String(), "--criteria is required") {
		t.Fatalf("stderr should say criteria is required: %q", e.String())
	}
}

func TestRun_should_RejectMissingDir_When_FlagOn(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--criteria", "x"},
		{"--criteria", "x", "--dir", "/nonexistent-goal-pilot-dir"},
	} {
		var o, e bytes.Buffer
		if code := run(context.Background(), args, true, &o, &e); code != 2 {
			t.Fatalf("args %v: code = %d, want 2", args, code)
		}
		if o.Len() != 0 {
			t.Fatalf("args %v: stdout should be empty: %q", args, o.String())
		}
	}
}
