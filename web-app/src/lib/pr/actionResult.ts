/** Why a PR action failed, reduced to what the UI words differently (ux.md S3 error table). */
export type PrFailureKind = "already_merged" | "not_attached" | "rate_limited" | "auth" | "network" | "invalid_url" | "not_found" | "other";

/** `notice` is an outcome the server reported on a success (e.g. "already tracking"); the host shows it instead of its default text. */
export type PrActionResult = { ok: true; notice?: string } | { ok: false; kind: PrFailureKind; message: string };

export const OK: PrActionResult = { ok: true };

export function failure(kind: PrFailureKind, message: string): PrActionResult {
  return { ok: false, kind, message };
}

/** The per-PR operations the list drives. Every one carries the row's own ref (never "the active PR"). */
export interface PullRequestActions {
  refresh: () => Promise<PrActionResult>;
  merge: (pr: PrTarget, method: "merge" | "squash" | "rebase") => Promise<PrActionResult>;
  close: (pr: PrTarget) => Promise<PrActionResult>;
  /** Absent when the surface cannot steer the session; the menu item is then not offered. A success carries the server's outcome as `notice`. */
  askToFix?: (pr: PrTarget) => Promise<PrActionResult>;
  /** `null` clears the pin. */
  pin: (pr: PrTarget | null) => Promise<PrActionResult>;
  detach: (pr: PrTarget) => Promise<PrActionResult>;
  attachByUrl: (url: string) => Promise<PrActionResult>;
}

export interface PrTarget {
  host: string;
  owner: string;
  repo: string;
  number: number;
}
