import { renderHook, waitFor } from "@testing-library/react";
import { PROGRAMS, type ProgramOption } from "@/lib/constants/programs";

// jest.setup.js globally mocks this hook; these tests need the real one.
const { useAvailablePrograms } = jest.requireActual<{ useAvailablePrograms: () => ProgramOption[] }>("./useAvailablePrograms");

const mockList = jest.fn();
jest.mock("@connectrpc/connect", () => ({ createClient: () => ({ listProgramsConfig: mockList }) }));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

describe("useAvailablePrograms", () => {
  beforeAll(() => {
    // jsdom has no fetch; the hook bails out early without one.
    global.fetch = jest.fn() as unknown as typeof fetch;
  });

  it("useAvailablePrograms_should_CarryCommandAndCliFlags_When_ListProgramsConfigMapped", async () => {
    mockList.mockResolvedValue({
      programs: [{ id: "aider", label: "Aider", description: "", command: "aider", cliFlags: "--yes-always" }],
    });
    const { result } = renderHook(() => useAvailablePrograms());
    await waitFor(() => expect(result.current[0].value).toBe("aider"));
    expect(result.current[0]).toMatchObject({ value: "aider", command: "aider", cliFlags: "--yes-always" });
  });

  it("useAvailablePrograms_should_KeepStaticProgramsWorking_When_CommandFieldsAbsent", () => {
    mockList.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useAvailablePrograms());
    expect(result.current).toBe(PROGRAMS);
    expect(result.current.every((p) => p.command === undefined && p.cliFlags === undefined)).toBe(true);
  });
});
