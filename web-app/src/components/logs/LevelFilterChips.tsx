import { chipRow, chip } from "./LevelFilterChips.css";

const LEVELS = ["ALL", "ERROR", "WARN", "INFO", "DEBUG"] as const;

interface LevelFilterChipsProps {
  active: string[];
  onChange: (levels: string[]) => void;
}

export function LevelFilterChips({ active, onChange }: LevelFilterChipsProps) {
  const handleClick = (level: string) => {
    if (level === "ALL") {
      onChange(["ALL"]);
    } else {
      const next = active.includes("ALL")
        ? [level]
        : active.includes(level)
          ? active.filter((l) => l !== level)
          : [...active, level];
      onChange(next.length === 0 ? ["ALL"] : next);
    }
  };

  return (
    <div className={chipRow} role="group" aria-label="Filter by log level">
      {LEVELS.map((level) => (
        <button
          key={level}
          className={chip({ level, isActive: active.includes(level) })}
          aria-pressed={active.includes(level)}
          onClick={() => handleClick(level)}
        >
          {level}
        </button>
      ))}
    </div>
  );
}
