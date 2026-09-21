/**
 * Shared jest.mock() registration for SessionCard's heavy dependency mocks.
 *
 * `jest.mock(...)` calls only affect the module registry of the file that
 * calls them, so this can't be a plain helper function — but importing this
 * module as the *first* import in a spec file (before importing SessionCard
 * or anything it transitively pulls in) runs these calls early enough to
 * intercept those requires, without relying on babel-plugin-jest-hoist to
 * hoist them (it only hoists `jest.mock` calls written directly in the
 * importing file). This collapses the ~10-line block that SessionCard.click.test.tsx,
 * SessionCard.dedup-integration.test.tsx, SessionCard.approval-suppression.test.tsx,
 * and SessionCard.pi-health-badge.test.tsx previously each repeated verbatim
 * into a single shared import.
 */

jest.mock("@connectrpc/connect", () => require("./sessionCardTestFixtures").mockConnect());
jest.mock("@connectrpc/connect-web", () => require("./sessionCardTestFixtures").mockConnectWeb());
jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./sessionCardTestFixtures").mockReviewQueueContext());
jest.mock("@/lib/contexts/SessionServiceContext", () => require("./sessionCardTestFixtures").mockSessionServiceContext());
jest.mock("@/lib/store", () => require("./sessionCardTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./sessionCardTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useTerminalSnapshot", () => require("./sessionCardTestFixtures").mockUseTerminalSnapshot());
jest.mock("@/lib/hooks/useFocusTrap", () => require("./sessionCardTestFixtures").mockUseFocusTrap());
jest.mock("@/components/ui/AppLink", () => require("./sessionCardTestFixtures").mockAppLink());
jest.mock("@/components/ui/Modal", () => require("./sessionCardTestFixtures").mockModal());
jest.mock("@/lib/hooks/useSessionActions", () => require("./sessionCardTestFixtures").mockUseSessionActions());
