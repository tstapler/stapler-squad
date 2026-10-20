package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/tstapler/stapler-squad/internal/claudehooks"
	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// ask_user_question.go is the AskUserQuestion branch of `ssq-hooks check`
// (ADR-005 rung 1). It asks the resident instance whether the operator already
// answered this question; if so it prints allow + updatedInput.answers, which
// makes Claude skip the dialog. In every other case it prints nothing and the
// native dialog stays.
//
// The caller must exit 0 afterwards: a command hook that exits 2 DENIES the
// question tool call (spike S2), so no failure here may become a non-zero exit.

// sessionUUIDEnv names the Stapler Squad session the hook runs in (set by the
// session launcher); it is the only context the resident needs.
const sessionUUIDEnv = "STAPLER_SESSION_UUID"

// defaultAskWait bounds the lookup when hook_block_ms is 0: the server answers
// from storage and never waits, so a slow reply means no answer.
const defaultAskWait = 250 * time.Millisecond

// askUserQuestionViaResident writes the allow output to out only when the
// resident returns one. Any error, timeout or empty reply writes nothing; a wait
// of hook_block_ms (capped) bounds the whole call.
func askUserQuestionViaResident(payload classifier.PermissionRequestPayload, dial hookipc.Dialer, lookupEnv func(string) (string, bool), out io.Writer) {
	if payload.HookEventName == "" {
		payload.HookEventName = "PreToolUse"
	}
	wait := defaultAskWait
	if ms := claudehooks.HookBlockMsFromEnv(lookupEnv); ms > 0 {
		wait = time.Duration(ms) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	endpoint, transport, err := dial(payload.Cwd)
	if err != nil {
		return
	}
	requestID, _, err := hookipc.NewRequestID(payload.SessionID, hookipc.ToolUseID(payload.ToolUseID))
	if err != nil {
		return
	}
	env := map[string]string{}
	if uuid, ok := lookupEnv(sessionUUIDEnv); ok && uuid != "" {
		env[sessionUUIDEnv] = uuid
	}
	reply, err := askWithin(ctx, transport, hookipc.ClassificationEnvelope{
		ProtocolVersion:     endpoint.ProtocolVersion,
		InstanceFingerprint: endpoint.InstanceFingerprint,
		RequestID:           requestID,
		Payload:             payload,
		Context:             hookipc.InvocationContext{Cwd: payload.Cwd, Env: env},
	})
	if err != nil || reply.Output == nil {
		return
	}
	if err := json.NewEncoder(out).Encode(reply.Output); err != nil {
		log.Warn("failed to write AskUserQuestion hook answer", "err", err)
	}
}

// askWithin returns when the transport does or ctx ends, whichever is first, so
// a transport that ignores its context cannot hold the hook past its cap.
func askWithin(ctx context.Context, t hookipc.Transport, e hookipc.ClassificationEnvelope) (hookipc.ClassificationReply, error) {
	type result struct {
		reply hookipc.ClassificationReply
		err   error
	}
	done := make(chan result, 1) // buffered: a late reply never blocks its goroutine
	go func() {
		r, err := t.Classify(ctx, e)
		done <- result{r, err}
	}()
	select {
	case r := <-done:
		return r.reply, r.err
	case <-ctx.Done():
		return hookipc.ClassificationReply{}, ctx.Err()
	}
}
