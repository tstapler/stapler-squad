// +feature: onboarding-hook-install
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { OnboardingModal } from "../OnboardingModal";

const mockGetHookStatus = jest.fn();
const mockInstallHooks = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: jest.fn(() => ({
    getHookStatus: (...args: unknown[]) => mockGetHookStatus(...args),
    installHooks: (...args: unknown[]) => mockInstallHooks(...args),
  })),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn(() => ({ unary: jest.fn(), stream: jest.fn() })),
}));

jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));

jest.mock("@/lib/contexts/OmnibarContext", () => ({
  useOmnibar: () => ({ open: jest.fn() }),
}));

// Advance the modal to the final (hooks) step and wait for the status fetch to settle.
async function gotoHooksStep() {
  render(<OnboardingModal isOpen onClose={jest.fn()} />);
  // 4 "Next" clicks: steps 1→2→3→4→5.
  for (let i = 0; i < 4; i++) {
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
  }
  // The step-5 effect fires getHookStatus and resolves async state; wait for it
  // so assertions don't race the resolved-promise setState (avoids act() warnings).
  await waitFor(() => expect(mockGetHookStatus).toHaveBeenCalled());
}

describe("onboarding-hook-install", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetHookStatus.mockResolvedValue({
      rulesInstalled: false,
      notificationsInstalled: false,
      rulesAvailable: true,
      notificationsAvailable: true,
    });
    mockInstallHooks.mockResolvedValue({
      status: { rulesInstalled: true, notificationsInstalled: true, rulesAvailable: true, notificationsAvailable: true },
      messages: ["Rule enforcement hook installed.", "Notification hooks installed."],
    });
  });

  it("OnboardingModal_should_fetchHookStatus_When_reachingHooksStep", async () => {
    await gotoHooksStep();
    expect(screen.getByText("Enable Claude Code hooks")).toBeInTheDocument();
    await waitFor(() => expect(mockGetHookStatus).toHaveBeenCalled());
  });

  it("OnboardingModal_should_callInstallHooksWithBothToggles_When_installClicked", async () => {
    await gotoHooksStep();
    await waitFor(() => expect(mockGetHookStatus).toHaveBeenCalled());

    fireEvent.click(screen.getByRole("button", { name: "Install" }));

    await waitFor(() =>
      expect(mockInstallHooks).toHaveBeenCalledWith({
        installRules: true,
        installNotifications: true,
      })
    );
  });

  it("OnboardingModal_should_disableRulesToggle_When_binaryUnavailable", async () => {
    mockGetHookStatus.mockResolvedValue({
      rulesInstalled: false,
      notificationsInstalled: false,
      rulesAvailable: false,
      notificationsAvailable: true,
    });
    await gotoHooksStep();
    await waitFor(() => expect(mockGetHookStatus).toHaveBeenCalled());

    const rulesCheckbox = screen.getByRole("checkbox", { name: /Enable rule enforcement/ });
    await waitFor(() => expect(rulesCheckbox).toBeDisabled());
  });
});
