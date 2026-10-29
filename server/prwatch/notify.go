package prwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Operator notifications (D15). Texts are plain language built only from
// validated owner/name, ints, counts, the operator's own config values and,
// for CoverageGap, the sanitized reason gh-signal gave. They never carry PR
// text, actor logins or session titles, and none of the raw enum words.

const (
	// GuideRelPath is the operator doc, relative to the Squad checkout.
	GuideRelPath = "docs/how-to/inbox-pr-watch.md"
	// GuideFallback is the Guide line text when no checkout root resolves.
	GuideFallback = GuideRelPath + " in the stapler-squad repository"

	// TitleCIOnly announces that human-feedback steering is off (effective
	// mode CI-only); it is raised once per process.
	TitleCIOnly = "PR watch: only CI failures will steer sessions"

	steerFailedMinGap   = 10 * time.Minute
	strangerWindow      = 7 * 24 * time.Hour
	defaultStrangerMax  = 3
	reasonMaxRunes      = 200
	defaultCoverageNext = "run gh-signal inbox watch %[1]s %[2]d --consumer %[3]s. To stop this notice instead, add %[1]s to inbox_pr_watch.ignore_repos."
)

var ansiSeq = regexp.MustCompile(`\x1b(\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(\x07|\x1b\\)|[@-Z\\-_])`)

// sanitizeReason strips escape sequences, control characters and newlines
// and cuts to n runes.
func sanitizeReason(s string, n int) string {
	s = ansiSeq.ReplaceAllString(strings.ToValidUTF8(s, ""), "")
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := []rune(b.String())
	if len(out) > n {
		out = out[:n]
	}
	return strings.TrimSpace(string(out))
}

// ResolveGuidePath returns the absolute path of the operator doc, found by
// walking up from each start directory (at most 6 levels), or GuideFallback.
// A relative path is never returned. exists may be nil (uses os.Stat).
func ResolveGuidePath(startDirs []string, exists func(string) bool) string {
	if exists == nil {
		exists = func(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
	}
	for _, start := range startDirs {
		if start == "" || !filepath.IsAbs(start) {
			continue
		}
		dir := start
		for i := 0; i < 6; i++ {
			if p := filepath.Join(dir, filepath.FromSlash(GuideRelPath)); exists(p) {
				return p
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return GuideFallback
}

// holdPhrase states why an item is waiting, in words (never the enum).
func holdPhrase(r HoldReason) string {
	switch r {
	case HoldBusy:
		return "the session is busy"
	case HoldGuardBusy:
		return "another notice was being written to the session"
	case HoldCoolingDown:
		return "the session was steered a moment ago"
	case HoldNoStatusSource, HoldNotTracked:
		return "Squad cannot tell whether the session is ready"
	case HoldSessionSuspended:
		return "the session is paused or not running"
	case HoldLoginUnresolved:
		return "Squad could not determine your GitHub login"
	case HoldWindowBatching:
		return "Squad is batching events and the window is still open"
	case HoldSteerFailed:
		return "the write into the session failed"
	case HoldCIDuplicateNewSHA:
		return "a CI result for a new commit is waiting"
	default:
		return "the session has not been ready to receive it"
	}
}

// Notifications builds, dedupes and clears the D15 notifications.
type Notifications struct {
	n        Notifier
	clock    Clock
	s        Settings
	sanitize func(string, int) string

	mu       sync.Mutex
	raised   map[string]time.Time // dedupe key -> last raised
	ciOnlyOK bool
}

// NewNotifications wires the builders. sanitize may be nil (a built-in
// sanitizer is used); the server passes session.SanitizeUntrusted.
func NewNotifications(n Notifier, clock Clock, s Settings, sanitize func(string, int) string) *Notifications {
	if sanitize == nil {
		sanitize = sanitizeReason
	}
	return &Notifications{n: n, clock: clock, s: s, sanitize: sanitize, raised: map[string]time.Time{}}
}

func (x *Notifications) footer() string {
	guide := x.s.GuidePath
	if guide == "" || !filepath.IsAbs(guide) {
		guide = GuideFallback
	}
	return "\nGuide: " + guide + "\nStatus: " + x.s.StatusURL
}

// send publishes unless suppress says the key is already raised. A failed
// Notify leaves the key unraised so the next call retries.
func (x *Notifications) send(n Notification, minGap time.Duration) error {
	now := x.clock.Now()
	x.mu.Lock()
	at, ok := x.raised[n.DedupeKey]
	if ok && (minGap == 0 || now.Sub(at) < minGap) {
		x.mu.Unlock()
		return nil
	}
	x.raised[n.DedupeKey] = now
	x.mu.Unlock()
	n.Message += x.footer()
	if err := x.n.Notify(n); err != nil {
		x.mu.Lock()
		if ok {
			x.raised[n.DedupeKey] = at
		} else {
			delete(x.raised, n.DedupeKey)
		}
		x.mu.Unlock()
		return err
	}
	return nil
}

func (x *Notifications) forget(keys ...string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, k := range keys {
		delete(x.raised, k)
	}
}

func prKey(session, repo string, pr int) string {
	return fmt.Sprintf("%s:%s#%d", session, repo, pr)
}

func coverageKey(repo string) string { return "prwatch-coverage:" + repo }

// CoverageGap raises one notice per repo until CoverageRestored(repo).
func (x *Notifications) CoverageGap(repo string, pr int, reason string) error {
	reason = x.sanitize(reason, reasonMaxRunes)
	reason = sanitizeReason(reason, reasonMaxRunes)
	said := "gh-signal said: " + reason + ". "
	if reason == "" {
		said = "gh-signal gave no reason. "
	}
	next := fmt.Sprintf(defaultCoverageNext, repo, pr, x.s.ConsumerID)
	if strings.Contains(strings.ToLower(reason), "rate limit") {
		next = fmt.Sprintf("wait; Squad checks again every 10 minutes and the notice clears once the webhook is in place. To stop this notice instead, add %s to inbox_pr_watch.ignore_repos.", repo)
	}
	return x.send(Notification{
		Kind:      NotifyCoverageGap,
		DedupeKey: coverageKey(repo),
		Title:     strings.ReplaceAll(TitleCoverageGap, "<owner>/<repo>", repo),
		Message: fmt.Sprintf("Squad cannot watch PR #%d in %s because no GitHub webhook reaches gh-signal for that repository, so sessions on its PRs will not be steered. %sNext step: %s",
			pr, repo, said, next),
	}, 0)
}

// CoverageRestored forgets the repo's gap so a later loss raises again.
func (x *Notifications) CoverageRestored(repo string) { x.forget(coverageKey(repo)) }

// SteerFailed raises at most once per 10 minutes per session and PR, and not
// again until a steer is delivered.
func (x *Notifications) SteerFailed(session, repo string, pr int) error {
	return x.send(Notification{
		Kind:        NotifySteerFailed,
		SessionUUID: session,
		DedupeKey:   "prwatch-steer-failed:" + prKey(session, repo, pr),
		Title:       TitleSteerFailed,
		Message: fmt.Sprintf("Squad could not write the PR #%d (%s) notice into this session because the write failed. The event stays pending and Squad will retry. Next step: if it keeps happening, open the session.",
			pr, repo),
	}, steerFailedMinGap)
}

// HeldLong raises once per session and PR until a steer is delivered.
func (x *Notifications) HeldLong(session, repo string, pr int, waited time.Duration, reason HoldReason) error {
	hours := int(x.s.HeldAgeCeiling / time.Hour)
	return x.send(Notification{
		Kind:        NotifyHeldLong,
		SessionUUID: session,
		DedupeKey:   "prwatch-held-long:" + prKey(session, repo, pr),
		Title:       TitleHeldLong,
		Message: fmt.Sprintf("An event for PR #%d (%s) has waited %d minutes because %s. Squad will drop it after %d hours. Next step: open the session or the PR yourself.",
			pr, repo, int(waited/time.Minute), holdPhrase(reason), hours),
	}, 0)
}

// HeldDropped raises once per session and PR until a steer is delivered.
func (x *Notifications) HeldDropped(session, repo string, pr int, reason HoldReason) error {
	hours := int(x.s.HeldAgeCeiling / time.Hour)
	return x.send(Notification{
		Kind:        NotifyHeldDropped,
		SessionUUID: session,
		DedupeKey:   "prwatch-held-drop:" + prKey(session, repo, pr),
		Title:       TitleHeldDropped,
		Message: fmt.Sprintf("An event for PR #%d (%s) waited more than %d hours because %s, and was dropped. Next step: open the PR to review it.",
			pr, repo, hours, holdPhrase(reason)),
	}, 0)
}

// SteerDelivered clears the failure and held notices for the session and PR.
func (x *Notifications) SteerDelivered(session, repo string, pr int) {
	k := prKey(session, repo, pr)
	x.forget("prwatch-steer-failed:"+k, "prwatch-held-long:"+k, "prwatch-held-drop:"+k)
}

const loginKey = "prwatch-login-unresolved"

// LoginUnresolved raises once until LoginResolved.
func (x *Notifications) LoginUnresolved() error {
	return x.send(Notification{
		Kind:      NotifyLoginUnresolved,
		DedupeKey: loginKey,
		Title:     TitleLoginUnresolved,
		Message:   "Squad could not determine your GitHub login for 10 minutes, so review and comment events are held (CI events are unaffected). Next step: run gh auth status and check that Squad can reach api.github.com.",
	}, 0)
}

// LoginResolved clears the login notice.
func (x *Notifications) LoginResolved() { x.forget(loginKey) }

// CIOnlyMode raises one notice per process when the effective human-feedback
// mode is CI-only (mode off, or allowlist with an empty list).
func (x *Notifications) CIOnlyMode() error {
	x.mu.Lock()
	done := x.ciOnlyOK
	x.ciOnlyOK = true
	x.mu.Unlock()
	if done {
		return nil
	}
	msg := "Comments and reviews will not steer sessions because inbox_pr_watch.human_feedback_mode leaves no one allowed to trigger them; only failing CI checks will. Next step: to steer on comments and reviews, set human_feedback_mode to all, or to allowlist and list the logins you trust."
	err := x.n.Notify(Notification{
		Kind:      NotifyInboxState,
		DedupeKey: "prwatch-ci-only",
		Title:     TitleCIOnly,
		Message:   msg + x.footer(),
	})
	if err != nil {
		x.mu.Lock()
		x.ciOnlyOK = false
		x.mu.Unlock()
	}
	return err
}

func inboxStateKey(st InboxState) string { return "prwatch-inbox-state:" + string(st) }

// inboxStateText returns title and message body for a state; ok=false for
// InboxStateOK and unknown states.
func (x *Notifications) inboxStateText(st InboxState, mode string) (title, msg string, ok bool) {
	switch st {
	case InboxStateUnreachable:
		return TitleInboxUnreachable, fmt.Sprintf("Squad cannot reach the gh-signal inbox at %s, so PR activity is not steering sessions. Next step: start the gh-signal-inbox service.", x.s.URL), true
	case InboxStateUnauthorized:
		return TitleInboxUnauthorized, fmt.Sprintf("The gh-signal inbox answered 401 to the token in %s. Next step: make sure that file holds the token the running inbox expects; restart gh-signal-inbox if the token was rotated.", x.s.TokenPath), true
	case InboxStateTokenMissing:
		return TitleTokenMissing, fmt.Sprintf("Squad found no readable token file at %s. Next step: check that gh-signal-inbox is installed and has created the file, or set inbox_pr_watch.token_path to the right file.", x.s.TokenPath), true
	case InboxStateTokenPerms:
		return TitleTokenPermissions, fmt.Sprintf("%s can be read by other users (mode %s). Squad refuses to use it. Next step: run chmod 600 on that file.", x.s.TokenPath, mode), true
	case InboxStateNonLoopback:
		return TitleNonLoopbackURL, fmt.Sprintf("Squad refuses inbox_pr_watch.url %s because it is not 127.0.0.1, ::1 or localhost. Next step: fix the url in the Squad config, or turn the inbox_pr_watch flag off.", x.s.URL), true
	case InboxStateLeaderNotHeld:
		return TitleLeaderNotHeld, fmt.Sprintf("Another Squad process holds %s, so this one does not poll the inbox. This is expected when two Squads run. Next step: otherwise stop the other process; this one retries every %d seconds.", x.s.LockPath, int(x.s.LeaderRetry/time.Second)), true
	}
	return "", "", false
}

func stateResolvedPhrase(st InboxState) string {
	switch st {
	case InboxStateUnreachable:
		return "The gh-signal inbox being unreachable"
	case InboxStateUnauthorized:
		return "The inbox rejecting Squad's token"
	case InboxStateTokenMissing:
		return "The missing token file"
	case InboxStateTokenPerms:
		return "The token file permissions problem"
	case InboxStateNonLoopback:
		return "The inbox URL problem"
	case InboxStateLeaderNotHeld:
		return "This Squad process being idle"
	}
	return "The problem"
}

// InboxState raises the notice for a state once until InboxStateRecovered.
// mode is the octal file mode, used by token_permissions only. Grace timing
// belongs to the caller (task 5.4).
func (x *Notifications) InboxState(st InboxState, mode string) error {
	title, msg, ok := x.inboxStateText(st, mode)
	if !ok {
		return nil
	}
	return x.send(Notification{Kind: NotifyInboxState, State: st, DedupeKey: inboxStateKey(st), Title: title, Message: msg}, 0)
}

// InboxStateRecovered overwrites the state's row with a recovery notice, only
// if the state had been raised, and forgets the key.
func (x *Notifications) InboxStateRecovered(st InboxState) error {
	key := inboxStateKey(st)
	x.mu.Lock()
	_, was := x.raised[key]
	x.mu.Unlock()
	if !was {
		return nil
	}
	err := x.n.Notify(Notification{
		Kind: NotifyInboxState, State: InboxStateOK, DedupeKey: key, Title: TitleRecovered,
		Message: stateResolvedPhrase(st) + " is resolved." + x.footer(),
	})
	if err == nil {
		x.forget(key)
	}
	return err
}

// StrangerTracker implements StrangerWatch: a rolling 7-day count of
// other-class steers per repo; more than the threshold raises one notice per
// repo per 7 days.
type StrangerTracker struct {
	notes     *Notifications
	clock     Clock
	threshold int

	mu     sync.Mutex
	steers map[string][]strangerSteer
	noted  map[string]time.Time
}

type strangerSteer struct {
	at time.Time
	pr int
}

var _ StrangerWatch = (*StrangerTracker)(nil)

// NewStrangerTracker builds the tracker; threshold <= 0 uses the default 3.
func NewStrangerTracker(notes *Notifications, clock Clock, threshold int) *StrangerTracker {
	if threshold <= 0 {
		threshold = defaultStrangerMax
	}
	return &StrangerTracker{notes: notes, clock: clock, threshold: threshold,
		steers: map[string][]strangerSteer{}, noted: map[string]time.Time{}}
}

// prune drops steers older than the window; caller holds mu.
func (t *StrangerTracker) prune(repo string, now time.Time) []strangerSteer {
	kept := t.steers[repo][:0]
	for _, s := range t.steers[repo] {
		if now.Sub(s.at) < strangerWindow {
			kept = append(kept, s)
		}
	}
	t.steers[repo] = kept
	if len(kept) <= t.threshold {
		delete(t.noted, repo)
	}
	return kept
}

// RecordSteer counts only other-class steers.
func (t *StrangerTracker) RecordSteer(repo string, pr int, class ActorClass) {
	if class != ActorOther {
		return
	}
	now := t.clock.Now()
	t.mu.Lock()
	t.steers[repo] = append(t.steers[repo], strangerSteer{at: now, pr: pr})
	kept := t.prune(repo, now)
	raise := len(kept) > t.threshold
	if at, ok := t.noted[repo]; raise && ok && now.Sub(at) < strangerWindow {
		raise = false
	}
	var count int
	var prs []int
	if raise {
		count = len(kept)
		seen := map[int]bool{}
		for _, s := range kept {
			if !seen[s.pr] {
				seen[s.pr] = true
				prs = append(prs, s.pr)
			}
		}
		sort.Ints(prs)
		t.noted[repo] = now
	}
	t.mu.Unlock()
	if raise && t.notes != nil {
		if err := t.notes.strangerActivity(repo, count, prs); err != nil {
			t.mu.Lock()
			delete(t.noted, repo)
			t.mu.Unlock()
		}
	}
}

// OtherCount is the rolling 7-day other-class count for the repo.
func (t *StrangerTracker) OtherCount(repo string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.prune(repo, t.clock.Now()))
}

func (x *Notifications) strangerActivity(repo string, count int, prs []int) error {
	refs := make([]string, len(prs))
	for i, p := range prs {
		refs[i] = fmt.Sprintf("#%d", p)
	}
	n := Notification{
		Kind:      NotifyStrangerActive,
		DedupeKey: "prwatch-stranger:" + repo,
		Title:     strings.ReplaceAll(TitleStrangerActivity, "<owner>/<repo>", repo),
		Message: fmt.Sprintf("%d steers on %s in the last 7 days (PRs %s) were caused by accounts that are not you, not a bot and not on your allowlist. Squad cannot tell whether they are welcome. Next step: if you do not recognize this activity, set inbox_pr_watch.human_feedback_mode to allowlist and list the logins you trust, or to off for CI-only steering, or add %s to inbox_pr_watch.ignore_repos.",
			count, repo, strings.Join(refs, ", "), repo) + x.footer(),
	}
	return x.n.Notify(n)
}
