import * as styles from "./JumpToLatestButton.css";

interface JumpToLatestButtonProps {
  newLineCount: number;
  onClick: () => void;
}

/**
 * JumpToLatestButton — fixed-position pill shown when the user has scrolled
 * away from the bottom during live tail. Shows queued new-line count.
 */
export function JumpToLatestButton({ newLineCount, onClick }: JumpToLatestButtonProps) {
  if (newLineCount === 0) return null;

  const label = `Jump to latest log entry, ${newLineCount} new line${newLineCount !== 1 ? "s" : ""}`;

  return (
    <button
      className={styles.pill}
      onClick={onClick}
      aria-label={label}
      data-testid="jump-to-latest"
    >
      ↓ {newLineCount} new line{newLineCount !== 1 ? "s" : ""}
    </button>
  );
}
