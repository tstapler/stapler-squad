import { useSearchParams } from "next/navigation";

jest.mock("next/navigation", () => ({
  useSearchParams: jest.fn(),
}));

it("reads the mocked search params", () => {
  (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("tab=a"));
  expect(useSearchParams().get("tab")).toBe("a");
});
