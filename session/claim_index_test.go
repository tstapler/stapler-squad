package session

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"strings"
	"testing"
	"time"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

const testIssueURL = "https://github.com/o/r/issues/1"

func newTestIdentity(t *testing.T) HostIdentity {
	t.Helper()
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	return identity
}

// newTestClaimIndex returns a ClaimIndex whose registry has pinned every
// identity in pinned, sharing stateDir.
func newTestClaimIndex(t *testing.T, stateDir string, clock Clock, pinned ...HostIdentity) *ClaimIndex {
	t.Helper()
	registry, err := NewHostRegistryWithClock(t.TempDir(), DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}
	for _, identity := range pinned {
		if _, accepted, err := registry.Advertise(newTestAdvertisement(t, identity, []string{"peer:8444"}, clock.Now())); err != nil || !accepted {
			t.Fatalf("Advertise(pin) = accepted %v, err %v", accepted, err)
		}
	}
	index, err := NewClaimIndex(stateDir, registry)
	if err != nil {
		t.Fatalf("NewClaimIndex() error = %v, want nil", err)
	}
	return index
}

func mustRecord(t *testing.T, index *ClaimIndex, record ClaimRecord) ClaimOutcome {
	t.Helper()
	outcome, err := index.RecordClaim(record)
	if err != nil {
		t.Fatalf("RecordClaim() error = %v, want nil", err)
	}
	return outcome
}

func TestClaimRecord_Verify_should_ReturnTrue_When_SignatureValid(t *testing.T) {
	hostA := newTestIdentity(t)
	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/backlog/v1/bl_01J", time.Now())
	if !record.Verify() {
		t.Fatalf("Verify() = false, want true for a record signed by its own key")
	}
}

func TestClaimRecord_Verify_should_ReturnFalse_When_SignatureByteFlipped(t *testing.T) {
	hostA := newTestIdentity(t)
	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/backlog/v1/bl_01J", time.Now())
	record.Signature[0] ^= 0xFF
	if record.Verify() {
		t.Fatalf("Verify() = true, want false after flipping a signature byte")
	}
}

func TestClaimRecord_Verify_should_ReturnFalse_When_SignedFieldTamperedOrWrongKey(t *testing.T) {
	hostA := newTestIdentity(t)
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ClaimRecord)
	}{
		{"external url changed", func(r *ClaimRecord) { r.ExternalURL = "https://github.com/o/r/issues/2" }},
		{"deep link changed", func(r *ClaimRecord) { r.ItemDeepLink = "ssq://evil/backlog/v1/bl_01J" }},
		{"wrong public key", func(r *ClaimRecord) { r.PublicKey = otherPub }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/backlog/v1/bl_01J", time.Now())
			tt.mutate(&record)
			if record.Verify() {
				t.Fatalf("Verify() = true, want false")
			}
		})
	}
}

func TestClaimRecord_Verify_should_IgnoreDisputedFlag(t *testing.T) {
	hostA := newTestIdentity(t)
	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", time.Now())
	record.Disputed = true
	if !record.Verify() {
		t.Fatalf("Verify() = false, want true: Disputed is not part of the signed payload")
	}
}

func TestClaimIndex_RecordClaim_should_PersistAndBeReadableViaCheckClaim_When_ValidSignedRecordGiven(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/backlog/v1/bl_01J", clock.Now())
	outcome := mustRecord(t, index, record)
	if !outcome.Accepted || !outcome.IsNew || outcome.Conflict {
		t.Fatalf("outcome = %+v, want accepted+new, no conflict", outcome)
	}

	got, ok := index.CheckClaim(testIssueURL)
	if !ok {
		t.Fatalf("CheckClaim() = not found, want found")
	}
	if got.ClaimingHostID.String() != hostA.ID.String() || got.ItemDeepLink != record.ItemDeepLink || got.Disputed {
		t.Fatalf("CheckClaim() = %+v, want hostA's undisputed record", got)
	}
	if _, ok := index.CheckClaim("https://github.com/o/r/issues/404"); ok {
		t.Fatalf("CheckClaim(unknown) = found, want not found")
	}
}

func TestClaimIndex_RecordClaim_should_BeVisibleToSecondProcessInstance_When_SharedStateDirUsed(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	stateDir := t.TempDir()
	hostA := newTestIdentity(t)
	writer := newTestClaimIndex(t, stateDir, clock, hostA)
	reader := newTestClaimIndex(t, stateDir, clock, hostA)

	mustRecord(t, writer, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now()))

	fresh := newTestClaimIndex(t, stateDir, clock, hostA)
	for name, idx := range map[string]*ClaimIndex{"fresh instance": fresh, "already-open instance": reader} {
		got, ok := idx.CheckClaim(testIssueURL)
		if !ok || got.ClaimingHostID.String() != hostA.ID.String() {
			t.Errorf("%s: CheckClaim() = (%+v, %v), want hostA's record", name, got, ok)
		}
	}
}

func TestClaimIndex_RecordClaim_should_RejectForgedRecord_When_PublicKeyDoesNotMatchPinnedHostRegistryKey(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	forger := newTestIdentity(t)
	forged := ClaimRecord{ExternalURL: testIssueURL, ClaimingHostID: hostA.ID, ItemDeepLink: "ssq://evil/x", ClaimedAt: clock.Now()}
	forged.Sign(forger)
	if !forged.Verify() {
		t.Fatalf("precondition: forged record must be internally consistent")
	}

	outcome := mustRecord(t, index, forged)
	if outcome.Accepted || outcome.IsNew {
		t.Fatalf("outcome = %+v, want silent rejection", outcome)
	}
	if _, ok := index.CheckClaim(testIssueURL); ok {
		t.Fatalf("CheckClaim() = found, want the forged record never stored")
	}
}

func TestClaimIndex_RecordClaim_should_Reject_When_FieldsMissingOrSignatureInvalid(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	tampered := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now())
	tampered.ItemDeepLink = "ssq://evil/x"
	noURL := NewSignedClaimRecord(hostA, "", "ssq://hostA/x", clock.Now())
	noHost := ClaimRecord{ExternalURL: testIssueURL, ClaimedAt: clock.Now()}
	noHost.Sign(hostA)

	for name, record := range map[string]ClaimRecord{"tampered": tampered, "no url": noURL, "no host id": noHost} {
		if outcome := mustRecord(t, index, record); outcome.Accepted {
			t.Errorf("%s: outcome = %+v, want rejected", name, outcome)
		}
	}
}

func TestClaimIndex_RecordClaim_should_ApplyLastWriteWinsAndLogConflict_When_DifferentHostClaimsSameExternalURL(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA, hostB := newTestIdentity(t), newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA, hostB)

	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	earlier := clock.Now()
	clock.Advance(time.Minute)
	later := clock.Now()

	mustRecord(t, index, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", earlier))
	outcome := mustRecord(t, index, NewSignedClaimRecord(hostB, testIssueURL, "ssq://hostB/x", later))
	if !outcome.Accepted || !outcome.Conflict || outcome.IsNew {
		t.Fatalf("outcome = %+v, want accepted conflict that changes no claimant", outcome)
	}
	got, _ := index.CheckClaim(testIssueURL)
	if got.ClaimingHostID.String() != hostA.ID.String() {
		t.Fatalf("winner = %s, want the earlier claimant hostA", got.ClaimingHostID)
	}

	logs := buf.String()
	for _, want := range []string{"claim_index.conflict_detected", hostA.ID.String(), hostB.ID.String(), testIssueURL} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q, got: %s", want, logs)
		}
	}
}

func TestClaimIndex_RecordClaim_should_BreakTiesByLexicallySmallerHostID_When_ClaimedAtEqual(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA, hostB := newTestIdentity(t), newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA, hostB)

	smaller, larger := hostA, hostB
	if larger.ID.String() < smaller.ID.String() {
		smaller, larger = larger, smaller
	}
	at := clock.Now()
	mustRecord(t, index, NewSignedClaimRecord(larger, testIssueURL, "ssq://larger/x", at))
	outcome := mustRecord(t, index, NewSignedClaimRecord(smaller, testIssueURL, "ssq://smaller/x", at))
	if !outcome.IsNew {
		t.Fatalf("outcome = %+v, want the tie-break winner to replace the stored claimant", outcome)
	}
	got, _ := index.CheckClaim(testIssueURL)
	if got.ClaimingHostID.String() != smaller.ID.String() || !got.Disputed {
		t.Fatalf("got %+v, want disputed record for the smaller HostID", got)
	}
}

func TestClaimIndex_RecordClaim_should_SetDisputedTrueOnLWWSelectedRecord_When_DifferentHostClaimsSameExternalURL(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA, hostB := newTestIdentity(t), newTestIdentity(t)
	stateDir := t.TempDir()
	index := newTestClaimIndex(t, stateDir, clock, hostA, hostB)

	mustRecord(t, index, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now()))
	if got, _ := index.CheckClaim(testIssueURL); got.Disputed {
		t.Fatalf("Disputed = true after a single claim, want false")
	}
	clock.Advance(time.Minute)
	mustRecord(t, index, NewSignedClaimRecord(hostB, testIssueURL, "ssq://hostB/x", clock.Now()))

	got, ok := index.CheckClaim(testIssueURL)
	if !ok || !got.Disputed {
		t.Fatalf("CheckClaim() = (%+v, %v), want Disputed record", got, ok)
	}
	persisted := newTestClaimIndex(t, stateDir, clock, hostA, hostB)
	if got, _ := persisted.CheckClaim(testIssueURL); !got.Disputed {
		t.Fatalf("Disputed flag not persisted across reload")
	}
}

func TestClaimIndex_RecordClaim_should_IgnoreDisputedFlag_When_ReceivedFromPeer(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now())
	record.Disputed = true
	mustRecord(t, index, record)
	if got, _ := index.CheckClaim(testIssueURL); got.Disputed {
		t.Fatalf("Disputed = true, want a peer-supplied flag ignored")
	}
}

func TestClaimIndex_RecordClaim_should_LeaveDisputedFalse_When_SameClaimingHostIDUpdatesExistingRecord(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	mustRecord(t, index, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/old", clock.Now()))
	clock.Advance(time.Minute)
	outcome := mustRecord(t, index, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/new", clock.Now()))
	if outcome.Conflict {
		t.Fatalf("outcome = %+v, want no conflict for the same host", outcome)
	}
	got, _ := index.CheckClaim(testIssueURL)
	if got.Disputed || got.ItemDeepLink != "ssq://hostA/new" {
		t.Fatalf("got %+v, want undisputed record with the updated deep link", got)
	}
}

func TestClaimIndex_RecordClaim_should_ReportNotNew_When_SameRecordDeliveredTwice(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA := newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, hostA)

	record := NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now())
	mustRecord(t, index, record)
	if outcome := mustRecord(t, index, record); !outcome.Accepted || outcome.IsNew {
		t.Fatalf("outcome = %+v, want accepted duplicate that is not new (bounds re-gossip)", outcome)
	}
}

func TestClaimIndex_ResolveDispute_should_ClearDisputedAndPersist_When_CalledForDisputedExternalURL(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	hostA, hostB := newTestIdentity(t), newTestIdentity(t)
	stateDir := t.TempDir()
	index := newTestClaimIndex(t, stateDir, clock, hostA, hostB)

	mustRecord(t, index, NewSignedClaimRecord(hostA, testIssueURL, "ssq://hostA/x", clock.Now()))
	clock.Advance(time.Minute)
	mustRecord(t, index, NewSignedClaimRecord(hostB, testIssueURL, "ssq://hostB/x", clock.Now()))

	if err := index.ResolveDispute(testIssueURL); err != nil {
		t.Fatalf("ResolveDispute() error = %v, want nil", err)
	}
	if got, _ := index.CheckClaim(testIssueURL); got.Disputed {
		t.Fatalf("Disputed = true after ResolveDispute, want false")
	}
	reloaded := newTestClaimIndex(t, stateDir, clock, hostA, hostB)
	if got, _ := reloaded.CheckClaim(testIssueURL); got.Disputed {
		t.Fatalf("Disputed = true after reload, want the cleared flag persisted")
	}
	if err := index.ResolveDispute("https://github.com/o/r/issues/404"); err != nil {
		t.Fatalf("ResolveDispute(unknown) error = %v, want nil no-op", err)
	}
}

func TestNewClaimIndex_should_ReturnError_When_HostRegistryNil(t *testing.T) {
	if _, err := NewClaimIndex(t.TempDir(), nil); err == nil {
		t.Fatalf("NewClaimIndex(nil registry) error = nil, want error")
	}
}

func TestClaimIndex_ForeignClaim_should_IgnoreSelfClaimAndReportOtherHostClaim(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	self, other := newTestIdentity(t), newTestIdentity(t)
	index := newTestClaimIndex(t, t.TempDir(), clock, self, other)

	if _, ok := index.ForeignClaim(testIssueURL, self.ID); ok {
		t.Fatal("ForeignClaim() on empty index = true, want false")
	}
	mustRecord(t, index, NewSignedClaimRecord(self, testIssueURL, "ssq://self/x", clock.Now()))
	if _, ok := index.ForeignClaim(testIssueURL, self.ID); ok {
		t.Fatal("ForeignClaim() for own claim = true, want false")
	}

	const otherURL = "https://github.com/o/r/issues/2"
	mustRecord(t, index, NewSignedClaimRecord(other, otherURL, "ssq://other/x", clock.Now()))
	got, ok := index.ForeignClaim(otherURL, self.ID)
	if !ok || got.ClaimingHostID.String() != other.ID.String() {
		t.Fatalf("ForeignClaim() = (%+v, %v), want other host's claim", got, ok)
	}
}
