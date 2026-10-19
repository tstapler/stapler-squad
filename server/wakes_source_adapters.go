package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// Adapters for the wake sources that read sessions, guidance requests and the
// PR poller (Story 4.4). Like wakes_adapters.go, this is the seam: server/wakes
// never imports the services these wrap.

// sessionStateLabel is the lowercase wake vocabulary for a lifecycle status.
func sessionStateLabel(s session.Status) string {
	switch s {
	case session.PermanentlyFailed:
		return "permanently_failed"
	default:
		return strings.ToLower(s.String())
	}
}

// wakeSessionStates reports "idle" for a live session ready for input, else
// the lifecycle status; a session the registry lost falls back to storage.
type wakeSessionStates struct {
	svc     *services.SessionService
	storage *session.Storage
}

func (w wakeSessionStates) SessionState(_ context.Context, sessionUUID string) (wakes.SessionFacts, error) {
	if inst := w.svc.FindLiveInstance(sessionUUID); inst != nil {
		if inst.IsActive() && w.svc.IsReadyForSteer(sessionUUID) {
			return wakes.SessionFacts{Exists: true, State: "idle"}, nil
		}
		return wakes.SessionFacts{Exists: true, State: sessionStateLabel(session.Status(inst.GetStatus()))}, nil
	}
	data, err := w.storage.FindInstanceDataByID(sessionUUID)
	switch {
	case errors.Is(err, session.ErrInstanceDataNotFound):
		return wakes.SessionFacts{}, nil
	case err != nil:
		return wakes.SessionFacts{}, err
	}
	return wakes.SessionFacts{Exists: true, State: sessionStateLabel(data.Status)}, nil
}

// wakeGuidance reads a guidance request's answered state.
type wakeGuidance struct{ storage *session.Storage }

func (w wakeGuidance) Guidance(ctx context.Context, id string) (wakes.GuidanceFacts, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return wakes.GuidanceFacts{}, nil
	}
	data, err := w.storage.GetGuidanceRequest(ctx, parsed)
	if errors.Is(err, session.ErrNotFound) {
		return wakes.GuidanceFacts{}, nil
	}
	if err != nil {
		return wakes.GuidanceFacts{}, err
	}
	return wakes.GuidanceFacts{
		Exists:              true,
		Answered:            data.Status() == session.GuidanceRequestStatusAnswered,
		Cancelled:           data.Status() == session.GuidanceRequestStatusCancelled,
		CancelReason:        string(data.CancelReason),
		Answer:              data.Answer,
		AnswererKind:        string(data.AnswererKind),
		AnswererSessionUUID: data.AnswererSessionUUID,
	}, nil
}

// wakePRSource adapts the UserPRCache poller (Spike S4: it lists open PRs
// only, so a merged PR is confirmed by a by-number lookup).
type wakePRSource struct{ cache *github.UserPRCache }

const prSubscriberBuffer = 4

func prKeyOf(p github.UserPR) wakes.PRKey {
	return wakes.PRKey{Repo: p.Owner + "/" + p.Repo, Number: p.Number}
}

func (s wakePRSource) Open(key wakes.PRKey) (wakes.PRView, bool) {
	for _, p := range s.cache.GetAll() {
		if prKeyOf(p) == key {
			return wakes.PRView{PRKey: key, CheckConclusion: p.CheckConclusion}, true
		}
	}
	return wakes.PRView{}, false
}

func (s wakePRSource) Subscribe(ctx context.Context) (<-chan []wakes.PRView, error) {
	id := "wake-dispatcher-" + uuid.NewString()
	in := make(chan []github.UserPR, prSubscriberBuffer)
	out := make(chan []wakes.PRView, prSubscriberBuffer)
	s.cache.Subscribe(id, in)
	go func() {
		defer s.cache.Unsubscribe(id)
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case prs := <-in:
				views := make([]wakes.PRView, len(prs))
				for i, p := range prs {
					views[i] = wakes.PRView{PRKey: prKeyOf(p), CheckConclusion: p.CheckConclusion}
				}
				select {
				case out <- views:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (s wakePRSource) Lookup(ctx context.Context, key wakes.PRKey) (wakes.PRFacts, error) {
	owner, repo, ok := strings.Cut(key.Repo, "/")
	if !ok {
		return wakes.PRFacts{}, fmt.Errorf("pr lookup: repo %q is not owner/repo", key.Repo)
	}
	ref, err := github.NewRepoRef(owner, repo)
	if err != nil {
		return wakes.PRFacts{}, fmt.Errorf("pr lookup: %w", err)
	}
	info, err := github.GetPRByNumber(ctx, ref, key.Number)
	if err != nil {
		return wakes.PRFacts{}, fmt.Errorf("pr lookup %s#%d: %w", key.Repo, key.Number, err)
	}
	return wakes.PRFacts{State: strings.ToLower(info.State)}, nil
}
