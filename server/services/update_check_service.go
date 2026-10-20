package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/changelog"
)

const (
	// UpdateCheckFlagName is opt-in: the check contacts api.github.com.
	UpdateCheckFlagName = "changelog:update-check"
	// ChangelogFlagName gates the changelog route, API and update badge.
	ChangelogFlagName = "changelog"

	updateCheckMinInterval = 6 * time.Hour
	updateCheckInitialWait = 30 * time.Second
	updateCheckTick        = 15 * time.Minute
)

// ChangelogEnabled reports the changelog flag (default on) from cfg.
func ChangelogEnabled(cfg *config.Config) bool {
	return cfg.GetFeatureFlagWithDefault(ChangelogFlagName, featureFlagDefault(ChangelogFlagName))
}

// UpdateCheckFlagEnabled reports the opt-in update-check flag (default off) from cfg.
func UpdateCheckFlagEnabled(cfg *config.Config) bool {
	return cfg.GetFeatureFlagWithDefault(UpdateCheckFlagName, featureFlagDefault(UpdateCheckFlagName))
}

// UpdateCheckResult is the JSON served by GET /api/update-check.
type UpdateCheckResult struct {
	Enabled        bool       `json:"enabled"`
	CurrentVersion string     `json:"current_version"`
	LatestVersion  string     `json:"latest_version,omitempty"`
	LatestURL      string     `json:"latest_url,omitempty"`
	IsOutdated     bool       `json:"is_outdated"`
	IsSourceBuild  bool       `json:"is_source_build"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// ReleaseInfo is what a successful latest-release lookup returns.
type ReleaseInfo struct {
	Version string
	URL     string
}

// UpdateCheckDeps are the service's seams, injected so tests need no network,
// config file or wall clock.
type UpdateCheckDeps struct {
	CurrentVersion string
	IsSourceBuild  bool
	// Enabled is read fresh on every tick (the flag is live-settable).
	Enabled func() bool
	Fetch   func(ctx context.Context) (*ReleaseInfo, error)
	Now     func() time.Time
}

// UpdateCheckService polls for a newer release at most once per
// updateCheckMinInterval. Source builds and a disabled flag never reach the
// network.
type UpdateCheckService struct {
	deps UpdateCheckDeps

	mu          sync.Mutex // serializes MaybeCheck
	lastAttempt time.Time
	latest      *ReleaseInfo
	lastErr     string

	result atomic.Pointer[UpdateCheckResult]
}

func NewUpdateCheckService(deps UpdateCheckDeps) *UpdateCheckService {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Enabled == nil {
		deps.Enabled = func() bool { return false }
	}
	s := &UpdateCheckService{deps: deps}
	s.publish(nil)
	return s
}

// Current returns the last published result.
func (s *UpdateCheckService) Current() UpdateCheckResult {
	r := *s.result.Load()
	r.Enabled = s.deps.Enabled()
	if !r.Enabled || r.IsSourceBuild {
		r.IsOutdated = false
	}
	return r
}

// MaybeCheck performs a check if enabled, not a source build and the TTL has
// elapsed. It reports whether a network request was made.
func (s *UpdateCheckService) MaybeCheck(ctx context.Context) bool {
	if s.deps.IsSourceBuild || !s.deps.Enabled() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.deps.Now()
	if !s.lastAttempt.IsZero() && now.Sub(s.lastAttempt) < updateCheckMinInterval {
		return false
	}
	s.lastAttempt = now

	rel, err := s.deps.Fetch(ctx)
	if err != nil {
		s.lastErr = err.Error()
		log.Info("update check failed", "err", err)
	} else {
		s.lastErr = ""
		s.latest = rel
	}
	s.publish(&now)
	return true
}

func (s *UpdateCheckService) publish(checkedAt *time.Time) {
	r := &UpdateCheckResult{
		CurrentVersion: s.deps.CurrentVersion,
		IsSourceBuild:  s.deps.IsSourceBuild,
		LastCheckedAt:  checkedAt,
		Error:          s.lastErr,
	}
	if s.latest != nil {
		r.LatestVersion, r.LatestURL = s.latest.Version, s.latest.URL
		r.IsOutdated = !s.deps.IsSourceBuild && changelog.Newer(s.latest.Version, s.deps.CurrentVersion)
	}
	if checkedAt == nil {
		if prev := s.result.Load(); prev != nil {
			r.LastCheckedAt = prev.LastCheckedAt
		}
	}
	s.result.Store(r)
}

// Start runs the poll loop until ctx is cancelled.
func (s *UpdateCheckService) Start(ctx context.Context) {
	go func() {
		timer := time.NewTimer(updateCheckInitialWait)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.MaybeCheck(ctx)
				timer.Reset(updateCheckTick)
			}
		}
	}()
}
