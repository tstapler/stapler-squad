import type { LinkedSession } from "@/lib/hooks/useBacklogService";
import { classifySessionKind } from "./sessionKind";

/**
 * A session the "Jump to session" panel can open a live terminal for: a Real
 * Session (classifySessionKind "work"/"review", per sessionKind.ts) that
 * isn't a Jules cloud session — jules_work has no PTY/Instance to attach to
 * even though classifySessionKind's role-agnostic fallback would otherwise
 * call it "work".
 */
export function isJumpableSession(session: Pick<LinkedSession, "role" | "sessionId">): boolean {
  if (session.role === "jules_work") return false;
  const kind = classifySessionKind(session);
  return kind === "work" || kind === "review";
}

/** The single most recent jumpable session across all roles, or undefined if none exists. */
export function getMostRecentJumpableSession(linkedSessions: LinkedSession[]): LinkedSession | undefined {
  return [...linkedSessions].reverse().find(isJumpableSession);
}

/** The most recent jumpable session for one specific role, or undefined if that role has none. */
export function getMostRecentJumpableSessionForRole(
  linkedSessions: LinkedSession[],
  role: string
): LinkedSession | undefined {
  return [...linkedSessions].reverse().find((s) => s.role === role && isJumpableSession(s));
}

/**
 * One entry per distinct role that has at least one jumpable session, each
 * paired with that role's own most recent session, ordered most-recently-
 * active role first — the role picker's menu contents.
 */
export function listJumpableRoleOptions(linkedSessions: LinkedSession[]): LinkedSession[] {
  const seenRoles = new Set<string>();
  const options: LinkedSession[] = [];
  for (const s of [...linkedSessions].reverse()) {
    if (!isJumpableSession(s) || seenRoles.has(s.role)) continue;
    seenRoles.add(s.role);
    options.push(s);
  }
  return options;
}
