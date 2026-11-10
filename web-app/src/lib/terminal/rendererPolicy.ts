export type RendererKind = "xterm" | "ghostty";

export interface RendererInputs {
  ghosttyFlag: boolean;
  mobileLayerFlag: boolean;
  /** `shouldAttachMobileInput()`. */
  attachable: boolean;
}

/** Ghostty has no soft-keyboard input path, so it is never chosen where the layer would attach but is switched off. */
export function resolveRenderer({ ghosttyFlag, mobileLayerFlag, attachable }: RendererInputs): RendererKind {
  if (!ghosttyFlag) return "xterm";
  if (attachable && !mobileLayerFlag) return "xterm";
  return "ghostty";
}
