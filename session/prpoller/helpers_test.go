package prpoller

import (
	"context"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func mustPRRef(t *testing.T, host, owner, repo string, n int) domain.PRRef {
	t.Helper()
	ref, err := domain.NewPRRef(host, owner, repo, n)
	if err != nil {
		t.Fatalf("NewPRRef(%q,%q,%q,%d): %v", host, owner, repo, n, err)
	}
	return ref
}

func newLegacyTestPoller(client prGitHubClient, insts ...*session.Instance) *PRStatusPoller {
	poller := NewPRStatusPoller(nil)
	poller.ghClient = client
	poller.ctx = context.Background()
	poller.cancel = func() {}
	poller.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	poller.SetInstances(insts)
	return poller
}
