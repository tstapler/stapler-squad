import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { CreateSessionRequest } from "@/gen/session/v1/session_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";

// Fan-out group membership is carried as session tags (see server/services/session_service_fanout.go).
export const FANOUT_GROUP_TAG_PREFIX = "fanout:";
export const FANOUT_VARIANT_TAG_PREFIX = "fanout-variant:";
export const MAX_FANOUT_VARIANTS = 5;

export function fanoutGroupIdOf(session: Pick<Session, "tags">): string | undefined {
  const tag = session.tags.find((t) => t.startsWith(FANOUT_GROUP_TAG_PREFIX));
  return tag?.slice(FANOUT_GROUP_TAG_PREFIX.length);
}

export function fanoutVariantOf(session: Pick<Session, "tags">): number {
  const tag = session.tags.find((t) => t.startsWith(FANOUT_VARIANT_TAG_PREFIX));
  return tag ? Number(tag.slice(FANOUT_VARIANT_TAG_PREFIX.length)) : 0;
}

export function fanoutComparisonPath(groupId: string): string {
  // Query param rather than /fanout/:groupId — the app is a static export, which cannot
  // pre-render dynamic path segments (same constraint as /sessions/summary?sessionId=).
  return `/fanout?groupId=${encodeURIComponent(groupId)}`;
}

function client() {
  return createClient(SessionService, getConnectTransport());
}

export async function fanOutSession(
  base: Partial<CreateSessionRequest>,
  count: number,
  programOverrides: string[] = [],
): Promise<{ groupId: string; sessions: Session[] }> {
  const res = await client().fanOutSession({ base, count, programOverrides });
  return { groupId: res.groupId, sessions: res.sessions };
}

export async function listFanoutMembers(groupId: string): Promise<Session[]> {
  const res = await client().listSessions({ includeArchived: true });
  return res.sessions
    .filter((s) => fanoutGroupIdOf(s) === groupId)
    .sort((a, b) => fanoutVariantOf(a) - fanoutVariantOf(b));
}

export async function pickFanoutWinner(groupId: string, winnerSessionId: string) {
  return client().pickFanoutWinner({ groupId, winnerSessionId });
}
