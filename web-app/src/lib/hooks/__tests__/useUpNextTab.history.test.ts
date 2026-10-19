/** Characterises the raw replaceState call in useUpNextTab before it moves behind the nav facade. */
import { act, renderHook } from "@testing-library/react";
import { useSearchParams } from "next/navigation";
import { useUpNextTab } from "../useUpNextTab";

jest.mock("next/navigation", () => ({ useSearchParams: jest.fn() }));

describe("useUpNextTab raw History use", () => {
  it("passes the existing history.state and the new ?tab= to replaceState", () => {
    (useSearchParams as jest.Mock).mockReturnValue(new URLSearchParams("tab=prs"));
    window.history.replaceState({ __next: "keep-me" }, "");
    const replaceState = jest.spyOn(window.history, "replaceState").mockImplementation(() => {});
    const { result } = renderHook(() => useUpNextTab(() => 0));

    act(() => result.current.setTab("queue"));

    expect(replaceState).toHaveBeenCalledTimes(1);
    expect(replaceState).toHaveBeenCalledWith({ __next: "keep-me" }, "", "?tab=queue");
    replaceState.mockRestore();
  });
});
