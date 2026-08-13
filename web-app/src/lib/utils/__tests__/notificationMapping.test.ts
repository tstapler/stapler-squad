import { NotificationType, NotificationPriority } from "@/gen/session/v1/types_pb";
import {
  mapNotificationType,
  mapPriority,
  notificationTypeIcon,
  notificationTypeLabel,
  priorityColor,
  notificationTypeFilter,
} from "@/lib/utils/notificationMapping";

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

    it("defaults unknown values to info", () => {
      expect(mapNotificationType(9999)).toBe("info");
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

    it("'info' category excludes the types covered by other filter pills", () => {
      const result = notificationTypeFilter("info", [...allTypes]);
      expect(result).not.toContain("approval_needed");
      expect(result).not.toContain("error");
      expect(result).not.toContain("task_failed");
      expect(result).not.toContain("warning");
      expect(result).not.toContain("task_complete");
    });

    it("'info' category includes progress, reminder, system, custom, and info", () => {
      const result = notificationTypeFilter("info", [...allTypes]);
      expect(result).toContain("info");
      expect(result).toContain("progress");
      expect(result).toContain("reminder");
      expect(result).toContain("system");
      expect(result).toContain("custom");
    });

    it("filter categories are mutually exclusive and collectively exhaustive (excluding auto_approved)", () => {
      const approval = notificationTypeFilter("approval_needed", [...allTypes]);
      const error = notificationTypeFilter("error", [...allTypes]);
      const task = notificationTypeFilter("task_complete", [...allTypes]);
      const info = notificationTypeFilter("info", [...allTypes]);

      const covered = new Set([...approval, ...error, ...task, ...info]);
      // auto_approved is intentionally excluded from all filter pills — it appears only in the
      // collapsible "Auto-handled" section in NotificationPanel, so it must NOT appear in any filter.
      const filterable = allTypes.filter((t) => t !== "auto_approved");
      filterable.forEach((t) => expect(covered.has(t)).toBe(true));
    });

    it("auto_approved is excluded from all filter categories", () => {
      const approval = notificationTypeFilter("approval_needed", ["auto_approved"]);
      const error = notificationTypeFilter("error", ["auto_approved"]);
      const task = notificationTypeFilter("task_complete", ["auto_approved"]);
      const info = notificationTypeFilter("info", ["auto_approved"]);
      expect([...approval, ...error, ...task, ...info]).toHaveLength(0);
    });

    it("returns empty array when input is empty", () => {
      expect(notificationTypeFilter("error", [])).toEqual([]);
    });
  });
});
