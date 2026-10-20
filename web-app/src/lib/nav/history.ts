// The only place outside an impl that touches raw History; every impl shares it so foreign state survives the same way.

const TRAY_MARKER = "ssqTray";

export function replaceSearchInHistory(search: string): void {
  const query = search === "" || search.startsWith("?") ? search : `?${search}`;
  window.history.replaceState(window.history.state, "", query === "" ? window.location.pathname : query);
}

export function replaceHashInHistory(hash: string): void {
  const url = hash
    ? `${window.location.pathname}${window.location.search}${hash.startsWith("#") ? hash : `#${hash}`}`
    : window.location.pathname + window.location.search;
  window.history.replaceState(window.history.state, "", url);
}

export function pushHistoryMarkerInHistory(key: string, value: unknown, replace = false): void {
  const state = { ...(window.history.state ?? {}), [key]: value };
  if (replace) window.history.replaceState(state, "");
  else window.history.pushState(state, "");
}

export function pushTrayEntryInHistory(replace = false): void {
  pushHistoryMarkerInHistory(TRAY_MARKER, true, replace);
}
