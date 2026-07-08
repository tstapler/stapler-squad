/** @type {import('jest').Config} */
module.exports = {
  projects: [
    {
      displayName: "web-app",
      preset: "ts-jest",
      testEnvironment: "jest-environment-jsdom",
      roots: ["<rootDir>/src"],
      testMatch: ["**/__tests__/**/*.ts?(x)", "**/?(*.)+(spec|test).ts?(x)"],
      moduleNameMapper: {
        // vanilla-extract .css.ts files — return callable proxy
        "\\.css\\.ts$": "<rootDir>/src/__mocks__/styleMock.js",
        "\\.module\\.css$": "identity-obj-proxy",
        "\\.css$": "<rootDir>/src/__mocks__/styleMock.js",
        // Path alias
        "^@/(.*)$": "<rootDir>/src/$1",
      },
      setupFilesAfterEnv: ["<rootDir>/jest.setup.js"],
      transform: {
        "^.+\\.[tj]sx?$": [
          "ts-jest",
          {
            tsconfig: {
              jsx: "react-jsx",
              module: "commonjs",
              moduleResolution: "node",
              esModuleInterop: true,
            },
          },
        ],
      },
      // react-markdown pulls in a deep tree of ESM-only unified/remark/rehype/
      // mdast/hast transitive deps (new one surfaces every level down). Rather
      // than hand-maintain an ever-growing allowlist of name prefixes against
      // pnpm's ".pnpm/<pkg>@<version>/node_modules/<pkg>" store layout, just
      // transform all of node_modules — ts-jest handles CJS and ESM alike.
      transformIgnorePatterns: [],
    },
    {
      displayName: "eslint-plugin-analytics",
      testEnvironment: "node",
      roots: ["<rootDir>/eslint-plugin-analytics"],
      testMatch: ["**/__tests__/**/*.js", "**/?(*.)+(spec|test).js"],
      transform: {},
    },
  ],
};
