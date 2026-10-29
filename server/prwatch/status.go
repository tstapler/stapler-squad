package prwatch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	statusTitleMax  = 40
	statusReasonMax = 200
	shortUUIDLen    = 8
	steerClassCI    = "ci" // key for steers with no actor class
)

// NextSteerState says how to read WatchedStatus.NextSteerAt.
type NextSteerState string

const (
	NextSteerNow     NextSteerState = "now"     // nothing held, no window
	NextSteerAt      NextSteerState = "at"      // a window closes at NextSteerAt
	NextSteerWaiting NextSteerState = "waiting" // something is held for a reason other than a window
)

// SettingCorrection is one key corrected, clamped or ignored at config load.
type SettingCorrection struct {
	Key       string `json:"key"`
	Bad       string `json:"bad"`
	Effective string `json:"effective"`
}

// HeldItem is one event left unacked. No text and no actor.
type HeldItem struct {
	SessionUUID string
	Repo        string
	PR          int
	Kind        Kind
	Reason      HoldReason
	Since       time.Time
}

// SettingsView is the effective configuration as shown to the operator.
// Allowlists appear as counts only: they hold logins.
type SettingsView struct {
	HumanFeedbackMode   HumanFeedbackMode   `json:"human_feedback_mode"`
	ModeDescription     string              `json:"mode_description"`
	CIOnly              bool                `json:"ci_only"`
	PollInterval        string              `json:"poll_interval"`
	MinSteerInterval    string              `json:"min_steer_interval"`
	IgnoreRepos         []string            `json:"ignore_repos"`
	ActorAllowlistCount int                 `json:"actor_allowlist_count"`
	BotAllowlistCount   int                 `json:"bot_actor_allowlist_count"`
	Corrections         []SettingCorrection `json:"corrections"`
}

// RepoStatus is one repository's coverage.
type RepoStatus struct {
	Repo     string        `json:"repo"`
	Coverage CoverageState `json:"coverage"`
	Reason   string        `json:"reason,omitempty"`
}

// WatchedStatus is one watched session.
type WatchedStatus struct {
	ShortUUID   string         `json:"session"`
	Title       string         `json:"title"`
	Repo        string         `json:"repo"`
	PR          int            `json:"pr"`
	State       SessionState   `json:"state"`
	Subscribed  bool           `json:"subscribed"`
	LastEventAt *time.Time     `json:"last_event_at,omitempty"`
	Coverage    CoverageState  `json:"coverage"` // covered | unavailable | unknown marker
	HeldCount   int            `json:"held"`
	NextSteer   NextSteerState `json:"next_steer"`
	NextSteerAt *time.Time     `json:"next_steer_at,omitempty"`
}

// SkippedStatus is one session that is not watched.
type SkippedStatus struct {
	ShortUUID string     `json:"session"`
	Title     string     `json:"title"`
	Repo      string     `json:"repo,omitempty"`
	PR        int        `json:"pr,omitempty"`
	Reason    SkipReason `json:"reason"`
}

// HeldStatus is one waiting event.
type HeldStatus struct {
	ShortUUID      string     `json:"session"`
	Repo           string     `json:"repo"`
	PR             int        `json:"pr"`
	Kind           Kind       `json:"kind"`
	Reason         HoldReason `json:"reason"`
	AgeSeconds     int64      `json:"age_seconds"`
	WindowClosesAt *time.Time `json:"window_closes_at,omitempty"`
}

// LastSteerStatus is the most recent delivered steer.
type LastSteerStatus struct {
	At         time.Time `json:"at"`
	Kind       Kind      `json:"kind"`
	Repo       string    `json:"repo"`
	PR         int       `json:"pr"`
	ShortUUID  string    `json:"session"`
	ActorClass string    `json:"actor_class"`
}

// StatusCounts are totals that equal the sum of their breakdowns.
type StatusCounts struct {
	Watched          int              `json:"watched"`
	Skipped          int              `json:"skipped"`
	SkippedByReason  map[string]int   `json:"skipped_by_reason"`
	CoverageMarkers  map[string]int   `json:"watched_by_coverage"`
	Steers           int              `json:"steers"`
	SteersByClass    map[string]int   `json:"steers_by_actor_class"`
	BaselineCount    int              `json:"baseline_count"`
	CatchupSteers    int64            `json:"catchup_steers"`
	TickOverruns     int64            `json:"tick_overruns"`
	SharedGuardDupes int64            `json:"duplicate_shared_guard"`
	CISteersInWindow int64            `json:"ci_steer_inside_feedback_window"`
	Counters         map[string]int64 `json:"counters"`
}

// Status is the in-memory snapshot behind the operator status surface (D14).
// It holds no token, PR text or actor login.
type Status struct {
	Enabled         bool       `json:"enabled"`
	Polls           bool       `json:"polls"` // leader and flag on
	LeaderHeld      bool       `json:"leader_held"`
	InboxState      InboxState `json:"inbox_state"`
	InboxErrorClass string     `json:"inbox_error_class,omitempty"`
	InboxErrorAt    *time.Time `json:"inbox_error_at,omitempty"`
	LoginResolved   bool       `json:"login_resolved"`
	Since           time.Time  `json:"since"`
	FlagOnAt        *time.Time `json:"flag_on_at,omitempty"`
	FirstRunLine    string     `json:"first_run_line"`

	Settings SettingsView `json:"settings"`
	Repos    []RepoStatus `json:"repos"`
	// CoverageFraction is nil when any repository is unknown (not computable).
	CoverageFraction *float64 `json:"coverage_fraction"`

	Watched   []WatchedStatus  `json:"watched"`
	Skipped   []SkippedStatus  `json:"skipped"`
	Held      []HeldStatus     `json:"held"`
	LastSteer *LastSteerStatus `json:"last_steer,omitempty"`
	Recent    []Disposition    `json:"recent"` // newest last, at most 200
	Counts    StatusCounts     `json:"counts"`
}

type sessionActivity struct {
	subscribed bool
	lastEvent  time.Time
}

// StatusTracker accumulates the state a Status needs. It implements Recorder
// so the engine can hand it dispositions directly. Safe for concurrent use.
type StatusTracker struct {
	clock    Clock
	counters *CounterSet
	ring     RingBuffer

	mu            sync.Mutex
	since         time.Time
	enabled       bool
	leaderHeld    bool
	inboxState    InboxState
	inboxErrClass string
	inboxErrAt    time.Time
	loginResolved bool
	flagOnAt      time.Time
	baseline      int
	settings      Settings
	corrections   []SettingCorrection
	sessions      []SessionRef
	coverage      map[string]RepoStatus
	activity      map[string]sessionActivity
	held          []HeldItem
	lastSteer     *LastSteerStatus
	steersByClass map[string]int
}

var _ Recorder = (*StatusTracker)(nil)

// NewStatusTracker returns a tracker reading time from clock and counters
// from counters (either may be shared with the engine; counters may be nil).
func NewStatusTracker(clock Clock, counters *CounterSet) *StatusTracker {
	if counters == nil {
		counters = NewCounterSet()
	}
	return &StatusTracker{
		clock:         clock,
		counters:      counters,
		since:         clock.Now(),
		inboxState:    InboxStateOK,
		coverage:      map[string]RepoStatus{},
		activity:      map[string]sessionActivity{},
		steersByClass: map[string]int{},
	}
}

// SetEnabled records the flag. The first transition to on stamps FlagOnAt and
// records baseline (the count at flag-on, so criterion 5 cannot disarm itself).
func (t *StatusTracker) SetEnabled(on bool, baseline int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if on && !t.enabled && t.flagOnAt.IsZero() {
		t.flagOnAt = t.clock.Now()
		t.baseline = baseline
	}
	t.enabled = on
}

func (t *StatusTracker) SetLeaderHeld(held bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.leaderHeld = held
}

// SetInbox records the inbox state and the error class (never the message).
func (t *StatusTracker) SetInbox(state InboxState, errClass string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if state != InboxStateOK && (t.inboxState != state || t.inboxErrClass != errClass) {
		t.inboxErrAt = t.clock.Now()
	}
	t.inboxState, t.inboxErrClass = state, errClass
	if state == InboxStateOK {
		t.inboxErrClass, t.inboxErrAt = "", time.Time{}
	}
}

func (t *StatusTracker) SetLoginResolved(ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loginResolved = ok
}

// SetSettings records the effective settings and the load-time corrections.
func (t *StatusTracker) SetSettings(s Settings, corrections []SettingCorrection) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.settings = s
	t.corrections = append([]SettingCorrection(nil), corrections...)
}

// SetSessions replaces the session view (watched and skipped).
func (t *StatusTracker) SetSessions(refs []SessionRef) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions = append([]SessionRef(nil), refs...)
}

// SetCoverage records one repository's coverage; reason is P0's text.
func (t *StatusTracker) SetCoverage(repo string, state CoverageState, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.coverage[repo] = RepoStatus{Repo: repo, Coverage: state, Reason: sanitizeStatusText(reason, statusReasonMax)}
}

// SetSubscribed records whether the session's PR is subscribed.
func (t *StatusTracker) SetSubscribed(sessionUUID string, subscribed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.activity[sessionUUID]
	a.subscribed = subscribed
	t.activity[sessionUUID] = a
}

// NoteEvent records the time of the session's latest event.
func (t *StatusTracker) NoteEvent(sessionUUID string, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	a := t.activity[sessionUUID]
	a.lastEvent = at
	t.activity[sessionUUID] = a
}

// SetHeld replaces the list of waiting events.
func (t *StatusTracker) SetHeld(items []HeldItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.held = append([]HeldItem(nil), items...)
}

// Record stores d in the ring buffer and, for a steer, updates the last-steer
// line and the per-actor-class count.
func (t *StatusTracker) Record(d Disposition) {
	t.ring.Record(d)
	if d.Disposition != DispositionSteered {
		return
	}
	class := string(d.ActorClass)
	if class == "" {
		class = steerClassCI
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steersByClass[class]++
	t.lastSteer = &LastSteerStatus{
		At: d.At, Kind: d.Kind, Repo: d.Repo, PR: d.PR,
		ShortUUID: shortUUID(d.SessionUUID), ActorClass: class,
	}
}

// Snapshot builds the Status at now. NextEligible is read through co (may be
// nil, meaning no window state).
func (t *StatusTracker) Snapshot(co Coalescer) Status {
	now := t.clock.Now()
	t.mu.Lock()
	defer t.mu.Unlock()

	st := Status{
		Enabled:         t.enabled,
		LeaderHeld:      t.leaderHeld,
		Polls:           t.enabled && t.leaderHeld,
		InboxState:      t.inboxState,
		InboxErrorClass: t.inboxErrClass,
		LoginResolved:   t.loginResolved,
		Since:           t.since,
		Watched:         []WatchedStatus{},
		Skipped:         []SkippedStatus{},
		Held:            []HeldStatus{},
		Recent:          t.ring.Entries(),
	}
	if !t.inboxErrAt.IsZero() {
		at := t.inboxErrAt
		st.InboxErrorAt = &at
	}
	if !t.flagOnAt.IsZero() {
		at := t.flagOnAt
		st.FlagOnAt = &at
	}
	st.Settings = t.settingsView()

	// Held items per session, with the window close time for window holds.
	heldBySession := map[string][]HeldItem{}
	for _, h := range t.held {
		heldBySession[h.SessionUUID] = append(heldBySession[h.SessionUUID], h)
		hs := HeldStatus{
			ShortUUID: shortUUID(h.SessionUUID), Repo: h.Repo, PR: h.PR, Kind: h.Kind,
			Reason: h.Reason, AgeSeconds: int64(now.Sub(h.Since) / time.Second),
		}
		if h.Reason == HoldWindowBatching && co != nil {
			key := CoalesceKey{SessionUUID: h.SessionUUID, Repo: h.Repo, PR: h.PR, Class: h.Kind.Class()}
			if at, ok := co.NextEligible(key); ok {
				a := at
				hs.WindowClosesAt = &a
			}
		}
		st.Held = append(st.Held, hs)
	}

	repoSeen := map[string]bool{}
	markers := map[string]int{}
	skipped := map[string]int{}
	for _, ref := range t.sessions {
		if ref.SkipReason != "" {
			st.Skipped = append(st.Skipped, SkippedStatus{
				ShortUUID: shortUUID(ref.UUID), Title: sanitizeStatusText(ref.Title, statusTitleMax),
				Repo: ref.Repo, PR: ref.PR, Reason: ref.SkipReason,
			})
			skipped[string(ref.SkipReason)]++
			continue
		}
		if ref.State == StateGone {
			continue
		}
		cov := t.repoCoverage(ref.Repo).Coverage
		repoSeen[ref.Repo] = true
		markers[string(cov)]++
		act := t.activity[ref.UUID]
		w := WatchedStatus{
			ShortUUID: shortUUID(ref.UUID), Title: sanitizeStatusText(ref.Title, statusTitleMax),
			Repo: ref.Repo, PR: ref.PR, State: ref.State, Subscribed: act.subscribed,
			Coverage: cov, HeldCount: len(heldBySession[ref.UUID]),
		}
		if !act.lastEvent.IsZero() {
			le := act.lastEvent
			w.LastEventAt = &le
		}
		w.NextSteer, w.NextSteerAt = nextSteer(co, ref, w.HeldCount > 0, now)
		st.Watched = append(st.Watched, w)
	}
	sort.SliceStable(st.Watched, func(i, j int) bool {
		if st.Watched[i].Repo != st.Watched[j].Repo {
			return st.Watched[i].Repo < st.Watched[j].Repo
		}
		return st.Watched[i].PR < st.Watched[j].PR
	})

	repos := make([]string, 0, len(repoSeen))
	for r := range repoSeen {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	uncovered, unknown := 0, 0
	for _, r := range repos {
		rs := t.repoCoverage(r)
		st.Repos = append(st.Repos, rs)
		switch rs.Coverage {
		case CoverageStateUnavailable:
			uncovered++
		case CoverageStateUnknown:
			unknown++
		}
	}
	if st.Repos == nil {
		st.Repos = []RepoStatus{}
	}
	if len(repos) > 0 && unknown == 0 {
		f := float64(len(repos)-uncovered) / float64(len(repos))
		st.CoverageFraction = &f
	}
	st.FirstRunLine = firstRunLine(len(st.Watched), len(repos), uncovered, unknown)

	snap := t.counters.Snapshot()
	byClass := make(map[string]int, len(t.steersByClass))
	steers := 0
	for k, v := range t.steersByClass {
		byClass[k] = v
		steers += v
	}
	st.LastSteer = t.lastSteer
	st.Counts = StatusCounts{
		Watched: len(st.Watched), Skipped: len(st.Skipped), SkippedByReason: skipped,
		CoverageMarkers: markers, Steers: steers, SteersByClass: byClass,
		BaselineCount:    t.baseline,
		CatchupSteers:    t.counters.Total(MetricCatchupSteers),
		TickOverruns:     t.counters.Total(MetricTickOverrun),
		SharedGuardDupes: t.counters.Total(MetricDuplicateSharedGuard),
		CISteersInWindow: t.counters.Total(MetricCISteerInsideFeedbackWindow),
		Counters:         snap,
	}
	return st
}

// repoCoverage requires t.mu held. A repo never reported is unknown.
func (t *StatusTracker) repoCoverage(repo string) RepoStatus {
	if rs, ok := t.coverage[repo]; ok {
		return rs
	}
	return RepoStatus{Repo: repo, Coverage: CoverageStateUnknown}
}

// settingsView requires t.mu held.
func (t *StatusTracker) settingsView() SettingsView {
	s := t.settings
	v := SettingsView{
		HumanFeedbackMode:   s.HumanFeedbackMode,
		PollInterval:        s.PollInterval.String(),
		MinSteerInterval:    s.MinSteerInterval.String(),
		IgnoreRepos:         append([]string{}, s.IgnoreRepos...),
		ActorAllowlistCount: len(s.ActorAllowlist),
		BotAllowlistCount:   len(s.BotActorAllowlist),
		Corrections:         append([]SettingCorrection{}, t.corrections...),
	}
	switch {
	case s.HumanFeedbackMode == ModeAll:
		v.ModeDescription = "comments and reviews from any account steer; CI failures steer"
	case s.HumanFeedbackMode == ModeAllowlist && len(s.ActorAllowlist) > 0:
		v.ModeDescription = "comments and reviews steer only from allowlisted accounts; CI failures steer"
	default: // off, or allowlist with an empty list
		v.CIOnly = true
		v.ModeDescription = "CI failures only; comments and reviews do not steer"
	}
	return v
}

// nextSteer gives one value per session. The human-feedback window is the
// binding constraint; CI for a new head SHA steers immediately (D12).
func nextSteer(co Coalescer, ref SessionRef, anyHeld bool, now time.Time) (NextSteerState, *time.Time) {
	if co != nil {
		key := CoalesceKey{SessionUUID: ref.UUID, Repo: ref.Repo, PR: ref.PR, Class: ClassHumanFeedback}
		if at, ok := co.NextEligible(key); ok && at.After(now) {
			a := at
			return NextSteerAt, &a
		}
	}
	if anyHeld {
		return NextSteerWaiting, nil
	}
	return NextSteerNow, nil
}

func firstRunLine(prs, repos, uncovered, unknown int) string {
	if prs == 0 {
		return "Not watching any PRs."
	}
	line := fmt.Sprintf("Watching %d %s in %d %s; %d %s not covered.",
		prs, plural(prs, "PR", "PRs"), repos, plural(repos, "repository", "repositories"),
		uncovered, plural(uncovered, "repository", "repositories"))
	if unknown > 0 {
		line += fmt.Sprintf(" Coverage not yet confirmed for %d %s.", unknown, plural(unknown, "repository", "repositories"))
	}
	return line
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func shortUUID(u string) string {
	if len(u) > shortUUIDLen {
		return u[:shortUUIDLen]
	}
	return u
}

var statusANSISeq = regexp.MustCompile(`\x1b(\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)?|[@-Z\\-_])`)

// sanitizeStatusText strips escape sequences and control characters,
// collapses whitespace and cuts to max runes. prwatch may not import session,
// so this mirrors session.SanitizeUntrusted for the operator surface.
func sanitizeStatusText(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	s = statusANSISeq.ReplaceAllString(s, "")
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r):
		default:
			if space && b.Len() > 0 {
				b.WriteRune(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := []rune(b.String())
	if len(out) > max {
		out = out[:max]
	}
	return string(out)
}
