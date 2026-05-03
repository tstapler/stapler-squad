import { OmnibarAction } from "./types";
import { OmnibarSessionData } from "@/components/sessions/Omnibar";
import { ThemeName } from "@/lib/contexts/ThemeContext";

export interface ActionDeps {
  navigate: (sessionId: string) => void;
  createSession: (data: OmnibarSessionData) => Promise<void>;
  pauseSession: (id: string) => Promise<void>;
  resumeSession: (id: string) => Promise<void>;
  deleteSession: (id: string) => Promise<void>;
  close: () => void;
  setTheme: (name: ThemeName) => void;
}

export function dispatchOmnibarAction(
  action: OmnibarAction,
  deps: ActionDeps
): void {
  switch (action.type) {
    case "navigate_session":
      deps.navigate(action.sessionId);
      deps.close();
      return;
    case "create_session": {
      const isOneOff = action.sessionType === "one_off";
      void deps.createSession({
        title: action.title ?? "",
        path: action.path,
        sessionType: isOneOff ? undefined : action.sessionType as "directory" | "new_worktree" | "existing_worktree",
        branch: action.branch,
        program: action.program ?? "",
        autoYes: false,
        oneOff: isOneOff,
      });
      deps.close();
      return;
    }
    case "clone_session":
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
      void deps.pauseSession(action.sessionId);
      deps.close();
      return;
    case "resume_session":
      void deps.resumeSession(action.sessionId);
      deps.close();
      return;
    case "delete_session":
      void deps.deleteSession(action.sessionId);
      deps.close();
      return;
    case "set_theme":
      deps.setTheme(action.themeName);
      deps.close();
      return;
    // TypeScript exhaustiveness: adding a new OmnibarAction variant without a case → compile error ✅
  }
}
