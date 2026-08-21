import { useReducer } from "react";
import { InputType, DetectionResult } from "../types";

export type OmnibarModeState =
  | { type: "discovery" }
  | { type: "creation"; detection?: DetectionResult }
  | { type: "creation_with_repo"; path: string; detection?: DetectionResult };

export type ModeAction =
  | { kind: "detect"; detection: DetectionResult }
  | { kind: "select_repo"; path: string }
  | { kind: "open_creation_direct" }
  | { kind: "new_prefix_typed"; query?: string }
  | { kind: "reset_to_discovery" };

const CREATION_TYPES = new Set<InputType>([
  InputType.LocalPath,
  InputType.PathWithBranch,
  InputType.GitHubPR,
  InputType.GitHubBranch,
  InputType.GitHubRepo,
  InputType.GitHubShorthand,
  InputType.Alias,
]);

export function modeReducer(
  state: OmnibarModeState,
  action: ModeAction
): OmnibarModeState {
  switch (action.kind) {
    case "detect": {
      if (CREATION_TYPES.has(action.detection.type)) {
        return { type: "creation", detection: action.detection };
      }
      return { type: "discovery" };
    }
    case "select_repo":
      return { type: "creation_with_repo", path: action.path };
    case "open_creation_direct":
      return { type: "creation" };
    case "new_prefix_typed":
      return { type: "creation_with_repo", path: action.query ?? "" };
    case "reset_to_discovery":
      return { type: "discovery" };
  }
}

export function useModeReducer(): [OmnibarModeState, React.Dispatch<ModeAction>] {
  return useReducer(modeReducer, { type: "discovery" });
}
