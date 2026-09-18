import { PresetDetector } from "./PresetDetector";
import { InputType } from "../types";
import type { LauncherPresetEntry } from "../../hooks/useLauncherPresets";

const mockPresets: LauncherPresetEntry[] = [
  { id: "codex", label: "Codex GPT-5", argv: ["codex", "--model", "gpt-5"], program: "", defaultPath: "" },
  { id: "remote-claude", label: "Remote Claude", argv: ["ssh", "-t", "host"], program: "", defaultPath: "" },
];

describe("PresetDetector", () => {
  let detector: PresetDetector;

  beforeEach(() => {
    detector = new PresetDetector(mockPresets);
  });

  // T-UNIT-TS-201
  it("PresetDetector_should_ResolveToPreset_When_KnownIdTyped", () => {
    const result = detector.detect("preset:codex");
    expect(result?.type).toBe(InputType.Preset);
    expect(result?.metadata?.preset).toEqual(mockPresets[0]);
  });

  // T-UNIT-TS-202
  it("PresetDetector_should_ReturnNotFound_When_UnknownIdTyped", () => {
    const result = detector.detect("preset:doesnotexist");
    expect(result?.type).toBe(InputType.PresetNotFound);
    expect(result?.metadata?.typedId).toBe("preset:doesnotexist");
  });

  // T-UNIT-TS-203
  it("PresetDetector_should_ReturnNull_When_NoPresetPrefix", () => {
    expect(detector.detect("hello world")).toBeNull();
  });

  // T-PITFALL-201
  it("PresetDetector_should_ReturnNull_When_PrefixOnlyWithNoId", () => {
    expect(detector.detect("preset:")).toBeNull();
  });

  // T-PITFALL-202
  it("PresetDetector_should_BeCaseSensitive_When_IdCaseDiffers", () => {
    const result = detector.detect("preset:Codex");
    expect(result?.type).toBe(InputType.PresetNotFound);
  });
});
