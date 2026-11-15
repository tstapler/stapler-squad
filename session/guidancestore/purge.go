package guidancestore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent/guidancerequest"
)

// PurgeTerminalGuidanceQuestions blanks questions_json on rows that were
// answered or cancelled before cutoff and returns how many it cleared.
// question_text stays, so history still reads.
func (r *Repo) PurgeTerminalGuidanceQuestions(ctx context.Context, cutoff time.Time) (int, error) {
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
