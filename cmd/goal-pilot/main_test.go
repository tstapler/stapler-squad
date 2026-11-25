package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRun_should_RefuseWithExit2_When_FlagOff(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := run(context.Background(), []string{"--criteria", "x"}, false, &o, &e); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if o.Len() != 0 {
		t.Fatalf("stdout should be empty: %q", o.String())
	}
}

func TestRun_should_RequireCriteria_When_FlagOn(t *testing.T) {
	t.Parallel()
	var o, e bytes.Buffer
	if code := run(context.Background(), nil, true, &o, &e); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}
