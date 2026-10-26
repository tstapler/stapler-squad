import type { WindowId } from "./windowTypes";

/** DOM ids linking a window's tab to its `tabpanel` (aria-controls / aria-labelledby). */
export const windowTabDomId = (id: WindowId) => `window-tab-${id}`;
export const windowPanelDomId = (id: WindowId) => `window-panel-${id}`;
