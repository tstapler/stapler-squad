"use strict";

const { RuleTester } = require("eslint");
const rule = require("../require-rpc-analytics");

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    parserOptions: { ecmaFeatures: { jsx: true } },
    sourceType: "module",
  },
});

ruleTester.run("require-rpc-analytics", rule, {
  valid: [
    // component calling createSession with track() in same function
    {
      filename: "/home/user/project/src/components/sessions/CreateSession.tsx",
      code: `
        function CreateSessionButton() {
          const { track } = useAnalytics();
          function handleClick() {
            track({ name: "session_created", category: "user_action" });
            createSession({ path: "/tmp" });
          }
          return null;
        }
      `,
    },
    // track() inside a useCallback within the same component
    {
      filename: "/home/user/project/src/components/sessions/CreateSession.tsx",
      code: `
        function CreateSessionButton() {
          const { track } = useAnalytics();
          const handleClick = useCallback(() => {
            track({ name: "session_created", category: "user_action" });
            createSession({ path: "/tmp" });
          }, [track]);
          return null;
        }
      `,
    },
    // file in lib/contexts/ — skipped entirely
    {
      filename: "/home/user/project/src/lib/contexts/OmnibarContext.tsx",
      code: `
        function OmnibarProvider() {
          function handleCreate() {
            createSession({ path: "/tmp" });
          }
          return null;
        }
      `,
    },
    // file in lib/hooks/ — skipped entirely
    {
      filename: "/home/user/project/src/lib/hooks/useSessionService.ts",
      code: `
        function useSessionActions() {
          function create(opts) {
            createSession(opts);
          }
          return { create };
        }
      `,
    },
    // analytics-exempt comment
    {
      filename: "/home/user/project/src/components/sessions/SessionList.tsx",
      code: `
        function SessionList() {
          // analytics-exempt
          listSessions();
          return null;
        }
      `,
    },
    // RPC not in a React component (lowercase function name) — not enforced
    {
      filename: "/home/user/project/src/utils/sessionUtils.ts",
      code: `
        function fetchSessions() {
          listSessions();
        }
      `,
    },
  ],

  invalid: [
    // component calling createSession without track()
    {
      filename: "/home/user/project/src/components/sessions/CreateSession.tsx",
      code: `
        function CreateSessionButton() {
          function handleClick() {
            createSession({ path: "/tmp" });
          }
          return null;
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // component calling deleteSession without track()
    {
      filename: "/home/user/project/src/components/sessions/DeleteSession.tsx",
      code: `
        function DeleteSessionButton() {
          function handleDelete() {
            deleteSession("session-id");
          }
          return null;
        }
      `,
      errors: [{ messageId: "missingTrack" }],
    },
    // arrow function component calling pauseSession without track()
    {
      filename: "/home/user/project/src/components/sessions/PauseButton.tsx",
      code: `
        const PauseButton = () => {
          pauseSession("session-id");
          return null;
        };
      `,
      errors: [{ messageId: "missingTrack" }],
    },
  ],
});
