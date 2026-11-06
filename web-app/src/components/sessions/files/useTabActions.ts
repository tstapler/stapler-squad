import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { baseName } from "./tabLabels";

/** Tab commands shared by the desktop strip and the mobile sheet. */
export function useTabActions(v2: FilesV2 | undefined) {
  return {
    closeOthers: (keep: string) => v2?.buffers.state.buffers.forEach((b) => { if (b.path !== keep) v2.buffers.close(b.path); }),
    copyPath: (path: string) => void navigator.clipboard?.writeText(path),
    reopen: () => {
      if (!v2) return;
      const top = v2.buffers.state.closed[v2.buffers.state.closed.length - 1];
      v2.buffers.reopenClosed();
      if (top) v2.feedback.announceSuccess("reopened", baseName(top.path));
    },
  };
}
