package prwatch

import (
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/status_text.golden")

func tm(h, m, s int) time.Time { return time.Date(2026, 10, 22, h, m, s, 0, time.UTC) }

// fixedStatus is self-consistent: one next-steer value per session, steers by
// class sum to the steered total, and waiting equals the held list.
func fixedStatus() Status {
	closes := tm(12, 8, 0)
	last := tm(12, 3, 11)
	ev := tm(12, 3, 11)
	return Status{
		Enabled: true, Polls: true, LeaderHeld: true, InboxState: InboxStateOK, LoginResolved: true,
		Since:        tm(11, 0, 0),
		FirstRunLine: "Watching 3 PRs in 3 repositories; 1 repository not covered. Coverage not yet confirmed for 1 repository.",
		Settings: SettingsView{
			HumanFeedbackMode: ModeAll, PollInterval: "5s", MinSteerInterval: "5m0s",
			IgnoreRepos: []string{"acme-corp/internal"},
			Corrections: []SettingCorrection{{Key: "min_steer_interval_seconds", Bad: "5000", Effective: "1800"}},
		},
		Repos: []RepoStatus{
			{Repo: "acme-corp/widget-api", Coverage: CoverageStateCovered},
			{Repo: "other/repo", Coverage: CoverageStateUnavailable, Reason: "no admin access to the repository"},
			{Repo: "third/repo", Coverage: CoverageStateUnknown},
		},
		Watched: []WatchedStatus{
			{ShortUUID: "3f9a21c4", Title: "fix flaky login test", Repo: "acme-corp/widget-api", PR: 123,
				State: StateLive, Subscribed: true, LastEventAt: &ev, Coverage: CoverageStateCovered,
				HeldCount: 1, NextSteer: NextSteerAt, NextSteerAt: &closes},
			{ShortUUID: "5a6b7c8d", Title: "retry logic", Repo: "other/repo", PR: 42, State: StateLive,
				Subscribed: true, Coverage: CoverageStateUnavailable, NextSteer: NextSteerNow},
			{ShortUUID: "9e8d7c6b", Title: "bump deps", Repo: "third/repo", PR: 7, State: StateSuspended,
				Subscribed: false, Coverage: CoverageStateUnknown, NextSteer: NextSteerNow},
		},
		Skipped: []SkippedStatus{
			{ShortUUID: "77be0d10", Title: "cache spike", Reason: SkipIgnoreRepos},
			{ShortUUID: "1c2d3e4f", Title: "docs cleanup", Reason: SkipBacklogPRPending},
		},
		Held: []HeldStatus{{ShortUUID: "3f9a21c4", Repo: "acme-corp/widget-api", PR: 123, Kind: KindComment,
			Reason: HoldWindowBatching, AgeSeconds: 180, WindowClosesAt: &closes}},
		LastSteer: &LastSteerStatus{At: last, Kind: KindReview, Repo: "acme-corp/widget-api", PR: 123,
			ShortUUID: "3f9a21c4", ActorClass: "other"},
		Recent: []Disposition{
			{At: tm(12, 1, 2), Kind: KindCI, Repo: "acme-corp/widget-api", PR: 123, Disposition: DispositionDropped, Reason: string(DropCoveredSameHeadSHA)},
			{At: tm(12, 2, 40), Kind: KindComment, Repo: "acme-corp/widget-api", PR: 123, Disposition: DispositionDropped, Reason: string(DropSelfActor)},
			{At: tm(12, 3, 11), Kind: KindReview, Repo: "acme-corp/widget-api", PR: 123, Disposition: DispositionSteered},
			{At: tm(12, 5, 0), Kind: KindComment, Repo: "acme-corp/widget-api", PR: 123, Disposition: DispositionHeld, Reason: string(HoldWindowBatching)},
		},
		Counts: StatusCounts{
			Steers:        6,
			SteersByClass: map[string]int{"self": 0, "bot": 1, "allowlisted": 2, "other": 2, "ci": 1},
		},
	}
}

func TestStatusText_should_MatchGoldenLayout_When_FormatTextRendered(t *testing.T) {
	got := RenderText(fixedStatus(), tm(12, 6, 0), "devbox")
	const path = "testdata/status_text.golden"
	if *updateGolden {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
	assert.Contains(t, got, "Run: gh-signal inbox watch other/repo 42 --consumer squad:devbox")
	assert.Contains(t, got, "Or add other/repo to inbox_pr_watch.ignore_repos")
}

func TestStatusText_should_UseOperatorVocabularyAndShowResetNote_When_Rendered(t *testing.T) {
	got := RenderText(fixedStatus(), tm(12, 6, 0), "devbox")
	assert.Contains(t, got, "cleared on restart")
	assert.Contains(t, got, "not steered: you wrote it (own comment)")
	var enums []string
	for _, r := range AllDropReasons() {
		enums = append(enums, string(r))
	}
	for _, r := range AllHoldReasons() {
		enums = append(enums, string(r))
	}
	for _, r := range AllSkipReasons() {
		if r != SkipIgnoreRepos { // also a config key, printed verbatim
			enums = append(enums, string(r))
		}
	}
	for _, e := range enums {
		assert.NotContains(t, got, e, "internal enum word leaked")
	}
}

func TestStatusText_should_BePlainASCIIWithoutEscapesOrIconOnlyMeaning_When_Rendered(t *testing.T) {
	st := fixedStatus()
	st.InboxState = InboxStateUnreachable
	for _, got := range []string{RenderText(st, tm(12, 6, 0), "devbox"), NotRunningText()} {
		for _, line := range strings.Split(got, "\n") {
			for _, r := range line {
				require.True(t, r >= 0x20 && r < 0x7f, "non-ASCII or control rune %q in %q", r, line)
			}
			if !strings.Contains(line, "Run:") {
				assert.LessOrEqual(t, len(line), 120, line)
			}
		}
		assert.NotRegexp(t, regexp.MustCompile("\x1b"), got)
	}
}

func TestStatusText_should_SayCoverageUnknownNotCovered_When_RepoUnknown(t *testing.T) {
	got := RenderText(fixedStatus(), tm(12, 6, 0), "devbox")
	assert.Contains(t, got, "third/repo   webhook coverage: unknown")
	assert.Contains(t, got, "webhook coverage: unknown")
}

func TestStatusText_should_ShowCIOnlyFallback_When_AllowlistEmpty(t *testing.T) {
	st := fixedStatus()
	st.Settings.HumanFeedbackMode = ModeAllowlist
	st.Settings.CIOnly = true
	got := RenderText(st, tm(12, 6, 0), "devbox")
	assert.Contains(t, got, "CI-only mode is in effect")
	assert.Contains(t, got, "allowlist is empty")
}
