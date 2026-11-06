export interface ChangelogEntry {
  scope?: string;
  text: string;
  prs?: number[];
  commits?: string[];
}

export interface ChangelogSection {
  title: string;
  entries: ChangelogEntry[];
}

export interface ChangelogRelease {
  version: string;
  date?: string;
  compare_url?: string;
  sections: ChangelogSection[];
}

export interface ChangelogResponse {
  current_version: string;
  branch?: string;
  commit?: string;
  is_source_build: boolean;
  releases: ChangelogRelease[];
}

export interface UpdateCheckResponse {
  enabled: boolean;
  current_version: string;
  latest_version?: string;
  latest_url?: string;
  is_outdated: boolean;
  is_source_build: boolean;
  last_checked_at?: string;
  error?: string;
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { signal });
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return (await res.json()) as T;
}

export const fetchChangelog = (signal?: AbortSignal) =>
  getJSON<ChangelogResponse>("/api/changelog", signal);

export const fetchUpdateCheck = (signal?: AbortSignal) =>
  getJSON<UpdateCheckResponse>("/api/update-check", signal);
