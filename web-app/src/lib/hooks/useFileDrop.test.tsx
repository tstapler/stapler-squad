import { renderHook, act } from "@testing-library/react";
import type React from "react";
import { useFileDrop } from "./useFileDrop";

const evt = (types: string[], files: File[] = []) =>
  ({ preventDefault: jest.fn(), dataTransfer: { types, files, dropEffect: "" } }) as unknown as React.DragEvent;

describe("useFileDrop", () => {
  it("shows overlay on file dragenter", () => {
    const { result } = renderHook(() => useFileDrop(true, jest.fn()));
    act(() => result.current.dropHandlers.onDragEnter(evt(["Files"])));
    expect(result.current.isDragOver).toBe(true);
  });

  it("stays visible until nested enters are balanced by leaves", () => {
    const { result } = renderHook(() => useFileDrop(true, jest.fn()));
    act(() => result.current.dropHandlers.onDragEnter(evt(["Files"])));
    act(() => result.current.dropHandlers.onDragEnter(evt(["Files"])));
    act(() => result.current.dropHandlers.onDragLeave(evt(["Files"])));
    expect(result.current.isDragOver).toBe(true);
    act(() => result.current.dropHandlers.onDragLeave(evt(["Files"])));
    expect(result.current.isDragOver).toBe(false);
  });

  it("drop clears overlay and passes all files in order", () => {
    const onFiles = jest.fn();
    const { result } = renderHook(() => useFileDrop(true, onFiles));
    const files = [new File(["a"], "a.png"), new File(["b"], "b.txt")];
    act(() => result.current.dropHandlers.onDragEnter(evt(["Files"])));
    act(() => result.current.dropHandlers.onDrop(evt(["Files"], files)));
    expect(result.current.isDragOver).toBe(false);
    expect(onFiles).toHaveBeenCalledWith(files);
  });

  it("ignores non-file drags such as pane reordering", () => {
    const { result } = renderHook(() => useFileDrop(true, jest.fn()));
    act(() => result.current.dropHandlers.onDragEnter(evt(["text/plain"])));
    expect(result.current.isDragOver).toBe(false);
  });

  it("does nothing when disabled (read-only or touch device)", () => {
    const onFiles = jest.fn();
    const { result } = renderHook(() => useFileDrop(false, onFiles));
    const e = evt(["Files"], [new File(["a"], "a.png")]);
    act(() => result.current.dropHandlers.onDragEnter(e));
    act(() => result.current.dropHandlers.onDrop(e));
    expect(result.current.isDragOver).toBe(false);
    expect(onFiles).not.toHaveBeenCalled();
    expect(e.preventDefault).toHaveBeenCalled(); // keeps the browser from opening the file
  });
});
