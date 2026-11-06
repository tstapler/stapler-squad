package config

import "testing"

func TestSessionMultiPR_ShouldApplyReaderOrderAtReadTime(t *testing.T) {
	t.Parallel()
	scopes := []string{SessionMultiPRScopePoller, SessionMultiPRScopeBacklog, SessionMultiPRScopeActions, SessionMultiPRScopeUI}
	resolved := func(c *Config) []bool {
		out := make([]bool, len(scopes))
		for i, s := range scopes {
			out[i] = c.SessionMultiPR(s)
		}
		return out
	}
	equal := func(got, want []bool) bool {
		for i := range want {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	c := &Config{}
	if got := resolved(c); !equal(got, []bool{false, false, false, false}) {
		t.Fatalf("defaults: %v", got)
	}

	// A later scope stored first stays off until the earlier ones are on.
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopeUI, true)
	if got := resolved(c); !equal(got, []bool{false, false, false, false}) {
		t.Fatalf("ui stored alone: %v", got)
	}
	for i, s := range scopes[:3] {
		c.ApplyFeatureFlagScope(SessionMultiPRFlag, s, true)
		want := make([]bool, 4)
		for j := 0; j <= i; j++ {
			want[j] = true
		}
		if i == 2 {
			want[3] = true // ui was stored first and now its three prerequisites are on
		}
		if got := resolved(c); !equal(got, want) {
			t.Fatalf("after enabling %s: %v, want %v", s, got, want)
		}
	}

	// Disabling the poller turns every later reader off at once, and re-enabling restores them.
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopePoller, false)
	if got := resolved(c); !equal(got, []bool{false, false, false, false}) {
		t.Fatalf("poller off: %v", got)
	}
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopePoller, true)
	if got := resolved(c); !equal(got, []bool{true, true, true, true}) {
		t.Fatalf("poller back on: %v", got)
	}
	if c.SessionMultiPR("reader:nope") || c.SessionMultiPR(SessionMultiPRScopeRefresherOff) {
		t.Error("an unknown scope, and the kill-switch scope, are never readers")
	}
}

func TestSessionMultiPRRefresher_ShouldBeOnUnlessKillSwitchSetExplicitly(t *testing.T) {
	t.Parallel()
	c := &Config{}
	if !c.SessionMultiPRRefresher() {
		t.Fatal("refresher must run by default")
	}
	c.ApplyFeatureFlag(SessionMultiPRFlag, true)
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopePoller, true)
	if !c.SessionMultiPRRefresher() {
		t.Fatal("a global flip and a reader scope must not change the refresher")
	}
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopeRefresherOff, true)
	if c.SessionMultiPRRefresher() {
		t.Fatal("refresher_off=true must stop the refresher")
	}
	c.ApplyFeatureFlagScope(SessionMultiPRFlag, SessionMultiPRScopeRefresherOff, false)
	if !c.SessionMultiPRRefresher() {
		t.Fatal("an explicit false keeps the refresher on")
	}
	if !c.SessionMultiPR(SessionMultiPRScopePoller) {
		t.Error("the kill switch is not in the reader order")
	}
}

func TestArtifactScanV2Enabled_ShouldDefaultOff(t *testing.T) {
	t.Parallel()
	c := &Config{}
	if c.ArtifactScanV2Enabled() {
		t.Fatal("default must be off")
	}
	c.ApplyFeatureFlag(ArtifactScanV2Flag, true)
	if !c.ArtifactScanV2Enabled() {
		t.Fatal("explicit on must enable")
	}
}
