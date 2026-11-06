import { useSearchParams } from "@/lib/nav";

jest.mock("@/lib/nav", () => ({
  useSearchParams: jest.fn(),
}));

it("reads the mocked search params", () => {
  (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("tab=a"));
  expect(useSearchParams().get("tab")).toBe("a");
});
