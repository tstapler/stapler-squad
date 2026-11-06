package notifications

// ApprovalHead is the next pending approval a collapsed approval row should show.
type ApprovalHead struct {
	ID       string
	Title    string
	Message  string
	Metadata map[string]string
}

// AdvanceApproval re-points the record that shows decidedID at head: twenty
// approvals of a session collapse into one record that addresses only the most
// recent, so deciding it must hand the row to the next pending approval instead
// of retiring a row that still stands for the others. The count follows the
// approvals still waiting. It reports whether a record was found.
func (s *NotificationHistoryStore) AdvanceApproval(decidedID string, head ApprovalHead) (bool, error) {
	if decidedID == "" || head.ID == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if r.NotificationType != notifTypeApprovalNeeded || (r.ID != decidedID && r.Metadata[metaApprovalID] != decidedID) {
			continue
		}
		r.ID, r.Title, r.Message, r.Metadata = head.ID, head.Title, head.Message, head.Metadata
		r.OccurrenceCount = max(r.OccurrenceCount-1, 1)
		r.IsRead, r.ReadAt = false, nil
		return true, s.saveToDisk()
	}
	return false, nil
}
