// Package sessionpr persists PullRequestState rows, the session/backlog-item
// links to them and the tombstones of detached links. session.EntRepository
// embeds *PRStore so existing callers keep their method set.
package sessionpr

import (
	"sync"
	"sync/atomic"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/session/ent"
)

// ItemSessionBatchChunk keeps IN (...) lists well under SQLite's bound-variable limit. It lives here, the
// lowest package that chunks, so the backlog code can import it instead of owning a second copy.
const ItemSessionBatchChunk = 500

// PRStore owns the pull_request_states, session_prs and session_pr_tombstones tables.
type PRStore struct {
	client *ent.Client
	// driver runs the raw join in the backfill; nil when built from a bare client.
	driver dialect.Driver

	// verifyCountersTrusted flips true once this process's PR startup maintenance has succeeded.
	verifyCountersTrusted atomic.Bool
	drops                 prDropTracker
	// afterLinkHook (tests only) runs inside AttachSessionPR's transaction after the link write.
	afterLinkHook func() error
	hookMu        sync.Mutex
}

// New returns a PRStore over client, which must already hold the migrated schema.
func New(client *ent.Client) *PRStore {
	return &PRStore{client: client}
}

// SetDriver supplies the driver behind client for the backfill's raw join.
func (s *PRStore) SetDriver(d dialect.Driver) { s.driver = d }

// SetAfterLinkHook installs a test hook run inside AttachSessionPR's transaction after the link write.
func (s *PRStore) SetAfterLinkHook(fn func() error) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	s.afterLinkHook = fn
}

// AfterLinkHook returns the hook installed by SetAfterLinkHook, or nil.
func (s *PRStore) AfterLinkHook() func() error {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	return s.afterLinkHook
}

// VerifyCountersTrusted reports whether this process's PR startup maintenance has succeeded.
func (s *PRStore) VerifyCountersTrusted() bool { return s.verifyCountersTrusted.Load() }
