import { modeReducer, OmnibarModeState } from "./useModeReducer";
import { InputType, DetectionResult } from "../types";

const discoveryState: OmnibarModeState = { type: "discovery" };

function makeDetection(type: InputType): DetectionResult {
  return { type, confidence: 1, parsedValue: "test", suggestedName: "test" };
}

describe("modeReducer", () => {
  describe("reset_to_discovery", () => {
    it("modeReducer_should_returnDiscovery_When_resetToDiscovery", () => {
      const state: OmnibarModeState = { type: "creation" };
      const result = modeReducer(state, { kind: "reset_to_discovery" });
      expect(result).toEqual({ type: "discovery" });
    });
  });

  describe("detect", () => {
    it("modeReducer_should_returnCreation_When_detectLocalPath", () => {
      const detection = makeDetection(InputType.LocalPath);
      const result = modeReducer(discoveryState, { kind: "detect", detection });
      expect(result).toEqual({ type: "creation", detection });
    });

    it("modeReducer_should_notTransition_When_unknownDetectionType", () => {
      const detection = makeDetection(InputType.Unknown);
      const result = modeReducer(discoveryState, { kind: "detect", detection });
      expect(result).toEqual({ type: "discovery" });
    });

    it("modeReducer_should_returnCreation_When_detectGitHubPR", () => {
      const detection = makeDetection(InputType.GitHubPR);
      const result = modeReducer(discoveryState, { kind: "detect", detection });
      expect(result).toEqual({ type: "creation", detection });
    });

    it("modeReducer_should_returnCreation_When_detectAlias", () => {
      const detection = makeDetection(InputType.Alias);
      const result = modeReducer(discoveryState, { kind: "detect", detection });
      expect(result).toEqual({ type: "creation", detection });
    });
  });

  describe("open_creation_direct", () => {
    it("modeReducer_should_transitionToCreation_When_openCreationDirect", () => {
      const result = modeReducer(discoveryState, { kind: "open_creation_direct" });
      expect(result).toEqual({ type: "creation" });
    });
  });

  describe("new_prefix_typed", () => {
    it("modeReducer_should_transitionToCreationWithRepo_When_newPrefixTyped", () => {
      const result = modeReducer(discoveryState, { kind: "new_prefix_typed", query: "my-project" });
      expect(result).toEqual({ type: "creation_with_repo", path: "my-project" });
    });

    it("modeReducer_should_useEmptyPath_When_newPrefixTypedWithNoQuery", () => {
      const result = modeReducer(discoveryState, { kind: "new_prefix_typed" });
      expect(result).toEqual({ type: "creation_with_repo", path: "" });
    });
  });

  describe("select_repo", () => {
    it("modeReducer_should_transitionToCreationWithRepo_When_selectRepo", () => {
      const result = modeReducer(discoveryState, { kind: "select_repo", path: "/home/user/repo" });
      expect(result).toEqual({ type: "creation_with_repo", path: "/home/user/repo" });
    });
  });
});
