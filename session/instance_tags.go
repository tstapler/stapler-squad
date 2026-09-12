package session

import "slices"

// instance_tags.go contains tag management delegation methods for Instance.
// All methods delegate to TagManager with stateMutex protection.

// UnclassifiedTag is the sentinel tag applied by the (Phase 4) LLM-fallback poller when
// classification fails, times out, or returns an out-of-vocabulary result. Defined here
// (not in session/session_tag_poller.go, per the plan's Domain Glossary) because Phase 3's
// suppression guard needs it ahead of Phase 4's poller landing — Unclassified is never
// suppressible: it is removed automatically the moment any other tag is present
// (dropUnclassifiedIfOtherTagsPresentLocked, instance_actor_setters.go), not via user removal.
const UnclassifiedTag = "Unclassified"

// ensureTagManager lazily initializes the tagManager if it was not set up
// (e.g., when Instance is created via struct literal in tests).
// Must be called with stateMutex held.
func (i *Instance) ensureTagManager() {
	if i.tagManager.tags == nil {
		i.tagManager = NewTagManager(&i.Tags)
	}
}

// suppressIfProvenanced records tag's removal into SuppressedRuleTags and clears its
// RuleTagProvenance entry, unless tag is UnclassifiedTag (never suppressible, ADR-002/Task
// 3.2.1a) or tag has no provenance (a plain user tag — nothing to suppress). Lazily
// initializes both maps. Must be called with i.mu held. Shared by RemoveTag/SetTags so the
// two mutation paths (and the Tag Editor Modal's actual save path, SetTags) cannot drift —
// see plan.md Story 3.2.1's Blocker 2 lineage.
func (i *Instance) suppressIfProvenanced(tag string) {
	if tag == UnclassifiedTag {
		if i.RuleTagProvenance != nil {
			delete(i.RuleTagProvenance, tag)
		}
		return
	}
	if i.RuleTagProvenance == nil {
		return
	}
	if _, hasProvenance := i.RuleTagProvenance[tag]; !hasProvenance {
		return
	}
	delete(i.RuleTagProvenance, tag)
	if i.SuppressedRuleTags == nil {
		i.SuppressedRuleTags = make(map[string]bool)
	}
	i.SuppressedRuleTags[tag] = true
}

// clearSuppression removes tag's SuppressedRuleTags entry, if any — called when a user
// explicitly re-adds a tag via AddTag/SetTags, making it fair game for a rule to claim
// provenance again on the next fixpoint pass. Must be called with i.mu held.
func (i *Instance) clearSuppression(tag string) {
	if i.SuppressedRuleTags != nil {
		delete(i.SuppressedRuleTags, tag)
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
		i.clearSuppression(tag)
		i.snapshot.Store(buildSnapshot(i))
	}
	return err
}

// RemoveTag removes a tag from the instance. Delegates to TagManager.Remove.
func (i *Instance) RemoveTag(tag string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ensureTagManager()
	i.suppressIfProvenanced(tag)
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
//
// This is the Tag Editor Modal's actual save path (TagEditor.tsx's handleSave -> UpdateSession
// RPC -> instance.SetTags) — see plan.md Story 3.2.1's Blocker 2 lineage. Diffs old vs. new
// tags and applies the exact same suppression/provenance bookkeeping as RemoveTag/AddTag,
// via the shared suppressIfProvenanced/clearSuppression helpers, all under this one lock
// acquisition (no separate lock, no calling the public RemoveTag/AddTag methods themselves).
func (i *Instance) SetTags(tags []string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ensureTagManager()

	oldTags := make(map[string]bool, len(i.Tags))
	for _, t := range i.Tags {
		oldTags[t] = true
	}
	newTags := make(map[string]bool, len(tags))
	for _, t := range tags {
		newTags[t] = true
	}

	err := i.tagManager.Set(tags)
	if err != nil {
		return err
	}

	for t := range oldTags {
		if !newTags[t] {
			i.suppressIfProvenanced(t)
		}
	}
	for t := range newTags {
		if !oldTags[t] {
			i.clearSuppression(t)
		}
	}

	i.snapshot.Store(buildSnapshot(i))
	return nil
}

// ApplyLLMTagResult applies tags produced by the Phase 4 LLM fallback poller
// (headless.GenerateSessionTags), attributing each surviving tag to ruleID (llmSentinelRuleID
// in production) and otherwise following the exact same suppression/coexistence rules as the
// sync fixpoint path (reclassifyTagsLocked, instance_actor_setters.go) — see Story 4.3.2. The
// poller is an external caller rather than an actor setter, so this takes i.mu itself, the
// same direct-lock pattern AddTag/RemoveTag/SetTags already use.
func (i *Instance) ApplyLLMTagResult(tags []string, ruleID string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.RuleTagProvenance == nil {
		i.RuleTagProvenance = make(map[string]string)
	}
	for _, tag := range filterSuppressedTags(tags, i.SuppressedRuleTags) {
		if !slices.Contains(i.Tags, tag) {
			i.Tags = append(i.Tags, tag)
		}
		i.RuleTagProvenance[tag] = ruleID
	}
	dropUnclassifiedIfOtherTagsPresentLocked(&instanceState{inst: i})

	i.snapshot.Store(buildSnapshot(i))
}
