export type FilesLayout = "mobile" | "desktop";

export const MOBILE_CONTAINER_MAX = 768;
export const PHONE_SHORT_SIDE_MAX = 600;

export interface LayoutInput {
  /** Files-tab container width in CSS px. */
  containerWidth: number;
  /** `(pointer: coarse)` matches. */
  pointerCoarse: boolean;
  /** `min(screen.width, screen.height)`; `screen`, not the visual viewport, so the soft keyboard and rotation cannot flip it. */
  screenShortSide: number;
}

/** ux.md section 0: mobile for a narrow container or a phone-class touch device, else desktop. */
export function chooseFilesLayout({ containerWidth, pointerCoarse, screenShortSide }: LayoutInput): FilesLayout {
  if (containerWidth < MOBILE_CONTAINER_MAX) return "mobile";
  return pointerCoarse && screenShortSide < PHONE_SHORT_SIDE_MAX ? "mobile" : "desktop";
}
