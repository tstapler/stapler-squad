package svctest

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

var IdleSnapshot = domain.StopSnapshot{
	CapturedAt:       time.Date(2026, 10, 13, 12, 3, 40, 0, time.UTC),
	LastActivityAt:   time.Date(2026, 10, 13, 12, 3, 10, 0, time.UTC),
	Bucket:           domain.BucketSafeIdle,
	TmuxAlive:        true,
	ControllerActive: true,
}
