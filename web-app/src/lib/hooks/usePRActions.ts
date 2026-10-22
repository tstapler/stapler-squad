"use client";

import { useCallback, useRef, useState } from "react";
import { ConnectError, createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";
import {
  ClosePRDirectRequestSchema,
  GitHubUserService,
  MergePRDirectRequestSchema,
  PRDraftToggleRequestSchema,
  type ClosePRDirectRequest,
  type MergePRDirectRequest,
  type PRActionResponse,
  type PRDraftToggleRequest,
} from "@/gen/session/v1/github_user_pb";
import type { UserPR } from "@/gen/session/v1/types_pb";

export type MergeMethod = "merge" | "squash" | "rebase";
export type PRActionName = "merge" | "close" | "toDraft" | "toReady";

export interface PRActionClient {
  mergePRDirect(req: MergePRDirectRequest): Promise<PRActionResponse>;
  closePRDirect(req: ClosePRDirectRequest): Promise<PRActionResponse>;
  convertPRToDraft(req: PRDraftToggleRequest): Promise<PRActionResponse>;
  markPRReady(req: PRDraftToggleRequest): Promise<PRActionResponse>;
}

const target = (pr: UserPR) => ({
  owner: pr.owner,
  repo: pr.repo,
  number: pr.number,
  host: pr.host || "github.com",
});

/**
 * One card's PR actions. `inflight` names the running action so every button can disable
 * itself; `run` resolves true on success and sets `error` (cleared on the next run) otherwise.
 */
export function usePRActions(client?: PRActionClient) {
  const [inflight, setInflight] = useState<PRActionName | null>(null);
  const [error, setError] = useState<string | null>(null);
  const clientRef = useRef(client);
  clientRef.current = client;

  const run = useCallback(
    async (name: PRActionName, pr: UserPR, method: MergeMethod = "merge"): Promise<boolean> => {
      setError(null);
      setInflight(name);
      const c = clientRef.current ?? (createClient(GitHubUserService, getConnectTransport()) as PRActionClient);
      try {
        switch (name) {
          case "merge":
            await c.mergePRDirect(create(MergePRDirectRequestSchema, { ...target(pr), method }));
            break;
          case "close":
            await c.closePRDirect(create(ClosePRDirectRequestSchema, target(pr)));
            break;
          case "toDraft":
            await c.convertPRToDraft(create(PRDraftToggleRequestSchema, target(pr)));
            break;
          case "toReady":
            await c.markPRReady(create(PRDraftToggleRequestSchema, target(pr)));
            break;
        }
        return true;
      } catch (err) {
        const e = ConnectError.from(err);
        setError(e.rawMessage || e.message);
        return false;
      } finally {
        setInflight(null);
      }
    },
    [],
  );

  return { inflight, error, run };
}
