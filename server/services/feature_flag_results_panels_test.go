package services

import "testing"

func TestResultsPanelFlagsRegisteredDefaultOff(t *testing.T) {
	for _, name := range []string{"backlog-triage-results-panel", "backlog-review-results-panel"} {
		found := false
		for _, kf := range knownFeatureFlags {
			if kf.name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("flag %q not registered in knownFeatureFlags", name)
		}
		if featureFlagDefault(name) {
			t.Errorf("flag %q must default to off", name)
		}
	}
}
