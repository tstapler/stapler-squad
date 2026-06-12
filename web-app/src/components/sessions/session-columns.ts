export type ColumnKey =
  | "agent"
  | "memory"
  | "elapsed"
  | "diff"
  | "branch";

export interface ColumnDef {
  key: ColumnKey;
  label: string;
  /** Width in the CSS grid template. */
  gridWidth: string;
  defaultVisible: boolean;
}

export const COLUMN_DEFS: ColumnDef[] = [
  { key: "agent",   label: "Agent",       gridWidth: "20px", defaultVisible: true  },
  { key: "memory",  label: "Memory",      gridWidth: "auto", defaultVisible: true  },
  { key: "elapsed", label: "Last active", gridWidth: "32px", defaultVisible: true  },
  { key: "diff",    label: "Diff",        gridWidth: "auto", defaultVisible: false },
  { key: "branch",  label: "Branch",      gridWidth: "auto", defaultVisible: false },
];

export const DEFAULT_VISIBLE_COLUMNS: ColumnKey[] = COLUMN_DEFS
  .filter((c) => c.defaultVisible)
  .map((c) => c.key);

/** Build a CSS gridTemplateColumns value from the current visible set. */
export function buildRowGridTemplate(visible: ColumnKey[]): string {
  const cols = ["8px", "1fr"]; // dot + name always present
  for (const def of COLUMN_DEFS) {
    if (visible.includes(def.key)) cols.push(def.gridWidth);
  }
  cols.push("auto"); // actions always present
  return cols.join(" ");
}
