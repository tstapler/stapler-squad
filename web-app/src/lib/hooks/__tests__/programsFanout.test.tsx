import React from "react";
import { render, waitFor } from "@testing-library/react";
import type { ProgramOption } from "@/lib/constants/programs";

// jest.setup.js globally mocks this hook; the guard needs the real one.
const { useAvailablePrograms, resetAvailableProgramsForTest } = jest.requireActual<{
  useAvailablePrograms: () => ProgramOption[];
  resetAvailableProgramsForTest: () => void;
}>("@/lib/hooks/useAvailablePrograms");

const mockListProgramsConfig = jest.fn();

jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({ listProgramsConfig: (...a: unknown[]) => mockListProgramsConfig(...a) }),
}));

function Consumer() {
  return <span data-testid="n">{useAvailablePrograms().length}</span>;
}

describe("programs consumer fan-out", () => {
  beforeEach(() => {
    resetAvailableProgramsForTest();
    global.fetch = jest.fn() as unknown as typeof fetch; // the hook bails out without fetch
    mockListProgramsConfig.mockReset();
    mockListProgramsConfig.mockResolvedValue({
      programs: [{ id: "p1", label: "P1", description: "", command: "p1", cliFlags: "" }],
    });
  });

  it("programsFanout_should_IssueOneListProgramsConfig_When_20ConsumersMountTogether", async () => {
    const { getAllByTestId } = render(
      <>
        {Array.from({ length: 20 }, (_, i) => (
          <Consumer key={i} />
        ))}
      </>,
    );
    await waitFor(() => expect(getAllByTestId("n").every((e) => e.textContent === "1")).toBe(true));
    expect(mockListProgramsConfig).toHaveBeenCalledTimes(1);
  });
});
