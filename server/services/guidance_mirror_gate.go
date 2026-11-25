package services

import "github.com/tstapler/stapler-squad/server/services/featureflags"

// AskUserQuestionRenderGate is the flag read NewGuidanceHookMirror takes as its enabled port. reg must be non-nil.
func AskUserQuestionRenderGate(reg *featureflags.Registry) func() bool {
	fr := flagReader{reg: reg}
	return func() bool { return fr.effectiveFlag(askUserQuestionStructuredFlagName, scopeStageRender) }
}
