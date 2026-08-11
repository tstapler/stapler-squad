/**
 * Regression test for the Settings page nav.
 *
 * /settings/pipeline-modes has real content (pipeline mode CRUD) but was only
 * reachable by typing the URL directly — Settings itself had no tab or link
 * to it in the tab bar (docs/tasks/backlog-feature-improvement.md, Manual
 * Gates / Non-Configurable Pipeline Steps sections). This asserts a
 * discoverable nav entry exists alongside the other 4 tabs.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import SettingsPage from "../page";

jest.mock("@/lib/analytics/usePageView", () => ({
  usePageView: () => undefined,
}));
jest.mock("@/lib/contexts/OnboardingContext", () => ({
  useOnboardingContext: () => ({ triggerOnboarding: jest.fn() }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: {} }),
}));
jest.mock("@/components/settings/GlobalDefaultsForm", () => ({
  GlobalDefaultsForm: () => <div data-testid="global-defaults-form" />,
}));
jest.mock("@/components/settings/ProfilesManager", () => ({
  ProfilesManager: () => <div data-testid="profiles-manager" />,
}));
jest.mock("@/components/settings/DirectoryRulesManager", () => ({
  DirectoryRulesManager: () => <div data-testid="directory-rules-manager" />,
}));
jest.mock("@/components/settings/AliasesManager", () => ({
  AliasesManager: () => <div data-testid="aliases-manager" />,
}));
jest.mock("@/components/settings/PushNotificationSettings", () => ({
  PushNotificationSettings: () => <div data-testid="push-notification-settings" />,
}));
jest.mock("@/components/settings/ThemePicker", () => ({
  ThemePicker: () => <div data-testid="theme-picker" />,
}));
jest.mock("@/app/config/ConfigPageContent", () => ({
  ConfigPageContent: () => <div data-testid="config-page-content" />,
}));
jest.mock("../KeyboardShortcutsTab", () => ({
  KeyboardShortcutsTab: () => <div data-testid="keyboard-shortcuts-tab" />,
}));

describe("SettingsPage", () => {
  it("SettingsPage_should_renderPipelineModesNavLink_alongsideTheFourTabs", () => {
    render(<SettingsPage />);

    // The 4 existing tabs are still present.
    expect(screen.getByRole("tab", { name: "General" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Config Files" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Appearance" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Keyboard Shortcuts" })).toBeInTheDocument();

    // New: a discoverable nav entry to /settings/pipeline-modes, no longer
    // reachable only by typing the URL directly.
    const link = screen.getByTestId("settings-pipeline-modes-tab-link");
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute("href", "/settings/pipeline-modes");
  });
});
