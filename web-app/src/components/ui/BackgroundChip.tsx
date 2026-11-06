import { chip } from "./BackgroundChip.css";

/** Text chip marking a notification whose session is hidden (Story 5.3, UX C6): text, not color alone. */
export function BackgroundChip() {
  return (
    <span className={chip} data-testid="notification-background-chip">
      Background
    </span>
  );
}
