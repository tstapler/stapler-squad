package notifications

// Metadata keys the question notifications carry (server/services/approval_question.go).
const (
	metaQuestionShape   = "question_shape"
	metaQuestionOptions = "question_options"
	metaQuestionID      = "question_id"
	metaApprovalID      = "approval_id"
)

// questionKey is what tells two question notifications of one session apart: the
// question text plus its option labels, so a re-asked identical question
// collapses while a different question stays its own card. It is empty for every
// other notification, which keeps the (session, type) collapse.
func questionKey(r *NotificationRecord) string {
	if r.Metadata[metaQuestionShape] == "" {
		return ""
	}
	return r.Message + "\x00" + r.Metadata[metaQuestionOptions]
}

// ResolveQuestion retires the unread records of one answered question and
// returns how many it retired.
func (s *NotificationHistoryStore) ResolveQuestion(questionID string) (int, error) {
	return s.resolveByMetadata(metaQuestionID, questionID)
}

// ResolveApproval retires the unread records of one decided approval.
func (s *NotificationHistoryStore) ResolveApproval(approvalID string) (int, error) {
	return s.resolveByMetadata(metaApprovalID, approvalID)
}

// resolveByMetadata marks read every unread record whose metadata[key] is id.
// An empty id matches nothing.
func (s *NotificationHistoryStore) resolveByMetadata(key, id string) (int, error) {
	if id == "" {
		return 0, nil
	}
	return s.MarkReadWhere(func(r *NotificationRecord) bool { return r.Metadata[key] == id })
}
