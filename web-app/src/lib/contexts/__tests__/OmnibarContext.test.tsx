import React from "react";
import { render, act } from "@testing-library/react";
import { OmnibarProvider } from "../OmnibarContext";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------
//
// OmnibarContext always renders <Omnibar onRunWorkflow={handleRunWorkflow} .../>
// regardless of open/closed state, so replacing Omnibar with a stub that stashes
// its props lets these tests invoke handleRunWorkflow directly without driving
// the full omnibar UI (keyboard shortcuts, detectors, etc).

let capturedOnRunWorkflow: ((slug: string, arg: string) => Promise<void>) | undefined;

jest.mock("@/components/sessions/Omnibar", () => ({
  Omnibar: (props: { onRunWorkflow?: (slug: string, arg: string) => Promise<void> }) => {
    capturedOnRunWorkflow = props.onRunWorkflow;
    return null;
  },
}));

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

const mockShowActionToast = jest.fn();
jest.mock("@/lib/contexts/NotificationContext", () => ({
  useNotifications: () => ({ showActionToast: mockShowActionToast }),
}));

const mockRunWorkflowRPC = jest.fn();
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({
    createSession: jest.fn(),
    runWorkflow: mockRunWorkflowRPC,
  }),
}));

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: () => ({ createBacklogItemFromChat: jest.fn() }),
}));

let mockWorkflows: Array<{
  id: string;
  slug: string;
  name: string;
  description?: string;
  targetDirectory?: string;
  sessionType?: string;
  inputTemplate?: string;
}> = [];
jest.mock("@/lib/hooks/useWorkflows", () => ({
  useWorkflows: () => ({ workflows: mockWorkflows }),
}));

jest.mock("@/lib/contexts/AuthContext", () => ({
  useAuth: () => ({ authEnabled: false, authenticated: false, loading: false }),
}));

jest.mock("@/lib/hooks/useAliases", () => ({
  useAliases: () => ({ aliases: [] }),
}));

jest.mock("@/lib/hooks/useLauncherPresets", () => ({
  useLauncherPresets: () => ({
    presets: [],
    loading: false,
    loadError: null,
    refetch: jest.fn(),
  }),
}));

jest.mock("@/lib/hooks/useGitHubEnterpriseHosts", () => ({
  useGitHubEnterpriseHosts: () => ({ hosts: [], refetch: jest.fn() }),
}));

jest.mock("@/lib/hooks/useConfiguredRemotes", () => ({
  useConfiguredRemotes: () => ({ remotes: [], loading: false }),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function renderProvider() {
  render(
    <OmnibarProvider>
      <div />
    </OmnibarProvider>
  );
}

async function runWorkflow(slug: string, arg: string) {
  if (!capturedOnRunWorkflow) {
    throw new Error("Omnibar mock never received onRunWorkflow");
  }
  const onRunWorkflow = capturedOnRunWorkflow;
  await act(async () => {
    await onRunWorkflow(slug, arg);
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("OmnibarContext handleRunWorkflow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockWorkflows = [{ id: "wf-1", slug: "topic-synthesis", name: "Topic Synthesis" }];
    capturedOnRunWorkflow = undefined;
  });

  it("shows a visible error toast (not just console.error) when the slug is unknown", async () => {
    const consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    renderProvider();

    await runWorkflow("does-not-exist", "some topic");

    expect(mockShowActionToast).toHaveBeenCalledWith(
      "Workflow '@does-not-exist' not found — try reloading",
      "error",
      "run-workflow:does-not-exist"
    );
    expect(mockRunWorkflowRPC).not.toHaveBeenCalled();
    expect(mockPush).not.toHaveBeenCalled();
    consoleErrorSpy.mockRestore();
  });

  it("navigates to the new session on success and shows no error toast", async () => {
    mockRunWorkflowRPC.mockResolvedValue("session-123");
    renderProvider();

    await runWorkflow("topic-synthesis", "some topic");

    expect(mockRunWorkflowRPC).toHaveBeenCalledWith({ id: "wf-1", arg: "some topic" });
    expect(mockPush).toHaveBeenCalledWith("/?session=session-123");
    expect(mockShowActionToast).not.toHaveBeenCalled();
  });

  it("shows a visible error toast when the RPC resolves but returns no session (the pre-existing swallowed-failure path)", async () => {
    // useSessionService.runWorkflow never throws on failure -- it catches internally
    // and resolves to null, so a falsy sessionId is itself the failure signal.
    mockRunWorkflowRPC.mockResolvedValue(null);
    renderProvider();

    await runWorkflow("topic-synthesis", "some topic");

    expect(mockShowActionToast).toHaveBeenCalledWith(
      "Failed to run workflow '@topic-synthesis' — try again",
      "error",
      "run-workflow:topic-synthesis"
    );
    expect(mockPush).not.toHaveBeenCalled();
  });

  it("shows a visible error toast if the RPC call itself throws", async () => {
    const consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
    mockRunWorkflowRPC.mockRejectedValue(new Error("network exploded"));
    renderProvider();

    await runWorkflow("topic-synthesis", "some topic");

    expect(mockShowActionToast).toHaveBeenCalledWith(
      "network exploded",
      "error",
      "run-workflow:topic-synthesis"
    );
    consoleErrorSpy.mockRestore();
  });
});
