package sessioncreate

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestFanoutWinnerEligibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status domain.Status
		claude detection.DetectedStatus
		wantOK bool
	}{
		{"stopped", domain.Stopped, detection.StatusUnknown, true},
		{"paused", domain.Paused, detection.StatusUnknown, true},
		{"active idle", domain.Active, detection.StatusIdle, true},
		{"active ready", domain.Active, detection.StatusReady, true},
		{"active still working", domain.Active, detection.StatusProcessing, false},
		{"creating", domain.Creating, detection.StatusUnknown, false},
	}
	for _, c := range cases {
		err := fanoutWinnerEligibility(c.status, c.claude)
		if (err == nil) != c.wantOK {
			t.Errorf("%s: eligible=%v, err=%v", c.name, err == nil, err)
		}
	}
}
