export const UP_NEXT_TABS = ["prs", "stuck", "worktrees", "queue"] as const;
export type UpNextTab = (typeof UP_NEXT_TABS)[number];

export const UP_NEXT_TAB_STORAGE_KEY = "up-next-tab";

export function parseUpNextTab(raw: string | null | undefined): UpNextTab | null {
  return (UP_NEXT_TABS as readonly string[]).includes(raw ?? "") ? (raw as UpNextTab) : null;
}

export interface UpNextTabParams {
  item?: string | null;
  tab?: string | null;
}

/** Tab implied by the URL alone (`?item=` means Stuck), or null when the URL has no opinion. */
export function tabFromUrl({ item, tab }: UpNextTabParams): UpNextTab | null {
  if (item) return "stuck";
  return parseUpNextTab(tab);
}

/** Never throws: storage can be blocked (SecurityError) or unavailable (SSR). */
export function readStoredTab(): UpNextTab | null {
  try {
    return parseUpNextTab(window.localStorage.getItem(UP_NEXT_TAB_STORAGE_KEY));
  } catch {
    return null;
  }
}

export function writeStoredTab(tab: UpNextTab): void {
  try {
    window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, tab);
  } catch {
    // Storage unavailable: tab switching still works via the URL.
  }
}

/** Precedence: ?item= (Stuck) > valid ?tab= > stored tab > "prs". */
export function resolveInitialTab(params: UpNextTabParams, stored: string | null): UpNextTab {
  return tabFromUrl(params) ?? parseUpNextTab(stored) ?? "prs";
}
