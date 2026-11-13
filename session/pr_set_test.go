package session

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
)

func unverifiedTerminal(v domain.PRView, exhausted bool) domain.PRView {
	v.State = domain.PRStateUnknown
	v.StoredTerminal = true
	if exhausted {
		v.PollFailCount = 5
		v.FirstFailAt = fixturetest.PrTestEpoch
		v.LastPolledAt = fixturetest.PrTestEpoch.Add(31 * time.Minute)
	}
	return v
}

func TestNewPRRef_ShouldCanonicalizeIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		host, owner, repo      string
		wantHost, wantO, wantR string
	}{
		{"empty host is github.com", "", "Acme", "App", "github.com", "acme", "app"},
		{"mixed case host", "GitHub.com", "acme", "app", "github.com", "acme", "app"},
		{"ghe host kept lowercased", "ghe.acme.io", "Acme", "App", "ghe.acme.io", "acme", "app"},
		{"upper-case ghe host", "GHE.Acme.IO", "ACME", "APP", "ghe.acme.io", "acme", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref := fixturetest.MustPRRef(t, tt.host, tt.owner, tt.repo, 12)
			if ref.Host() != tt.wantHost || ref.Owner() != tt.wantO || ref.Repo() != tt.wantR || ref.Number() != 12 {
				t.Errorf("got %s, want %s/%s/%s#12", ref, tt.wantHost, tt.wantO, tt.wantR)
			}
		})
	}

	a := fixturetest.MustPRRef(t, "", "Acme", "App", 12)
	b := fixturetest.MustPRRef(t, "github.com", "acme", "app", 12)
	if a != b {
		t.Errorf("refs differing only in case/empty host must be equal: %v vs %v", a, b)
	}
	if ghe := fixturetest.MustPRRef(t, "ghe.acme.io", "acme", "app", 12); ghe == b {
		t.Error("a GHE ref must differ from github.com")
	}
}

func TestNewPRRef_ShouldRejectIncompleteInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		owner, repo string
		number      int
	}{
		{"empty owner", "", "app", 1},
		{"empty repo", "acme", "", 1},
		{"zero number", "acme", "app", 0},
		{"negative number", "acme", "app", -3},
		{"blank owner", "  ", "app", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if ref, err := domain.NewPRRef("", tt.owner, tt.repo, tt.number); err == nil || !ref.IsZero() {
				t.Errorf("got (%v, %v), want zero ref and an error", ref, err)
			}
		})
	}
}

func TestPRRef_ShouldPrintItselfInStringAndJSON(t *testing.T) {
	t.Parallel()
	ref := fixturetest.MustPRRef(t, "", "Acme", "App", 12)
	if got := fmt.Sprint(ref); got != "github.com/acme/app#12" {
		t.Errorf("fmt.Sprint = %q", got)
	}
	raw, err := json.Marshal(domain.PRView{Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Ref string }
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Ref != "github.com/acme/app#12" {
		t.Errorf("json Ref = %q, want the printed ref (not {})", out.Ref)
	}
	if !(domain.PRRef{}).IsZero() || ref.IsZero() {
		t.Error("IsZero must be true only for PRRef{}")
	}
}

func TestParsePRState_ShouldRejectUnknownStrings(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"open", "closed", "merged", "unknown"} {
		if got, err := domain.ParsePRState(s); err != nil || string(got) != s {
			t.Errorf("ParsePRState(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := domain.ParsePRState("MERGED"); err == nil {
		t.Error("ParsePRState must be case-sensitive and reject MERGED")
	}
	terminal := map[domain.PRState]bool{domain.PRStateOpen: false, domain.PRStateUnknown: false, domain.PRStateClosed: true, domain.PRStateMerged: true}
	for st, want := range terminal {
		if st.Terminal() != want {
			t.Errorf("%s.Terminal() = %v, want %v", st, st.Terminal(), want)
		}
	}
}

func TestParsePRRoleAndAttachSource_ShouldValidate(t *testing.T) {
	t.Parallel()
	if r, err := domain.ParsePRRole("followup"); err != nil || r != domain.PRRoleFollowup {
		t.Errorf("ParsePRRole(followup) = %v, %v", r, err)
	}
	if r, err := domain.ParsePRRole("rework"); err == nil || r != domain.PRRoleUnknown {
		t.Errorf("ParsePRRole(rework) = %v, %v; want unknown + error", r, err)
	}
	for _, s := range []string{"created_from_url", "push_scan", "branch_discovery", "pr_creation_service", "report_pr_created", "jules", "backfill", "manual"} {
		if _, err := domain.ParseAttachSource(s); err != nil {
			t.Errorf("ParseAttachSource(%q): %v", s, err)
		}
	}
	if _, err := domain.ParseAttachSource("webhook"); err == nil {
		t.Error("ParseAttachSource must reject webhook")
	}
	if !domain.AttachSourceManual.Verified() || !domain.AttachSourceReportPRCreated.Verified() || domain.AttachSourceBackfill.Verified() {
		t.Error("only manual and report_pr_created are verified sources")
	}
}

func TestPRView_Terminal_ShouldUseEffectiveTerminal(t *testing.T) {
	t.Parallel()
	open := fixturetest.PrRow(t, 1, domain.PRStateOpen, "a")
	tests := []struct {
		name string
		view domain.PRView
		want bool
	}{
		{"open", open, false},
		{"merged", fixturetest.PrRow(t, 2, domain.PRStateMerged, "a"), true},
		{"closed", fixturetest.PrRow(t, 3, domain.PRStateClosed, "a"), true},
		{"unknown", fixturetest.PrRow(t, 4, domain.PRStateUnknown, "a"), false},
		{"unverified terminal, not exhausted", unverifiedTerminal(fixturetest.PrRow(t, 5, domain.PRStateUnknown, "a"), false), false},
		{"unverified terminal, exhausted", unverifiedTerminal(fixturetest.PrRow(t, 6, domain.PRStateUnknown, "a"), true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.view.Terminal(); got != tt.want {
				t.Errorf("Terminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPRView_VerifyExhausted_ShouldNeedCountAndSpan(t *testing.T) {
	t.Parallel()
	base := unverifiedTerminal(fixturetest.PrRow(t, 7, domain.PRStateUnknown, "a"), true)
	tests := []struct {
		name   string
		mutate func(*domain.PRView)
		want   bool
	}{
		{"five failures over 31 minutes", func(*domain.PRView) {}, true},
		{"four failures", func(v *domain.PRView) { v.PollFailCount = 4 }, false},
		{"span just under 30 minutes", func(v *domain.PRView) { v.LastPolledAt = v.FirstFailAt.Add(29*time.Minute + 59*time.Second) }, false},
		{"span exactly 30 minutes", func(v *domain.PRView) { v.LastPolledAt = v.FirstFailAt.Add(30 * time.Minute) }, true},
		{"no first failure time", func(v *domain.PRView) { v.FirstFailAt = time.Time{} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := base
			tt.mutate(&v)
			if got := v.VerifyExhausted(); got != tt.want {
				t.Errorf("VerifyExhausted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActivePR_ShouldPickPinThenBranchThenNewestNonTerminal(t *testing.T) {
	t.Parallel()
	rows := func() []domain.PRView {
		return []domain.PRView{
			fixturetest.PrRow(t, 11, domain.PRStateMerged, "feat-a"),
			fixturetest.PrRow(t, 12, domain.PRStateOpen, "feat-b"),
			fixturetest.PrRow(t, 13, domain.PRStateOpen, "feat-c"),
		}
	}
	tests := []struct {
		name   string
		pinned int
		branch string
		want   int
	}{
		{"branch matches the newest open PR", 0, "feat-c", 13},
		{"branch matches an older open PR", 0, "feat-b", 12},
		{"no branch match falls back to newest non-terminal", 0, "main", 13},
		{"pin beats branch and recency", 11, "feat-c", 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prs := rows()
			for i := range prs {
				prs[i].Pinned = prs[i].Ref.Number() == tt.pinned
			}
			got := domain.ActivePR(prs, tt.branch)
			if got == nil || got.Ref.Number() != tt.want {
				t.Fatalf("ActivePR = %v, want #%d", got, tt.want)
			}
		})
	}
}

func TestActivePR_ShouldRankUnknownBelowOpen_WhenBothNonTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rows []domain.PRView
		want int
	}{
		{"older open beats newer unknown", []domain.PRView{fixturetest.PrRow(t, 12, domain.PRStateOpen, "x"), fixturetest.PrRow(t, 14, domain.PRStateUnknown, "x")}, 12},
		{"merged plus unknown gives unknown", []domain.PRView{fixturetest.PrRow(t, 11, domain.PRStateMerged, "x"), fixturetest.PrRow(t, 14, domain.PRStateUnknown, "x")}, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ActivePR(tt.rows, "main")
			if got == nil || got.Ref.Number() != tt.want {
				t.Fatalf("ActivePR = %v, want #%d", got, tt.want)
			}
		})
	}
}

func TestActivePR_ShouldUseEffectiveTerminal_WhenSeededUnknownTerminalRow(t *testing.T) {
	t.Parallel()
	older := func(n int, exhausted bool) domain.PRView {
		v := unverifiedTerminal(fixturetest.PrRow(t, n, domain.PRStateUnknown, "x"), exhausted)
		v.AttachedAt = fixturetest.PrTestEpoch // older than every fixturetest.PrRow(n>0)
		return v
	}
	tests := []struct {
		name string
		rows []domain.PRView
		want int
	}{
		{"exhausted seeded row does not outrank newer verified merged", []domain.PRView{older(14, true), fixturetest.PrRow(t, 13, domain.PRStateMerged, "x")}, 13},
		{"exhausted seeded vs closed: newer by time wins", []domain.PRView{older(14, true), fixturetest.PrRow(t, 9, domain.PRStateClosed, "x")}, 9},
		{"not-exhausted seeded row is non-terminal unknown, above terminal", []domain.PRView{older(14, false), fixturetest.PrRow(t, 13, domain.PRStateMerged, "x")}, 14},
		{"not-exhausted seeded row ranks below an open PR", []domain.PRView{older(14, false), fixturetest.PrRow(t, 12, domain.PRStateOpen, "x")}, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.ActivePR(tt.rows, "main")
			if got == nil || got.Ref.Number() != tt.want {
				t.Fatalf("ActivePR = %v, want #%d", got, tt.want)
			}
		})
	}
}

func TestActivePR_ShouldReturnNil_WhenNoPRs(t *testing.T) {
	t.Parallel()
	if got := domain.ActivePR(nil, "main"); got != nil {
		t.Errorf("ActivePR(nil) = %v, want nil", got)
	}
	if got := domain.ActivePR([]domain.PRView{}, ""); got != nil {
		t.Errorf("ActivePR(empty) = %v, want nil", got)
	}
}

func TestActivePR_ShouldReturnNewestOpen_WhenStackedAndBranchMatchesNeither(t *testing.T) {
	t.Parallel()
	got := domain.ActivePR([]domain.PRView{fixturetest.PrRow(t, 20, domain.PRStateOpen, "stack-1"), fixturetest.PrRow(t, 21, domain.PRStateOpen, "stack-2")}, "stack-3")
	if got == nil || got.Ref.Number() != 21 {
		t.Fatalf("ActivePR = %v, want #21", got)
	}
	// Merged-only set still yields its newest PR.
	got = domain.ActivePR([]domain.PRView{fixturetest.PrRow(t, 5, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 6, domain.PRStateMerged, "b")}, "main")
	if got == nil || got.Ref.Number() != 6 {
		t.Fatalf("merged-only ActivePR = %v, want #6", got)
	}
}

func TestActivePR_ShouldReturnACopy_WhenCallerMutatesResult(t *testing.T) {
	t.Parallel()
	prs := []domain.PRView{fixturetest.PrRow(t, 1, domain.PRStateOpen, "a")}
	got := domain.ActivePR(prs, "a")
	got.State = domain.PRStateMerged
	if prs[0].State != domain.PRStateOpen {
		t.Error("ActivePR must return a copy, not a pointer into the input")
	}
}

func TestItemPRVerdict_ShouldBeDone_WhenMergedAndClosedUnmerged(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rows []domain.PRView
		want ItemVerdict
	}{
		{"merged plus closed", []domain.PRView{fixturetest.PrRow(t, 11, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 12, domain.PRStateClosed, "b")}, ItemVerdictDone},
		{"only closed", []domain.PRView{fixturetest.PrRow(t, 12, domain.PRStateClosed, "b")}, ItemVerdictAllClosedUnmerged},
		{"merged plus open", []domain.PRView{fixturetest.PrRow(t, 11, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 13, domain.PRStateOpen, "c")}, ItemVerdictOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ItemPRVerdict(tt.rows); got != tt.want {
				t.Errorf("ItemPRVerdict = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestItemPRVerdict_ShouldHold_WhenUnknownOrUnverifiedTerminal(t *testing.T) {
	t.Parallel()
	merged := fixturetest.PrRow(t, 11, domain.PRStateMerged, "a")
	tests := []struct {
		name string
		rows []domain.PRView
	}{
		{"merged plus unknown", []domain.PRView{merged, fixturetest.PrRow(t, 13, domain.PRStateUnknown, "c")}},
		{"merged plus unverified terminal", []domain.PRView{merged, unverifiedTerminal(fixturetest.PrRow(t, 12, domain.PRStateUnknown, "b"), false)}},
		{"merged plus exhausted unverified terminal", []domain.PRView{merged, unverifiedTerminal(fixturetest.PrRow(t, 12, domain.PRStateUnknown, "b"), true)}},
		{"only exhausted unverified terminal is not abandoned", []domain.PRView{unverifiedTerminal(fixturetest.PrRow(t, 12, domain.PRStateUnknown, "b"), true)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ItemPRVerdict(tt.rows); got != ItemVerdictOpen {
				t.Errorf("ItemPRVerdict = %s, want %s", got, ItemVerdictOpen)
			}
		})
	}
}

func TestItemPRVerdict_ShouldCountOnce_WhenSameStateViaSessionAndItemLink(t *testing.T) {
	t.Parallel()
	viaSession := fixturetest.PrRow(t, 11, domain.PRStateMerged, "a")
	viaSession.SessionUUID = "s1"
	viaItem := viaSession
	viaItem.SessionUUID, viaItem.BacklogItemID = "", "item-1"
	rows := []domain.PRView{viaSession, viaItem}
	if got := domain.DistinctPRs(rows); len(got) != 1 {
		t.Fatalf("distinctPRs kept %d views, want 1", len(got))
	}
	if got := ItemPRVerdict(rows); got != ItemVerdictDone {
		t.Errorf("ItemPRVerdict = %s, want done", got)
	}
}

func TestItemPRVerdict_ShouldBeNoPRs_WhenEmptySet(t *testing.T) {
	t.Parallel()
	if got := ItemPRVerdict(nil); got != ItemVerdictNoPRs {
		t.Errorf("ItemPRVerdict(nil) = %s, want no_prs", got)
	}
}

func TestPRsAllTerminal_ShouldBeFalse_WhenEmptyRowsAndLegacyNumberSet(t *testing.T) {
	t.Parallel()
	if PRsAllTerminal(nil, 21) {
		t.Error("empty rows with legacy number 21 must not read as terminal")
	}
	if PRsAllTerminal(nil, 0) {
		t.Error("an empty set is never terminal through PRsAllTerminal")
	}
	if !HasNoPRs(nil, 0) || HasNoPRs(nil, 21) {
		t.Error("HasNoPRs is the explicit no-PRs path: true only for no rows and no legacy number")
	}
}

func TestPRsAllTerminal_ShouldUseEffectiveTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rows []domain.PRView
		want bool
	}{
		{"merged and closed", []domain.PRView{fixturetest.PrRow(t, 1, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 2, domain.PRStateClosed, "b")}, true},
		{"one open", []domain.PRView{fixturetest.PrRow(t, 1, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 2, domain.PRStateOpen, "b")}, false},
		{"unknown blocks", []domain.PRView{fixturetest.PrRow(t, 1, domain.PRStateMerged, "a"), fixturetest.PrRow(t, 2, domain.PRStateUnknown, "b")}, false},
		{"unverified terminal not exhausted blocks", []domain.PRView{unverifiedTerminal(fixturetest.PrRow(t, 1, domain.PRStateUnknown, "a"), false)}, false},
		{"unverified terminal exhausted releases", []domain.PRView{unverifiedTerminal(fixturetest.PrRow(t, 1, domain.PRStateUnknown, "a"), true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PRsAllTerminal(tt.rows, 0); got != tt.want {
				t.Errorf("PRsAllTerminal = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLiveWork_ShouldBeLiveOnlyInsideWindow_WhenUnendedAndActive(t *testing.T) {
	t.Parallel()
	now := fixturetest.PrTestEpoch
	const window = 10 * time.Minute
	tests := []struct {
		name  string
		facts LiveWorkFacts
		want  bool
	}{
		{"active 2 minutes ago", LiveWorkFacts{Unended: true, LastMeaningfulChangeAt: now.Add(-2 * time.Minute), TurnEnded: true}, true},
		{"idle 11 minutes", LiveWorkFacts{Unended: true, LastMeaningfulChangeAt: now.Add(-11 * time.Minute), TurnEnded: true}, false},
		{"exactly at the window edge is not live", LiveWorkFacts{Unended: true, LastMeaningfulChangeAt: now.Add(-window)}, false},
		{"ended session", LiveWorkFacts{Unended: false, Working: true, LastMeaningfulChangeAt: now}, false},
		{"zero time and idle", LiveWorkFacts{Unended: true}, false},
		{"mid-turn is live without a recent change", LiveWorkFacts{Unended: true, Working: true}, true},
		{"working but turn ended and stale", LiveWorkFacts{Unended: true, Working: true, TurnEnded: true, LastMeaningfulChangeAt: now.Add(-time.Hour)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsLiveWork(tt.facts, now, window); got != tt.want {
				t.Errorf("IsLiveWork = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLiveWorkHoldExpired_ShouldExpireAtMaxHold(t *testing.T) {
	t.Parallel()
	const maxHold = time.Hour
	merged := fixturetest.PrTestEpoch
	tests := []struct {
		name string
		now  time.Time
		at   time.Time
		want bool
	}{
		{"just before", merged.Add(maxHold - time.Second), merged, false},
		{"exactly at max hold", merged.Add(maxHold), merged, true},
		{"after", merged.Add(2 * maxHold), merged, true},
		{"zero merge time never expires", merged.Add(24 * time.Hour), time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := LiveWorkHoldExpired(tt.now, tt.at, maxHold); got != tt.want {
				t.Errorf("LiveWorkHoldExpired = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssignRole_ShouldFollowTable_WhenAttachSourceAndOrderVary(t *testing.T) {
	withRole := func(v domain.PRView, r domain.PRRole) domain.PRView { v.Role = r; return v }
	primary := withRole(fixturetest.PrRow(t, 11, domain.PRStateMerged, "feat/a"), domain.PRRolePrimary)
	tests := []struct {
		name     string
		existing []domain.PRView
		base     string
		want     domain.PRRole
	}{
		{"empty session gets primary", nil, "main", domain.PRRolePrimary},
		{"only followups, no primary yet", []domain.PRView{withRole(fixturetest.PrRow(t, 12, domain.PRStateOpen, "b"), domain.PRRoleFollowup)}, "main", domain.PRRolePrimary},
		{"primary exists, unrelated base", []domain.PRView{primary}, "main", domain.PRRoleFollowup},
		{"base equals another head", []domain.PRView{primary}, "feat/a", domain.PRRoleStacked},
		{"base equals head of non-primary link", []domain.PRView{primary, withRole(fixturetest.PrRow(t, 12, domain.PRStateOpen, "feat/b"), domain.PRRoleFollowup)}, "feat/b", domain.PRRoleStacked},
		{"empty base never stacks", []domain.PRView{withRole(fixturetest.PrRow(t, 11, domain.PRStateOpen, ""), domain.PRRolePrimary)}, "", domain.PRRoleFollowup},
		{"backfill import into session with merged primary", []domain.PRView{primary}, "", domain.PRRoleFollowup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.AssignRole(tt.existing, tt.base); got != tt.want {
				t.Errorf("AssignRole = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstanceSnapshot_ActiveCheckConclusion(t *testing.T) {
	t.Parallel()
	snap := &domain.InstanceSnapshot{GitHub: domain.GitHubIntegration{GitHubCheckConclusion: "success"}}
	if got := snap.ActiveCheckConclusion(); got != "success" {
		t.Fatalf("no links: want scalar, got %q", got)
	}
	snap.GitHub.PRs = []domain.PRView{
		{State: domain.PRStateMerged, StoredTerminal: true, CheckConclusion: "success"},
		{State: domain.PRStateOpen, CheckConclusion: "failure"},
	}
	if got := snap.ActiveCheckConclusion(); got != "failure" {
		t.Fatalf("with links: want the active PR's conclusion, got %q", got)
	}
}
