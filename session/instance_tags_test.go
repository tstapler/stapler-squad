package session

import (
	"errors"
	"strings"
	"testing"
)

func TestAddTag_ValidTag(t *testing.T) {
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
	inst := &Instance{Title: "test", Tags: []string{"a", "b"}}

	inst.RemoveTag("not-here")

	tags := inst.GetTags()
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags unchanged, got %d: %v", len(tags), tags)
	}
}

func TestHasTag(t *testing.T) {
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
