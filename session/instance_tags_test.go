package session

import (
	"errors"
	"strings"
	"testing"
)

func TestAddTag_ValidTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}

	err := inst.AddTag("frontend")
	if err != nil {
		t.Fatalf("AddTag with valid tag should succeed, got: %v", err)
	}

	tags := inst.GetTags()
	if len(tags) != 1 || tags[0] != "frontend" {
		t.Errorf("expected tags=[frontend], got %v", tags)
	}
}

func TestAddTag_DuplicateTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"frontend"}}

	err := inst.AddTag("frontend")
	if err == nil {
		t.Fatal("AddTag with duplicate tag should return error")
	}

	var dupErr ErrDuplicateTag
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected ErrDuplicateTag, got %T: %v", err, err)
	}
	if dupErr.Tag != "frontend" {
		t.Errorf("expected ErrDuplicateTag.Tag=frontend, got %q", dupErr.Tag)
	}
}

func TestAddTag_TooLong(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}
	longTag := strings.Repeat("a", MaxTagLength+1)

	err := inst.AddTag(longTag)
	if err == nil {
		t.Fatal("AddTag with tag exceeding MaxTagLength should return error")
	}

	var tooLongErr ErrTagTooLong
	if !errors.As(err, &tooLongErr) {
		t.Fatalf("expected ErrTagTooLong, got %T: %v", err, err)
	}
	if tooLongErr.Tag != longTag {
		t.Errorf("expected ErrTagTooLong.Tag to match input")
	}
	if tooLongErr.MaxLen != MaxTagLength {
		t.Errorf("expected ErrTagTooLong.MaxLen=%d, got %d", MaxTagLength, tooLongErr.MaxLen)
	}
}

func TestAddTag_ThenGetTags(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}

	if err := inst.AddTag("backend"); err != nil {
		t.Fatalf("AddTag(backend) failed: %v", err)
	}
	if err := inst.AddTag("urgent"); err != nil {
		t.Fatalf("AddTag(urgent) failed: %v", err)
	}

	tags := inst.GetTags()
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "backend" || tags[1] != "urgent" {
		t.Errorf("expected [backend, urgent], got %v", tags)
	}

	// Verify GetTags returns a copy, not a reference
	tags[0] = "modified"
	original := inst.GetTags()
	if original[0] != "backend" {
		t.Error("GetTags should return a copy, but modification affected the original")
	}
}

func TestSetTags_Deduplicates(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}

	err := inst.SetTags([]string{"a", "b", "a", "c", "b"})
	if err != nil {
		t.Fatalf("SetTags should succeed, got: %v", err)
	}

	tags := inst.GetTags()
	if len(tags) != 3 {
		t.Fatalf("expected 3 deduplicated tags, got %d: %v", len(tags), tags)
	}
	// Order should be preserved for first occurrence
	if tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", tags)
	}
}

func TestSetTags_ValidatesLength(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}
	longTag := strings.Repeat("x", MaxTagLength+1)

	err := inst.SetTags([]string{"valid", longTag, "also-valid"})
	if err == nil {
		t.Fatal("SetTags with one invalid tag should return error")
	}

	var tooLongErr ErrTagTooLong
	if !errors.As(err, &tooLongErr) {
		t.Fatalf("expected ErrTagTooLong, got %T: %v", err, err)
	}
}

func TestSetTags_ReplacesExisting(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"old-tag"}}

	err := inst.SetTags([]string{"new-tag-1", "new-tag-2"})
	if err != nil {
		t.Fatalf("SetTags should succeed, got: %v", err)
	}

	tags := inst.GetTags()
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "new-tag-1" || tags[1] != "new-tag-2" {
		t.Errorf("expected [new-tag-1, new-tag-2], got %v", tags)
	}
}

func TestRemoveTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"a", "b", "c"}}

	inst.RemoveTag("b")

	tags := inst.GetTags()
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags after removal, got %d: %v", len(tags), tags)
	}
	if tags[0] != "a" || tags[1] != "c" {
		t.Errorf("expected [a, c], got %v", tags)
	}
}

func TestRemoveTag_NotPresent(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"a", "b"}}

	inst.RemoveTag("not-here")

	tags := inst.GetTags()
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags unchanged, got %d: %v", len(tags), tags)
	}
}

func TestHasTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"frontend", "urgent"}}

	if !inst.HasTag("frontend") {
		t.Error("HasTag should return true for existing tag")
	}
	if !inst.HasTag("urgent") {
		t.Error("HasTag should return true for existing tag")
	}
	if inst.HasTag("backend") {
		t.Error("HasTag should return false for non-existing tag")
	}
}

func TestAddTag_MaxLengthBoundary(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}

	// Exactly MaxTagLength should succeed
	exactTag := strings.Repeat("z", MaxTagLength)
	err := inst.AddTag(exactTag)
	if err != nil {
		t.Fatalf("AddTag with exactly MaxTagLength chars should succeed, got: %v", err)
	}

	// MaxTagLength+1 should fail
	inst2 := &Instance{Title: "test"}
	overTag := strings.Repeat("z", MaxTagLength+1)
	err = inst2.AddTag(overTag)
	if err == nil {
		t.Fatal("AddTag with MaxTagLength+1 chars should return error")
	}
}

func TestSetTags_EmptySlice(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test", Tags: []string{"old"}}

	err := inst.SetTags([]string{})
	if err != nil {
		t.Fatalf("SetTags with empty slice should succeed, got: %v", err)
	}

	tags := inst.GetTags()
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d: %v", len(tags), tags)
	}
}

// --- Story 3.2.1: provenance-aware suppression/retraction (RemoveTag/AddTag/SetTags) ---

func TestRemoveTag_should_SuppressTag_When_TagHasRuleProvenance(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{"Bugfix", "MyTag"},
		RuleTagProvenance: map[string]string{"Bugfix": "seed-bugfix"},
	}

	inst.RemoveTag("Bugfix")

	if !inst.SuppressedRuleTags["Bugfix"] {
		t.Fatal("expected SuppressedRuleTags[\"Bugfix\"] == true after removing a provenanced tag")
	}
	if _, ok := inst.RuleTagProvenance["Bugfix"]; ok {
		t.Fatal("expected RuleTagProvenance[\"Bugfix\"] to be deleted after suppression")
	}
}

func TestRemoveTag_should_NotSuppress_When_TagHasNoProvenance(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{"Bugfix", "MyTag"},
		RuleTagProvenance: map[string]string{"Bugfix": "seed-bugfix"},
	}

	inst.RemoveTag("MyTag")

	if _, ok := inst.SuppressedRuleTags["MyTag"]; ok {
		t.Fatal("expected SuppressedRuleTags to have no entry for a plain user tag with no provenance")
	}
}

func TestAddTag_should_ClearSuppression_When_SuppressedTagReAdded(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:              "test",
		SuppressedRuleTags: map[string]bool{"Bugfix": true},
	}

	if err := inst.AddTag("Bugfix"); err != nil {
		t.Fatalf("AddTag(Bugfix) failed: %v", err)
	}

	if _, ok := inst.SuppressedRuleTags["Bugfix"]; ok {
		t.Fatal("expected SuppressedRuleTags[\"Bugfix\"] to be cleared after re-adding the tag")
	}
}

func TestSetTags_should_ApplySameSuppressionLogicAsRemoveTag_When_ProvenancedTagDroppedFromEditedList(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{"Bugfix", "MyTag"},
		RuleTagProvenance: map[string]string{"Bugfix": "seed-bugfix"},
	}

	if err := inst.SetTags([]string{"MyTag"}); err != nil {
		t.Fatalf("SetTags failed: %v", err)
	}

	if !inst.SuppressedRuleTags["Bugfix"] {
		t.Fatal("expected SuppressedRuleTags[\"Bugfix\"] == true after SetTags drops a provenanced tag")
	}
	if _, ok := inst.RuleTagProvenance["Bugfix"]; ok {
		t.Fatal("expected RuleTagProvenance[\"Bugfix\"] to be deleted after SetTags suppression")
	}
}

func TestSetTags_should_ClearSuppression_When_UserReAddsPreviouslySuppressedTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:              "test",
		Tags:               []string{"MyTag"},
		SuppressedRuleTags: map[string]bool{"Bugfix": true},
	}

	if err := inst.SetTags([]string{"MyTag", "Bugfix"}); err != nil {
		t.Fatalf("SetTags failed: %v", err)
	}

	if _, ok := inst.SuppressedRuleTags["Bugfix"]; ok {
		t.Fatal("expected SuppressedRuleTags[\"Bugfix\"] to be cleared after SetTags re-adds the tag")
	}
}

func TestSetTags_should_LeaveNoDanglingProvenanceEntry_When_TagRemovedViaFullReplace(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{"Bugfix"},
		RuleTagProvenance: map[string]string{"Bugfix": "seed-bugfix"},
	}

	if err := inst.SetTags([]string{}); err != nil {
		t.Fatalf("SetTags failed: %v", err)
	}

	for tag := range inst.RuleTagProvenance {
		if !inst.HasTag(tag) {
			t.Fatalf("dangling RuleTagProvenance entry %q for a tag absent from Tags", tag)
		}
	}
}

func TestRemoveTag_should_NeverSuppressUnclassifiedSentinel_When_UnclassifiedRemoved(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{UnclassifiedTag},
		RuleTagProvenance: map[string]string{UnclassifiedTag: "llm"},
	}

	inst.RemoveTag(UnclassifiedTag)

	if _, ok := inst.SuppressedRuleTags[UnclassifiedTag]; ok {
		t.Fatal("Unclassified must never enter SuppressedRuleTags, even via direct RemoveTag")
	}
}

func TestSetTags_should_NeverSuppressUnclassifiedSentinel_When_UnclassifiedClearedViaFullReplace(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{UnclassifiedTag},
		RuleTagProvenance: map[string]string{UnclassifiedTag: "llm"},
	}

	if err := inst.SetTags([]string{}); err != nil {
		t.Fatalf("SetTags failed: %v", err)
	}

	if _, ok := inst.SuppressedRuleTags[UnclassifiedTag]; ok {
		t.Fatal("Unclassified must never enter SuppressedRuleTags, even via SetTags full replace")
	}
}

// ── Story 4.3.2: ApplyLLMTagResult ──────────────────────────────────────────

func TestApplyLLMTagResult_should_RecordLLMProvenance_When_TagApplied(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test"}

	inst.ApplyLLMTagResult([]string{"Feature"}, llmSentinelRuleID)

	if !inst.HasTag("Feature") {
		t.Fatalf("expected Feature tag to be applied, got tags=%v", inst.GetTags())
	}
	if inst.RuleTagProvenance["Feature"] != llmSentinelRuleID {
		t.Errorf("RuleTagProvenance[Feature] = %q, want %q", inst.RuleTagProvenance["Feature"], llmSentinelRuleID)
	}
}

func TestApplyLLMTagResult_should_DropSuppressedCandidate_When_UserPreviouslyRemovedSameTag(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:              "test",
		SuppressedRuleTags: map[string]bool{"Feature": true},
	}

	inst.ApplyLLMTagResult([]string{"Feature"}, llmSentinelRuleID)

	if inst.HasTag("Feature") {
		t.Fatal("ApplyLLMTagResult must not re-add a tag the user previously suppressed")
	}
	if _, ok := inst.RuleTagProvenance["Feature"]; ok {
		t.Fatal("ApplyLLMTagResult must not record provenance for a suppressed candidate")
	}
}

func TestApplyLLMTagResult_should_DropUnclassified_When_RealTagAppliedByLaterPoll(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:             "test",
		Tags:              []string{UnclassifiedTag},
		RuleTagProvenance: map[string]string{UnclassifiedTag: llmSentinelRuleID},
	}

	inst.ApplyLLMTagResult([]string{"Feature"}, llmSentinelRuleID)

	tags := inst.GetTags()
	if len(tags) != 1 || tags[0] != "Feature" {
		t.Fatalf("expected tags=[Feature] with Unclassified dropped, got %v", tags)
	}
	if _, ok := inst.RuleTagProvenance[UnclassifiedTag]; ok {
		t.Fatal("Unclassified provenance entry must be removed once a real tag is present")
	}
}

func TestFilterSuppressedTags_should_DropOnlySuppressedCandidates_When_MixedCandidateListGiven(t *testing.T) {
	t.Parallel()
	candidates := []string{"Bugfix", "Feature", "Urgent"}
	suppressed := map[string]bool{"Feature": true}

	got := filterSuppressedTags(candidates, suppressed)

	want := []string{"Bugfix", "Urgent"}
	if len(got) != len(want) {
		t.Fatalf("filterSuppressedTags(%v, %v) = %v, want %v", candidates, suppressed, got, want)
	}
	for idx, tag := range want {
		if got[idx] != tag {
			t.Fatalf("filterSuppressedTags(%v, %v) = %v, want %v", candidates, suppressed, got, want)
		}
	}
}
