"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";
import { AttachPullRequestResult, MergeMethod, SessionService } from "@/gen/session/v1/session_pb";
import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";
import { PullRequestRefSchema, type PrHold, type Session, type SessionPullRequest } from "@/gen/session/v1/types_pb";
import { outcomeMessage } from "@/lib/pr/nudgeOutcomeCopy";
import { prTarget } from "@/lib/pr/pullRequests";
import { OK, failure, type PrActionResult, type PrTarget, type PullRequestActions } from "@/lib/pr/actionResult";
import { classifyPrError, classifyPrMessage } from "@/lib/pr/classifyPrError";

const METHODS = { merge: MergeMethod.MERGE, squash: MergeMethod.SQUASH, rebase: MergeMethod.REBASE } as const;
const ref = (t: PrTarget) => create(PullRequestRefSchema, t);

interface SessionPullRequestsState {
  pullRequests: SessionPullRequest[];
  holds: PrHold[];
  actions: PullRequestActions;
}

/**
 * The session's PR list plus the per-PR operations. Every operation passes the row's own ref, and the
 * list updates from the response (or a refresh after merge/close) until the next session push replaces it.
 */
export function useSessionPullRequests(session: Session): SessionPullRequestsState {
  const [local, setLocal] = useState<SessionPullRequest[] | null>(null);
  const sessionId = session.id;
  const client = useMemo(() => createClient(SessionService, getConnectTransport()), []);

  // A newer list pushed with the session wins over the last response.
  useEffect(() => setLocal(null), [session.pullRequests]);

  const idRef = useRef(sessionId);
  idRef.current = sessionId;
  const titleRef = useRef(session.title);
  titleRef.current = session.title;

  const withList = useCallback(
    async (call: () => Promise<{ pullRequests: SessionPullRequest[] }>): Promise<PrActionResult> => {
      try {
        setLocal((await call()).pullRequests);
        return OK;
      } catch (err) {
        return classifyPrError(err);
      }
    },
    [],
  );

  const actions = useMemo<PullRequestActions>(() => {
    const refresh = () => withList(() => client.refreshSessionPullRequests({ id: idRef.current })); // abort-signal-exempt: one-shot user action
    const verbatim = async (call: () => Promise<{ success: boolean; message: string }>): Promise<PrActionResult> => {
      try {
        const res = await call();
        if (!res.success) return classifyPrMessage(res.message);
      } catch (err) {
        return classifyPrError(err);
      }
      await refresh();
      return OK;
    };
    return {
      refresh,
      merge: (pr, method) => verbatim(() => client.mergePR({ id: idRef.current, pr: ref(pr), mergeMethod: METHODS[method] })), // abort-signal-exempt: one-shot user action
      close: (pr) => verbatim(() => client.closePR({ id: idRef.current, pr: ref(pr) })), // abort-signal-exempt: one-shot user action
      pin: (pr) => withList(() => client.pinSessionPullRequest({ id: idRef.current, pr: pr ? ref(pr) : undefined })), // abort-signal-exempt: one-shot user action
      detach: (pr) => withList(() => client.detachSessionPullRequest({ id: idRef.current, pr: ref(pr) })), // abort-signal-exempt: one-shot user action
      attachByUrl: async (url) => {
        try {
          const res = await client.attachSessionPullRequest({ id: idRef.current, url }); // abort-signal-exempt: one-shot user action
          setLocal(res.pullRequests);
          if (res.result === AttachPullRequestResult.ALREADY_TRACKED) {
            return { ok: true, notice: `This session is already tracking ${res.pr ? prTarget(res.pr) : "that pull request"}.` };
          }
          return OK;
        } catch (err) {
          return classifyPrError(err);
        }
      },
      askToFix: async (pr) => {
        try {
          const res = await client.askSessionToFixPullRequest({ id: idRef.current, pr: ref(pr) }); // abort-signal-exempt: one-shot user action
          const { text } = outcomeMessage(res.outcome, res.detail, titleRef.current);
          const delivered = res.outcome === NudgeOutcome.DELIVERED || res.outcome === NudgeOutcome.DUPLICATE || res.outcome === NudgeOutcome.NOTHING_TO_FIX;
          return delivered ? { ok: true, notice: `${text} (${prTarget(pr)}).` } : failure("other", text);
        } catch (err) {
          return classifyPrError(err);
        }
      },
    };
  }, [client, withList]);

  return { pullRequests: local ?? session.pullRequests, holds: session.prHolds, actions };
}
