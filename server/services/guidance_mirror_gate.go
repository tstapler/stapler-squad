package services

import (
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

// AskUserQuestionRenderGate is the flag read NewGuidanceHookMirror takes as its enabled port. reg must be non-nil.
func AskUserQuestionRenderGate(reg *featureflags.Registry) func() bool {
	fr := flagsvc.FlagReader{Reg: reg}
	return func() bool {
		return fr.EffectiveFlag(flagsvc.AskUserQuestionStructuredFlagName, flagsvc.ScopeStageRender)
	}
}
