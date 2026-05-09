import {
  TOAST_STALE_MS,
  ACTIONABLE_TOAST_STALE_MS,
  TOAST_DEDUP_WINDOW_MS,
  isActionable,
  toastAutoCloseMs,
  toastAutoMinimizeMs,
} from "@/lib/notification-policy";

describe("notification-policy", () => {
  describe("constants", () => {
    it("TOAST_STALE_MS is 5 minutes", () => {
      expect(TOAST_STALE_MS).toBe(5 * 60 * 1000);
    });

    it("ACTIONABLE_TOAST_STALE_MS is 6 minutes", () => {
      expect(ACTIONABLE_TOAST_STALE_MS).toBe(6 * 60 * 1000);
    });

    it("ACTIONABLE_TOAST_STALE_MS is longer than TOAST_STALE_MS", () => {
      expect(ACTIONABLE_TOAST_STALE_MS).toBeGreaterThan(TOAST_STALE_MS);
    });

    it("TOAST_DEDUP_WINDOW_MS is 10 seconds", () => {
      expect(TOAST_DEDUP_WINDOW_MS).toBe(10_000);
    });
  });

  describe("isActionable", () => {
    it("returns true for approval_needed", () => {
      expect(isActionable("approval_needed")).toBe(true);
    });

    it("returns true for question", () => {
      expect(isActionable("question")).toBe(true);
    });

    it("returns false for error", () => {
      expect(isActionable("error")).toBe(false);
    });

    it("returns false for warning", () => {
      expect(isActionable("warning")).toBe(false);
    });

    it("returns false for task_complete", () => {
      expect(isActionable("task_complete")).toBe(false);
    });

    it("returns false for task_failed", () => {
      expect(isActionable("task_failed")).toBe(false);
    });

    it("returns false for info", () => {
      expect(isActionable("info")).toBe(false);
    });

    it("returns false for undefined", () => {
      expect(isActionable(undefined)).toBe(false);
    });
  });

  describe("toastAutoCloseMs", () => {
    it("returns ACTIONABLE_TOAST_STALE_MS for approval_needed", () => {
      expect(toastAutoCloseMs("approval_needed")).toBe(ACTIONABLE_TOAST_STALE_MS);
    });

    it("returns ACTIONABLE_TOAST_STALE_MS for question", () => {
      expect(toastAutoCloseMs("question")).toBe(ACTIONABLE_TOAST_STALE_MS);
    });

    it("returns 12 seconds for error", () => {
      expect(toastAutoCloseMs("error")).toBe(12_000);
    });

    it("returns 12 seconds for task_failed", () => {
      expect(toastAutoCloseMs("task_failed")).toBe(12_000);
    });

    it("returns 8 seconds for warning", () => {
      expect(toastAutoCloseMs("warning")).toBe(8_000);
    });

    it("returns 8 seconds for info", () => {
      expect(toastAutoCloseMs("info")).toBe(8_000);
    });

    it("returns 8 seconds for task_complete", () => {
      expect(toastAutoCloseMs("task_complete")).toBe(8_000);
    });

    it("returns 8 seconds for undefined (default)", () => {
      expect(toastAutoCloseMs(undefined)).toBe(8_000);
    });

    it("actionable types get longer close time than non-actionable", () => {
      expect(toastAutoCloseMs("approval_needed")).toBeGreaterThan(toastAutoCloseMs("error"));
      expect(toastAutoCloseMs("question")).toBeGreaterThan(toastAutoCloseMs("warning"));
    });
  });

  describe("toastAutoMinimizeMs", () => {
    it("returns 0 for approval_needed (never minimize — requires user action)", () => {
      expect(toastAutoMinimizeMs("approval_needed")).toBe(0);
    });

    it("returns 0 for question (never minimize — requires user action)", () => {
      expect(toastAutoMinimizeMs("question")).toBe(0);
    });

    it("returns 5 seconds for error", () => {
      expect(toastAutoMinimizeMs("error")).toBe(5_000);
    });

    it("returns 5 seconds for task_failed", () => {
      expect(toastAutoMinimizeMs("task_failed")).toBe(5_000);
    });

    it("returns 5 seconds for warning", () => {
      expect(toastAutoMinimizeMs("warning")).toBe(5_000);
    });

    it("returns 0 for info (no minimize)", () => {
      expect(toastAutoMinimizeMs("info")).toBe(0);
    });

    it("returns 0 for task_complete (no minimize)", () => {
      expect(toastAutoMinimizeMs("task_complete")).toBe(0);
    });

    it("returns 0 for undefined (no minimize)", () => {
      expect(toastAutoMinimizeMs(undefined)).toBe(0);
    });
  });
});
