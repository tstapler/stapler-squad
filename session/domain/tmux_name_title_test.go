package domain

import (
	"testing"
	"time"
)

func TestClaimsTitle_should_ReserveTheTmuxName_When_RenamedSessionIsStillLive(t *testing.T) {
	t.Parallel()
	now := time.Now()
	renamed := InstanceData{Title: "new", TmuxTitle: "old"}
	archived := InstanceData{Title: "new", TmuxTitle: "old", ArchivedAt: &now}

	for _, tc := range []struct {
		d     InstanceData
		title string
		want  bool
	}{{renamed, "new", true}, {renamed, "old", true}, {renamed, "other", false}, {archived, "old", false}} {
		if got := tc.d.ClaimsTitle(tc.title); got != tc.want {
			t.Errorf("ClaimsTitle(%q) on %+v = %v, want %v", tc.title, tc.d, got, tc.want)
		}
	}
	if got := renamed.TmuxNameTitle(); got != "old" {
		t.Errorf("TmuxNameTitle = %q, want old", got)
	}
}
