package session

import (
	"os"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// detectAdoptableByPath is DetectByPath minus conversations this instance must
// not take: ones not modified since the last explicit clear, ones owned by
// another live session, and (for a session that never had a conversation) ones
// that began before it was created. Candidates are tried newest-first; returns
// nil, nil when none qualify.
func (i *Instance) detectAdoptableByPath(detector *HistoryFileDetector, path string) (*HistoryFileInfo, error) {
	candidates, err := detector.DetectAllByPath(path)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}

	i.claudeSessionMu.RLock()
	clearedAt := i.conversationClearedAt
	i.claudeSessionMu.RUnlock()

	var notBefore time.Time
	if snap := i.Snapshot(); !snap.EverHadConversationHistory {
		notBefore = snap.CreatedAt
	}

	for idx := range candidates {
		c := &candidates[idx]
		if !clearedAt.IsZero() {
			if st, statErr := os.Stat(c.HistoryFilePath); statErr == nil && !st.ModTime().After(clearedAt) {
				log.Debug("tryextractconversationuuid: found jsonl predates last explicit clear, skipping recovery",
					"session", i.Title, "path", c.HistoryFilePath, "clearedAt", clearedAt)
				continue
			}
		}
		// Cross-session ownership guard (worktree-envvars-hijack Story 1.4.2): the
		// guard's selfUUID exclusion lets a cold-restoring session re-adopt its own
		// conversation; only a still-live sibling's is refused.
		if i.conversationOwnershipGuard != nil {
			if ownerUUID, owned := i.conversationOwnershipGuard(c.ConversationUUID, path); owned {
				log.Warn("tryextractconversationuuid: conversation UUID owned by another live session, not adopting",
					"session", i.Title, "path", path, "owner_uuid", ownerUUID)
				continue
			}
		}
		if !notBefore.IsZero() {
			if started, ok := jsonlStartTime(c.HistoryFilePath); ok && started.Before(notBefore) {
				log.Debug("tryextractconversationuuid: skipping conversation that predates session",
					"session", i.Title, "conv_uuid", c.ConversationUUID)
				continue
			}
		}
		return c, nil
	}
	return nil, nil //nolint:nilnil
}
