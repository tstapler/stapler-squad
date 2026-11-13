package artifacts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

var t0 = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

func allKinds() []domain.ArtifactRecord {
	return []domain.ArtifactRecord{
		domain.PRRecord{Host: "github.com", Owner: "acme", Repo: "app", Number: 12, Source: domain.PRSourceCreated, Branch: "feat-b", Base: "feat-a", FirstSeenAt: t0},
		domain.BranchRecord{Name: "feat-b", Action: "created", FirstSeenAt: t0},
		domain.CommitRecord{SHA: "1a2b3c4", Subject: "fix: token refresh", Branch: "feat-b", FirstSeenAt: t0},
		domain.IssueRecord{Owner: "acme", Repo: "app", Number: 45, Relation: "fixes", FirstSeenAt: t0},
		domain.PlanDocRecord{Path: "project_plans/x/plan.md", FirstSeenAt: t0},
		domain.CILinkRecord{URL: "https://github.com/acme/app/actions/runs/123", FirstSeenAt: t0},
		domain.CommandRecord{Type: "gh_pr_create", Command: "gh pr create", Detail: "t", Result: "ok", FirstSeenAt: t0},
	}
}

func TestRecords_ShouldRoundTripEveryKind(t *testing.T) {
	t.Parallel()
	in := domain.RecordList(allKinds())
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out domain.RecordList
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != len(in) {
		t.Fatalf("round trip kept %d of %d records: %s", len(out), len(in), raw)
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("record %d changed: %#v -> %#v", i, in[i], out[i])
		}
	}
}

func TestArtifactRecord_ShouldDropStrayField_AndSkipUnknownKind_Stray(t *testing.T) {
	t.Parallel()
	r, err := domain.ParseRecord([]byte(`{"kind":"commit","host":"x","sha":"abc1234","subject":"s"}`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := r.(domain.CommitRecord)
	if !ok || c.SHA != "abc1234" {
		t.Fatalf("parsed %#v", r)
	}
	again, _ := json.Marshal(c)
	if strings.Contains(string(again), "host") {
		t.Fatalf("stray field survived: %s", again)
	}
}

func TestArtifactRecord_ShouldDropStrayField_AndSkipUnknownKind_Unknown(t *testing.T) {
	before := domain.UnknownRecordsSkipped()
	var l domain.RecordList
	if err := json.Unmarshal([]byte(`[{"kind":"unknown"},{"kind":"plan_doc","path":"a.md"},{"nokind":1}]`), &l); err != nil {
		t.Fatal(err)
	}
	if len(l) != 1 {
		t.Fatalf("want only the plan_doc, got %#v", l)
	}
	if got := domain.UnknownRecordsSkipped() - before; got != 2 {
		t.Fatalf("skipped counter moved by %d, want 2", got)
	}
}

// preChangeBlob is the blob struct before v2, kept as the older binary's decoder.
type preChangeBlob struct {
	PRURLs          []string                 `json:"pr_urls"`
	CommitSHAs      []string                 `json:"commit_shas"`
	ExternalURLs    []string                 `json:"external_urls"`
	Commands        []domain.CommandArtifact `json:"commands,omitempty"`
	ScanOffsetBytes int64                    `json:"scan_offset_bytes"`
	LastScannedAt   time.Time                `json:"last_scanned_at"`
	ScanState       domain.ScanState         `json:"scan_state,omitempty"`
}

func TestBlobV2_ShouldBeReadableByV1Decoder_WhenWrittenByThisVersion(t *testing.T) {
	t.Parallel()
	b := &domain.SessionArtifactsBlob{
		PRURLs: []string{"https://github.com/acme/app/pull/12"}, CommitSHAs: []string{"1a2b3c4"},
		ExternalURLs: []string{"https://example.com"}, Commands: []domain.CommandArtifact{{Type: "git_commit"}},
		ScanOffsetBytes: 7, ScanVersion: 2, Records: allKinds(),
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var old preChangeBlob
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("older binary cannot load a v2 blob: %v", err)
	}
	if len(old.PRURLs) != 1 || len(old.CommitSHAs) != 1 || len(old.ExternalURLs) != 1 || len(old.Commands) != 1 || old.ScanOffsetBytes != 7 {
		t.Fatalf("v1 lists lost: %+v", old)
	}
}

func TestBlobV1_ShouldLoadAndKeepPRURLs_WhenStoredBeforeV2(t *testing.T) {
	t.Parallel()
	var b domain.SessionArtifactsBlob
	if err := json.Unmarshal([]byte(`{"pr_urls":["https://github.com/a/b/pull/1"],"scan_offset_bytes":5}`), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.PRURLs) != 1 || len(b.Records) != 0 || b.Version() != 1 || !b.HasData() {
		t.Fatalf("%+v", b)
	}
}

func TestScanVersion_ShouldBeTwo(t *testing.T) {
	t.Parallel()
	if domain.ScanVersion != 2 {
		t.Fatalf("ScanVersion = %d", domain.ScanVersion)
	}
}

func commits(n int, subject string) []domain.ArtifactRecord {
	out := make([]domain.ArtifactRecord, n)
	for i := range out {
		out[i] = domain.CommitRecord{SHA: strings.Repeat("a", 7) + string(rune('a'+i%26)) + itoa(i), Subject: subject, FirstSeenAt: t0.Add(time.Duration(i) * time.Second)}
	}
	return out
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestCap_ShouldKeepNewestPerKind_AndBoundTotalBlobBytes(t *testing.T) {
	t.Parallel()
	b := &domain.SessionArtifactsBlob{Records: commits(5000, strings.Repeat("s", 400))}
	enforceCaps(b)
	raw, _ := json.Marshal(b)
	if len(raw) > MaxBlobBytes {
		t.Fatalf("blob is %d bytes, cap %d", len(raw), MaxBlobBytes)
	}
	if n := len(b.Records); n == 0 || n > maxCommitRecords {
		t.Fatalf("kept %d commits, per-kind cap %d", n, maxCommitRecords)
	}
	last := b.Records[len(b.Records)-1].(domain.CommitRecord)
	if !strings.HasSuffix(last.SHA, "4999") {
		t.Fatalf("oldest-first drop must keep the newest, last=%s", last.SHA)
	}
}

func TestCap_ShouldTruncateCommandResultTo512Bytes(t *testing.T) {
	t.Parallel()
	b := &domain.SessionArtifactsBlob{Records: []domain.ArtifactRecord{domain.CommandRecord{Type: "x", Result: strings.Repeat("r", 5000), FirstSeenAt: t0}}}
	enforceCaps(b)
	if got := len(b.Records[0].(domain.CommandRecord).Result); got > MaxResultBytes {
		t.Fatalf("result is %d bytes", got)
	}
}

func TestMerge_ShouldDedupRecordsAndUpgradeMentionedPR(t *testing.T) {
	t.Parallel()
	mentioned := domain.PRRecord{Host: "github.com", Owner: "a", Repo: "b", Number: 1, Source: domain.PRSourceMentioned, FirstSeenAt: t0}
	created := domain.PRRecord{Host: "github.com", Owner: "a", Repo: "b", Number: 1, Source: domain.PRSourceCreated, Branch: "f", FirstSeenAt: t0.Add(time.Hour)}
	got := domain.MergeRecords([]domain.ArtifactRecord{mentioned}, []domain.ArtifactRecord{created, created})
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %#v", got)
	}
	pr := got[0].(domain.PRRecord)
	if pr.Source != domain.PRSourceCreated || pr.Branch != "f" || !pr.FirstSeenAt.Equal(t0) {
		t.Fatalf("upgrade lost data: %#v", pr)
	}
}
