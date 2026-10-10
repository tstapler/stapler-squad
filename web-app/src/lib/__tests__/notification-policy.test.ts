import {
  TOAST_STALE_MS,
  ACTIONABLE_TOAST_STALE_MS,
  TOAST_DEDUP_WINDOW_MS,
  NATIVE_HIGH_TTL_MS,
  NATIVE_MEDIUM_TTL_MS,
  hasLongToastLifetime,
  isPinned,
  partitionToasts,
  toastAutoCloseMs,
  toastAutoMinimizeMs,
  nativeAutoCloseMs,
} from "@/lib/notification-policy";
import { NotificationPriority } from "@/gen/session/v1/types_pb";
import fs from "fs";
import path from "path";

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

  describe("hasLongToastLifetime", () => {
    it("returns true for approval_needed", () => {
      expect(hasLongToastLifetime("approval_needed")).toBe(true);
    });

    it("returns true for question", () => {
      expect(hasLongToastLifetime("question")).toBe(true);
    });

    it("returns false for error", () => {
      expect(hasLongToastLifetime("error")).toBe(false);
    });

    it("returns false for warning", () => {
      expect(hasLongToastLifetime("warning")).toBe(false);
    });

    it("returns false for task_complete", () => {
      expect(hasLongToastLifetime("task_complete")).toBe(false);
    });

    it("returns false for task_failed", () => {
      expect(hasLongToastLifetime("task_failed")).toBe(false);
    });

    it("returns false for info", () => {
      expect(hasLongToastLifetime("info")).toBe(false);
    });

    it("returns false for undefined", () => {
      expect(hasLongToastLifetime(undefined)).toBe(false);
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

  describe("nativeAutoCloseMs", () => {
    it("nativeAutoCloseMs_should_return30000_When_priorityIsUrgent", () => {
      expect(nativeAutoCloseMs(NotificationPriority.URGENT)).toBe(30_000);
      expect(nativeAutoCloseMs(NotificationPriority.URGENT)).toBe(NATIVE_HIGH_TTL_MS);
    });

    it("nativeAutoCloseMs_should_return30000_When_priorityIsHigh", () => {
      expect(nativeAutoCloseMs(NotificationPriority.HIGH)).toBe(30_000);
      expect(nativeAutoCloseMs(NotificationPriority.HIGH)).toBe(NATIVE_HIGH_TTL_MS);
    });

    it("nativeAutoCloseMs_should_return15000_When_priorityIsMedium", () => {
      expect(nativeAutoCloseMs(NotificationPriority.MEDIUM)).toBe(15_000);
      expect(nativeAutoCloseMs(NotificationPriority.MEDIUM)).toBe(NATIVE_MEDIUM_TTL_MS);
    });

    it("nativeAutoCloseMs_should_return15000_When_priorityIsUnspecified", () => {
      expect(nativeAutoCloseMs(NotificationPriority.UNSPECIFIED)).toBe(15_000);
      expect(nativeAutoCloseMs(NotificationPriority.UNSPECIFIED)).toBe(NATIVE_MEDIUM_TTL_MS);
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

    it("returns 3 seconds for info (auto-minimize)", () => {
      expect(toastAutoMinimizeMs("info")).toBe(3_000);
    });

    it("returns 3 seconds for task_complete (auto-minimize)", () => {
      expect(toastAutoMinimizeMs("task_complete")).toBe(3_000);
    });

    it("returns 3 seconds for undefined (auto-minimize)", () => {
      expect(toastAutoMinimizeMs(undefined)).toBe(3_000);
    });
  });
});

describe("pending-decision stack policy", () => {
  const t = (id: string, pinned = false) => ({ id, isPendingDecision: pinned });

  describe("partitionToasts", () => {
    it("partitionToasts_should_keep_pinned_first_and_report_overflow_when_cap_3", () => {
      const toasts = [t("approval", true), t("info1"), t("info2"), t("error", true), t("info3")];
      const result = partitionToasts(toasts, 3);
      expect(result.visible.map((n) => n.id)).toEqual(["approval", "error", "info3"]);
      expect(result.overflow).toBe(2);
    });

    it("collapses pinned beyond the cap into the overflow, keeping arrival order", () => {
      const toasts = [t("a", true), t("b", true), t("c", true), t("d", true)];
      const result = partitionToasts(toasts, 3);
      expect(result.visible.map((n) => n.id)).toEqual(["a", "b", "c"]);
      expect(result.overflow).toBe(1);
      expect(result.pinnedCount).toBe(4);
    });

    it("shows nothing but counts everything when the cap is 0 (keyboard open)", () => {
      const result = partitionToasts([t("a", true), t("b")], 0);
      expect(result.visible).toEqual([]);
      expect(result.overflow).toBe(2);
      expect(result.pinnedCount).toBe(1);
    });

    it("returns everything when the cap exceeds the deck", () => {
      const result = partitionToasts([t("a"), t("b")], 3);
      expect(result.visible.map((n) => n.id)).toEqual(["a", "b"]);
      expect(result.overflow).toBe(0);
    });
  });

  describe("isPinned", () => {
    it("isPinned_should_read_only_isPendingDecision_and_never_list_types_when_grepped", () => {
      expect(isPinned({ isPendingDecision: true })).toBe(true);
      expect(isPinned({ isPendingDecision: false })).toBe(false);
      expect(isPinned({})).toBe(false);

      const source = fs.readFileSync(path.join(process.cwd(), "src/lib/notification-policy.ts"), "utf8");
      const body = source.slice(source.indexOf("export function isPinned"));
      const fn = body.slice(0, body.indexOf("\n}\n"));
      expect(fn).not.toMatch(/approval_needed|question|"error"|task_failed|"warning"/);
    });
  });

  it("client_should_have_no_actionable_type_set_when_grepped", () => {
    const libDir = path.join(process.cwd(), "src/lib");
    const offenders: string[] = [];
    const walk = (dir: string) => {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          if (entry.name !== "__tests__") walk(full);
        } else if (/\.(ts|tsx)$/.test(entry.name) && !/\.test\./.test(entry.name)) {
          if (/ACTIONABLE_TYPES|isActionableNotification|new Set<[^>]*>\(\[\s*"approval_needed"/.test(fs.readFileSync(full, "utf8"))) {
            offenders.push(path.relative(libDir, full));
          }
        }
      }
    };
    walk(libDir);
    expect(offenders).toEqual([]);
  });
});
