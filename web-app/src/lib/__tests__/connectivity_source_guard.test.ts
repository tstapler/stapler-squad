// T-OF-04: one connectivity source. Notification code reads the shared hook and never
// navigator.onLine, online/offline events or a second connection reader (TE-7).
import fs from "fs";
import path from "path";

const SRC = path.join(process.cwd(), "src");
const FILES = [
  "components/ui/NotificationPanel.tsx",
  "components/ui/ToastStack.tsx",
  "components/ui/NotificationToast.tsx",
  "components/ui/NotificationItem.tsx",
  "components/ui/BackgroundActivity.tsx",
  "components/ui/TrayList.tsx",
  "components/ui/TrayHandle.tsx",
  "components/ui/trayState.ts",
  "lib/contexts/NotificationContext.tsx",
  "lib/hooks/useTrayBulkActions.ts",
  "lib/hooks/useBackgroundSessions.ts",
];

describe("notification connectivity source", () => {
  it.each(FILES)("%s has no private connectivity reader", (file) => {
    const code = fs.readFileSync(path.join(SRC, file), "utf8").replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");
    expect(code).not.toMatch(/navigator\.onLine|addEventListener\(\s*["'](?:online|offline)["']|selectConnectionState|new (?:WebSocket|EventSource)\(/);
  });
});
