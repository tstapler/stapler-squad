package ratelimit

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

type Scheduler struct {
	mu sync.Mutex

	sessionID     string
	timer         *time.Timer
	resetTime     time.Time
	fireTime      time.Time // actual time the timer will fire (for inspection/testing)
	bufferSeconds int

	onRecovery func() error

	sessionRunning func() bool
}

func NewScheduler(sessionID string) *Scheduler {
	return &Scheduler{
		sessionID:     sessionID,
		bufferSeconds: DefaultResetBuffer,
	}
}

func (s *Scheduler) SetRecoveryCallback(callback func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRecovery = callback
}

func (s *Scheduler) SetSessionStatusCheck(callback func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionRunning = callback
}

func (s *Scheduler) SetBuffer(seconds int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bufferSeconds = seconds
}

func (s *Scheduler) ScheduleRecovery(resetTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
	}

	var waitDuration time.Duration
	if resetTime.IsZero() {
		// No reset time known — fall back to 30 minutes so we don't spam the session.
		waitDuration = DefaultFallbackWait
	} else {
		waitDuration = time.Until(resetTime)
		if waitDuration < 0 {
			waitDuration = 0
		}
	}

	waitDuration += time.Duration(s.bufferSeconds) * time.Second

	log.Info("scheduling recovery", "session", s.sessionID, "wait", waitDuration)

	s.resetTime = resetTime
	s.fireTime = time.Now().Add(waitDuration)
	s.timer = time.AfterFunc(waitDuration, func() {
		s.executeRecovery()
	})
}

func (s *Scheduler) CancelRecovery() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
		log.Info("cancelled recovery", "session", s.sessionID)
	}
}

func (s *Scheduler) executeRecovery() {
	s.mu.Lock()
	callback := s.onRecovery
	sessionCheck := s.sessionRunning
	s.mu.Unlock()

	if sessionCheck != nil && !sessionCheck() {
		log.Info("session not running, skipping recovery", "session", s.sessionID)
		return
	}

	if callback != nil {
		log.Info("executing recovery", "session", s.sessionID)
		if err := callback(); err != nil {
			log.Warn("recovery failed", "session", s.sessionID, "err", err)
		}
	}
}

func (s *Scheduler) GetScheduledTime() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		return s.resetTime, true
	}
	return time.Time{}, false
}

// GetFireTime returns the actual time the scheduled timer will fire.
// Returns zero time and false when no timer is scheduled.
// Useful for testing the fallback wait duration.
func (s *Scheduler) GetFireTime() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		return s.fireTime, true
	}
	return time.Time{}, false
}

func (s *Scheduler) IsScheduled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timer != nil
}
