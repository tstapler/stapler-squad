package server

import "testing"

func TestSoakStatus_ExitsOne_WhenConfigDiffUnavailable(t *testing.T) {
	t.Parallel()
	src := soakTestSource(soakReadyLedger(), nil) // ConfigDiff left nil
	rec := getSoak(t, src, "class=operator&config_diff=1")
	if got := soakExit(rec); got != "1" {
		t.Errorf("an unavailable config diff must not read as clean: exit=%q", got)
	}
}
