package questionkit

// Notification metadata keys of a question notification that cross the package boundary.
const (
	MetaQuestionID = "question_id"
	// MetaGuidanceRequestID is the durable GuidanceRequest row mirrored for the question.
	MetaGuidanceRequestID = "guidance_request_id"
	MetaReplyUnavailable  = "reply_unavailable"
	// MetaPeerTitle names the unresponsive answerer of a peer-help escalation; the
	// descriptor builder labels the answer action "Answer for {peer}" with it.
	MetaPeerTitle = "peer_title"
)
