import { lineNumbers } from "@codemirror/view";
import { absLineOfDocLine } from "./gapTable";

/** Line numbers read through the gap table, so line 401 after a loaded window shows 401. */
export const absoluteLineNumbers = () =>
  lineNumbers({ formatNumber: (n, state) => String(absLineOfDocLine(state, n)) });
