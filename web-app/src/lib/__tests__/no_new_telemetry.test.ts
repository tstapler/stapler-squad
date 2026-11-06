// T-OB-05: the notification surfaces add no telemetry beyond the existing audit log (OBS-3).
import fs from "fs";
import path from "path";

const SRC = path.join(process.cwd(), "src");
const FILES = [
  "components/ui/NotificationPanel.tsx",
  "components/ui/ToastStack.tsx",
  "components/ui/NotificationToast.tsx",
  "components/ui/TrayList.tsx",
  "components/ui/TrayHandle.tsx",
  "components/ui/TrayConfirm.tsx",
  "components/ui/TrayOverflowMenu.tsx",
  "components/ui/TraySettings.tsx",
  "components/ui/TrayErrorBoundary.tsx",
  "components/ui/BackgroundActivity.tsx",
  "components/sessions/ReadOnlyBanner.tsx",
  "components/sessions/SessionUnavailableCard.tsx",
  "lib/hooks/useTrayBulkActions.ts",
  "lib/hooks/useTrayFocus.ts",
  "lib/hooks/useTrayDismissal.ts",
  "lib/hooks/useUndoWindow.ts",
  "lib/hooks/useToastTimers.ts",
  "lib/hooks/useBackgroundSessions.ts",
  "lib/hooks/useNotificationRecord.ts",
];

describe("notification surfaces add no telemetry", () => {
  it.each(FILES)("%s calls no analytics, beacon or tracking API", (file) => {
    const code = fs.readFileSync(path.join(SRC, file), "utf8").replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    expect(code).not.toMatch(/useAnalytics|\btrack\(|sendBeacon|gtag\(|posthog|datadogRum|Sentry\./);
  });
});
