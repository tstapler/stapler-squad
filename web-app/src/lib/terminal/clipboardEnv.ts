import { copyToClipboard } from "@/lib/clipboard";
import type { ClipboardEnv } from "./terminalClipboard";

/** Browser clipboard for terminal actions; `report` is how each renderer surfaces the outcome (its toast). */
export function browserClipboardEnv(report: (ok: boolean) => void): ClipboardEnv {
  return {
    copy: copyToClipboard,
    readText: () => (navigator.clipboard?.readText ? navigator.clipboard.readText() : Promise.reject(new Error("clipboard read unavailable"))),
    report,
  };
}
