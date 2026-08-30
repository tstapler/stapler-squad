/**
 * Regression test for a review-pass finding: OmnibarCreationPanel's Presets section defaulted
 * to collapsed whenever `presets.length === 0` — which is also true when the backend reports a
 * load_error (GetLauncherPresets returns zero presets alongside the error). That left a
 * malformed-config error hidden behind a collapsed section, contradicting the "fails loudly"
 * requirement. This renders the real composed OmnibarCreationPanel (not just the isolated
 * OmnibarPresetList) so the collapse-state bug is caught the way OmnibarPresetList.test.tsx's
 * isolated unit test could not.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { OmnibarCreationPanel } from "../OmnibarCreationPanel";
import type { OmnibarCreationPanelProps } from "../OmnibarCreationPanel";
import type { OmnibarFormState } from "../Omnibar";

const DEFAULT_FORM_STATE: OmnibarFormState = {
  sessionName: "test-session",
  branch: "",
  program: "claude",
  category: "",
  autoYes: false,
  useTitleAsBranch: true,
  sessionType: "new_worktree",
  existingWorktree: "",
  workingDir: "",
  parentDir: "",
  projectName: "",
  newProjectSessionType: "new_worktree",
  firstPrompt: "",
  createIfMissing: false,
  autonomousMode: false,
  autoApprove: false,
  extraArgs: [],
};

function buildProps(overrides: Partial<OmnibarCreationPanelProps> = {}): OmnibarCreationPanelProps {
  return {
    formState: DEFAULT_FORM_STATE,
    setFormField: jest.fn(),
    onSubmit: jest.fn(),
    onCancel: jest.fn(),
    worktrees: [],
    isSubmitting: false,
    canSubmit: true,
    error: null,
    showAdvanced: false,
    onToggleAdvanced: jest.fn(),
    uploadBaseUrl: "/api",
    onAttachedImagesChange: jest.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  global.fetch = jest.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
});

describe("OmnibarCreationPanel Presets section auto-expand", () => {
  // aria-expanded is asserted directly (not visual CSS visibility) because jsdom does not
  // apply real stylesheets: the collapsed/expanded classes toggle CSS-only visibility, so a
  // node stays reachable via getByTestId/toBeVisible() in jsdom regardless of collapse state
  // -- confirmed empirically: this exact bug (Presets section defaulting to collapsed whenever
  // load_error accompanies zero presets, hiding the "fails loudly" error behind a closed
  // section) still passed a toBeVisible()-based version of this test even with the bug present.
  it("OmnibarCreationPanel_should_ExpandPresetsSection_When_LoadErrorPresentAndZeroPresets", () => {
    render(
      <OmnibarCreationPanel
        {...buildProps({
          launcherPresets: [],
          launcherPresetsLoading: false,
          launcherPresetsLoadError: 'duplicate preset id "codex" (positions 1 and 2)',
        })}
      />
    );

    expect(screen.getByTestId("preset-section-header")).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByTestId("preset-config-error")).toHaveTextContent(
      'duplicate preset id "codex" (positions 1 and 2)'
    );
  });

  it("OmnibarCreationPanel_should_StayCollapsed_When_NoPresetsAndNoLoadError", () => {
    render(
      <OmnibarCreationPanel
        {...buildProps({
          launcherPresets: [],
          launcherPresetsLoading: false,
          launcherPresetsLoadError: null,
        })}
      />
    );

    // A genuinely empty, error-free state is cheap to show collapsed (design/ux.md §5.1.1).
    expect(screen.getByTestId("preset-section-header")).toHaveAttribute("aria-expanded", "false");
  });

  it("OmnibarCreationPanel_should_ExpandPresetsSection_When_PresetsLoaded", () => {
    render(
      <OmnibarCreationPanel
        {...buildProps({
          launcherPresets: [{ id: "codex", label: "Codex", argv: ["codex"], program: "", defaultPath: "" }],
          launcherPresetsLoading: false,
          launcherPresetsLoadError: null,
        })}
      />
    );

    expect(screen.getByTestId("preset-section-header")).toHaveAttribute("aria-expanded", "true");
  });
});
