package services

import (
	"testing"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

func TestResultsPanelFlagsRegisteredDefaultOff(t *testing.T) {
	for _, name := range []string{"backlog-triage-results-panel", "backlog-review-results-panel"} {
		found := false
		for _, kf := range featureflags.Current().Specs() {
			if kf.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("flag %q not registered in the flag registry", name)
		}
		if FeatureFlagDefault(name) {
			t.Errorf("flag %q must default to off", name)
		}
	}
}
