package questionkit

// Notification metadata keys of a question notification that cross the package boundary.
const (
	MetaQuestionID = "question_id"
	// MetaGuidanceRequestID is the durable GuidanceRequest row mirrored for the question.
	MetaGuidanceRequestID = "guidance_request_id"
	MetaReplyUnavailable  = "reply_unavailable"
)
