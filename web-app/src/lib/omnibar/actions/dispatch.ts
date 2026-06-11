import { OmnibarAction } from "./types";
import { OmnibarSessionData } from "@/components/sessions/Omnibar";
import { ThemeName } from "@/lib/contexts/ThemeContext";
import type { AnalyticsProvider } from "@/lib/analytics/types";

export interface ActionDeps {
  navigate: (sessionId: string) => void;
  createSession: (data: OmnibarSessionData) => Promise<void>;
  pauseSession: (id: string) => Promise<void>;
  resumeSession: (id: string) => Promise<void>;
  deleteSession: (id: string) => Promise<void>;
  close: () => void;
  setTheme: (name: ThemeName) => void;
  spawnShell?: (sessionId?: string, workingDir?: string, shellCommand?: string) => void;
  /** Optional analytics provider — tracking is best-effort; missing it never blocks the action */
  analytics?: Pick<AnalyticsProvider, "track">;
}

export function dispatchOmnibarAction(
  action: OmnibarAction,
  deps: ActionDeps
): void {
  /** Convenience: call track if analytics dep is provided — best-effort, never throws */
  const track = deps.analytics ? deps.analytics.track.bind(deps.analytics) : null;

  switch (action.type) {
    case "navigate_session":
      if (track) track({ name: "omnibar.navigate_session", category: "user_action" });
      deps.navigate(action.sessionId);
      deps.close();
      return;
    case "create_session": {
      const isOneOff = action.sessionType === "one_off";
      const isAutonomous = action.sessionType === "autonomous";
      if (track) track({ name: "omnibar.create_session", category: "user_action", labels: { sessionType: action.sessionType } });
      void deps.createSession({
        title: action.title ?? "",
        path: action.path,
        sessionType: (isOneOff || isAutonomous) ? undefined : action.sessionType as "directory" | "new_worktree" | "existing_worktree",
        branch: action.branch,
        program: action.program ?? "",
        autoYes: false,
        oneOff: isOneOff ? true : undefined,
        autonomousMode: isAutonomous ? true : undefined,
        permissionMode: isAutonomous ? "auto" : undefined,
      });
      deps.close();
      return;
    }
    case "clone_session":
      if (track) track({ name: "omnibar.clone_session", category: "user_action" });
      void deps.createSession({
        title: `${action.label} (clone)`,
        path: action.sourcePath,
        program: action.sourceProgram,
        sessionType: "new_worktree",
        autoYes: false,
      });
      deps.close();
      return;
    case "pause_session":
      if (track) track({ name: "omnibar.pause_session", category: "user_action" });
      void deps.pauseSession(action.sessionId);
      deps.close();
      return;
    case "resume_session":
      if (track) track({ name: "omnibar.resume_session", category: "user_action" });
      void deps.resumeSession(action.sessionId);
      deps.close();
      return;
    case "delete_session":
      if (track) track({ name: "omnibar.delete_session", category: "user_action" });
      void deps.deleteSession(action.sessionId);
      deps.close();
      return;
    case "set_theme":
      if (track) track({ name: "omnibar.set_theme", category: "user_action" });
      deps.setTheme(action.themeName);
      deps.close();
      return;
    case "spawn_shell":
      if (track) track({ name: "omnibar.spawn_shell", category: "user_action" });
      deps.spawnShell?.(action.sessionId, action.workingDir, action.shellCommand);
      deps.close();
      return;
    case "auto_fix":
      if (track) track({ name: "omnibar.auto_fix", category: "user_action" });
      void deps.createSession({
        title: action.title,
        path: "",
        sessionType: undefined,
        program: action.program ?? "",
        autoYes: false,
        autonomousMode: true,
        permissionMode: "auto",
      });
      deps.close();
      return;
    // TypeScript exhaustiveness: adding a new OmnibarAction variant without a case → compile error ✅
  }
}
