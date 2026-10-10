package notifications

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Reasons a row is planned for removal (PrunePlanRow.Reason).
const (
	PruneReasonRoutine          = "routine"
	PruneReasonUnreadActionable = "unread_actionable"
)

// backupTimeLayout is the UTC yyyymmddThhmmssZ stamp of a prune backup name.
const backupTimeLayout = "20060102T150405Z"

// ErrPruneBackupExists is returned when the timestamped backup already exists;
// a backup is never overwritten.
var ErrPruneBackupExists = errors.New("prune backup already exists")

// PruneDecision is a classifier's verdict for one stored row. The zero value
// is "undeterminable", which is always kept.
type PruneDecision struct {
	// Visible rows are kept and counted; false with Hidden false is undeterminable.
	Visible bool
	// Hidden rows belong to a hidden session and may be removed.
	Hidden bool
	// Group is the resolved session title plus hidden kind; Form is the
	// identity form the row matched by; ByAlias flags a rename-alias or
	// tombstone match. All three are reporting only.
	Group   string
	Form    string
	ByAlias bool
}

// PruneClassifier classifies one stored row. It is called with the store lock
// held, so it must not call back into the store. The store holds whichever
// session ID form each producer used (UUID, title or tmux name); the
// classifier is responsible for resolving every form.
type PruneClassifier func(r *NotificationRecord) PruneDecision

// PrunePlanRow is one row planned for removal.
type PrunePlanRow struct {
	ID      string
	Reason  string
	Group   string
	Form    string
	ByAlias bool
}

// PrunePlan is the outcome of a prune: what would be (or was) removed and what
// was kept, so the caller can report and audit it.
type PrunePlan struct {
	Remove               []PrunePlanRow
	KeptUnreadActionable int
	Undeterminable       int
	Visible              int
	// Applied is true when rows were deleted from the store.
	Applied bool
	// BackupPath is the timestamped backup written before an apply that
	// removed rows; empty otherwise.
	BackupPath string
}

// IDs returns the planned row ids in store order.
func (p PrunePlan) IDs() []string {
	ids := make([]string, len(p.Remove))
	for i, r := range p.Remove {
		ids[i] = r.ID
	}
	return ids
}

// ReasonCounts counts planned removals by reason.
func (p PrunePlan) ReasonCounts() map[string]int {
	out := map[string]int{}
	for _, r := range p.Remove {
		out[r.Reason]++
	}
	return out
}

// PruneOptions controls PruneByPredicate.
type PruneOptions struct {
	// Apply deletes; false is a dry run that writes nothing.
	Apply bool
	// IncludeUnreadActionable also removes unread pending-decision rows of
	// hidden sessions; by default they are kept.
	IncludeUnreadActionable bool
	// Now stamps the backup name (injected so tests never read the clock).
	Now time.Time
	// BeforeApply runs under the store lock once the plan is final and before
	// the backup and the delete; an error aborts with nothing written. It must
	// not call the store.
	BeforeApply func(PrunePlan) error
}

// PruneByPredicate removes the rows classify marks hidden. Keep rules, in
// order: undeterminable and visible rows are always kept; an unread pending
// decision is kept unless opts.IncludeUnreadActionable. On Apply with rows to
// remove it runs BeforeApply, copies the store file to
// <file>.pre-prune-<UTC yyyymmddThhmmssZ>.bak (never overwriting), then
// deletes and persists. It is keyed on current state, so a re-run removes
// nothing more.
func (s *NotificationHistoryStore) PruneByPredicate(classify PruneClassifier, opts PruneOptions) (PrunePlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	plan, drop := s.planPruneLocked(classify, opts)
	if !opts.Apply || len(plan.Remove) == 0 {
		return plan, nil
	}
	if opts.BeforeApply != nil {
		if err := opts.BeforeApply(plan); err != nil {
			return plan, err
		}
	}
	backup, err := s.writePruneBackupLocked(opts.Now)
	if err != nil {
		return plan, err
	}
	plan.BackupPath = backup

	previous := s.records
	remaining := make([]*NotificationRecord, 0, len(previous)-len(drop))
	for _, r := range previous {
		if _, gone := drop[r.ID]; !gone {
			remaining = append(remaining, r)
		}
	}
	s.records = remaining
	if err := s.saveToDisk(); err != nil {
		s.records = previous
		return plan, err
	}
	plan.Applied = true
	return plan, nil
}

func (s *NotificationHistoryStore) planPruneLocked(classify PruneClassifier, opts PruneOptions) (PrunePlan, map[string]struct{}) {
	var plan PrunePlan
	drop := map[string]struct{}{}
	for _, r := range s.records {
		d := classify(r)
		switch {
		case d.Visible:
			plan.Visible++
			continue
		case !d.Hidden:
			plan.Undeterminable++
			continue
		}
		reason := PruneReasonRoutine
		if IsPendingDecision(r.NotificationType, r.Metadata, r.IsRead) {
			if !opts.IncludeUnreadActionable {
				plan.KeptUnreadActionable++
				continue
			}
			reason = PruneReasonUnreadActionable
		}
		plan.Remove = append(plan.Remove, PrunePlanRow{ID: r.ID, Reason: reason, Group: d.Group, Form: d.Form, ByAlias: d.ByAlias})
		drop[r.ID] = struct{}{}
	}
	return plan, drop
}

// writePruneBackupLocked copies the on-disk store file; the copy is what
// restores the pre-prune state byte for byte.
func (s *NotificationHistoryStore) writePruneBackupLocked(now time.Time) (string, error) {
	// #nosec G304 -- s.filePath is the internal config-dir store path, never RPC input.
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return "", fmt.Errorf("read store for backup: %w", err)
	}
	path := s.filePath + ".pre-prune-" + now.UTC().Format(backupTimeLayout) + ".bak"
	// #nosec G304 -- derived from the internal store path plus a timestamp.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrPruneBackupExists, path)
		}
		return "", fmt.Errorf("create backup: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // best-effort: a partial backup must not look valid
		return "", fmt.Errorf("write backup: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close backup: %w", err)
	}
	return path, nil
}
