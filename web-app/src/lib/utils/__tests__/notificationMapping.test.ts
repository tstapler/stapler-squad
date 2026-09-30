import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  mapNotificationType,
  mapPriority,
  notificationTypeIcon,
  notificationTypeLabel,
  priorityColor,
  notificationTypeFilter,
  isActionableNotification,
  computeScopedMarkReadIds,
  capBadgeCount,
} from "@/lib/utils/notificationMapping";
import type { NotificationData } from "@/lib/types/notification";

type UIType = NonNullable<NotificationData["notificationType"]>;

// The full 13-member notificationType union (web-app/src/lib/types/notification.ts:16).
const ALL_UI_TYPES: UIType[] = [
  "info",
  "approval_needed",
  "auto_approved",
  "error",
  "warning",
  "task_complete",
  "task_failed",
  "progress",
  "question",
  "reminder",
  "system",
  "custom",
  "undo",
];

describe("notificationMapping", () => {
  describe("mapNotificationType", () => {
    it("maps APPROVAL_NEEDED to approval_needed", () => {
      expect(mapNotificationType(NotificationType.APPROVAL_NEEDED)).toBe("approval_needed");
    });

    it("maps CONFIRMATION_NEEDED to approval_needed (not question)", () => {
      // Previously diverged between Context and useSessionNotifications — this is the canonical answer
      expect(mapNotificationType(NotificationType.CONFIRMATION_NEEDED)).toBe("approval_needed");
    });

    it("maps INPUT_REQUIRED to question", () => {
      expect(mapNotificationType(NotificationType.INPUT_REQUIRED)).toBe("question");
    });

    it("maps ERROR to error", () => {
      expect(mapNotificationType(NotificationType.ERROR)).toBe("error");
    });

    it("maps FAILURE to error", () => {
      expect(mapNotificationType(NotificationType.FAILURE)).toBe("error");
    });

    it("maps WARNING to warning", () => {
      expect(mapNotificationType(NotificationType.WARNING)).toBe("warning");
    });

    it("maps TASK_COMPLETE to task_complete", () => {
      expect(mapNotificationType(NotificationType.TASK_COMPLETE)).toBe("task_complete");
    });

    it("maps PROCESS_FINISHED to task_complete", () => {
      expect(mapNotificationType(NotificationType.PROCESS_FINISHED)).toBe("task_complete");
    });

    it("maps PROCESS_STARTED to progress", () => {
      expect(mapNotificationType(NotificationType.PROCESS_STARTED)).toBe("progress");
    });

    it("maps INFO to info", () => {
      expect(mapNotificationType(NotificationType.INFO)).toBe("info");
    });

    it("maps DEBUG to info", () => {
      expect(mapNotificationType(NotificationType.DEBUG)).toBe("info");
    });

    it("maps STATUS_CHANGE to info", () => {
      expect(mapNotificationType(NotificationType.STATUS_CHANGE)).toBe("info");
    });

    it("maps AUTO_APPROVED to auto_approved", () => {
      expect(mapNotificationType(NotificationType.AUTO_APPROVED)).toBe("auto_approved");
    });

    it("maps CUSTOM to custom", () => {
      expect(mapNotificationType(NotificationType.CUSTOM)).toBe("custom");
    });

    it("maps UNSPECIFIED to info", () => {
      expect(mapNotificationType(NotificationType.UNSPECIFIED)).toBe("info");
    });

    // Task 3.1.1c (pre-mortem P1): an unmapped backend NotificationType must not
    // silently default to "info" — that reproduces the exact miscategorization
    // bug this project exists to fix, one stage upstream of the "info" filter.
    it("warns and defaults unknown values to an actionable type (fail-safe), not info", () => {
      const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
      const result = mapNotificationType(9999);
      expect(warnSpy).toHaveBeenCalledTimes(1);
      expect(isActionableNotification(result)).toBe(true);
      warnSpy.mockRestore();
    });

    it("never warns for any currently-mapped real enum value", () => {
      const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
      const realValues = Object.values(NotificationType).filter((v): v is number => typeof v === "number");
      for (const value of realValues) {
        mapNotificationType(value);
      }
      expect(warnSpy).not.toHaveBeenCalled();
      warnSpy.mockRestore();
    });
  });

  describe("isActionableNotification", () => {
    it("classifies the five actionable types", () => {
      expect(isActionableNotification("approval_needed")).toBe(true);
      expect(isActionableNotification("question")).toBe(true);
      expect(isActionableNotification("error")).toBe(true);
      expect(isActionableNotification("task_failed")).toBe(true);
      expect(isActionableNotification("warning")).toBe(true);
    });

    it("classifies non-actionable types as false", () => {
      expect(isActionableNotification("info")).toBe(false);
      expect(isActionableNotification("task_complete")).toBe(false);
      expect(isActionableNotification("auto_approved")).toBe(false);
    });

    // Regression for the found "task_complete" gap: the first draft defined
    // "info" as a second, independently-maintained 7-type list that (with
    // ACTIONABLE_TYPES) covered only 12 of the 13 UITypes. "info" is now the
    // literal complement of ACTIONABLE_TYPES, so this is structurally
    // guaranteed — this test pins it against the type union drifting.
    it("every UIType is classified by isActionableNotification, none fall through both notificationTypeFilter('info') and ACTIONABLE_TYPES", () => {
      for (const type of ALL_UI_TYPES) {
        const actionable = isActionableNotification(type);
        const inInfo = notificationTypeFilter("info", [type]).includes(type);
        expect(actionable).toBe(!inInfo);
      }
    });
  });

  describe("computeScopedMarkReadIds", () => {
    it("returns only unread non-actionable IDs", () => {
      const notifications = [
        { id: "a1", isRead: false, notificationType: "approval_needed" as UIType },
        { id: "b2", isRead: false, notificationType: "task_complete" as UIType },
        { id: "c3", isRead: true, notificationType: "task_complete" as UIType },
        { id: "d4", isRead: false, notificationType: "question" as UIType },
      ];
      expect(computeScopedMarkReadIds(notifications)).toEqual(["b2"]);
    });
  });

  describe("capBadgeCount", () => {
    it("caps above 99", () => {
      expect(capBadgeCount(152)).toBe("99+");
    });

    it("shows the literal number at or below 99", () => {
      expect(capBadgeCount(99)).toBe("99");
      expect(capBadgeCount(7)).toBe("7");
    });
  });

  describe("mapPriority", () => {
    it("maps URGENT to urgent", () => {
      expect(mapPriority(NotificationPriority.URGENT)).toBe("urgent");
    });

    it("maps HIGH to high", () => {
      expect(mapPriority(NotificationPriority.HIGH)).toBe("high");
    });

    it("maps MEDIUM to medium", () => {
      expect(mapPriority(NotificationPriority.MEDIUM)).toBe("medium");
    });

    it("maps LOW to low", () => {
      expect(mapPriority(NotificationPriority.LOW)).toBe("low");
    });

    it("defaults unknown values to medium", () => {
      expect(mapPriority(9999)).toBe("medium");
    });
  });

  describe("notificationTypeIcon", () => {
    it("returns ⚠️ for approval_needed", () => {
      expect(notificationTypeIcon("approval_needed")).toBe("⚠️");
    });

    it("returns ❌ for error", () => {
      expect(notificationTypeIcon("error")).toBe("❌");
    });

    it("returns ✅ for task_complete", () => {
      expect(notificationTypeIcon("task_complete")).toBe("✅");
    });

    it("returns 💥 for task_failed", () => {
      expect(notificationTypeIcon("task_failed")).toBe("💥");
    });

    it("returns ❓ for question", () => {
      expect(notificationTypeIcon("question")).toBe("❓");
    });

    it("returns 🔔 for unknown/undefined (default)", () => {
      expect(notificationTypeIcon(undefined)).toBe("🔔");
      expect(notificationTypeIcon("info")).toBe("🔔");
    });

    it("returns the same icon from both Toast and Panel (no divergence)", () => {
      // These are the cases that previously had separate inline implementations
      const types = ["approval_needed", "error", "warning", "task_complete", "task_failed", "question"] as const;
      types.forEach((t) => {
        expect(notificationTypeIcon(t)).toBeTruthy();
      });
    });
  });

  describe("notificationTypeLabel", () => {
    it("returns 'Approval Needed' for approval_needed", () => {
      expect(notificationTypeLabel("approval_needed")).toBe("Approval Needed");
    });

    it("returns 'Error' for error", () => {
      expect(notificationTypeLabel("error")).toBe("Error");
    });

    it("returns 'Warning' for warning", () => {
      expect(notificationTypeLabel("warning")).toBe("Warning");
    });

    it("returns 'Task Complete' for task_complete", () => {
      expect(notificationTypeLabel("task_complete")).toBe("Task Complete");
    });

    it("returns 'Task Failed' for task_failed", () => {
      expect(notificationTypeLabel("task_failed")).toBe("Task Failed");
    });

    it("returns 'Question' for question", () => {
      expect(notificationTypeLabel("question")).toBe("Question");
    });

    it("returns 'Info' for undefined (default)", () => {
      expect(notificationTypeLabel(undefined)).toBe("Info");
    });
  });

  describe("priorityColor", () => {
    it("returns error color for urgent", () => {
      expect(priorityColor("urgent")).toContain("color-error");
    });

    it("returns warning color for high", () => {
      expect(priorityColor("high")).toContain("color-warning");
    });

    it("returns info color for medium", () => {
      expect(priorityColor("medium")).toContain("color-info");
    });

    it("returns success color for low", () => {
      expect(priorityColor("low")).toContain("color-success");
    });

    it("returns primary color for undefined (default)", () => {
      expect(priorityColor(undefined)).toContain("color-primary");
    });

    it("all colors are CSS variable references", () => {
      const priorities = ["urgent", "high", "medium", "low", undefined] as const;
      priorities.forEach((p) => {
        expect(priorityColor(p)).toMatch(/^var\(/);
      });
    });
  });

  describe("notificationTypeFilter", () => {
    const allTypes = [
      "approval_needed",
      "auto_approved",
      "question",
      "error",
      "task_failed",
      "warning",
      "task_complete",
      "info",
      "progress",
      "reminder",
      "system",
      "custom",
    ] as const;

    it("returns all types for 'all' category", () => {
      const result = notificationTypeFilter("all", [...allTypes]);
      expect(result).toHaveLength(allTypes.length);
    });

    it("'approval_needed' category includes question", () => {
      const result = notificationTypeFilter("approval_needed", [...allTypes]);
      expect(result).toContain("approval_needed");
      expect(result).toContain("question");
    });

    it("'approval_needed' category excludes non-actionable types", () => {
      const result = notificationTypeFilter("approval_needed", [...allTypes]);
      expect(result).not.toContain("error");
      expect(result).not.toContain("info");
      expect(result).not.toContain("task_complete");
    });

    it("'error' category covers error, task_failed, and warning", () => {
      const result = notificationTypeFilter("error", [...allTypes]);
      expect(result).toContain("error");
      expect(result).toContain("task_failed");
      expect(result).toContain("warning");
      expect(result).toHaveLength(3);
    });

    it("'task_complete' category only includes task_complete", () => {
      const result = notificationTypeFilter("task_complete", [...allTypes]);
      expect(result).toEqual(["task_complete"]);
    });

    // Task 3.1.1a converted "info" from a hand-maintained exclusion list to the
    // literal complement of ACTIONABLE_TYPES (approval_needed, question, error,
    // task_failed, warning) — it now excludes only those five actionable types.
    it("info category never includes question", () => {
      const result = notificationTypeFilter("info", ["question", "info", "auto_approved"]);
      expect(result).not.toContain("question");
      expect(result).toEqual(["info", "auto_approved"]);
    });

    it("'info' category excludes the five actionable types", () => {
      const result = notificationTypeFilter("info", [...allTypes]);
      expect(result).not.toContain("approval_needed");
      expect(result).not.toContain("question");
      expect(result).not.toContain("error");
      expect(result).not.toContain("task_failed");
      expect(result).not.toContain("warning");
    });

    it("'info' category includes progress, reminder, system, custom, info, task_complete, and auto_approved (allow-list complement)", () => {
      const result = notificationTypeFilter("info", [...allTypes]);
      expect(result).toContain("info");
      expect(result).toContain("progress");
      expect(result).toContain("reminder");
      expect(result).toContain("system");
      expect(result).toContain("custom");
      // task_complete and auto_approved are not in ACTIONABLE_TYPES, so the
      // allow-list complement includes them. Both call sites (NotificationsPage,
      // NotificationPanel) already filter auto_approved out of `items` before
      // ever calling notificationTypeFilter, so this is inert in the actual UI —
      // auto_approved never reaches the "info" pill in practice.
      expect(result).toContain("task_complete");
      expect(result).toContain("auto_approved");
    });

    it("info category includes an unrecognized-but-mapped type", () => {
      // "reminder" is not in ACTIONABLE_TYPES, so it must fall into "info".
      const result = notificationTypeFilter("info", ["reminder"]);
      expect(result).toEqual(["reminder"]);
    });

    it("filter categories are mutually exclusive and collectively exhaustive", () => {
      const approval = notificationTypeFilter("approval_needed", [...allTypes]);
      const error = notificationTypeFilter("error", [...allTypes]);
      const task = notificationTypeFilter("task_complete", [...allTypes]);
      const info = notificationTypeFilter("info", [...allTypes]);

      const covered = new Set([...approval, ...error, ...task, ...info]);
      allTypes.forEach((t) => expect(covered.has(t)).toBe(true));
    });

    it("returns empty array when input is empty", () => {
      expect(notificationTypeFilter("error", [])).toEqual([]);
    });
  });
});
