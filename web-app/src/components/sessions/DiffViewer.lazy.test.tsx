/** DiffViewer is only mounted while the diff tab is shown, so mounting it is what triggers the diff fetch. */
import { render } from "@testing-library/react";

const ensureDiff = jest.fn();
jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  useSessionVcsContext: () => ({ diff: null, diffLoading: true, refreshDiff: jest.fn(), ensureDiff }),
}));
jest.mock("@/components/shared/DiffRenderer", () => ({ DiffRenderer: () => null }));
jest.mock("./CIStatusBadge", () => ({ CIStatusBadge: () => null }));

import { DiffViewer } from "./DiffViewer";

describe("DiffViewer lazy diff", () => {
  beforeEach(() => ensureDiff.mockClear());

  it("DiffViewer_should_RequestDiffOnce_When_Mounted", () => {
    const { rerender } = render(<DiffViewer />);
    rerender(<DiffViewer />);
    expect(ensureDiff).toHaveBeenCalledTimes(1);
  });

  it("DiffViewer_should_NotRequestDiff_When_NotMounted", () => {
    expect(ensureDiff).not.toHaveBeenCalled();
  });
});
