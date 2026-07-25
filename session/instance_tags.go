package session

// instance_tags.go contains tag management delegation methods for Instance.
// All methods delegate to TagManager with stateMutex protection.

// ensureTagManager lazily initializes the tagManager if it was not set up
// (e.g., when Instance is created via struct literal in tests).
// Must be called with stateMutex held.
func (i *Instance) ensureTagManager() {
	if i.tagManager.tags == nil {
		i.tagManager = NewTagManager(&i.Tags)
	}
}

// AddTag adds a tag to the instance. Delegates to TagManager.Add.
// Returns ErrTagTooLong if the tag exceeds MaxTagLength, or ErrDuplicateTag if it already exists.
func (i *Instance) AddTag(tag string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ensureTagManager()
	err := i.tagManager.Add(tag)
	if err == nil {
		i.snapshot.Store(buildSnapshot(i))
	}
	return err
}

// RemoveTag removes a tag from the instance. Delegates to TagManager.Remove.
func (i *Instance) RemoveTag(tag string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ensureTagManager()
	i.tagManager.Remove(tag)
	i.snapshot.Store(buildSnapshot(i))
}

// HasTag returns true if the instance has the specified tag. Delegates to TagManager.Has.
func (i *Instance) HasTag(tag string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.tagManager.tags == nil {
		// Fallback for struct-literal created instances (read-only path, no init needed)
		for _, t := range i.Tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	return i.tagManager.Has(tag)
}

// GetTags returns a copy of the instance's tags. Delegates to TagManager.All.
func (i *Instance) GetTags() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.tagManager.tags == nil {
		// Fallback for struct-literal created instances (read-only path)
		result := make([]string, len(i.Tags))
		copy(result, i.Tags)
		return result
	}
	return i.tagManager.All()
}

// SetTags replaces all tags with a new deduplicated set. Delegates to TagManager.Set.
// Returns ErrTagTooLong on the first tag that exceeds MaxTagLength.
func (i *Instance) SetTags(tags []string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ensureTagManager()
	err := i.tagManager.Set(tags)
	if err == nil {
		i.snapshot.Store(buildSnapshot(i))
	}
	return err
}
