export interface ExternalRef {
  /** "owner/repo" */
  repo: string;
  number: string;
}

const ISSUE_OR_PULL_PATH = /^\/([^/]+)\/([^/]+)\/(?:issues|pull)\/(\d+)\/?$/;

/**
 * Parses a GitHub (or GHE) issue/PR URL into its repo and number. Returns
 * undefined for anything that does not have the `/<owner>/<repo>/(issues|pull)/<n>`
 * shape so callers can fall back to the bare external id.
 */
export function parseExternalRef(externalUrl: string | undefined): ExternalRef | undefined {
  if (!externalUrl) return undefined;
  let url: URL;
  try {
    url = new URL(externalUrl);
  } catch {
    return undefined;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return undefined;
  const match = ISSUE_OR_PULL_PATH.exec(url.pathname);
  if (!match) return undefined;
  return { repo: `${match[1]}/${match[2]}`, number: match[3] };
}
