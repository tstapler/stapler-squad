import { dispatchOmnibarAction, ActionDeps } from "./dispatch";
import { OmnibarAction } from "./types";

function makeDeps(): jest.Mocked<ActionDeps> {
  return {
    navigate: jest.fn(),
    createSession: jest.fn().mockResolvedValue(undefined),
    pauseSession: jest.fn().mockResolvedValue(undefined),
    resumeSession: jest.fn().mockResolvedValue(undefined),
    deleteSession: jest.fn().mockResolvedValue(undefined),
    close: jest.fn(),
    setTheme: jest.fn(),
    spawnShell: jest.fn(),
  };
}

describe("dispatchOmnibarAction", () => {
  describe("navigate_session", () => {
    it("dispatchOmnibarAction_should_callNavigate_When_navigateSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "navigate_session", sessionId: "s1", label: "Session 1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.navigate).toHaveBeenCalledWith("s1");
    });

    it("dispatchOmnibarAction_should_callClose_When_navigateSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "navigate_session", sessionId: "s1", label: "Session 1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("create_session", () => {
    it("dispatchOmnibarAction_should_callCreateSession_When_createSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = {
        type: "create_session",
        path: "/home/user/repo",
        sessionType: "directory",
        title: "My Session",
        program: "claude",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.createSession).toHaveBeenCalledWith(
        expect.objectContaining({ path: "/home/user/repo", sessionType: "directory" })
      );
      expect(deps.close).toHaveBeenCalled();
    });

    it("dispatchOmnibarAction_should_passEmptyProgram_When_programNotSpecified", () => {
      const deps = makeDeps();
      const action: OmnibarAction = {
        type: "create_session",
        path: "/home/user/repo",
        sessionType: "directory",
        title: "My Session",
        // program intentionally omitted - backend should use config default
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.createSession).toHaveBeenCalledWith(
        expect.objectContaining({ program: "" })
      );
    });
  });

  describe("create_session (one-off)", () => {
    it("dispatchOmnibarAction_should_setOneOffTrue_When_sessionTypeIsOneOff", () => {
      const deps = makeDeps();
      const action: OmnibarAction = {
        type: "create_session",
        path: "",
        sessionType: "one_off",
        title: "scratch session",
        program: "claude",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.createSession).toHaveBeenCalledWith(
        expect.objectContaining({ oneOff: true, sessionType: undefined })
      );
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("clone_session", () => {
    it("dispatchOmnibarAction_should_callCreateSession_When_cloneSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = {
        type: "clone_session",
        sourceSessionId: "s1",
        sourcePath: "/home/user/repo",
        sourceProgram: "claude",
        label: "My Session",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.createSession).toHaveBeenCalledWith(
        expect.objectContaining({
          path: "/home/user/repo",
          program: "claude",
          sessionType: "new_worktree",
        })
      );
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("pause_session", () => {
    it("dispatchOmnibarAction_should_callPauseSession_When_pauseSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "pause_session", sessionId: "s1", label: "Session 1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.pauseSession).toHaveBeenCalledWith("s1");
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("resume_session", () => {
    it("dispatchOmnibarAction_should_callResumeSession_When_resumeSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "resume_session", sessionId: "s1", label: "Session 1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.resumeSession).toHaveBeenCalledWith("s1");
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("delete_session", () => {
    it("dispatchOmnibarAction_should_callDeleteSession_When_deleteSessionAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "delete_session", sessionId: "s1", label: "Session 1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.deleteSession).toHaveBeenCalledWith("s1");
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("set_theme", () => {
    it("dispatchOmnibarAction_should_callSetTheme_When_setThemeAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "set_theme", themeName: "matrix" };
      dispatchOmnibarAction(action, deps);
      expect(deps.setTheme).toHaveBeenCalledWith("matrix");
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("spawn_shell", () => {
    it("dispatchOmnibarAction_should_callSpawnShell_When_spawnShellAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = {
        type: "spawn_shell",
        sessionId: "s1",
        workingDir: "/home/user/repo",
        shellCommand: "bash",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.spawnShell).toHaveBeenCalledWith("s1", "/home/user/repo", "bash");
    });

    it("dispatchOmnibarAction_should_callClose_When_spawnShellAction", () => {
      const deps = makeDeps();
      const action: OmnibarAction = { type: "spawn_shell", sessionId: "s1" };
      dispatchOmnibarAction(action, deps);
      expect(deps.close).toHaveBeenCalled();
    });
  });

  describe("run_workflow", () => {
    it("dispatchOmnibarAction_should_callRunWorkflow_When_runWorkflowAction", () => {
      const deps = makeDeps();
      deps.runWorkflow = jest.fn();
      const action: OmnibarAction = {
        type: "run_workflow",
        workflowSlug: "my-workflow",
        workflowArg: "some arg",
        label: "My Workflow",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.runWorkflow).toHaveBeenCalledWith("my-workflow", "some arg");
    });

    it("dispatchOmnibarAction_should_callAnalyticsTrack_When_runWorkflowAction", () => {
      const deps = makeDeps();
      deps.runWorkflow = jest.fn();
      deps.analytics = { track: jest.fn() };
      const action: OmnibarAction = {
        type: "run_workflow",
        workflowSlug: "daily-standup",
        workflowArg: "",
        label: "Daily Standup",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.analytics.track).toHaveBeenCalledWith(
        expect.objectContaining({ name: "omnibar.run_workflow", labels: expect.objectContaining({ slug: "daily-standup" }) })
      );
    });

    it("dispatchOmnibarAction_should_callClose_When_runWorkflowAction", () => {
      const deps = makeDeps();
      deps.runWorkflow = jest.fn();
      const action: OmnibarAction = {
        type: "run_workflow",
        workflowSlug: "my-workflow",
        workflowArg: "",
        label: "My Workflow",
      };
      dispatchOmnibarAction(action, deps);
      expect(deps.close).toHaveBeenCalled();
    });

    it("dispatchOmnibarAction_should_noOpRunWorkflow_When_runWorkflowDepAbsent", () => {
      const deps = makeDeps();
      // runWorkflow dep intentionally absent (not set in makeDeps)
      const action: OmnibarAction = {
        type: "run_workflow",
        workflowSlug: "my-workflow",
        workflowArg: "",
        label: "My Workflow",
      };
      // Should not throw even with missing runWorkflow dep
      expect(() => dispatchOmnibarAction(action, deps)).not.toThrow();
      expect(deps.close).toHaveBeenCalled();
    });
  });
});
