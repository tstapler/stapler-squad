package wakes

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const idleComposer = "some output\n" +
	"────────────────────────────────────────\n" +
	"❯ \n" +
	"────────────────────────────────────────\n" +
	"  ? for shortcuts\n"

const permissionDialog = "Bash command\n  rm -rf build\n\nDo you want to proceed?\n❯ 1. Yes\n  2. No\n"

func readDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../session/testdata/reply_dialogs/" + name)
	require.NoError(t, err)
	return string(b)
}

// paneDouble is a fault-injecting Pane: it records every byte Submit receives.
type paneDouble struct {
	mu         sync.Mutex
	capture    string
	captureErr error
	busy       bool
	submitErr  error
	writes     []string
	locks      int
}

func (p *paneDouble) setCapture(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capture = s
}

func (p *paneDouble) written() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.writes...)
}

func (p *paneDouble) Lock() (PaneLease, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy {
		return nil, false
	}
	p.locks++
	return &doubleLease{p: p}, true
}

type doubleLease struct {
	p        *paneDouble
	released bool
}

func (l *doubleLease) Capture() (string, error) {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	return l.p.capture, l.p.captureErr
}

func (l *doubleLease) Submit(_ context.Context, text string) error {
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	l.released = true
	if l.p.submitErr != nil {
		return l.p.submitErr
	}
	l.p.writes = append(l.p.writes, text)
	return nil
}

func (l *doubleLease) Release() { l.released = true }

// fakeSteerer routes WriteWake through the real WriteIfIdle over a paneDouble.
type fakeSteerer struct {
	mu    sync.Mutex
	ready map[string]bool
	pane  *paneDouble
}

func newFakeSteerer() *fakeSteerer {
	return &fakeSteerer{ready: map[string]bool{}, pane: &paneDouble{capture: idleComposer}}
}

func (s *fakeSteerer) setReady(owner string, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready[owner] = v
}

func (s *fakeSteerer) IsReadyForSteer(owner string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready[owner]
}

func (s *fakeSteerer) WriteWake(ctx context.Context, _ string, text string) (WriteOutcome, error) {
	return WriteIfIdle(ctx, s.pane, text)
}

// fakeOwners serves OwnerFacts per session; unknown owners are live.
type fakeOwners struct {
	mu    sync.Mutex
	facts map[string]OwnerFacts
	err   error
}

func newFakeOwners() *fakeOwners { return &fakeOwners{facts: map[string]OwnerFacts{}} }

func (o *fakeOwners) set(owner string, f OwnerFacts) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.facts[owner] = f
}

func (o *fakeOwners) Owner(_ context.Context, owner string) (OwnerFacts, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return OwnerFacts{}, o.err
	}
	if f, ok := o.facts[owner]; ok {
		return f, nil
	}
	return OwnerFacts{Exists: true, Live: true}, nil
}

type recordingNotifier struct {
	mu   sync.Mutex
	list []Notification
}

func (n *recordingNotifier) Notify(x Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.list = append(n.list, x)
}

func (n *recordingNotifier) all() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Notification(nil), n.list...)
}

// spyTarget records revive calls.
type spyTarget struct {
	mu        sync.Mutex
	archived  bool
	recovered int
	started   int
	startErr  error
	onStart   func()
}

func (s *spyTarget) IsArchived() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.archived
}
func (s *spyTarget) RecoverFromStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recovered++
}
func (s *spyTarget) Start(bool) error {
	s.mu.Lock()
	s.started++
	fn, err := s.onStart, s.startErr
	s.mu.Unlock()
	if fn != nil {
		fn()
	}
	return err
}
func (s *spyTarget) calls() (recovered, started int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recovered, s.started
}

var errBoom = errors.New("boom")

type rowOpts struct {
	owner       string
	payload     string
	action      domain.WakeActionKind
	spec        domain.WakeSpec
	nextFire    *time.Time
	expiresAt   *time.Time
	allowRevive bool
	maxFires    int
	generation  int
}

// newRow creates a pending row.
func newRow(t *testing.T, repo *session.EntRepository, o rowOpts) *session.WakeTriggerData {
	t.Helper()
	if o.owner == "" {
		o.owner = "owner-1"
	}
	if o.payload == "" {
		o.payload = "check the build"
	}
	if o.action == "" {
		o.action = domain.WakeActionSteerText
	}
	if o.spec == nil {
		at := t0
		o.spec = domain.AtSpec{At: at}
		o.nextFire = &at
	}
	action, err := domain.NewWakeAction(o.action, o.payload)
	require.NoError(t, err)
	d, _, err := repo.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: o.owner, Spec: o.spec, Action: action, NextFireAt: o.nextFire,
		ExpiresAt: o.expiresAt, AllowRevive: o.allowRevive, MaxFires: o.maxFires, Generation: o.generation,
		PendingCap: 100,
	})
	require.NoError(t, err)
	return d
}

// claimedRow creates a row and claims it at now.
func claimedRow(t *testing.T, repo *session.EntRepository, now time.Time, o rowOpts) *session.WakeTriggerData {
	t.Helper()
	d := newRow(t, repo, o)
	applied, err := repo.ClaimWakeTrigger(context.Background(), uuid.MustParse(d.ID), now)
	require.NoError(t, err)
	require.True(t, applied)
	got, err := repo.GetWakeTrigger(context.Background(), uuid.MustParse(d.ID))
	require.NoError(t, err)
	return got
}

func statusOf(t *testing.T, repo *session.EntRepository, id string) domain.WakeStatus {
	t.Helper()
	d, err := repo.GetWakeTrigger(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	return d.Status
}
