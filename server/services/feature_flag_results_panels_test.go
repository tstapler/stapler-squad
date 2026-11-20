package services

import (
	"testing"
)

func TestResultsPanelFlagsRegisteredDefaultOff(t *testing.T) {
	for _, name := range []string{"backlog-triage-results-panel", "backlog-review-results-panel"} {
		found := false
		for _, kf := range testFlagRegistry().Specs() {
			if kf.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("flag %q not registered in the flag registry", name)
		}
		if testFlagRegistry().Default(name) {
			t.Errorf("flag %q must default to off", name)
		}
	}
}
