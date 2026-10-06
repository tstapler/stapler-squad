// +feature: launcher-presets-detector
/**
 * PresetDetector detects preset:<id> shorthand, resolving directly to a configured
 * launcher preset. Priority 37 — after AliasDetector (36), before GitHubShorthandDetector (40).
 *
 * Simpler than AliasDetector by design (no browse mode, no label/flags suffix): presets are
 * authored once in a hand-edited file, not typed conversationally as often as @alias names.
 *
 * IMPORTANT: PresetDetector must claim ALL input starting with "preset:" and never return null
 * for such input (once an id is present), so SessionSearchDetector never claims it.
 */

import { Detector } from "../detector";
import { DetectionResult, InputType } from "../types";
import type { LauncherPresetEntry } from "../../hooks/useLauncherPresets";

/** Shape of `DetectionResult.metadata` when `type === InputType.Preset`. */
export interface PresetMetadata {
  preset: LauncherPresetEntry;
}

/** Shape of `DetectionResult.metadata` when `type === InputType.PresetNotFound`. */
export interface PresetNotFoundMetadata {
  typedId: string;
}

export class PresetDetector implements Detector {
  name = "PresetDetector";
  priority = 37;

  private presets: LauncherPresetEntry[];

  constructor(presets: LauncherPresetEntry[]) {
    this.presets = presets;
  }

  detect(input: string): DetectionResult | null {
    if (!input.startsWith("preset:")) return null;

    const id = input.slice("preset:".length).trim();
    if (!id) return null;

    // Case-sensitive: presets are authored once in a file the user controls, not typed
    // conversationally as often as @alias names (unlike FindAlias's case-insensitive match).
    const found = this.presets.find((p) => p.id === id);

    if (!found) {
      return {
        type: InputType.PresetNotFound,
        confidence: 1.0,
        parsedValue: input,
        suggestedName: id,
        metadata: { typedId: input },
      };
    }

    return {
      type: InputType.Preset,
      confidence: 1.0,
      parsedValue: input,
      suggestedName: found.label,
      metadata: { preset: found },
    };
  }
}
