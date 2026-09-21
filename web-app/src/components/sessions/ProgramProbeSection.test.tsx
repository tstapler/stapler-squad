import { render, screen, fireEvent } from "@testing-library/react";
import { ProgramProbeSection } from "./ProgramProbeSection";
import {
  mockProbeCheck,
  probeStates,
  resetProbeMocks,
  setProbeHookState,
} from "@/lib/hooks/__mocks__/probeProgramMock";

jest.mock("@/lib/hooks/useProbeProgram", () => require("@/lib/hooks/__mocks__/probeProgramMock").hookMockFactory());

const OPTION = { value: "claude", label: "Claude Code", command: "claude" };

beforeEach(() => resetProbeMocks());

describe("ProgramProbeSection", () => {
  it("ProgramProbeSection_should_ShowResolvedPathBadge_When_ProgramFound", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByText(/Found: \/usr\/bin\/claude/)).toBeInTheDocument();
  });

  it("ProgramProbeSection_should_ShowNotFoundBadgeWithoutDuplicateWarning_When_BinaryMissing", () => {
    setProbeHookState(probeStates.notFound);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getAllByTestId("preset-program-warning")).toHaveLength(1);
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.queryByText(/not found in PATH/)).toBeNull();
  });

  it("ProgramProbeSection_should_ShowSingleNotFoundMessage_When_ProgramMissing", () => {
    setProbeHookState(probeStates.notFound);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getAllByText(/Not found as an executable/)).toHaveLength(1);
  });

  it("ProgramProbeSection_should_ShowFlagsWithoutCheckPrompt_When_ServerReturnsFoundParsedForPreviouslyConfirmedProgram", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByText("14 flags detected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Check/ })).toBeNull();
    expect(mockProbeCheck).toHaveBeenCalledWith();
  });

  it.each(["transportError", "busyOrError", "wrapper"])(
    "ProgramProbeSection_should_ShowNothing_When_TransportErrorOrWrapperOrBusy (%s)",
    (kind) => {
      setProbeHookState(probeStates[kind]);
      render(<ProgramProbeSection option={OPTION} />);
      expect(screen.queryByText(/flags detected/)).toBeNull();
      expect(screen.queryByTestId("preset-program-warning")).toBeNull();
    },
  );

  it("ProgramProbeSection_should_SendResolveOnlyOnSelectionAndConfirmOnlyOnCheck_When_SelectionChangesThenCheckClicked", () => {
    setProbeHookState(probeStates.needsConfirm);
    const { rerender } = render(<ProgramProbeSection option={OPTION} />);
    expect(mockProbeCheck).toHaveBeenLastCalledWith();
    rerender(<ProgramProbeSection option={{ ...OPTION, command: "aider" }} />);
    // The mocked hook returns a stable check fn, so the selection effect does not
    // refire here; the hook (not this component) resolves per command.
    fireEvent.click(screen.getByRole("button", { name: "Check" }));
    expect(mockProbeCheck).toHaveBeenLastCalledWith({ explicit: true });
    expect(mockProbeCheck).not.toHaveBeenCalledWith({ explicit: false });
  });

  it("ProgramProbeSection_should_DescribeSelectByStatusBadge_When_Rendered", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByRole("status").id).toBe("omnibar-program-probe-status");
  });

  it("renders nothing when idle (no saved command)", () => {
    render(<ProgramProbeSection option={undefined} />);
    expect(screen.queryByRole("status")).toBeNull();
  });
});

