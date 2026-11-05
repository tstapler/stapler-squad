"use client";

import { useContext } from "react";
import type { UserPR } from "@/gen/session/v1/types_pb";
import { SessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useSessionsForPullRequest } from "@/lib/hooks/useSessionsForPullRequest";
import { PRSessionChip } from "./PRSessionChip";

function Chip({ pr }: { pr: UserPR }) {
  return <PRSessionChip sessions={useSessionsForPullRequest(pr)} />;
}

/** The Up Next card's "session: name" chip; mounts nothing, not even the flag read, outside the sessions store. */
export function PRTrackingSessionChip({ pr }: { pr: UserPR }) {
  return useContext(SessionServiceContext) ? <Chip pr={pr} /> : null;
}
