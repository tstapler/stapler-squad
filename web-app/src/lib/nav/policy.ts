/** Move focus to the layout `main` when the pathname changes (UX D5). Flip off to fall back to Next's behaviour of leaving focus alone. */
export const FOCUS_ON_NAVIGATE = true;

/** A route that is still pending after this long shows its skeleton; faster loads never flash one. */
export const PENDING_DELAY_MS = 150;

/** Redirect chains settle inside this window, so assistive tech hears only the final title. */
export const ANNOUNCE_DELAY_MS = 100;

/** The announcement is cleared afterwards so a later identical title is a real text change. */
export const ANNOUNCE_CLEAR_MS = 1000;
