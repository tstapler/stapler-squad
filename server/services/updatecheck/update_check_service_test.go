package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type updateHarness struct {
	svc     *UpdateCheckService
	now     time.Time
	enabled bool
	calls   int
	rel     *ReleaseInfo
	err     error
}

func newUpdateHarness(current string, source bool) *updateHarness {
	h := &updateHarness{now: time.Unix(1_700_000_000, 0), enabled: true, rel: &ReleaseInfo{Version: "2.6.0", URL: "https://example.com/r"}}
	h.svc = NewUpdateCheckService(UpdateCheckDeps{
		CurrentVersion: current,
		IsSourceBuild:  source,
		Enabled:        func() bool { return h.enabled },
		Now:            func() time.Time { return h.now },
		Fetch: func(context.Context) (*ReleaseInfo, error) {
			h.calls++
			return h.rel, h.err
		},
	})
	return h
}

func TestUpdateCheck_DisabledMakesNoRequest(t *testing.T) {
	h := newUpdateHarness("2.5.0", false)
	h.enabled = false
	assert.False(t, h.svc.MaybeCheck(context.Background()))
	assert.Zero(t, h.calls)
	assert.False(t, h.svc.Current().Enabled)
	assert.False(t, h.svc.Current().IsOutdated)
}

func TestUpdateCheck_SourceBuildNeverChecksOrNudges(t *testing.T) {
	h := newUpdateHarness("2.5.0", true)
	assert.False(t, h.svc.MaybeCheck(context.Background()))
	assert.Zero(t, h.calls)
	cur := h.svc.Current()
	assert.True(t, cur.IsSourceBuild)
	assert.False(t, cur.IsOutdated)
}

func TestUpdateCheck_TTLSuppressesRedundantRequests(t *testing.T) {
	h := newUpdateHarness("2.5.0", false)
	ctx := context.Background()

	assert.True(t, h.svc.MaybeCheck(ctx))
	cur := h.svc.Current()
	assert.True(t, cur.IsOutdated)
	assert.Equal(t, "2.6.0", cur.LatestVersion)
	assert.Equal(t, "https://example.com/r", cur.LatestURL)
	assert.NotNil(t, cur.LastCheckedAt)

	h.now = h.now.Add(updateCheckMinInterval - time.Minute)
	assert.False(t, h.svc.MaybeCheck(ctx), "inside the 6h TTL")
	assert.Equal(t, 1, h.calls)

	h.now = h.now.Add(2 * time.Minute)
	assert.True(t, h.svc.MaybeCheck(ctx), "past the TTL")
	assert.Equal(t, 2, h.calls)
}

func TestUpdateCheck_UpToDateAndDevVersion(t *testing.T) {
	h := newUpdateHarness("2.6.0", false)
	h.svc.MaybeCheck(context.Background())
	assert.False(t, h.svc.Current().IsOutdated)

	h = newUpdateHarness("dev", false)
	h.svc.MaybeCheck(context.Background())
	assert.False(t, h.svc.Current().IsOutdated, "a non-numeric version is never nudged")
}

func TestUpdateCheck_ErrorIsRecordedAndKeepsLastGoodResult(t *testing.T) {
	h := newUpdateHarness("2.5.0", false)
	ctx := context.Background()
	h.svc.MaybeCheck(ctx)

	h.now = h.now.Add(updateCheckMinInterval + time.Second)
	h.err = errors.New("boom")
	h.rel = nil
	assert.True(t, h.svc.MaybeCheck(ctx))
	cur := h.svc.Current()
	assert.Equal(t, "boom", cur.Error)
	assert.True(t, cur.IsOutdated, "a failed refresh keeps the last known release")
}
