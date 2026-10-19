import React from "react";
import { render, screen } from "@testing-library/react";
import SettingsPage from "../page";
import { routes } from "@/lib/routes";
import { FLAG_EXTRAS } from "../features/flagExtras";

jest.mock("@/lib/analytics/usePageView", () => ({ usePageView: () => undefined }));
jest.mock("@/lib/contexts/OnboardingContext", () => ({
  useOnboardingContext: () => ({ triggerOnboarding: jest.fn() }),
}));
jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlags: () => ({ flags: {} }),
}));
jest.mock("@/app/config/ConfigPageContent", () => ({ ConfigPageContent: () => null }));
jest.mock("../KeyboardShortcutsTab", () => ({ KeyboardShortcutsTab: () => null }));
jest.mock("@/components/settings/GlobalDefaultsForm", () => ({ GlobalDefaultsForm: () => null }));
jest.mock("@/components/settings/ProfilesManager", () => ({ ProfilesManager: () => null }));
jest.mock("@/components/settings/DirectoryRulesManager", () => ({ DirectoryRulesManager: () => null }));
jest.mock("@/components/settings/AliasesManager", () => ({ AliasesManager: () => null }));
jest.mock("@/components/settings/ProgramsManager", () => ({ ProgramsManager: () => null }));
jest.mock("@/components/settings/PushNotificationSettings", () => ({ PushNotificationSettings: () => null }));
jest.mock("@/components/settings/ThemePicker", () => ({ ThemePicker: () => null }));

describe("Backlog Quota navigation", () => {
  it("renders a Settings index link to the quota page", () => {
    render(<SettingsPage />);
    const link = screen.getByTestId("settings-backlog-quota-link");
    expect(link).toHaveAttribute("href", routes.settingsBacklogQuota);
    expect(routes.settingsBacklogQuota).toBe("/settings/backlog-quota");
  });

  it("registers a flagExtras link on the backlog flag row", () => {
    render(<>{FLAG_EXTRAS.backlog.renderExtras!({ flag: {} as never, pending: false, setFlag: jest.fn() })}</>);
    expect(screen.getByTestId("feature-flag-quota-link")).toHaveAttribute("href", routes.settingsBacklogQuota);
  });
});
