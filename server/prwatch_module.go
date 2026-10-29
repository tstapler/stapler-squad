package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

const prwatchLeaderLockName = "prwatch.leader.lock"

// prwatchWiring carries the ports the poller needs; startPRWatch builds them
// from the server dependencies and tests pass fakes.
type prwatchWiring struct {
	Settings    prwatch.Settings
	Corrections []prwatch.SettingCorrection
	Inbox       prwatch.Inbox
	Sessions    prwatch.Sessions
	Steerer     prwatch.Steerer
	Snapshots   prwatch.PRSnapshotSource
	SelfLogin   prwatch.SelfLogin
	Notifier    prwatch.Notifier
	Leader      prwatch.LeaderGuard
	Clock       prwatch.Clock
	Log         prwatch.Logger
	// Enabled is the live feature flag, read every tick.
	Enabled func() bool
}

type prwatchModule struct {
	poller   *prwatch.Poller
	leader   *prwatch.Leadership
	login    *prwatch.SelfLoginResolver
	status   *prwatch.StatusTracker
	counters *prwatch.CounterSet
	enabled  func() bool
	interval time.Duration
}

// newPRWatchModule assembles the engine from ports. It starts no goroutine and
// makes no inbox call.
func newPRWatchModule(w prwatchWiring) *prwatchModule {
	s := w.Settings
	counters := prwatch.NewCounterSet()
	status := prwatch.NewStatusTracker(w.Clock, counters)
	status.SetSettings(s, w.Corrections)

	login := prwatch.NewSelfLoginResolver(w.SelfLogin, w.Clock, w.Notifier, w.Log)
	notes := prwatch.NewNotifications(w.Notifier, w.Clock, s, session.SanitizeUntrusted)
	coalescer := prwatch.NewWindowCoalescer(w.Clock, s, counters)
	followup := prwatch.NewFollowupObserver(w.Sessions, w.Clock, w.Log, s.FollowupObserve)
	stranger := prwatch.NewStrangerTracker(notes, w.Clock, s.StrangerNoticeThreshold)
	leader := prwatch.NewLeadership(w.Leader, w.Clock, w.Log, s.LeaderRetry)

	engine := prwatch.NewDeliverer(prwatch.DeliverDeps{
		Inbox: w.Inbox, Sessions: w.Sessions, Steerer: w.Steerer, Coalescer: coalescer,
		Recorder: status, Metrics: counters, Clock: w.Clock, Log: w.Log,
		Filter: prwatch.NewActorFilter(s, login), Mode: s.HumanFeedbackMode,
		Notes: notes, Stranger: stranger, Followup: followup,
	})
	poller := prwatch.NewPoller(prwatch.LoopDeps{
		Inbox: w.Inbox, Sessions: w.Sessions, Engine: engine, Notifier: w.Notifier,
		Coalescer: coalescer, Metrics: counters, Log: w.Log, Clock: w.Clock, Settings: s,
		Catchup: prwatch.NewCatchup(prwatch.CatchupDeps{
			Snapshots: w.Snapshots, Steerer: w.Steerer, Coalescer: coalescer,
			Metrics: counters, Log: w.Log, Settings: s,
		}),
		Leader:   leader,
		State:    prwatch.NewInboxStateTracker(notes, w.Clock, w.Log, s),
		Followup: followup,
		HeldAge:  prwatch.NewHeldAge(w.Clock, s, notes),
		Status:   status,
		Login:    login,
		Enabled:  w.Enabled,
	})
	return &prwatchModule{
		poller: poller, leader: leader, login: login, status: status, counters: counters,
		enabled: w.Enabled, interval: s.PollInterval,
	}
}

// reportPRWatchNonLoopback records InboxStateNonLoopback so the notification
// and the status output show why no poller runs. No inbox call is made.
func reportPRWatchNonLoopback(w prwatchWiring) *prwatch.StatusTracker {
	status := prwatch.NewStatusTracker(w.Clock, prwatch.NewCounterSet())
	status.SetSettings(w.Settings, w.Corrections)
	status.SetEnabled(true, 0)
	status.SetInbox(prwatch.InboxStateNonLoopback, "non_loopback_url")
	notes := prwatch.NewNotifications(w.Notifier, w.Clock, w.Settings, session.SanitizeUntrusted)
	prwatch.NewInboxStateTracker(notes, w.Clock, w.Log, w.Settings).Observe(prwatch.InboxStateNonLoopback)
	return status
}

// Start runs the poller, the leader-lock release and the self-login resolver
// under srv.backgroundTasksWG; all end when ctx is cancelled. The resolver
// waits for the flag so a flag-off process never calls GitHub.
func (m *prwatchModule) Start(ctx context.Context, srv *Server) {
	srv.goBackground(func() { m.poller.Run(ctx) })
	srv.goBackground(func() { m.leader.Watch(ctx) })
	srv.goBackground(func() {
		if !m.waitEnabled(ctx) {
			return
		}
		m.login.Run(ctx)
	})
}

func (m *prwatchModule) waitEnabled(ctx context.Context) bool {
	wait := max(m.interval, time.Second)
	for m.enabled != nil && !m.enabled() {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(wait):
		}
	}
	return ctx.Err() == nil
}

// Status is the in-memory snapshot served by /actuator/inbox-pr-watch.
func (m *prwatchModule) Status() prwatch.Status { return m.status.Snapshot(nil) }

type prwatchClock struct{}

func (prwatchClock) Now() time.Time { return time.Now() }

type prwatchLogger struct{}

func (prwatchLogger) Debug(msg string, kv ...any) { log.Debug(msg, kv...) }
func (prwatchLogger) Info(msg string, kv ...any)  { log.Info(msg, kv...) }
func (prwatchLogger) Warn(msg string, kv ...any)  { log.Warn(msg, kv...) }

// prwatchEnabled reads the feature flag from the live config on every call.
func prwatchEnabled() bool {
	return config.LoadConfig().GetFeatureFlag(config.FeatureInboxPRWatch)
}

// prwatchLeaderGuard locks <config dir>/prwatch.leader.lock, falling back to an
// always-leader guard when the config dir cannot be resolved (as wakes does).
func prwatchLeaderGuard() (prwatch.LeaderGuard, string) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return wakes.NoLeader{}, ""
	}
	path := filepath.Join(dir, prwatchLeaderLockName)
	return wakes.FileLeader{Path: path}, path
}

// prwatchStatusURL is the operator status URL for the listen address (D14).
func prwatchStatusURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" || port == "0" {
		port = "8543"
	}
	return "http://localhost:" + port + "/actuator/inbox-pr-watch?format=text"
}

// prwatchSettings maps the resolved config onto the engine's settings, so
// prwatch never imports config.
func prwatchSettings(c config.InboxPRWatchSettings, host, statusURL, guide, lockPath string) prwatch.Settings {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return prwatch.Settings{
		ConsumerID: "squad:" + host, URL: c.URL, TokenPath: c.TokenPath,
		StatusURL: statusURL, GuidePath: guide, LockPath: lockPath,
		PollInterval:            sec(c.PollIntervalSeconds),
		ReconcileInterval:       sec(c.ReconcileIntervalSecs),
		CoverageRecheck:         time.Duration(c.CoverageRecheckMinutes) * time.Minute,
		MinSteerInterval:        sec(c.MinSteerIntervalSeconds),
		HeldNoticeAfter:         time.Duration(c.HeldNoticeAfterMinutes) * time.Minute,
		HeldAgeCeiling:          time.Duration(c.HeldAgeCeilingHours) * time.Hour,
		StateNotifyGrace:        sec(c.StateNotifyGraceSeconds),
		LeaderRetry:             sec(c.LeaderRetrySeconds),
		FollowupObserve:         sec(c.FollowupObserveSeconds),
		IgnoreRepos:             c.IgnoreRepos,
		IgnoreBotActors:         c.IgnoreBotActors,
		BotActorAllowlist:       c.BotActorAllowlist,
		HumanFeedbackMode:       prwatch.HumanFeedbackMode(c.EffectiveHumanFeedbackMode),
		ActorAllowlist:          c.ActorAllowlist,
		StrangerNoticeThreshold: c.StrangerNoticeThreshold,
	}
}

// startPRWatch builds the PR watch consumer and starts it on srv's background
// WaitGroup. The flag is read live every tick, so with it off nothing polls and
// nothing touches the inbox. A non-loopback URL builds no poller (one warning).
func startPRWatch(ctx context.Context, srv *Server, deps *ServerDependencies) {
	if deps.SessionService == nil || deps.Storage == nil {
		return
	}
	cfg := config.LoadConfig()
	cs, corrections := cfg.InboxPRWatchSettings()
	config.LogInboxPRWatchCorrections(corrections, log.Warn)
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	leader, lockPath := prwatchLeaderGuard()
	settings := prwatchSettings(cs, host, prwatchStatusURL(srv.GetAddr()), prwatchGuidePath(), lockPath)
	client, err := inbox.NewClient(inbox.Config{URL: settings.URL, TokenPath: settings.TokenPath, ConsumerID: settings.ConsumerID})
	if err != nil {
		if errors.Is(err, inbox.ErrNonLoopbackURL) {
			// The error text carries the raw URL; log only a host-less description.
			log.Warn("prwatch not started: inbox url is not a loopback address")
			if prwatchEnabled() {
				status := reportPRWatchNonLoopback(prwatchWiring{
					Settings: settings, Notifier: prwatchNotifier{bus: deps.EventBus},
					Clock: prwatchClock{}, Log: prwatchLogger{},
				})
				SetPRWatchStatusProvider(func() prwatch.Status { return status.Snapshot(nil) })
				srv.shutdownHooks = append(srv.shutdownHooks, func() { SetPRWatchStatusProvider(nil) })
			}
			return
		}
		log.Warn("prwatch not started: invalid inbox url")
		return
	}
	var sc []prwatch.SettingCorrection
	for _, c := range corrections {
		sc = append(sc, prwatch.SettingCorrection{Key: c.Key, Bad: c.Value, Effective: c.Effective})
	}
	svc := deps.SessionService
	snapshots := func() []*session.InstanceSnapshot {
		insts := svc.GetInstances()
		out := make([]*session.InstanceSnapshot, 0, len(insts))
		for _, inst := range insts {
			if inst != nil {
				out = append(out, inst.Snapshot())
			}
		}
		return out
	}
	m := newPRWatchModule(prwatchWiring{
		Settings: settings, Corrections: sc, Inbox: prwatchInbox{c: client},
		Sessions:  newPRWatchSessions(snapshots, svc, deps.Storage, settings.IgnoreRepos),
		Steerer:   prwatchSteerer{svc: svc},
		Snapshots: newPRWatchSnapshots(snapshots),
		SelfLogin: prwatchSelfLogin{},
		Notifier:  prwatchNotifier{bus: deps.EventBus},
		Leader:    leader, Clock: prwatchClock{}, Log: prwatchLogger{},
		Enabled: prwatchEnabled,
	})
	SetPRWatchStatusProvider(m.Status)
	srv.shutdownHooks = append(srv.shutdownHooks, func() { SetPRWatchStatusProvider(nil) })
	m.Start(ctx, srv)
	log.Info("prwatch registered", "consumer", fmt.Sprint(settings.ConsumerID), "flag", config.FeatureInboxPRWatch)
}
