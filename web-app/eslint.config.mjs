// Flat config; replaces .eslintrc.json. next/core-web-vitals is expanded into explicit plugins
// (docs/reference/eslint-rules-from-next.md). Scope is src/** only, as `next lint` covered.
import reactPlugin from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import jsxA11y from "eslint-plugin-jsx-a11y";
import importPlugin from "eslint-plugin-import";
import tsPlugin from "@typescript-eslint/eslint-plugin";
import tsParser from "@typescript-eslint/parser";
import boundaries from "eslint-plugin-boundaries";
import analytics from "eslint-plugin-analytics";
import rpcLifecycle from "eslint-plugin-rpc-lifecycle";
import globals from "globals";

const sharedTransportMessage =
  "Build unary transports only in lib/api/transport.ts (getConnectTransport / getKeepaliveConnectTransport) or lib/transport/*, so every caller shares the auth, timing and in-flight-gauge interceptors and one connection pool.";

const connectTransportImportBan = {
  name: "@connectrpc/connect-web",
  importNames: ["createConnectTransport"],
  message: sharedTransportMessage,
};

const dynamicConnectWebImport = {
  selector: "ImportExpression[source.value='@connectrpc/connect-web']",
  message: sharedTransportMessage,
};

export default [
  {
    files: ["src/**/*.{js,jsx,mjs,cjs,ts,tsx,mts,cts}"],
    languageOptions: {
      parser: tsParser,
      parserOptions: { sourceType: "module", ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, ...globals.node },
    },
    // eslintrc mode never reported unused disables; keep parity so the before/after diff stays empty.
    linterOptions: { reportUnusedDisableDirectives: "off" },
    plugins: {
      react: reactPlugin,
      "react-hooks": reactHooks,
      "jsx-a11y": jsxA11y,
      import: importPlugin,
      "@typescript-eslint": tsPlugin,
      boundaries,
      analytics,
      "rpc-lifecycle": rpcLifecycle,
      // No-op stub: six `eslint-disable @next/next/no-img-element` comments remain in src/. Removing
      // them would put their files' pre-existing warnings under the changed-files gate. Delete both.
      "@next/next": { rules: { "no-img-element": { meta: { schema: [] }, create: () => ({}) } } },
    },
    settings: {
      react: { version: "detect" },
      // Without a resolver `boundaries` cannot classify extensionless or "@/" imports and silently passes them.
      "import/parsers": { "@typescript-eslint/parser": [".ts", ".mts", ".cts", ".tsx", ".d.ts"] },
      "import/resolver": {
        node: { extensions: [".js", ".jsx", ".ts", ".tsx"] },
        typescript: { alwaysTryTypes: true },
      },
      "boundaries/elements": [
        { type: "app", pattern: "src/app/**" },
        { type: "layout", pattern: "src/components/layout/**" },
        { type: "providers", pattern: "src/components/providers/**" },
        { type: "sessions", pattern: "src/components/sessions/**" },
        { type: "unfinished", pattern: "src/components/unfinished/**" },
        { type: "history", pattern: "src/components/history/**" },
        { type: "logs", pattern: "src/components/logs/**" },
        { type: "settings", pattern: "src/components/settings/**" },
        { type: "telemetry", pattern: "src/components/telemetry/**" },
        { type: "ui", pattern: "src/components/ui/**" },
        {
          type: "shared-vcs-widget",
          mode: "full",
          pattern: ["src/components/shared/vcs-widget/**", "src/components/shared/VcsWidget.*"],
        },
        { type: "shared", pattern: "src/components/shared/**" },
        { type: "lib", pattern: "src/lib/**" },
        { type: "gen", pattern: "src/gen/**" },
      ],
      "boundaries/ignore": ["**/*.test.*", "**/*.spec.*", "src/lib/__tests__/**", "src/lib/test-generators/**"],
    },
    rules: {
      ...reactPlugin.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      "import/no-anonymous-default-export": "warn",
      "react/no-unknown-property": "off",
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "react/jsx-no-target-blank": "off",
      "jsx-a11y/alt-text": ["warn", { elements: ["img"], img: ["Image"] }],
      "jsx-a11y/aria-props": "warn",
      "jsx-a11y/aria-proptypes": "warn",
      "jsx-a11y/aria-unsupported-elements": "warn",
      "jsx-a11y/role-has-required-aria-props": "warn",
      "jsx-a11y/role-supports-aria-props": "warn",
      "react/jsx-max-props-per-line": "off",
      "no-restricted-syntax": [
        "error",
        dynamicConnectWebImport,
        {
          selector: "Literal[value='100vh']",
          message: "Use var(--viewport-height) instead of '100vh'. iOS virtual keyboard shrinks the visual viewport but not 100vh, hiding content behind the keyboard. ViewportProvider updates --viewport-height via the visualViewport API. See project_plans/ux-overhaul/.",
        },
        {
          selector: "TemplateElement[value.raw=/\\b100vh\\b/]",
          message: "Use var(--viewport-height) instead of '100vh' in template literals. See project_plans/ux-overhaul/.",
        },
        {
          selector: "Literal[value='100dvh']",
          message: "Use var(--viewport-height, 100dvh) instead of bare '100dvh'. dvh alone bypasses ViewportProvider's iOS keyboard handling (visualViewport API). The CSS variable is the single source of truth for both Android URL-bar and iOS keyboard shrinkage.",
        },
        {
          selector: "TemplateElement[value.raw=/\\b100dvh\\b/]",
          message: "Use var(--viewport-height, 100dvh) instead of bare '100dvh' in template literals. See previous note on ViewportProvider.",
        },
        {
          selector: "Literal[value='100lvh']",
          message: "Do not use '100lvh' — it equals the large (URL-bar-hidden) viewport height, the same problematic behaviour as '100vh' on Android Chrome. Use var(--viewport-height, 100dvh) instead.",
        },
        {
          selector: "TemplateElement[value.raw=/\\b100lvh\\b/]",
          message: "Do not use '100lvh' in template literals — same issue as '100vh' on Android Chrome. Use var(--viewport-height, 100dvh).",
        },
        {
          selector: "JSXAttribute[name.name='style'] > JSXExpressionContainer > ObjectExpression > Property > Literal[value=/^#[0-9a-fA-F]{3,8}$/]",
          message: "Use vars.color.* tokens from theme.css.ts instead of hardcoded hex values in style props.",
        },
        {
          selector: "JSXAttribute[name.name='style'] > JSXExpressionContainer > ObjectExpression > Property[key.name=/^(flexDirection|flexWrap|display|alignItems|alignSelf|justifyContent|justifySelf|order|flex|float|clear|gridColumn|gridRow|gridArea)$/]",
          message: "Avoid inline style for layout-managed properties. Use a data-* attribute + selectors in the matching .css.ts file so CSS cascade and theme overrides can win. See docs/reference/css-architecture.md.",
        },
        {
          selector: "ImportDeclaration[source.value='@/lib/telemetry']",
          message: "Use useAnalytics().track() instead of the legacy track() from lib/telemetry.",
        },
      ],
      "analytics/require-on-click": "warn",
      "analytics/require-omnibar-dispatch": "error",
      "analytics/require-page-analytics": "error",
      "analytics/require-rpc-analytics": "error",
      "rpc-lifecycle/require-abort-signal": "error",
      "boundaries/dependencies": [
        "error",
        {
          default: "disallow",
          rules: [
            {
              from: "app",
              allow: [
                "layout",
                "providers",
                "sessions",
                "unfinished",
                "history",
                "logs",
                "settings",
                "telemetry",
                "ui",
                "shared-vcs-widget",
                "shared",
                "lib",
                "gen",
              ],
            },
            {
              from: "layout",
              allow: [
                "providers",
                "sessions",
                "unfinished",
                "history",
                "logs",
                "settings",
                "telemetry",
                "ui",
                "shared-vcs-widget",
                "shared",
                "lib",
                "gen",
              ],
            },
            { from: "providers", allow: ["ui", "shared-vcs-widget", "shared", "lib", "gen"] },
            {
              from: ["sessions", "unfinished", "history", "logs", "settings", "telemetry"],
              allow: ["providers", "ui", "shared-vcs-widget", "shared", "lib", "gen"],
            },
            { from: "ui", allow: ["lib", "gen"] },
            { from: "shared-vcs-widget", allow: ["shared-vcs-widget", "ui", "lib", "gen"] },
            { from: "shared", allow: ["lib", "gen"] },
            { from: "lib", allow: ["ui", "sessions", "gen"] },
            { from: "gen", allow: [] },
          ],
        },
      ],
    },
  },
  {
    files: [
      "src/components/sessions/**/*.ts",
      "src/components/sessions/**/*.tsx",
      "src/lib/files/**/*.ts",
      "src/lib/files/**/*.tsx",
    ],
    ignores: ["src/lib/files/analytics/**", "src/**/__tests__/**", "src/**/*.test.ts", "src/**/*.test.tsx"],
    rules: { "analytics/no-direct-files-track": "error" },
  },
  {
    files: ["src/**/*.ts", "src/**/*.tsx"],
    rules: {
      "@typescript-eslint/no-non-null-assertion": "warn",
      "@typescript-eslint/consistent-type-imports": "warn",
    },
  },
  {
    // Must precede the CodeMirror block below: a later block replaces this rule's options for files both match,
    // so that block repeats connectTransportImportBan.
    files: ["src/**/*.ts", "src/**/*.tsx"],
    ignores: [
      "src/lib/api/transport.ts",
      "src/lib/transport/**",
      "src/components/backlog/backlogItemDetailTestFixtures.ts",
      "src/**/*.test.*",
      "src/**/*.spec.*",
      "src/**/__tests__/**",
    ],
    rules: { "no-restricted-imports": ["error", { paths: [connectTransportImportBan] }] },
  },
  {
    files: [
      "src/components/sessions/**/*.ts",
      "src/components/sessions/**/*.tsx",
      "src/lib/files/**/*.ts",
      "src/lib/files/**/*.tsx",
    ],
    ignores: [
      "src/components/sessions/files/engine/**",
      "src/components/sessions/files/extensions/**",
      "src/**/__tests__/**",
      "src/**/*.test.ts",
      "src/**/*.test.tsx",
      "src/**/*.spike.ts",
    ],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          paths: [connectTransportImportBan],
          patterns: [
            {
              group: ["@codemirror/*", "@replit/codemirror-vim", "@lezer/*", "codemirror"],
              message: "ADR-036: import the EditorEngine/Highlighter/MergeSurface ports from files/engine; CodeMirror stays inside files/engine/ and files/extensions/.",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["src/app/test/**/*.tsx", "src/app/test/**/*.ts"],
    rules: { "no-restricted-syntax": "off" },
  },
  {
    files: ["src/**/*.css.ts"],
    rules: {
      "no-restricted-syntax": [
        "warn",
        {
          selector: "Property > Literal[value=/^#[0-9a-fA-F]{3,8}$/]",
          message: "Hardcoded hex colors are not allowed in .css.ts files. Use vars.color.* tokens from theme-contract.css.ts.",
        },
        {
          selector: "Property[key.name='zIndex'] > Literal[raw=/^[0-9]/]",
          message: "Hardcoded z-index numbers are not allowed. Add a named slot to zIndex in web-app/src/styles/theme-contract.css.ts and reference zIndex.yourSlot. This prevents invisible ordering collisions.",
        },
      ],
    },
  },
  {
    files: ["src/components/**/*.tsx"],
    rules: {
      "no-restricted-syntax": [
        "warn",
        dynamicConnectWebImport,
        {
          selector: "TSInterfaceDeclaration[id.name=/Props$/] > TSInterfaceBody > TSPropertySignature:nth-child(13) ~ TSPropertySignature",
          message: "Component props interface has >12 props. Consider splitting the component or grouping props into a sub-interface.",
        },
      ],
    },
  },
  {
    files: ["src/lib/**/*.ts", "src/lib/**/*.tsx", "src/app/**/*.ts", "src/app/**/*.tsx"],
    ignores: ["src/lib/nav/**"],
    rules: {
      "no-restricted-syntax": [
        "warn",
        dynamicConnectWebImport,
        {
          selector: "Literal[value=/^(Executing|Processing|Needs Approval|Input Required|Idle|Unknown|Success|Error|Tests Failing|Rate Limited|Waiting for Agent)$/]",
          message: "Use DetectedStatus enum from @/gen/session/v1/types_pb instead of raw strings for detected status values.",
        },
      ],
    },
  },
  {
    files: ["src/**/*.ts", "src/**/*.tsx"],
    ignores: ["src/lib/nav/**", "src/**/*.test.*", "src/**/*.spec.*", "src/**/__tests__/**"],
    rules: {
      "no-restricted-properties": [
        "error",
        {
          property: "pushState",
          message: "Raw History calls bypass the router. Use useReplaceSearch, replaceHash or pushTrayEntry from @/lib/nav.",
        },
        {
          property: "replaceState",
          message: "Raw History calls bypass the router. Use useReplaceSearch, replaceHash or pushTrayEntry from @/lib/nav.",
        },
      ],
    },
  },
  {
    files: ["src/**/*.ts", "src/**/*.tsx"],
    ignores: [
      "src/lib/nav/**",
      "src/lib/lazy/**",
      "src/**/*.test.*",
      "src/**/*.spec.*",
      "src/**/__tests__/**",
    ],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["next", "next/*", "react-router", "react-router-dom"],
              allowTypeImports: true,
              message: "Import useRouter, usePathname, useSearchParams, Link and Redirect from @/lib/nav. Rewrite existing imports with: node web-app/scripts/codemods/next-nav-to-facade.mjs <file-or-dir>.",
            },
            {
              group: ["@/lib/nav/entry", "**/nav/entry"],
              message: "The router entry is for src/main.tsx only; app code imports @/lib/nav.",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["src/main.tsx"],
    rules: {
      "@typescript-eslint/no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["next", "next/*", "react-router", "react-router-dom"],
              allowTypeImports: true,
              message: "main.tsx reaches the router only through @/lib/nav/entry.",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["src/**/*.test.ts", "src/**/*.test.tsx", "src/**/__tests__/**"],
    rules: { "no-restricted-syntax": "off" },
  },
];
