"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";
import {
  GitHubUserService,
  NudgeSessionForPRRequestSchema,
  PRKeySchema,
  type NudgeOutcome,
  type NudgeSessionForPRRequest,
  type NudgeSessionForPRResponse,
} from "@/gen/session/v1/github_user_pb";
import type { UserPR } from "@/gen/session/v1/types_pb";

/** Message and "Requested just now" lifetime; matches the server's duplicate window. */
export const NUDGE_STATE_TTL_MS = 60_000;

export interface NudgeClient {
  nudgeSessionForPR(req: NudgeSessionForPRRequest): Promise<NudgeSessionForPRResponse>;
}

export type NudgeErrorKind = "rate_limit" | "failed_precondition" | "network";

export type NudgeState =
  | { status: "idle" }
  | { status: "pending"; sessionId: string }
  | { status: "done"; sessionId: string; outcome: NudgeOutcome; detail: string }
  | { status: "error"; sessionId: string; kind: NudgeErrorKind; message: string };

const IDLE: NudgeState = { status: "idle" };

function toErrorState(sessionId: string, err: unknown): NudgeState {
  const e = ConnectError.from(err);
  const message = e.rawMessage || e.message;
  if (e.code === Code.ResourceExhausted) return { status: "error", sessionId, kind: "rate_limit", message };
  if (e.code === Code.FailedPrecondition) return { status: "error", sessionId, kind: "failed_precondition", message };
  return { status: "error", sessionId, kind: "network", message: `Could not send request: ${message}` };
}

export interface UseNudgePROptions {
  /** Injected in tests; defaults to the shared Connect transport, created on first use. */
  client?: NudgeClient;
}

/**
 * One card's nudge lifecycle. State lives in component memory only (the server's duplicate
 * answer is the source of truth after a reload). A `done` result clears after 60 s; errors
 * stay until the next action.
 */
export function useNudgePR({ client }: UseNudgePROptions = {}) {
  const [state, setState] = useState<NudgeState>(IDLE);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clientRef = useRef(client);
  clientRef.current = client;

  const clearTimer = () => {
    if (timerRef.current !== null) clearTimeout(timerRef.current);
    timerRef.current = null;
  };
  useEffect(() => clearTimer, []);

  const dismiss = useCallback(() => {
    clearTimer();
    setState(IDLE);
  }, []);

  const nudge = useCallback(async (pr: UserPR, sessionId: string): Promise<NudgeState> => {
    clearTimer();
    setState({ status: "pending", sessionId });
    const c = clientRef.current ?? (createClient(GitHubUserService, getConnectTransport()) as NudgeClient);
    const req = create(NudgeSessionForPRRequestSchema, {
      pr: create(PRKeySchema, { host: pr.host || "github.com", owner: pr.owner, repo: pr.repo, number: pr.number }),
      sessionId,
    });
    try {
      const res = await c.nudgeSessionForPR(req);
      const done: NudgeState = { status: "done", sessionId, outcome: res.outcome, detail: res.detail };
      setState(done);
      timerRef.current = setTimeout(() => setState(IDLE), NUDGE_STATE_TTL_MS);
      return done;
    } catch (err) {
      const failed = toErrorState(sessionId, err);
      setState(failed);
      return failed;
    }
  }, []);

  return { state, nudge, dismiss };
}
