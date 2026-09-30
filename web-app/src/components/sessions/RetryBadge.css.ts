import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "4px",
  padding: "4px 12px",
  borderRadius: "12px",
  fontSize: "12px",
  fontWeight: 600,
  whiteSpace: "nowrap",
});

// Attempt < max: healthy headroom remains, neutral tone matching the plain
// idle status-badge palette — not the warning/error palettes, which are
// reserved for the final attempt and PermanentlyFailed respectively.
export const neutral = style({
  background: vars.statusBadge.idleBg,
  color: vars.statusBadge.idleFg,
  border: `1px solid ${vars.statusBadge.idleBorder}`,
});

// Final attempt: the next failure exhausts the budget — warning tokens, not
// error, since the session is still actively retrying (not yet given up).
export const warning = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});
