package session

// TagManager provides CRUD operations for session tags.
// It is a pure data structure with no I/O or external dependencies.
// Thread safety is provided by Instance.mu -- callers must hold
// the lock when calling TagManager methods.
//
// TagManager stores a pointer to the Instance.Tags slice so that mutations
// are automatically visible via inst.Tags (used by instance_adapter.go,
// review_queue_poller.go, and ToInstanceData for serialization).
type TagManager struct {
	tags *[]string // points to Instance.Tags for zero-sync compatibility
}

// NewTagManager creates a TagManager backed by the given slice pointer.
func NewTagManager(tags *[]string) TagManager {
	return TagManager{tags: tags}
}

// Add adds a tag if it does not already exist and does not exceed MaxTagLength.
// Returns ErrTagTooLong if the tag exceeds MaxTagLength.
// Returns ErrDuplicateTag if the tag already exists.
func (tm *TagManager) Add(tag string) error {
	if len(tag) > MaxTagLength {
		return ErrTagTooLong{Tag: tag, MaxLen: MaxTagLength}
	}
	for _, existing := range *tm.tags {
		if existing == tag {
			return ErrDuplicateTag{Tag: tag}
		}
	}
	*tm.tags = append(*tm.tags, tag)
	return nil
}

// Remove removes a tag by value. No-op if the tag does not exist.
func (tm *TagManager) Remove(tag string) {
	newTags := make([]string, 0, len(*tm.tags))
	for _, existing := range *tm.tags {
		if existing != tag {
			newTags = append(newTags, existing)
		}
	}
	*tm.tags = newTags
}

// Has returns true if the tag exists.
func (tm *TagManager) Has(tag string) bool {
	for _, existing := range *tm.tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// All returns a copy of the tag slice.
func (tm *TagManager) All() []string {
	result := make([]string, len(*tm.tags))
	copy(result, *tm.tags)
	return result
}

// Set replaces all tags with a new deduplicated set.
// Returns ErrTagTooLong on the first tag that exceeds MaxTagLength.
// Returns ErrTooManyTags if the deduplicated count exceeds MaxTagCount.
func (tm *TagManager) Set(tags []string) error {
	seen := make(map[string]struct{}, len(tags))
	deduped := make([]string, 0, len(tags))
	for _, tag := range tags {
		if len(tag) > MaxTagLength {
			return ErrTagTooLong{Tag: tag, MaxLen: MaxTagLength}
		}
		if _, exists := seen[tag]; !exists {
			seen[tag] = struct{}{}
			deduped = append(deduped, tag)
		}
	}
	if len(deduped) > MaxTagCount {
		return ErrTooManyTags{Count: len(deduped), MaxCount: MaxTagCount}
	}
	*tm.tags = deduped
	return nil
}
