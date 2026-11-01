package tymux

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// modelDaemon is an in-memory tymuxd: unique ids, name-keyed listing, dead/live
// liveness, NotFound for unknown ids. It records every kill so the model can
// check that Close only ever kills what its Instance owns.
type modelDaemon struct {
	fakeTransport
	mu       sync.Mutex
	next     int
	sessions map[string]*modelDaemonSession
	kills    []string
}

type modelDaemonSession struct {
	name, cwd string
	live      bool
}

func newModelDaemon() *modelDaemon {
	return &modelDaemon{sessions: map[string]*modelDaemonSession{}}
}

func (d *modelDaemon) proto(id string, s *modelDaemonSession) *v1.Session {
	l := v1.Liveness_LIVENESS_LIVE
	if !s.live {
		l = v1.Liveness_LIVENESS_DEAD
	}
	out := fakeSession(id, id+"-pane", s.cwd, l)
	out.Name = s.name
	return out
}

func (d *modelDaemon) CreateSession(_ context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	id := fmt.Sprintf("d%03d", d.next)
	s := &modelDaemonSession{name: req.Msg.GetName(), cwd: req.Msg.GetCwd(), live: true}
	d.sessions[id] = s
	return connect.NewResponse(d.proto(id, s)), nil
}

func (d *modelDaemon) KillSession(_ context.Context, req *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := req.Msg.GetSessionId()
	d.kills = append(d.kills, id)
	if _, ok := d.sessions[id]; !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	}
	delete(d.sessions, id)
	return connect.NewResponse(&v1.KillSessionResponse{}), nil
}

func (d *modelDaemon) ListSessions(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]string, 0, len(d.sessions))
	for id := range d.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*v1.Session, 0, len(ids))
	for _, id := range ids {
		out = append(out, d.proto(id, d.sessions[id]))
	}
	return connect.NewResponse(&v1.ListSessionsResponse{Sessions: out}), nil
}

func (d *modelDaemon) ReviveSession(_ context.Context, req *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := req.Msg.GetSessionId()
	s, ok := d.sessions[id]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	}
	s.live = true
	return connect.NewResponse(&v1.ReviveSessionResponse{Session: d.proto(id, s)}), nil
}

func (d *modelDaemon) CapturePane(_ context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := strings.TrimSuffix(req.Msg.GetPaneId(), "-pane")
	s, ok := d.sessions[id]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
	}
	l := v1.Liveness_LIVENESS_LIVE
	if !s.live {
		l = v1.Liveness_LIVENESS_DEAD
	}
	return connect.NewResponse(&v1.PaneSnapshot{Liveness: l}), nil
}

func (d *modelDaemon) ids() map[string]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[string]bool{}
	for id := range d.sessions {
		out[id] = true
	}
	return out
}

// modelInstance is one stapler-squad Instance's tymux-facing state: the live
// manager plus the persisted record a restart restores from.
type modelInstance struct {
	mgr    TymuxManager
	label  string
	dir    string
	stored string // persisted TymuxSessionID ("" = legacy / never persisted)
	closed bool
}

type identityModel struct {
	t      *testing.T
	d      *modelDaemon
	dirs   []string
	labels []string
	insts  []*modelInstance
	rng    *rand.Rand
	trace  []string
	// shadows are extra views of a record; they are not Instances of their own.
	shadows []TymuxManager
}

func (m *identityModel) log(format string, a ...any) {
	m.trace = append(m.trace, fmt.Sprintf(format, a...))
}

func (m *identityModel) fresh(label, dir string) *modelInstance {
	mgr := NewTymuxGRPCSession(m.d)
	mgr.SetSessionLabel(label)
	return &modelInstance{mgr: mgr, label: label, dir: dir}
}

func (m *identityModel) live() []*modelInstance {
	var out []*modelInstance
	for _, in := range m.insts {
		if !in.closed {
			out = append(out, in)
		}
	}
	return out
}

func (m *identityModel) labelCount(label string) int {
	n := 0
	for _, in := range m.live() {
		if in.label == label {
			n++
		}
	}
	return n
}

func (m *identityModel) pick() *modelInstance {
	l := m.live()
	if len(l) == 0 {
		return nil
	}
	return l[m.rng.Intn(len(l))]
}

func (m *identityModel) restartProcess() {
	// A fresh process: no claims survive, every manager is rebuilt from its record.
	claimedSessions.Range(func(k, _ any) bool { claimedSessions.Delete(k); return true })
	storedIDHolders.Range(func(k, _ any) bool { storedIDHolders.Delete(k); return true })
	m.shadows = nil // views of the dead process are gone with it
	live := m.live()
	for _, in := range live {
		in.mgr = NewTymuxGRPCSession(m.d)
		in.mgr.SetSessionLabel(in.label)
		in.mgr.SetStoredSessionID(in.stored)
	}
	// Production restores run concurrently in unspecified order; the model picks
	// one seeded order per restart so every seed replays identically.
	for _, i := range m.rng.Perm(len(live)) {
		_ = live[i].mgr.RestoreWithWorkDir(live[i].dir)
	}
	for _, in := range live {
		// Titles are unique in production, so (c) is only asserted for unique labels.
		if in.stored != "" && m.d.ids()[in.stored] && m.labelCount(in.label) == 1 {
			if got := in.mgr.GetSessionIdentifier(); got != in.stored {
				m.t.Fatalf("invariant (c): record with stored id %s restored as %q\ntrace:\n%s", in.stored, got, strings.Join(m.trace, "\n"))
			}
		}
	}
	for _, in := range live {
		in.stored = in.mgr.GetSessionIdentifier()
	}
}

// shadowClose closes view and checks the record's session (id) is gone from the
// daemon and that nothing else was killed.
func (m *identityModel) shadowClose(view TymuxManager, id, kind string) {
	existed := m.d.ids()[id]
	before := len(m.d.kills)
	_ = view.Close()
	for _, k := range m.d.kills[before:] {
		if k != id {
			m.t.Fatalf("invariant (b): %s close of %s killed %s\ntrace:\n%s", kind, id, k, strings.Join(m.trace, "\n"))
		}
	}
	if existed && m.d.ids()[id] {
		m.t.Fatalf("%s close of %s left the record's session running\ntrace:\n%s", kind, id, strings.Join(m.trace, "\n"))
	}
}

func (m *identityModel) step() {
	op := m.rng.Intn(16)
	switch op {
	case 0, 1: // create + start (twins arise: dirs and labels come from tiny pools)
		in := m.fresh(m.labels[m.rng.Intn(len(m.labels))], m.dirs[m.rng.Intn(len(m.dirs))])
		if err := in.mgr.Start(in.dir); err != nil {
			m.t.Fatalf("start: %v", err)
		}
		in.stored = in.mgr.GetSessionIdentifier()
		m.insts = append(m.insts, in)
		m.log("create label=%q dir=%s -> %s", in.label, in.dir, in.stored)
	case 2: // close
		if in := m.pick(); in != nil {
			owned := in.mgr.GetSessionIdentifier()
			before := len(m.d.kills)
			_ = in.mgr.Close()
			in.closed = true
			for _, k := range m.d.kills[before:] {
				if k != owned {
					m.t.Fatalf("invariant (b): Close of %s killed %s\ntrace:\n%s", owned, k, strings.Join(m.trace, "\n"))
				}
			}
			m.log("close %s", owned)
		}
	case 3: // persist (ToInstanceData)
		if in := m.pick(); in != nil {
			in.stored = in.mgr.GetSessionIdentifier()
			m.log("persist %s", in.stored)
		}
	case 4: // rename: label changes, record keeps the id
		if in := m.pick(); in != nil {
			in.label = m.labels[m.rng.Intn(len(m.labels))]
			in.mgr.SetSessionLabel(in.label)
			in.stored = in.mgr.GetSessionIdentifier() // Rename saves the whole record
			m.log("rename %s -> %q", in.mgr.GetSessionIdentifier(), in.label)
		}
	case 5, 6: // process restart
		m.log("restart process")
		m.restartProcess()
	case 7: // daemon restart: everything reloads dead
		m.d.mu.Lock()
		for _, s := range m.d.sessions {
			s.live = false
		}
		m.d.mu.Unlock()
		m.log("daemon restart")
	case 8: // daemon loses a session
		ids := m.d.ids()
		for id := range ids {
			m.d.mu.Lock()
			delete(m.d.sessions, id)
			m.d.mu.Unlock()
			m.log("daemon lost %s", id)
			break
		}
	case 9: // health recovery: IsAlive false -> Start
		if in := m.pick(); in != nil && !in.mgr.IsAlive() {
			if err := in.mgr.Start(in.dir); err != nil {
				m.t.Fatalf("recovery start: %v", err)
			}
			in.stored = in.mgr.GetSessionIdentifier()
			m.log("recover -> %s", in.stored)
		}
	case 11: // a shadow view of the same record (registry throwaway, liveness shadow)
		if in := m.pick(); in != nil && in.stored != "" && m.d.ids()[in.stored] {
			before := len(m.d.ids())
			view := NewTymuxGRPCSession(m.d)
			view.SetSessionLabel(in.label)
			view.SetStoredSessionID(in.stored)
			if err := view.RestoreWithWorkDir(in.dir); err != nil {
				m.t.Fatalf("shadow restore: %v", err)
			}
			if got := view.GetSessionIdentifier(); got != in.stored || len(m.d.ids()) != before {
				m.t.Fatalf("shadow of %s restored as %q (daemon sessions %d -> %d)\ntrace:\n%s", in.stored, got, before, len(m.d.ids()), strings.Join(m.trace, "\n"))
			}
			m.shadows = append(m.shadows, view)
			m.log("shadow of %s", in.stored)
		}
	case 12: // closing a shadow kills the record's session (the record owns it)
		if len(m.shadows) > 0 {
			view := m.shadows[0]
			m.shadows = m.shadows[1:]
			id := view.GetSessionIdentifier()
			m.shadowClose(view, id, "restored shadow")
			m.log("shadow close %s", id)
		}
	case 13: // recovery replaced a lost session but the new id is not saved yet
		if in := m.pick(); in != nil && in.stored != "" {
			lost := in.mgr.GetSessionIdentifier()
			m.d.mu.Lock()
			delete(m.d.sessions, lost)
			m.d.mu.Unlock()
			if err := in.mgr.Start(in.dir); err != nil {
				m.t.Fatalf("unsaved recovery start: %v", err)
			}
			m.log("recover %s -> %s without persisting (record still stores %s)", lost, in.mgr.GetSessionIdentifier(), in.stored)
		}
	case 14: // legacy record (no id) that shares a label with a record storing an id
		if src := m.pick(); src != nil && src.stored != "" {
			leg := m.fresh(src.label, src.dir)
			m.insts = append(m.insts, leg) // restored at the next process restart
			m.log("legacy record label=%q next to %s", src.label, src.stored)
		}
	case 15: // an unrestored view of a record is closed (stopped/deleted before restore)
		if in := m.pick(); in != nil && in.stored != "" {
			view := NewTymuxGRPCSession(m.d)
			view.SetSessionLabel(in.label)
			view.SetStoredSessionID(in.stored)
			m.shadowClose(view, in.stored, "unrestored shadow")
			m.log("unrestored shadow close %s", in.stored)
		}
	case 10: // duplicate: same dir and label as another, fresh record
		if src := m.pick(); src != nil {
			in := m.fresh(src.label, src.dir)
			if err := in.mgr.Start(in.dir); err != nil {
				m.t.Fatalf("dup start: %v", err)
			}
			in.stored = in.mgr.GetSessionIdentifier()
			m.insts = append(m.insts, in)
			m.log("duplicate of %q -> %s", src.label, in.stored)
		}
	}
}

func (m *identityModel) check() {
	seen := map[string]int{}
	for _, in := range m.live() {
		if id := in.mgr.GetSessionIdentifier(); id != "" {
			seen[id]++
		}
	}
	for id, n := range seen {
		if n > 1 {
			m.t.Fatalf("invariant (a): %d live Instances hold %s\ntrace:\n%s", n, id, strings.Join(m.trace, "\n"))
		}
	}
	// (d) a daemon session that no Instance owns is a leak.
	for id := range m.d.ids() {
		if seen[id] == 0 {
			m.t.Fatalf("invariant (d): daemon session %s leaked (no live owner)\ntrace:\n%s", id, strings.Join(m.trace, "\n"))
		}
	}
}

// TestIdentityModel_RandomOperationSequences drives N Instances x one daemon
// through seeded random operation sequences and asserts the identity
// invariants after every step.
func TestIdentityModel_RandomOperationSequences(t *testing.T) {
	for seed := int64(1); seed <= 60; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			resetClaims(t)
			m := &identityModel{
				t:      t,
				d:      newModelDaemon(),
				dirs:   []string{t.TempDir(), t.TempDir()},
				labels: []string{"a", "b", "twin"},
				rng:    rand.New(rand.NewSource(seed)),
			}
			t.Cleanup(func() {
				for _, in := range m.live() {
					_ = in.mgr.Close()
				}
			})
			for i := 0; i < 40; i++ {
				m.step()
				m.check()
			}
			for _, in := range m.live() {
				require.NoError(t, in.mgr.Close())
				in.closed = true
			}
			require.Empty(t, m.d.ids(), "closing every Instance must leave no daemon session")
		})
	}
}
