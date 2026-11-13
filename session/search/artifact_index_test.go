package search

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/tstapler/stapler-squad/session"
)

type fakeLinks map[session.PRRef][]string

func (f fakeLinks) SessionUUIDsByPR(_ context.Context, ref session.PRRef) ([]string, error) {
	return f[ref], nil
}

func mustRef(t *testing.T, owner, repo string, n int) session.PRRef {
	t.Helper()
	ref, err := session.NewPRRef("", owner, repo, n)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func blobOf(recs ...domain.ArtifactRecord) *domain.SessionArtifactsBlob {
	return &domain.SessionArtifactsBlob{Records: recs}
}

func prRec(owner, repo string, n int, src domain.PRSource) domain.PRRecord {
	return domain.PRRecord{Host: "github.com", Owner: owner, Repo: repo, Number: n, Source: src}
}

func sessionsOf(hits []ArtifactHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.SessionID)
	}
	return out
}

func TestArtifactIndex_Search_ShouldRetrieveByPRRefSHAPrefixAndBranch(t *testing.T) {
	t.Parallel()
	x := NewArtifactIndex(nil)
	x.Update("u42", blobOf(prRec("acme", "app", 42, domain.PRSourceCreated)))
	x.Update("u43", blobOf(prRec("acme", "app", 43, domain.PRSourceCreated)))
	x.Update("usha", blobOf(domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", Subject: "fix login"}))
	x.Update("ufb", blobOf(domain.BranchRecord{Name: "feat-b", Action: "pushed"}))
	x.Update("ufa", blobOf(domain.BranchRecord{Name: "feat-a", Action: "pushed"}))

	tests := []struct {
		query     string
		wantFirst string
		wantType  domain.RecordKind
	}{
		{"acme/app#42", "u42", domain.KindPR},
		{"Acme/App#42", "u42", domain.KindPR},
		{"1a2b3c4", "usha", domain.KindCommit},
		{"feat-b", "ufb", domain.KindBranch},
	}
	for _, tt := range tests {
		hits := x.Search(tt.query, 10)
		if len(hits) == 0 || hits[0].SessionID != tt.wantFirst || hits[0].Type != tt.wantType {
			t.Errorf("Search(%q) = %+v, want first %s/%s", tt.query, hits, tt.wantFirst, tt.wantType)
		}
	}
}

func TestArtifactIndex_Update_ShouldReplaceASessionWithoutRestart(t *testing.T) {
	t.Parallel()
	x := NewArtifactIndex(nil)
	x.Update("u1", blobOf(domain.BranchRecord{Name: "old-branch"}))
	x.Update("u1", blobOf(domain.BranchRecord{Name: "new-branch"}))
	for _, h := range x.Search("old-branch", 5) {
		if h.Text == "old-branch" {
			t.Fatalf("stale document still indexed: %+v", h)
		}
	}
	if got := sessionsOf(x.Search("new-branch", 5)); !reflect.DeepEqual(got, []string{"u1"}) {
		t.Fatalf("new document missing: %v", got)
	}
	if x.DocCount() != 1 {
		t.Fatalf("DocCount = %d, want 1", x.DocCount())
	}
	x.Update("u1", nil)
	if x.DocCount() != 0 {
		t.Fatalf("DocCount after nil update = %d, want 0", x.DocCount())
	}
}

func TestArtifactIndex_SessionsForPR_ShouldUnionBothStoresAndDedupe(t *testing.T) {
	t.Parallel()
	ref := mustRef(t, "acme", "app", 12)
	x := NewArtifactIndex(fakeLinks{ref: {"u1", "u3"}})
	x.Update("u2", blobOf(prRec("acme", "app", 12, domain.PRSourceCreated)))
	x.Update("u3", blobOf(prRec("Acme", "App", 12, domain.PRSourceMentioned)))
	x.Update("u4", blobOf(prRec("acme", "app", 13, domain.PRSourceCreated)))

	got, err := x.SessionsForPR(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	want := []SessionPRHit{
		{"u1", PRSourceSessionPR}, {"u3", PRSourceSessionPR}, {"u2", PRSourceArtifact},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SessionsForPR = %+v, want %+v", got, want)
	}
}

func TestArtifactIndex_SessionsForPR_ShouldNeverSeedSessionPRFromArtifacts(t *testing.T) {
	t.Parallel()
	ref := mustRef(t, "acme", "app", 12)
	links := fakeLinks{}
	x := NewArtifactIndex(links)
	x.Update("u2", blobOf(prRec("acme", "app", 12, domain.PRSourceCreated)))
	if _, err := x.SessionsForPR(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("index wrote to the link source: %v", links)
	}
	if _, err := x.SessionsForPR(context.Background(), session.PRRef{}); err == nil {
		t.Fatal("zero ref must be rejected")
	}
}

func TestArtifactIndex_SessionsForSHAPrefix_ShouldMatchSevenCharPrefixAndLonger(t *testing.T) {
	t.Parallel()
	x := NewArtifactIndex(nil)
	x.Update("u1", blobOf(domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b"}))
	x.Update("u2", blobOf(domain.CommitRecord{SHA: "1a2b3c4ffffffffffffffffffffffffffffffffff"}))
	for prefix, want := range map[string][]string{
		"1a2b3c4":   {"u1", "u2"},
		"1A2B3C4D5": {"u1"},
		"1a2b3c":    nil, // under 7 chars is a text query, not a lookup
		"deadbee":   {},
	} {
		got := x.SessionsForSHAPrefix(prefix)
		if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Errorf("SessionsForSHAPrefix(%q) = %v, want %v", prefix, got, want)
		}
	}
	x.Update("u1", nil)
	if got := x.SessionsForSHAPrefix("1a2b3c4"); !reflect.DeepEqual(got, []string{"u2"}) {
		t.Errorf("removed session still found: %v", got)
	}
}

// ADR-004: the artifact index is a separate corpus, so indexing artifacts must not
// move a single history hit, order or score.
func TestArtifactIndex_ShouldLeaveHistorySearchUnchanged(t *testing.T) {
	t.Parallel()
	history := NewSearchEngine()
	corpus := []string{
		"fixing the login bug in acme/app#42 today",
		"deploy the feature branch feat-b to staging",
		"commit 1a2b3c4 touched the login handler",
		"unrelated chatter about lunch",
	}
	for i, c := range corpus {
		if err := history.IndexMessage(fmt.Sprintf("h%d", i), 0, "user", c, time.Unix(int64(i), 0)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() []SearchResult {
		res, err := history.Search("login acme/app#42 feat-b 1a2b3c4", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return res.Results
	}
	before := snapshot()
	if len(before) == 0 {
		t.Fatal("fixture produced no history hits")
	}

	x := NewArtifactIndex(nil)
	for i := range 100 {
		x.Update(fmt.Sprintf("a%d", i), blobOf(
			prRec("acme", "app", 42, domain.PRSourceCreated),
			domain.BranchRecord{Name: "feat-b"},
			domain.CommitRecord{SHA: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", Subject: "login"},
		))
	}
	if x.DocCount() != 300 {
		t.Fatalf("DocCount = %d, want 300", x.DocCount())
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("history result changed after indexing artifacts:\nbefore %+v\nafter  %+v", before, after)
	}
	if n := history.GetDocStore().Count(); n != len(corpus) {
		t.Fatalf("history doc store grew to %d", n)
	}
}

func TestArtifactIndex_ShouldReportDocCountMetric(t *testing.T) {
	x := NewArtifactIndex(nil)
	x.Update("u1", blobOf(domain.BranchRecord{Name: "a-branch"}, domain.PlanDocRecord{Path: "docs/x.md"}))
	if got := ArtifactIndexDocsObserved(); got != 2 {
		t.Fatalf("ssq_artifact_index_docs = %d, want 2", got)
	}
}

func TestArtifactIndex_Update_ShouldNotIndexSecrets_WhenLegacyBlobIsUnredacted(t *testing.T) {
	t.Parallel()
	x := NewArtifactIndex(nil)
	x.Update("u1", blobOf(domain.CILinkRecord{URL: "https://ci.example.com/runs/9?token=abcd1234secret"}))
	hits := x.Search("runs", 10)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if got := hits[0].Text; got != "https://ci.example.com/runs/9?token=[redacted]" {
		t.Fatalf("display text = %q", got)
	}
}
