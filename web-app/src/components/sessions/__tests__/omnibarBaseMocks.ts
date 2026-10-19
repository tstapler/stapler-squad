/**
 * Side-effect import: registers the baseline Omnibar module mocks. Import it BEFORE the
 * component under test; jest.doMock is not hoisted, so import order makes it take effect.
 * `@/lib/hooks/useAliasSuggestions` is left to each test because its mock variant differs.
 */
/* eslint-disable @typescript-eslint/no-require-imports */
const fixtures = () => require("./omnibarTestFixtures");

jest.doMock("next/navigation", () => fixtures().mockNextNavigationModule());
jest.doMock("@/lib/contexts/ThemeContext", () => fixtures().mockThemeContextModule());
jest.doMock("@/lib/config", () => fixtures().mockConfigModule());
jest.doMock("@/lib/hooks/usePathCompletions", () => fixtures().mockUsePathCompletionsModule());
jest.doMock("@/lib/hooks/usePathHistory", () => fixtures().mockUsePathHistoryModule());
jest.doMock("@/lib/hooks/useSessionSearch", () => fixtures().mockUseSessionSearchModule());
jest.doMock("@/lib/hooks/useWorktreeSuggestions", () => fixtures().mockUseWorktreeSuggestionsModule());
jest.doMock("@/lib/hooks/useAliases", () => fixtures().mockUseAliasesModule());
jest.doMock("@/lib/hooks/useAtCommandSuggestions", () => fixtures().mockUseAtCommandSuggestionsModule());
jest.doMock("@/lib/hooks/useAvailablePrograms", () => fixtures().mockUseAvailableProgramsModule());
jest.doMock("@/lib/hooks/useSlashCommands", () => fixtures().mockUseSlashCommandsModule());
jest.doMock("@/lib/hooks/useSlashCommandSuggestions", () => fixtures().mockUseSlashCommandSuggestionsModule());
jest.doMock("@/lib/store", () => fixtures().mockStoreModule());
jest.doMock("@/lib/store/sessionsSlice", () => fixtures().mockSessionsSliceModule());
jest.doMock("@/components/sessions/OmnibarResultList", () => fixtures().mockOmnibarResultListModule());
jest.doMock("@/lib/api/transport", () => fixtures().mockApiTransportModule());
