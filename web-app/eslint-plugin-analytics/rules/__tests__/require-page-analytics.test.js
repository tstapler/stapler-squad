"use strict";

const { RuleTester } = require("eslint");
const rule = require("../require-page-analytics");

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2020,
    parserOptions: { ecmaFeatures: { jsx: true } },
    sourceType: "module",
  },
});

ruleTester.run("require-page-analytics", rule, {
  valid: [
    // page with usePageView()
    {
      filename: "/home/user/project/src/app/sessions/page.tsx",
      code: `
        import { usePageView } from "@/lib/analytics";
        export default function SessionsPage() {
          usePageView();
          return null;
        }
      `,
    },
    // page with useAnalytics().track("page_view", ...)
    {
      filename: "/home/user/project/src/app/sessions/page.tsx",
      code: `
        export default function SessionsPage() {
          const analytics = useAnalytics();
          analytics.track("page_view");
          return null;
        }
      `,
    },
    // non-page file — rule does not apply
    {
      filename: "/home/user/project/src/components/sessions/SessionList.tsx",
      code: `
        export default function SessionList() {
          return null;
        }
      `,
    },
    // file-level analytics-exempt comment
    {
      filename: "/home/user/project/src/app/test/page.tsx",
      code: `
        // analytics-exempt
        export default function TestPage() {
          return null;
        }
      `,
    },
    // non-app page path — rule does not apply
    {
      filename: "/home/user/project/pages/index.tsx",
      code: `
        export default function HomePage() {
          return null;
        }
      `,
    },
  ],

  invalid: [
    // page without pageView or track
    {
      filename: "/home/user/project/src/app/sessions/page.tsx",
      code: `
        export default function SessionsPage() {
          return null;
        }
      `,
      errors: [{ messageId: "missingPageView" }],
    },
    // page with analytics but not page_view
    {
      filename: "/home/user/project/src/app/settings/page.tsx",
      code: `
        export default function SettingsPage() {
          const analytics = useAnalytics();
          analytics.track("button_click");
          return null;
        }
      `,
      errors: [{ messageId: "missingPageView" }],
    },
    // nested page path
    {
      filename: "/home/user/project/src/app/sessions/detail/page.tsx",
      code: `
        export default function SessionDetailPage() {
          return null;
        }
      `,
      errors: [{ messageId: "missingPageView" }],
    },
  ],
});
