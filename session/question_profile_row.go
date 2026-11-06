package session

import (
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ProfileGuidanceParams is the guidance row for a question a harness profile
// detected: source ask_user_question_hook (there is no terminal_detect source,
// ADR-003) with the profile name in questions_json.profile. ok is false for a
// profile that is not in DefaultQuestionProfiles, so an unmeasured harness
// never gets a replyable row.
func ProfileGuidanceParams(sessionUUID string, d dtypes.ProfileDialog) (domain.GuidanceRequestParams, bool) {
	if !DefaultQuestionProfiles().Has(d.Profile) || len(d.Options) == 0 {
		return domain.GuidanceRequestParams{}, false
	}
	opts := make([]domain.Option, len(d.Options))
	for i, l := range d.Options {
		opts[i] = domain.Option{Label: l}
	}
	payload, err := domain.NewQuestionPayload([]domain.Question{{
		Text: d.Question, Header: d.Header, Options: opts, Profile: d.Profile,
	}})
	if err != nil {
		return domain.GuidanceRequestParams{}, false
	}
	return domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: sessionUUID, AskerSessionUUID: sessionUUID,
		Source: domain.QuestionSourceHook, Payload: payload,
	}, true
}
