package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
)

// GuidanceQuestionsRetention is how long a terminal (answered or cancelled) row
// keeps its questions_json: option text can quote user content (pre-mortem #18).
const GuidanceQuestionsRetention = 30 * 24 * time.Hour

// PurgeTerminalGuidanceQuestions blanks questions_json on rows that were
// answered or cancelled before cutoff and returns how many it cleared.
// question_text stays, so history still reads.
func (r *EntRepository) PurgeTerminalGuidanceQuestions(ctx context.Context, cutoff time.Time) (int, error) {
	n, err := r.client.GuidanceRequest.Update().
		Where(
			guidancerequest.QuestionsJSONNEQ(""),
			guidancerequest.Or(guidancerequest.AnsweredAtLT(cutoff), guidancerequest.CancelledAtLT(cutoff)),
		).
		SetQuestionsJSON("").
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("purge terminal guidance questions: %w", err)
	}
	return n, nil
}
