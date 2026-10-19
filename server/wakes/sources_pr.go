package wakes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ErrUnavailable means the requested trigger needs a source this process does
// not have, or the source does not hold the subject (typed `unavailable`).
var ErrUnavailable = errors.New("unavailable")

// ErrForbidden means the owner may not watch the subject: another session's
// guidance request, or a session in another repository (typed FORBIDDEN).
var ErrForbidden = errors.New("forbidden")

// ciFinal reports whether a poller check conclusion is a finished one.
func ciFinal(conclusion string) bool { return conclusion != "" && conclusion != "pending" }

func ciKindHolds(kind, conclusion string) bool {
	switch kind {
	case CIKindSuccess:
		return conclusion == "success"
	case CIKindFailure:
		return conclusion == "failure"
	case CIKindCompleted:
		return ciFinal(conclusion)
	}
	return false
}

func prCellKey(k PRKey) string { return fmt.Sprintf("%s#%d", k.Repo, k.Number) }

// registerPRSource installs the pr_event and ci_event predicates. Their events
// come from the poller watcher (startPRWatcher), not the bus (Spike S4).
func (d *Dispatcher) registerPRSource() {
	d.RegisterPredicate(domain.WakeTriggerCIEvent, func(_ context.Context, spec domain.WakeSpec) (bool, error) {
		s, ok := spec.(domain.CIEventSpec)
		if !ok || d.cfg.PRs == nil {
			return false, nil
		}
		view, open := d.cfg.PRs.Open(PRKey{Repo: s.Repo, Number: s.Number})
		return open && ciKindHolds(s.Kind, view.CheckConclusion), nil
	})
	d.RegisterPredicate(domain.WakeTriggerPREvent, func(ctx context.Context, spec domain.WakeSpec) (bool, error) {
		s, ok := spec.(domain.PREventSpec)
		if !ok || d.cfg.PRs == nil {
			return false, nil
		}
		key := PRKey{Repo: s.Repo, Number: s.Number}
		if _, open := d.cfg.PRs.Open(key); open {
			return false, nil // still open: no network call for the common case
		}
		facts, err := d.cfg.PRs.Lookup(ctx, key)
		if err != nil {
			return false, fmt.Errorf("pr_event predicate %s: %w", prCellKey(key), err)
		}
		return facts.State == s.Kind, nil
	})
}

// checkSource reports ErrUnavailable when spec's PR is not one the poller
// holds, so schedule_wake refuses instead of arming a trigger nothing can fire.
func (d *Dispatcher) checkSource(ctx context.Context, owner string, spec domain.WakeSpec) error {
	var key PRKey
	switch s := spec.(type) {
	case domain.PREventSpec:
		key = PRKey{Repo: s.Repo, Number: s.Number}
	case domain.CIEventSpec:
		key = PRKey{Repo: s.Repo, Number: s.Number}
	case domain.SessionStateSpec:
		if d.cfg.Sessions == nil {
			return fmt.Errorf("%w: session state is not wired", ErrUnavailable)
		}
		return d.authorizeSessionTarget(ctx, owner, s.SessionUUID)
	case domain.GuidanceAnsweredSpec:
		if d.cfg.Guidance == nil {
			return fmt.Errorf("%w: guidance requests are not wired", ErrUnavailable)
		}
		return d.authorizeGuidanceRequest(ctx, owner, s.GuidanceRequestID)
	default:
		return nil
	}
	if d.cfg.PRs == nil {
		return fmt.Errorf("%w: no pull-request poller is running", ErrUnavailable)
	}
	if _, open := d.cfg.PRs.Open(key); !open {
		return fmt.Errorf("%w: %s is not an open pull request of a connected account", ErrUnavailable, prCellKey(key))
	}
	return nil
}

// authorizeGuidanceRequest limits a guidance_answered wake to the asker, since
// the wake echoes the answer into the owner's pane.
func (d *Dispatcher) authorizeGuidanceRequest(ctx context.Context, owner, requestID string) error {
	f, err := d.cfg.Guidance.Guidance(ctx, requestID)
	if err != nil {
		return fmt.Errorf("%w: guidance request lookup: %v", ErrUnavailable, err)
	}
	if !f.Exists || f.AskerSessionUUID == "" || f.AskerSessionUUID != owner {
		return fmt.Errorf("%w: guidance request %s is not yours", ErrForbidden, requestID)
	}
	return nil
}

// authorizeSessionTarget limits a session_state wake to the owner itself or a
// session in the owner's repository (same boundary as PeerFaultOf).
func (d *Dispatcher) authorizeSessionTarget(ctx context.Context, owner, target string) error {
	if target == owner {
		return nil
	}
	denied := fmt.Errorf("%w: session %s is not in your workspace", ErrForbidden, target)
	tf, err := d.cfg.Owners.Owner(ctx, target)
	if err != nil || !tf.Exists {
		return denied
	}
	of, err := d.cfg.Owners.Owner(ctx, owner)
	if err != nil || !of.Exists || tf.WorkspaceKey == "" || tf.WorkspaceKey != of.WorkspaceKey {
		return denied
	}
	return nil
}

// prWatcher diffs successive poller snapshots into pr_event and ci_event
// firings. The poller lists open PRs only, so a merged or closed PR is a PR
// that vanished; a by-number lookup confirms which.
type prWatcher struct {
	d       *Dispatcher
	prev    map[PRKey]string // check conclusion by open PR
	seeded  bool
	pending map[PRKey]struct{} // vanished PRs still to confirm
}

// startPRWatcher runs the watcher when a poller is wired.
func (d *Dispatcher) startPRWatcher(ctx context.Context) {
	if d.cfg.PRs == nil {
		return
	}
	ch, err := d.cfg.PRs.Subscribe(ctx)
	if err != nil {
		log.Warn("wake dispatcher: PR poller subscribe failed", errKey, err)
		return
	}
	w := &prWatcher{d: d, prev: map[PRKey]string{}, pending: map[PRKey]struct{}{}}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case snap, ok := <-ch:
				if !ok {
					return
				}
				w.onSnapshot(ctx, snap)
			}
		}
	}()
}

func (w *prWatcher) onSnapshot(ctx context.Context, snap []PRView) {
	cur := make(map[PRKey]string, len(snap))
	for _, v := range snap {
		cur[v.PRKey] = v.CheckConclusion
	}
	if w.seeded {
		for key, now := range cur {
			if before, known := w.prev[key]; known && !ciFinal(before) && ciFinal(now) {
				w.fireCI(ctx, key, now)
			}
		}
		for key := range w.prev {
			if _, open := cur[key]; !open {
				w.pending[key] = struct{}{}
			}
		}
	}
	w.prev, w.seeded = cur, true
	w.confirmVanished(ctx, cur)
}

func (w *prWatcher) fireCI(ctx context.Context, key PRKey, conclusion string) {
	detail := eventJSON(map[string]any{"pr": key.Number, "conclusion": conclusion})
	w.d.fireCell(ctx, EventMatch{
		Type: domain.WakeTriggerCIEvent, Key: prCellKey(key), Kinds: []string{CIKindCompleted, conclusion},
	}, detail)
}

// confirmVanished resolves each vanished PR that has an armed wake. A failed
// lookup leaves it pending for the next poll; one with no armed wake, or that
// turns out still open, is dropped.
func (w *prWatcher) confirmVanished(ctx context.Context, open map[PRKey]string) {
	for key := range w.pending {
		if _, reopened := open[key]; reopened || !w.armed(key) {
			delete(w.pending, key)
			continue
		}
		facts, err := w.d.cfg.PRs.Lookup(ctx, key)
		if err != nil {
			log.Warn("wake dispatcher: PR lookup failed; retrying next poll", "pr", prCellKey(key), errKey, err)
			continue
		}
		delete(w.pending, key)
		if facts.State != PRStateMerged && facts.State != PRStateClosed {
			continue
		}
		w.d.fireCell(ctx, EventMatch{
			Type: domain.WakeTriggerPREvent, Key: prCellKey(key), Kinds: []string{facts.State},
		}, eventJSON(map[string]any{"pr": key.Number, "state": facts.State}))
	}
}

func (w *prWatcher) armed(key PRKey) bool {
	return len(w.d.index.lookup(domain.WakeTriggerPREvent, prCellKey(key))) > 0
}

func eventJSON(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}
