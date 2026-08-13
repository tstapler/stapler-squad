import { renderHook, act } from "@testing-library/react";
import { useResizablePanel, ResizablePanelOptions } from "./useResizablePanel";

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (k: string) => store[k] ?? null,
    setItem: (k: string, v: string) => {
      store[k] = v;
    },
    removeItem: (k: string) => {
      delete store[k];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, "localStorage", { value: localStorageMock });

describe("useResizablePanel", () => {
  const defaultOptions: ResizablePanelOptions = {
    storageKey: "test-panel",
    defaultWidth: 300,
    minWidth: 200,
    maxWidthFraction: 0.5,
  };

  beforeEach(() => {
    localStorageMock.clear();
    jest.clearAllMocks();
  });

  describe("initial state from defaults", () => {
    it("should initialize width to defaultWidth when localStorage is empty", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(300);
    });

    it("should initialize collapsed to false when localStorage is empty", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.collapsed).toBe(false);
    });

    it("should return containerRef as RefObject", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.containerRef).toBeDefined();
      expect(result.current.containerRef.current).toBeNull();
    });

    it("should return handleProps with required event handlers", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.handleProps).toHaveProperty("onPointerDown");
      expect(result.current.handleProps).toHaveProperty("onPointerMove");
      expect(result.current.handleProps).toHaveProperty("onPointerUp");
      expect(result.current.handleProps).toHaveProperty("onPointerCancel");
    });

    it("should return collapse and expand functions", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(typeof result.current.collapse).toBe("function");
      expect(typeof result.current.expand).toBe("function");
    });
  });

  describe("initial state from localStorage", () => {
    it("should read stored width from localStorage on init", () => {
      localStorageMock.setItem("test-panel", "350");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(350);
    });

    it("should read stored collapsed state from localStorage on init", () => {
      localStorageMock.setItem("test-panelCollapsed", "true");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.collapsed).toBe(true);
    });

    it("should respect minWidth when reading from localStorage", () => {
      localStorageMock.setItem("test-panel", "100");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(200); // clamped to minWidth
    });

    it("should handle invalid stored width as fallback to defaultWidth", () => {
      localStorageMock.setItem("test-panel", "not-a-number");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(300);
    });

    it("should handle null stored width as fallback to defaultWidth", () => {
      localStorageMock.setItem("test-panel", "");
      localStorageMock.removeItem("test-panel");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(300);
    });

    it("should handle invalid stored collapsed as fallback to false", () => {
      localStorageMock.setItem("test-panelCollapsed", "invalid");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.collapsed).toBe(false);
    });
  });

  describe("persist on width change", () => {
    it("should write width to localStorage after state update", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      act(() => {
        // Simulate width change by manually triggering the effect path
        // We need to trigger a state update that writes to localStorage
        result.current.collapse();
        result.current.expand();
      });

      // The width persists via useEffect, verify localStorage is written
      expect(localStorageMock.getItem("test-panel")).toBe("300");
    });

    it("should update localStorage when width changes", () => {
      const { result, rerender } = renderHook(() =>
        useResizablePanel(defaultOptions)
      );

      // Initial state
      expect(localStorageMock.getItem("test-panel")).toBe("300");

      // Collapse and expand changes width internally
      act(() => {
        result.current.collapse();
      });

      act(() => {
        result.current.expand();
      });

      // Width should be persisted
      expect(localStorageMock.getItem("test-panel")).toBe("300");
    });
  });

  describe("collapse", () => {
    it("should set collapsed to true", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.collapsed).toBe(false);

      act(() => {
        result.current.collapse();
      });

      expect(result.current.collapsed).toBe(true);
    });

    it("should save current width to lastWidthRef before collapsing", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      // Initial width is 300
      expect(result.current.width).toBe(300);

      act(() => {
        result.current.collapse();
      });

      // After collapse, calling expand should restore to 300
      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(300);
    });

    it("should persist collapsed state to localStorage", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      act(() => {
        result.current.collapse();
      });

      expect(localStorageMock.getItem("test-panelCollapsed")).toBe("true");
    });
  });

  describe("expand", () => {
    it("should set collapsed to false", () => {
      localStorageMock.setItem("test-panelCollapsed", "true");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.collapsed).toBe(true);

      act(() => {
        result.current.expand();
      });

      expect(result.current.collapsed).toBe(false);
    });

    it("should restore width from lastWidthRef after collapse", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      // Initial width is 300
      expect(result.current.width).toBe(300);

      // Collapse saves the current width
      act(() => {
        result.current.collapse();
      });

      expect(result.current.collapsed).toBe(true);

      // Expand restores the saved width
      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(300);
      expect(result.current.collapsed).toBe(false);
    });

    it("should use defaultWidth if lastWidthRef is not set", () => {
      // Start fresh without calling collapse first
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(300); // defaultWidth
    });
  });

  describe("expand with lastWidthRef from localStorage", () => {
    it("should restore stored width when expanding after localStorage init with collapsed=true", () => {
      localStorageMock.setItem("test-panel", "400");
      localStorageMock.setItem("test-panelCollapsed", "true");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(400);
      expect(result.current.collapsed).toBe(true);

      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(400);
      expect(result.current.collapsed).toBe(false);
    });

    it("should not use defaultWidth when expanding after stored width exists", () => {
      localStorageMock.setItem("test-panel", "350");
      localStorageMock.setItem("test-panelCollapsed", "true");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(350);
      expect(result.current.width).not.toBe(defaultOptions.defaultWidth);
    });
  });

  describe("collapse and expand cycles", () => {
    it("should preserve width across multiple collapse/expand cycles", () => {
      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      act(() => {
        result.current.collapse();
      });

      expect(result.current.collapsed).toBe(true);

      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(300);
      expect(result.current.collapsed).toBe(false);

      act(() => {
        result.current.collapse();
      });

      expect(result.current.collapsed).toBe(true);

      act(() => {
        result.current.expand();
      });

      expect(result.current.width).toBe(300);
      expect(result.current.collapsed).toBe(false);
    });
  });

  describe("different storage keys", () => {
    it("should use different storage keys for different panels", () => {
      const options1 = { ...defaultOptions, storageKey: "panel-1" };
      const options2 = { ...defaultOptions, storageKey: "panel-2" };

      const { result: result1 } = renderHook(() => useResizablePanel(options1));
      const { result: result2 } = renderHook(() => useResizablePanel(options2));

      // Panel 1: collapse
      act(() => {
        result1.current.collapse();
      });

      // Panel 2: remains expanded
      expect(result2.current.collapsed).toBe(false);

      // Verify separate storage keys
      expect(localStorageMock.getItem("panel-1Collapsed")).toBe("true");
      expect(localStorageMock.getItem("panel-2Collapsed")).toBe("false");
    });
  });

  describe("edge cases", () => {
    it("should handle localStorage exceptions gracefully", () => {
      const mockStorage = {
        getItem: jest.fn(() => {
          throw new Error("Storage error");
        }),
        setItem: jest.fn(() => {
          throw new Error("Storage error");
        }),
        clear: jest.fn(),
      };

      Object.defineProperty(window, "localStorage", { value: mockStorage });

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(300); // Falls back to defaultWidth
      expect(result.current.collapsed).toBe(false); // Falls back to false

      // Restore original mock
      Object.defineProperty(window, "localStorage", { value: localStorageMock });
    });

    it("should clamp width to minWidth constraint", () => {
      localStorageMock.setItem("test-panel", "150");

      const { result } = renderHook(() => useResizablePanel(defaultOptions));

      expect(result.current.width).toBe(200); // minWidth is 200
    });

    it("should handle custom minWidth in different options", () => {
      const options = { ...defaultOptions, minWidth: 150 };
      localStorageMock.setItem("test-panel", "140");

      const { result } = renderHook(() => useResizablePanel(options));

      expect(result.current.width).toBe(150); // clamped to minWidth
    });
  });
});
