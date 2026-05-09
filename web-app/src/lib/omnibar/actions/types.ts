export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string; label: string }
  | { type: "create_session"; path: string; sessionType: string; branch?: string; program?: string; title?: string }
  | { type: "clone_session"; sourceSessionId: string; sourcePath: string; sourceProgram: string; label: string }
  | { type: "pause_session"; sessionId: string; label: string }
  | { type: "resume_session"; sessionId: string; label: string }
  | { type: "delete_session"; sessionId: string; label: string };

export type OmnibarActionType = OmnibarAction["type"];
