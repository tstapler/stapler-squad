"use client";

import { useEffect, useMemo, useState } from "react";
import type { BannerKind } from "@/lib/files/bannerPriority";
import type { WindowedFile, WindowedState } from "@/lib/files/windowedFile";
import type { LineRange } from "./engine/EditorEngine";
import { FALLBACK_CHIP_TEXT } from "@/lib/files/transport/router";
import { BannerStack, type BannerSpec } from "./BannerStack";
import { usePendingDelay } from "./PendingState";
import { chip, states } from "./LargeFileStates.css";
import { visuallyHidden } from "@/styles/a11y.css";

/** A loading burst is announced politely once, and only if it outlasts this (ux.md 8.12). */
export const ANNOUNCE_AFTER_MS = 500;
export const INDEXING_TEXT = "Indexing file...";

export const rangeText = (r: LineRange) => `${r.startLine.toLocaleString("en-US")}-${r.endLine.toLocaleString("en-US")}`;
const span = (ranges: readonly LineRange[]): LineRange => ({
  startLine: Math.min(...ranges.map((r) => r.startLine)),
  endLine: Math.max(...ranges.map((r) => r.endLine)),
});

/** Quiet chip for a demoted transport: no dialog, no toast. */
export function FallbackChip() {
  return <span className={chip} role="status" data-testid="fallback-chip">{FALLBACK_CHIP_TEXT}</span>;
}

/** The "Indexing file..." chip shown while the server builds the line index. */
export function IndexingChip() {
  return <span className={chip} role="status" data-testid="indexing-chip"><span aria-hidden="true">&#8987;</span>{INDEXING_TEXT}</span>;
}

/** Text for the polite live region: "Loading lines A-B..." after 500 ms (once per burst), failures at once. */
export function useLoadingAnnouncement(state: Pick<WindowedState, "loading" | "failed">): string {
  const busy = state.loading.length > 0;
  const [burst, setBurst] = useState<string | null>(null);
  const loadingText = busy ? `Loading lines ${rangeText(span(state.loading))}...` : "";
  useEffect(() => {
    if (!busy) { setBurst(null); return; }
    const id = setTimeout(() => setBurst((b) => b ?? loadingText), ANNOUNCE_AFTER_MS);
    return () => clearTimeout(id);
    // The text of the first range seen in a burst is the one announced; later ranges in it stay quiet.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [busy]);
  const failed = state.failed.length > 0 ? `Couldn't load lines ${rangeText(span(state.failed))}` : "";
  return failed || burst || "";
}

interface Props {
  file: WindowedFile;
  state: WindowedState;
}

type Spec = Omit<BannerSpec, "seq">;

function specs(file: WindowedFile, state: WindowedState, jumpVisible: boolean): Spec[] {
  const out: Spec[] = [];
  const add = (kind: BannerKind, id: string, body: string, actions?: BannerSpec["actions"]) => out.push({ id, kind, body, actions, testId: `large-file-${id}` });
  for (const range of state.failed) {
    add("load_failed", `failed-${range.startLine}`, `Couldn't load lines ${rangeText(range)}.`, [{ label: "Retry", onClick: () => file.retry(range) }]);
  }
  if (state.stale) add("load_failed", "stale", "The file changed on disk. Use Reload from disk to read the new version at this line.");
  if (state.jump !== null && jumpVisible) {
    const body = state.indexing ? `Going to line ${state.jump.toLocaleString("en-US")}... waiting for the file index` : `Going to line ${state.jump.toLocaleString("en-US")}...`;
    add("jump_progress", "jump", body, [{ label: "Cancel", onClick: () => file.cancelJump() }]);
  }
  if (state.notice) add("informational", "notice", state.notice);
  return out;
}

/** Large-file banners (load failure with Retry, far-jump progress with Cancel, clamp notice) and the polite live region. */
export function LargeFileStates({ file, state }: Props) {
  const jumpVisible = usePendingDelay(state.jump !== null);
  const announcement = useLoadingAnnouncement(state);
  const banners = useMemo(() => specs(file, state, jumpVisible).map((b, i) => ({ ...b, seq: i })), [file, state, jumpVisible]);
  return (
    <div className={states}>
      <BannerStack banners={banners} />
      <div className={visuallyHidden} role="status" aria-live="polite" data-testid="large-file-announce">{announcement}</div>
    </div>
  );
}
