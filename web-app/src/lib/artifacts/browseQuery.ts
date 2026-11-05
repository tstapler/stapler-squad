/** URL <-> request mapping and the search-box rule for the /artifacts page (design/ux.md S8, S9). */

export const MIN_QUERY_CHARS = 2;

export const ARTIFACT_TYPE_OPTIONS = [
  { value: "pr", label: "Pull request" },
  { value: "commit", label: "Commit" },
  { value: "branch", label: "Branch" },
  { value: "issue", label: "Issue" },
  { value: "plan_doc", label: "Plan doc" },
  { value: "ci_link", label: "CI link" },
  { value: "command", label: "Command" },
] as const;

export const PR_STATE_OPTIONS = [
  { value: "open", label: "Open" },
  { value: "merged", label: "Merged" },
  { value: "closed", label: "Closed" },
  { value: "unknown", label: "Unknown state" },
] as const;

export interface BrowseParams {
  q: string;
  type: string;
  repo: string;
  state: string;
  /** Raw `owner/repo#12` (or PR URL) as it appears in the URL. */
  pr: string;
  /** "all" turns off the single-match shortcut so a View all action can show the list. */
  view: string;
}

export const EMPTY_PARAMS: BrowseParams = { q: "", type: "", repo: "", state: "", pr: "", view: "" };

type ParamReader = { get(name: string): string | null };

export function parseBrowseParams(params: ParamReader): BrowseParams {
  const read = (k: keyof BrowseParams) => (params.get(k) ?? "").trim();
  const type = read("type");
  const state = read("state");
  return {
    q: read("q"),
    type: ARTIFACT_TYPE_OPTIONS.some((o) => o.value === type) ? type : "",
    repo: read("repo"),
    state: PR_STATE_OPTIONS.some((o) => o.value === state) ? state : "",
    pr: read("pr"),
    view: read("view"),
  };
}

export function toSearch(p: BrowseParams): string {
  const sp = new URLSearchParams();
  (["type", "repo", "state", "pr", "q", "view"] as const).forEach((k) => {
    if (p[k]) sp.set(k, p[k]);
  });
  return sp.toString();
}

export function activeFilterCount(p: BrowseParams): number {
  return [p.type, p.repo, p.state].filter(Boolean).length;
}

export type QueryCheck =
  | { kind: "empty" }
  | { kind: "too_short"; message: string }
  | { kind: "ok"; shaHint: boolean };

export const TOO_SHORT_MESSAGE = "Enter at least 2 characters";
export const SHA_HINT = "Looks like a commit SHA. Use at least 7 characters to match a commit.";

/** 2+ characters run the search; 1 is rejected inline; a 2 to 6 character all-hex query also gets the SHA hint. */
export function checkQuery(raw: string): QueryCheck {
  const q = raw.trim();
  const length = [...q].length;
  if (length === 0) return { kind: "empty" };
  if (length < MIN_QUERY_CHARS) return { kind: "too_short", message: TOO_SHORT_MESSAGE };
  return { kind: "ok", shaHint: length < 7 && /^[0-9a-f]+$/i.test(q) };
}

export interface PrTarget {
  host: string;
  owner: string;
  repo: string;
  number: number;
}

const PR_REF = /^([\w.-]+)\/([\w.-]+)#(\d+)$/;
const PR_URL = /^https?:\/\/([^/\s]+)\/([\w.-]+)\/([\w.-]+)\/pull\/(\d+)\/?$/;

/** Parses the `pr` filter; null when it is neither `owner/repo#N` nor a PR URL. */
export function parsePrParam(raw: string): PrTarget | null {
  const ref = PR_REF.exec(raw);
  if (ref) return { host: "", owner: ref[1], repo: ref[2], number: Number(ref[3]) };
  const url = PR_URL.exec(raw);
  if (url) return { host: url[1] === "github.com" ? "" : url[1], owner: url[2], repo: url[3], number: Number(url[4]) };
  return null;
}

export function prLabelOf(t: PrTarget): string {
  return `${t.owner}/${t.repo}#${t.number}`;
}

export type BrowseMode = "browse" | "search" | "pr";

/** pr alone is the exact lookup; q (with or without pr) is a search; otherwise browse. */
export function modeOf(p: BrowseParams): BrowseMode {
  if (p.q) return "search";
  return p.pr ? "pr" : "browse";
}
