package main

import "testing"

func TestDryRunTouchesNothing(t *testing.T) {
	if code := run([]string{"-dry-run", "-dir", t.TempDir() + "/c"}); code != 0 {
		t.Fatalf("code %d", code)
	}
}

func TestBadFlagIsUsageError(t *testing.T) {
	if code := run([]string{"-nope"}); code != 2 {
		t.Fatalf("code %d", code)
	}
}
