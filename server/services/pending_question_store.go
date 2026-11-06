package services

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// PendingQuestionStore is the in-memory registry of single-select
// AskUserQuestion dialogs a hidden session is waiting on (ADR-010 decision 2).
// It is keyed by the instance UUID. The hook stream only feeds it; which
// question is on screen is decided by session.DialogMatch at claim time.
const (
	pendingQuestionMaxTotal      = 256
	pendingQuestionMaxPerSession = 8
	pendingQuestionTTL           = 30 * time.Minute // INFERRED
	// pendingParallelWindow bounds how far apart two questions may be and still
	// count as asked in the same turn. INFERRED: parallel tool-call hooks fire together.
	pendingParallelWindow        = 30 * time.Second
	pendingQuestionMaxLabelRunes = 80
	pendingQuestionMaxOptions    = 7
	pendingQuestionTokenRunes    = 40
)

// QuestionShape classifies an AskUserQuestion payload.
type QuestionShape string

const (
	// QuestionShapeSingle: one question, single select, 1..7 non-empty labels.
	QuestionShapeSingle QuestionShape = "single"
	// QuestionShapeMulti: more than one question, or multiSelect.
	QuestionShapeMulti QuestionShape = "multi"
	// QuestionShapeUnknown: any other payload shape; never replyable.
	QuestionShapeUnknown QuestionShape = "unknown"
)

type pendingState uint8

const (
	pendingRegistered pendingState = iota
	pendingClaimed
	pendingClaimedDeleted
)

// PendingQuestion is one registered, replyable question.
type PendingQuestion struct {
	QuestionID  string
	SessionUUID string
	Dialog      session.QuestionDialog
	// Token labels the card, the audit line and the log; it is never an identity.
	Token     string
	CreatedAt time.Time
}

type pendingEntry struct {
	q      PendingQuestion
	state  pendingState
	claim  uint64
	expiry time.Time
}

// QuestionKey names one question of one session; the pair is always compared
// together, so a question id of another session is never a hit.
type QuestionKey struct {
	SessionUUID string
	QuestionID  string
}

// PendingQuestionClaim is the single-use capability of a claimed question. Its
// fields are unexported and its only constructor is PendingQuestionStore.Claim.
type PendingQuestionClaim struct {
	store *PendingQuestionStore
	q     PendingQuestion
	token uint64
}

// SessionUUID is the instance the question belongs to.
func (c PendingQuestionClaim) SessionUUID() string { return c.q.SessionUUID }

// Question is the claimed question.
func (c PendingQuestionClaim) Question() PendingQuestion { return c.q }

// Valid reports whether the claim came from Claim (a zero claim is refused by
// every consumer).
func (c PendingQuestionClaim) Valid() bool { return c.store != nil && c.token != 0 }

// Deleted reports whether the session was deleted while the claim was held.
func (c PendingQuestionClaim) Deleted() bool {
	if !c.Valid() {
		return true
	}
	return c.store.claimDeleted(c)
}

// ClaimResult is the outcome of PendingQuestionStore.Claim other than a claim.
type ClaimResult uint8

const (
	// ClaimOK: a claim was returned.
	ClaimOK ClaimResult = iota
	// ClaimNoPending: no registered, unexpired entry (answered, expired, unknown,
	// claimed by another reply, or another session's question).
	ClaimNoPending
)

// PendingQuestionStore is safe for concurrent use.
type PendingQuestionStore struct {
	mu        sync.Mutex
	now       func() time.Time
	entries   map[string]*pendingEntry // by question id
	bySession map[string][]string      // session uuid -> question ids, oldest first
	// unreplyable holds when each session recently asked a question that was never
	// registered (multi-select, unproofed). Only the parallel window matters.
	unreplyable map[string][]time.Time
	nextClaim   uint64
}

// NewPendingQuestionStore builds an empty store. now is injected by tests.
func NewPendingQuestionStore(now func() time.Time) *PendingQuestionStore {
	if now == nil {
		now = time.Now
	}
	return &PendingQuestionStore{
		now: now, entries: map[string]*pendingEntry{}, bySession: map[string][]string{},
		unreplyable: map[string][]time.Time{},
	}
}

// NewQuestionID returns 128 random bits as hex.
func NewQuestionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Register stores q (stamping CreatedAt). A live entry of the same session with
// the same question text and labels is replaced; the oldest entry goes when a
// bound is hit. It returns the session's PendingCount including q, taken in the
// same critical section so a concurrent Register cannot split the two.
func (s *PendingQuestionStore) Register(q PendingQuestion) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.expireLocked(now)
	q.CreatedAt = now
	for _, id := range s.bySession[q.SessionUUID] {
		if e := s.entries[id]; e != nil && e.state == pendingRegistered && sameDialog(e.q.Dialog, q.Dialog) {
			s.removeLocked(id)
			break
		}
	}
	for len(s.bySession[q.SessionUUID]) >= pendingQuestionMaxPerSession {
		if !s.evictOldestLocked(s.bySession[q.SessionUUID]) {
			break
		}
	}
	for len(s.entries) >= pendingQuestionMaxTotal {
		if !s.evictOldestGlobalLocked() {
			break
		}
	}
	s.entries[q.QuestionID] = &pendingEntry{q: q, state: pendingRegistered, expiry: now.Add(pendingQuestionTTL)}
	s.bySession[q.SessionUUID] = append(s.bySession[q.SessionUUID], q.QuestionID)
	return s.pendingCountLocked(q.SessionUUID, now)
}

// NoteUnreplyable records that the session asked a question that is not
// registered, so a registered one arriving within the parallel window is not
// mistaken for the only question on screen.
func (s *PendingQuestionStore) NoteUnreplyable(sessionUUID string) {
	if sessionUUID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.unreplyable[sessionUUID] = append(s.recentUnreplyableLocked(sessionUUID, now), now)
}

func (s *PendingQuestionStore) recentUnreplyableLocked(sessionUUID string, now time.Time) []time.Time {
	kept := s.unreplyable[sessionUUID][:0]
	for _, at := range s.unreplyable[sessionUUID] {
		if now.Sub(at) <= pendingParallelWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(s.unreplyable, sessionUUID)
	}
	return kept
}

func (s *PendingQuestionStore) pendingCountLocked(sessionUUID string, now time.Time) int {
	n := len(s.recentUnreplyableLocked(sessionUUID, now))
	for _, id := range s.bySession[sessionUUID] {
		e := s.entries[id]
		if e != nil && e.state == pendingRegistered && now.Sub(e.q.CreatedAt) <= pendingParallelWindow {
			n++
		}
	}
	return n
}

func sameDialog(a, b session.QuestionDialog) bool {
	if a.Question != b.Question || len(a.Labels) != len(b.Labels) {
		return false
	}
	for i := range a.Labels {
		if a.Labels[i] != b.Labels[i] {
			return false
		}
	}
	return true
}

// Peek returns a copy of the registered, unexpired question for the session.
func (s *PendingQuestionStore) Peek(k QuestionKey) (PendingQuestion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now())
	e := s.entries[k.QuestionID]
	if e == nil || e.q.SessionUUID != k.SessionUUID || e.state != pendingRegistered {
		return PendingQuestion{}, false
	}
	return e.q, true
}

// PeekQuestions is Peek for many keys under one lock, for a history page.
func (s *PendingQuestionStore) PeekQuestions(keys []QuestionKey) map[QuestionKey]PendingQuestion {
	out := make(map[QuestionKey]PendingQuestion, len(keys))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now())
	for _, k := range keys {
		if e := s.entries[k.QuestionID]; e != nil && e.q.SessionUUID == k.SessionUUID && e.state == pendingRegistered {
			out[k] = e.q
		}
	}
	return out
}

// Forget drops a registered (not claimed) entry whose durable row was cleared.
func (s *PendingQuestionStore) Forget(k QuestionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[k.QuestionID]; e != nil && e.q.SessionUUID == k.SessionUUID && e.state == pendingRegistered {
		s.removeLocked(k.QuestionID)
	}
}

// PendingCount is how many AskUserQuestion dialogs the session may be waiting on
// at once: its registered, unclaimed questions plus unregistered ones, counted
// only inside pendingParallelWindow. Parallel questions arrive together; an older
// unanswered entry was answered in the terminal, which the store cannot observe,
// and must not keep disabling Reply until its TTL.
func (s *PendingQuestionStore) PendingCount(sessionUUID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.expireLocked(now)
	return s.pendingCountLocked(sessionUUID, now)
}

// Claim is one critical section: lookup by UUID, state and expiry check, then
// registered -> claimed. A question that belongs to another session is
// ClaimNoPending, never a different outcome.
func (s *PendingQuestionStore) Claim(k QuestionKey) (PendingQuestionClaim, ClaimResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.now())
	e := s.entries[k.QuestionID]
	if e == nil || e.q.SessionUUID != k.SessionUUID || e.state != pendingRegistered {
		return PendingQuestionClaim{}, ClaimNoPending
	}
	s.nextClaim++
	e.state, e.claim = pendingClaimed, s.nextClaim
	return PendingQuestionClaim{store: s, q: e.q, token: e.claim}, ClaimOK
}

// Release returns a claimed entry to registered after a failure strictly before
// the first byte. A claimed-deleted entry is removed instead and never returns.
func (s *PendingQuestionStore) Release(c PendingQuestionClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[c.q.QuestionID]
	if e == nil || e.claim != c.token {
		return
	}
	if e.state == pendingClaimedDeleted {
		s.removeLocked(c.q.QuestionID)
		return
	}
	e.state, e.claim = pendingRegistered, 0
}

// Consume removes a claimed entry after the digit was handed to the pane.
func (s *PendingQuestionStore) Consume(c PendingQuestionClaim) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[c.q.QuestionID]; e != nil && e.claim == c.token {
		s.removeLocked(c.q.QuestionID)
	}
}

// SessionDeleted removes every registered entry of the session and marks a
// claimed one claimed-deleted so the before-first-byte checks stop its write.
func (s *PendingQuestionStore) SessionDeleted(sessionUUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range append([]string(nil), s.bySession[sessionUUID]...) {
		e := s.entries[id]
		if e == nil {
			continue
		}
		if e.state == pendingClaimed {
			e.state = pendingClaimedDeleted
			continue
		}
		s.removeLocked(id)
	}
}

// Len is the number of entries (tests and the gauge).
func (s *PendingQuestionStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *PendingQuestionStore) claimDeleted(c PendingQuestionClaim) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[c.q.QuestionID]
	return e == nil || e.claim != c.token || e.state == pendingClaimedDeleted
}

func (s *PendingQuestionStore) expireLocked(now time.Time) {
	for id, e := range s.entries {
		if e.state == pendingRegistered && !now.Before(e.expiry) {
			s.removeLocked(id)
		}
	}
}

func (s *PendingQuestionStore) removeLocked(id string) {
	e := s.entries[id]
	if e == nil {
		return
	}
	delete(s.entries, id)
	ids := s.bySession[e.q.SessionUUID]
	for i, v := range ids {
		if v == id {
			ids = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(ids) == 0 {
		delete(s.bySession, e.q.SessionUUID)
	} else {
		s.bySession[e.q.SessionUUID] = ids
	}
}

// evictOldestLocked removes the oldest registered entry among ids; a claimed
// entry is never evicted (its reply is in flight).
func (s *PendingQuestionStore) evictOldestLocked(ids []string) bool {
	for _, id := range ids {
		if e := s.entries[id]; e != nil && e.state == pendingRegistered {
			s.removeLocked(id)
			return true
		}
	}
	return false
}

func (s *PendingQuestionStore) evictOldestGlobalLocked() bool {
	var oldestID string
	var oldest time.Time
	for id, e := range s.entries {
		if e.state != pendingRegistered {
			continue
		}
		if oldestID == "" || e.q.CreatedAt.Before(oldest) {
			oldestID, oldest = id, e.q.CreatedAt
		}
	}
	if oldestID == "" {
		return false
	}
	s.removeLocked(oldestID)
	return true
}

// QuestionToken is the whitespace-normalized first 40 runes of the question.
func QuestionToken(question string) string {
	norm := strings.Join(strings.Fields(question), " ")
	r := []rune(norm)
	if len(r) > pendingQuestionTokenRunes {
		r = r[:pendingQuestionTokenRunes]
	}
	return string(r)
}
