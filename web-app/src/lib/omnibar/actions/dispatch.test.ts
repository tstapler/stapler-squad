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
});
