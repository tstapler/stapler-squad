package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestGetFeatureFlag_NilConfig verifies that GetFeatureFlag on a nil *Config returns false.
func TestGetFeatureFlag_NilConfig(t *testing.T) {
	var cfg *Config
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag on nil config: got %v, want false", got)
	}
}

// TestGetFeatureFlag_NilMap verifies that a non-nil Config with a nil FeatureFlags map returns false.
func TestGetFeatureFlag_NilMap(t *testing.T) {
	cfg := &Config{} // FeatureFlags is nil by zero value
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag with nil map: got %v, want false", got)
	}
}

// TestGetFeatureFlag_MissingKey verifies that an absent key returns false.
func TestGetFeatureFlag_MissingKey(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"other": true,
		},
	}
	if got := cfg.GetFeatureFlag("backlog"); got != false {
		t.Errorf("GetFeatureFlag for missing key: got %v, want false", got)
	}
}

// TestGetFeatureFlag_Present verifies that a key in the map returns the stored value.
func TestGetFeatureFlag_Present(t *testing.T) {
	tests := []struct {
		name  string
		value bool
	}{
		{"returns true when flag is true", true},
		{"returns false when flag is explicitly false", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				FeatureFlags: map[string]bool{
					"backlog": tc.value,
				},
			}
			if got := cfg.GetFeatureFlag("backlog"); got != tc.value {
				t.Errorf("GetFeatureFlag: got %v, want %v", got, tc.value)
			}
		})
	}
}

// TestGetFeatureFlagWithDefault_NilConfig verifies that GetFeatureFlagWithDefault on a
// nil *Config returns defaultValue, not false.
func TestGetFeatureFlagWithDefault_NilConfig(t *testing.T) {
	var cfg *Config
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault on nil config: got %v, want true", got)
	}
}

// TestGetFeatureFlagWithDefault_NilMap verifies that a non-nil Config with a nil
// FeatureFlags map returns defaultValue.
func TestGetFeatureFlagWithDefault_NilMap(t *testing.T) {
	cfg := &Config{} // FeatureFlags is nil by zero value
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault with nil map: got %v, want true", got)
	}
}

// TestGetFeatureFlagWithDefault_MissingKey verifies that a genuinely absent key falls
// through to defaultValue, not false.
func TestGetFeatureFlagWithDefault_MissingKey(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"other": true,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != true {
		t.Errorf("GetFeatureFlagWithDefault for missing key: got %v, want true (the default)", got)
	}
}

// TestGetFeatureFlagWithDefault_ExplicitFalseOverridesDefault is the regression case
// this helper exists for: a flag graduated to "default true" must still respect an
// explicitly persisted false, distinguishing "never set" from "set to false" — which a
// plain map[string]bool lookup alone (GetFeatureFlag) cannot do.
func TestGetFeatureFlagWithDefault_ExplicitFalseOverridesDefault(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"terminal:resync-exec-gate-fast-lane": false,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", true); got != false {
		t.Errorf("GetFeatureFlagWithDefault: got %v, want false (explicit override of the true default)", got)
	}
}

// TestGetFeatureFlagWithDefault_ExplicitTrueMatchesDefault verifies the unsurprising
// case: an explicit true reads back as true regardless of defaultValue.
func TestGetFeatureFlagWithDefault_ExplicitTrueMatchesDefault(t *testing.T) {
	cfg := &Config{
		FeatureFlags: map[string]bool{
			"terminal:resync-exec-gate-fast-lane": true,
		},
	}
	if got := cfg.GetFeatureFlagWithDefault("terminal:resync-exec-gate-fast-lane", false); got != true {
		t.Errorf("GetFeatureFlagWithDefault: got %v, want true (explicit value, defaultValue irrelevant)", got)
	}
}

// TestSetFeatureFlag_InitializesMap verifies that SetFeatureFlag works when FeatureFlags is nil,
// initializing the map and persisting the flag to disk.
func TestSetFeatureFlag_InitializesMap(t *testing.T) {
	// Direct the config save to a temp directory so we don't touch real state.
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

	cfg := &Config{} // FeatureFlags is nil

	if err := cfg.SetFeatureFlag("backlog", true); err != nil {
		t.Fatalf("SetFeatureFlag returned error: %v", err)
	}

	// Map must be initialized and contain the flag.
	if cfg.FeatureFlags == nil {
		t.Fatal("SetFeatureFlag did not initialize FeatureFlags map")
	}
	if got := cfg.FeatureFlags["backlog"]; got != true {
		t.Errorf("FeatureFlags[backlog]: got %v, want true", got)
	}

	// Verify that the config was written to disk and is readable back.
	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath after SetFeatureFlag: %v", err)
	}
	if !reloaded.GetFeatureFlag("backlog") {
		t.Error("persisted config does not have backlog flag set to true")
	}
}

// TestSetFeatureFlag_UpdatesExistingMap verifies that SetFeatureFlag updates a pre-existing map
// and persists correctly.
func TestSetFeatureFlag_UpdatesExistingMap(t *testing.T) {
	tempHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origInstance := os.Getenv("STAPLER_SQUAD_INSTANCE")
	os.Setenv("HOME", tempHome)
	os.Setenv("STAPLER_SQUAD_INSTANCE", "shared")
	defer func() {
		os.Setenv("HOME", origHome)
		if origInstance == "" {
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
		} else {
			os.Setenv("STAPLER_SQUAD_INSTANCE", origInstance)
		}
	}()

	cfg := &Config{
		FeatureFlags: map[string]bool{
			"backlog": true,
		},
	}

	// Disable the flag.
	if err := cfg.SetFeatureFlag("backlog", false); err != nil {
		t.Fatalf("SetFeatureFlag returned error: %v", err)
	}

	if cfg.GetFeatureFlag("backlog") {
		t.Error("expected GetFeatureFlag to return false after SetFeatureFlag(false)")
	}

	// Verify persistence.
	configPath := filepath.Join(tempHome, ".stapler-squad", ConfigFileName)
	reloaded, err := LoadConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if reloaded.GetFeatureFlag("backlog") {
		t.Error("persisted config still shows backlog as true after disabling")
	}
}

func TestGetFeatureFlag_should_ReturnFalse_When_InboxPRWatchNeverSet(t *testing.T) {
	var nilCfg *Config
	if nilCfg.GetFeatureFlag(FeatureInboxPRWatch) {
		t.Fatal("nil config: inbox_pr_watch must be off")
	}
	if FeatureInboxPRWatch != "inbox_pr_watch" {
		t.Fatalf("flag constant = %q", FeatureInboxPRWatch)
	}
	cfg := &Config{FeatureFlags: map[string]bool{"other": true}}
	if cfg.GetFeatureFlag(FeatureInboxPRWatch) {
		t.Fatal("unset flag must be off")
	}
	cfg.FeatureFlags[FeatureInboxPRWatch] = true
	if !cfg.GetFeatureFlag(FeatureInboxPRWatch) {
		t.Fatal("flag set true must be on")
	}
}

func intp(v int) *int { return &v }

func TestInboxPRWatchConfig_should_ApplyDefaults_When_KeysAbsentOrInvalid(t *testing.T) {
	s, corr := ResolveInboxPRWatch(nil)
	if len(corr) != 0 {
		t.Fatalf("defaults produce corrections: %v", corr)
	}
	want := InboxPRWatchSettings{
		URL: "http://127.0.0.1:8789", TokenPath: "~/.local/state/gh-signal/inbox.token",
		PollIntervalSeconds: 5, ReconcileIntervalSecs: 60, CoverageRecheckMinutes: 10,
		MinSteerIntervalSeconds: 300, HeldNoticeAfterMinutes: 60, HeldAgeCeilingHours: 24,
		StateNotifyGraceSeconds: 120, LeaderRetrySeconds: 30, FollowupObserveSeconds: 120,
		IgnoreBotActors: true, HumanFeedbackMode: "all", EffectiveHumanFeedbackMode: "all",
		StrangerNoticeThreshold: 3,
	}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("defaults:\n got %+v\nwant %+v", s, want)
	}

	s, corr = ResolveInboxPRWatch(&InboxPRWatchConfig{
		PollIntervalSeconds: -3, ReconcileIntervalSecs: 4, CoverageRecheckMinutes: -1,
		HeldNoticeAfterMinutes: -5, StrangerNoticeThreshold: -2,
		MinSteerIntervalSeconds: intp(5000),
	})
	if s.PollIntervalSeconds != 5 || s.ReconcileIntervalSecs != 10 || s.CoverageRecheckMinutes != 10 ||
		s.HeldNoticeAfterMinutes != 60 || s.StrangerNoticeThreshold != 3 || s.MinSteerIntervalSeconds != 1800 {
		t.Fatalf("invalid values not corrected: %+v", s)
	}
	if len(corr) != 6 {
		t.Fatalf("want 6 corrections, got %d: %v", len(corr), corr)
	}

	s, corr = ResolveInboxPRWatch(&InboxPRWatchConfig{MinSteerIntervalSeconds: intp(0)})
	if s.MinSteerIntervalSeconds != 0 || len(corr) != 0 {
		t.Fatalf("min_steer 0 must be honored: %d %v", s.MinSteerIntervalSeconds, corr)
	}

	s, _ = ResolveInboxPRWatch(&InboxPRWatchConfig{IgnoreRepos: []string{"Acme/Widget", "bad", "a/b/c", "/x", "acme/widget"}})
	if !reflect.DeepEqual(s.IgnoreRepos, []string{"acme/widget"}) {
		t.Fatalf("ignore_repos = %v", s.IgnoreRepos)
	}
}

func TestInboxPRWatchConfig_should_FailClosedAndWarn_When_HumanFeedbackModeInvalidOrAllowlistEmptyOrIgnored(t *testing.T) {
	if DefaultHumanFeedbackMode != HumanFeedbackAll {
		t.Fatalf("default mode = %q", DefaultHumanFeedbackMode)
	}
	s, corr := ResolveInboxPRWatch(&InboxPRWatchConfig{HumanFeedbackMode: "everyone"})
	if s.EffectiveHumanFeedbackMode != "off" || len(corr) != 1 || corr[0].Key != "human_feedback_mode" {
		t.Fatalf("invalid mode: %+v %v", s, corr)
	}
	s, corr = ResolveInboxPRWatch(&InboxPRWatchConfig{HumanFeedbackMode: "allowlist"})
	if s.HumanFeedbackMode != "allowlist" || s.EffectiveHumanFeedbackMode != "off" || len(corr) != 1 {
		t.Fatalf("empty allowlist: %+v %v", s, corr)
	}
	s, corr = ResolveInboxPRWatch(&InboxPRWatchConfig{HumanFeedbackMode: "allowlist", ActorAllowlist: []string{"alice"}})
	if s.EffectiveHumanFeedbackMode != "allowlist" || len(corr) != 0 {
		t.Fatalf("valid allowlist: %+v %v", s, corr)
	}
	_, corr = ResolveInboxPRWatch(&InboxPRWatchConfig{ActorAllowlist: []string{"alice"}})
	if len(corr) != 1 || corr[0].Key != "actor_allowlist" {
		t.Fatalf("all + allowlist must warn: %v", corr)
	}
	if !s.AllowlistContainsLogin("ALICE") || s.AllowlistContainsLogin("bob") {
		t.Fatal("AllowlistContainsLogin")
	}
}

func TestInboxPRWatchConfig_should_ListEveryCorrectionAndLogOneWarnEach_When_ValuesAreInvalidOrClamped(t *testing.T) {
	_, corr := ResolveInboxPRWatch(&InboxPRWatchConfig{
		PollIntervalSeconds: -1, ReconcileIntervalSecs: 1, MinSteerIntervalSeconds: intp(9999),
		IgnoreRepos: []string{"bad"}, HumanFeedbackMode: "nope",
	})
	if len(corr) != 5 {
		t.Fatalf("want 5 corrections, got %v", corr)
	}
	var warns []string
	LogInboxPRWatchCorrections(corr, func(msg string, kv ...any) {
		warns = append(warns, fmt.Sprint(kv...))
	})
	if len(warns) != len(corr) {
		t.Fatalf("warns = %d, corrections = %d", len(warns), len(corr))
	}
	for i, c := range corr {
		if !strings.Contains(warns[i], c.Key) || !strings.Contains(warns[i], c.Effective) {
			t.Errorf("warn %q lacks key/effective of %v", warns[i], c)
		}
	}

	keys := InboxPRWatchConfigKeys()
	typ := reflect.TypeOf(InboxPRWatchConfig{})
	var tags []string
	for i := 0; i < typ.NumField(); i++ {
		tags = append(tags, strings.Split(typ.Field(i).Tag.Get("json"), ",")[0])
	}
	sort.Strings(tags)
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(tags, sorted) || len(keys) != 17 {
		t.Fatalf("keys %v do not match struct json tags %v", sorted, tags)
	}
}
