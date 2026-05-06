/**
 * Core notification domain types.
 *
 * This is the canonical definition. All layers (context, components, hooks)
 * import from here — never from UI components or contexts.
 */

export interface NotificationData {
  id: string;
  sessionId: string;
  sessionName: string;
  title?: string;
  message: string;
  timestamp: number;
  priority?: "urgent" | "high" | "medium" | "low";
  notificationType?: "info" | "approval_needed" | "auto_approved" | "error" | "warning" | "task_complete" | "task_failed" | "progress" | "question" | "reminder" | "system" | "custom";
  /** Source app name (e.g., "IntelliJ IDEA", "Visual Studio Code") */
  sourceApp?: string;
  /** macOS bundle ID for window activation */
  sourceBundleId?: string;
  /** Working directory where the notification originated */
  sourceWorkingDir?: string;
  /** Project name for additional context */
  sourceProject?: string;
  /** Additional metadata key-value pairs */
  metadata?: Record<string, string>;
  onView?: () => void;
  onDismiss?: () => void;
  onFocusWindow?: () => void;
  /**
   * Callback when user clicks "Dismiss" to acknowledge the notification.
   * This should trigger the backend acknowledge API to prevent re-notification.
   */
  onAcknowledge?: () => void;
  /** Called when user approves a pending tool-use request (approval_needed notifications only). */
  onApprove?: () => void;
  /** Called when user denies a pending tool-use request (approval_needed notifications only). */
  onDeny?: () => void;
}

export interface NotificationHistoryItem extends NotificationData {
  isRead: boolean;
  /** Server-provided occurrence count for deduplicated records. 0 means single/unknown (backward compat). */
  occurrenceCount?: number;
}
