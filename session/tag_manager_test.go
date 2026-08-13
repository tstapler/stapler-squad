package session

import (
	"strings"
	"testing"
)

func TestTagManager_Add(t *testing.T) {
	tags := []string{}
	tm := NewTagManager(&tags)

	// Add a new tag
	err := tm.Add("frontend")
	if err != nil {
		t.Fatalf("unexpected error adding tag: %v", err)
	}
	if len(tags) != 1 || tags[0] != "frontend" {
		t.Fatalf("expected tags to be [frontend], got %v", tags)
	}

	// Add a second tag
	err = tm.Add("backend")
	if err != nil {
		t.Fatalf("unexpected error adding second tag: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	// Add duplicate tag should return ErrDuplicateTag
	err = tm.Add("frontend")
	if err == nil {
		t.Fatal("expected error adding duplicate tag, got nil")
	}
	var dupErr ErrDuplicateTag
	if _, ok := err.(ErrDuplicateTag); !ok {
		t.Fatalf("expected ErrDuplicateTag, got %T: %v", err, err)
	}
	_ = dupErr
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags after duplicate add, got %d", len(tags))
	}

	// Add tag that exceeds MaxTagLength
	longTag := strings.Repeat("a", MaxTagLength+1)
	err = tm.Add(longTag)
	if err == nil {
		t.Fatal("expected error adding too-long tag, got nil")
	}
	if _, ok := err.(ErrTagTooLong); !ok {
		t.Fatalf("expected ErrTagTooLong, got %T: %v", err, err)
	}
}

func TestTagManager_Remove(t *testing.T) {
	tags := []string{"frontend", "backend", "urgent"}
	tm := NewTagManager(&tags)

	// Remove existing tag
	tm.Remove("backend")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags after remove, got %d", len(tags))
	}
	if tags[0] != "frontend" || tags[1] != "urgent" {
		t.Fatalf("expected [frontend urgent], got %v", tags)
	}

	// Remove non-existent tag (no-op)
	tm.Remove("nonexistent")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags after removing nonexistent, got %d", len(tags))
	}

	// Remove all tags
	tm.Remove("frontend")
	tm.Remove("urgent")
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags after removing all, got %d", len(tags))
	}
}

func TestTagManager_Has(t *testing.T) {
	tags := []string{"frontend", "backend"}
	tm := NewTagManager(&tags)

	if !tm.Has("frontend") {
		t.Fatal("expected Has(frontend) to be true")
	}
	if !tm.Has("backend") {
		t.Fatal("expected Has(backend) to be true")
	}
	if tm.Has("nonexistent") {
		t.Fatal("expected Has(nonexistent) to be false")
	}
}

func TestTagManager_All(t *testing.T) {
	tags := []string{"frontend", "backend"}
	tm := NewTagManager(&tags)

	result := tm.All()
	if len(result) != 2 || result[0] != "frontend" || result[1] != "backend" {
		t.Fatalf("expected [frontend backend], got %v", result)
	}

	// Verify it returns a copy, not a reference
	result[0] = "modified"
	if tags[0] != "frontend" {
		t.Fatal("All() returned a reference instead of a copy; original slice was mutated")
	}
}

func TestTagManager_Set(t *testing.T) {
	tags := []string{"old-tag"}
	tm := NewTagManager(&tags)

	// Replace with new tags
	err := tm.Set([]string{"new1", "new2", "new3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 3 || tags[0] != "new1" || tags[1] != "new2" || tags[2] != "new3" {
		t.Fatalf("expected [new1 new2 new3], got %v", tags)
	}

	// Set with empty slice
	err = tm.Set([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags, got %d", len(tags))
	}

	// Set with duplicates (should deduplicate)
	err = tm.Set([]string{"a", "b", "a", "c", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 3 || tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Fatalf("expected [a b c], got %v", tags)
	}

	// Set with a too-long tag
	longTag := strings.Repeat("x", MaxTagLength+1)
	err = tm.Set([]string{"valid", longTag})
	if err == nil {
		t.Fatal("expected error with too-long tag in Set, got nil")
	}
	if _, ok := err.(ErrTagTooLong); !ok {
		t.Fatalf("expected ErrTagTooLong, got %T: %v", err, err)
	}
}

func TestTagManager_BackingSliceMutation(t *testing.T) {
	// Verify that mutations through TagManager are visible via the backing slice
	tags := []string{}
	tm := NewTagManager(&tags)

	_ = tm.Add("hello")
	if len(tags) != 1 || tags[0] != "hello" {
		t.Fatalf("backing slice not updated after Add: %v", tags)
	}

	tm.Remove("hello")
	if len(tags) != 0 {
		t.Fatalf("backing slice not updated after Remove: %v", tags)
	}

	_ = tm.Set([]string{"x", "y"})
	if len(tags) != 2 {
		t.Fatalf("backing slice not updated after Set: %v", tags)
	}
}
