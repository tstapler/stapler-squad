package services

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// The six live-settable rollout flags of attention-inbox-consolidation. All are
// default OFF and global-only; they are plain registry flags read from config at
// decision time, so a flip takes effect without a restart. No env vars.
const (
	attentionProseQuestionDetectionFlagName   = "attention_prose_question_detection"
	sessionEventLogFlagName                   = "session_event_log"
	attentionAckBypassFixFlagName             = "attention_ack_bypass_fix"
	attentionIdleClockOutputFlagName          = "attention_idle_clock_output"
	attentionProseQuestionLLMTiebreakFlagName = "attention_prose_question_llm_tiebreak"
	attentionInboxConsolidationFlagName       = "attention_inbox_consolidation"
)

// AttentionFlags is one point-in-time read of the six flags. Callers that run a
// cycle (the review-queue poller) resolve it once per cycle and use that value
// throughout, so a mid-cycle flip cannot split one cycle across two behaviours.
type AttentionFlags struct {
	ProseQuestionDetection   bool
	SessionEventLog          bool
	AckBypassFix             bool
	IdleClockOutput          bool
	ProseQuestionLLMTiebreak bool
	InboxConsolidation       bool
}

// ResolveAttentionFlags reads the six flags from cfg.
// A nil reg falls back to featureflags.Current() until slice 7 of flags-stage2-plan.md.
func ResolveAttentionFlags(cfg *config.Config, reg *featureflags.Registry) AttentionFlags {
	reg = flagReader{reg: reg}.registry()
	on := func(name string) bool { return reg.Enabled(cfg, name) }
	return AttentionFlags{
		ProseQuestionDetection:   on(attentionProseQuestionDetectionFlagName),
		SessionEventLog:          on(sessionEventLogFlagName),
		AckBypassFix:             on(attentionAckBypassFixFlagName),
		IdleClockOutput:          on(attentionIdleClockOutputFlagName),
		ProseQuestionLLMTiebreak: on(attentionProseQuestionLLMTiebreakFlagName),
		InboxConsolidation:       on(attentionInboxConsolidationFlagName),
	}
}

// LoadAttentionFlags resolves the flags against the live config.
func LoadAttentionFlags(reg *featureflags.Registry) AttentionFlags {
	return ResolveAttentionFlags(config.LoadConfig(), reg)
}
