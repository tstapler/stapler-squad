module.exports = {
  projects: [
    {
      // Main project: TypeScript source under src/
      displayName: 'web-app',
      preset: 'ts-jest',
      testEnvironment: 'jsdom',
      roots: ['<rootDir>/src'],
      testMatch: ['**/__tests__/**/*.ts?(x)', '**/?(*.)+(spec|test).ts?(x)'],
      moduleNameMapper: {
        '^@/gen/session/v1/session_pb$': '<rootDir>/src/__mocks__/session_pb.js',
        // CSS matchers must precede the catch-all @/ alias so that
        // @/styles/theme.css is mocked before ts-jest can load theme.css.ts
        // and call createTheme() outside a vanilla-extract file scope.
        '\\.module\\.css$': 'identity-obj-proxy',
        '\\.css$': '<rootDir>/src/__mocks__/styleMock.js',
        '^@/(.*)$': '<rootDir>/src/$1',
      },
      setupFilesAfterEnv: ['<rootDir>/jest.setup.js'],
      transform: {
        '^.+\\.[tj]sx?$': [
          'ts-jest',
          {
            tsconfig: {
              jsx: 'react-jsx',
              module: 'commonjs',
              moduleResolution: 'node',
              esModuleInterop: true,
            },
          },
        ],
      },
    },
    {
      // ESLint plugin project: plain CJS .js tests (no TypeScript, no jsdom)
      displayName: 'eslint-plugin-analytics',
      testEnvironment: 'node',
      roots: ['<rootDir>/eslint-plugin-analytics'],
      testMatch: ['**/__tests__/**/*.js', '**/?(*.)+(spec|test).js'],
      transform: {},
    },
  ],
};
