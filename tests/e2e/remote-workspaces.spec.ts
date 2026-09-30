// @feature settings-remotes, remote-host-badge, session:create
/**
 * E2E tests for ssh-remote-workspaces (Phase 6 Epic 6.3, Story 6.3.1): add a remote via
 * Settings -> Remotes, test its connection, trust its (unknown, freshly-generated) host key,
 * verify the row shows "Connected" -- then create a session against that remote via the
 * Omnibar's remote selector and verify the resulting session card shows the host badge.
 *
 * Runs against a REAL SSH target (tests/e2e/sshd -- see its doc comment for why and how).
 * Every RPC here is a genuine SSH round trip (dial, host-key exchange, git/tmux exec over a
 * real SSH channel) -- nothing here mocks the SSH transport itself.
 *
 * This spec owns a DEDICATED sshd instance (test.beforeAll/afterAll below), rather than
 * sharing one global instance across the whole `npx playwright test` invocation the way
 * global-setup.ts does for the main app server. Deliberate: KnownHostsStore trust is keyed by
 * host:port, server-side, for the life of the test server -- a shared sshd would mean only the
 * FIRST project to ever dial it (chromium, which runs before chromium-dom by default) sees an
 * unknown host key, and every later project/retry would see an already-trusted host and skip
 * the TOFU dialog this spec exists to exercise. A fresh port + fresh host key per (file,
 * project) run sidesteps that entirely. See helpers/test-sshd.ts's doc comment for the same
 * reasoning from the other side.
 *
 * Scope note: RemoteConnectionIndicator's live disconnect/reconnect state transitions (its
 * badge is fed exclusively by RemoteHealthChangedEvent from a background health prober over
 * the WatchSessions stream, not by any RPC this spec calls directly) are NOT covered here --
 * deterministically forcing that transition in a real e2e run was judged out of this epic's
 * budget; RemoteConnectionIndicator.test.tsx already covers its render logic against a mocked
 * Redux state. See this story's implementation report for the full reasoning.
 */
import { test, expect } from "@playwright/test";
import { execSync } from "child_process";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { RemotesSettingsPage } from "./pages/RemotesSettingsPage";
import { dismissOnboardingIfPresent } from "./pages/OnboardingPage";
import { TestSSHD } from "./helpers/test-sshd";

// Unique per test run (not just per test) so a Playwright retry never collides with a remote
// a still-in-progress prior attempt already created.
const REMOTE_NAME = `e2e-ssh-remote-${Date.now()}`;

let testRepoDir: string;
let sshd: TestSSHD;

test.beforeAll(async () => {
  // A real local git repo — RemoteWorktreeOps.CreateWorktree (session/git/remote_worktree.go)
  // runs `git worktree add` against whatever repo path CreateSession is given, over the
  // SSH exec channel. tests/e2e/sshd execs on this SAME host, so a plain local repo here is
  // exactly what "the repo on the remote host" resolves to — mirroring
  // session_service_remote_test.go's newRemoteSessionFixture/initRemoteSessionTestRepo, which
  // does the identical thing for the Go integration test covering this same code path.
  testRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), "sq-e2e-remote-repo-"));
  execSync("git init -b main", { cwd: testRepoDir });
  execSync('git config user.email "test@example.com"', { cwd: testRepoDir });
  execSync('git config user.name "Test"', { cwd: testRepoDir });
  fs.writeFileSync(path.join(testRepoDir, "README.md"), "e2e remote session fixture\n");
  execSync("git add . && git commit -m init", { cwd: testRepoDir });

  sshd = new TestSSHD();
  await sshd.start();
});

test.afterAll(async () => {
  fs.rmSync(testRepoDir, { recursive: true, force: true });
  await sshd.stop();
});

test.describe("remote-workspaces", () => {
  test("add a remote, test connection, trust the host key, verify Connected status", async ({ page }) => {
    const host = sshd.getHost();
    const port = sshd.getPort();
    const user = sshd.getUser();
    const basePath = sshd.getBasePath();
    const remotes = new RemotesSettingsPage(page);

    await remotes.goto();
    await remotes.clickAddRemote();
    await remotes.fillRemoteForm({ name: REMOTE_NAME, host, user, port, basePath });
    await remotes.submitTestConnection();

    // GenerateRemoteIdentity round-tripped for real (a fresh Ed25519 keypair, not a stub).
    await remotes.waitForIdentityGenerated();

    // tests/e2e/sshd mints a fresh host key every run — TestRemoteConnection always reports
    // host_key_unknown=true here, exercising the real TOFU confirmation dialog end to end
    // (no separate "unknown host key" fixture needed).
    await remotes.trustHostKey();

    // TrustRemoteHostKey succeeding immediately calls CreateRemote and closes the form.
    await remotes.expectRemoteRowVisible(REMOTE_NAME);

    // Row-level "Test" re-dials using the now-trusted key — must succeed for real.
    await remotes.testSavedRemote(REMOTE_NAME);
    await remotes.expectStatus(REMOTE_NAME, "Connected");
  });

  test("creates a session on the configured remote via the Omnibar remote selector and shows the host badge", async ({ page }) => {
    const sessionTitle = `e2e-remote-session-${Date.now()}`;

    await page.goto("/", { waitUntil: "domcontentloaded", timeout: 15000 });
    await dismissOnboardingIfPresent(page);
    await page.keyboard.press("Control+Shift+K");
    await expect(page.getByRole("radiogroup", { name: "Session type" })).toBeVisible({ timeout: 5000 });

    // New Worktree is the default session type and the only one the backend accepts for a
    // remote target (server/services/session_service.go rejects any other type once a remote
    // is selected) — no radio click needed.
    await page.getByLabel("Session source input").fill(testRepoDir);
    // Overwrite whatever title LocalPath detection auto-filled, for a deterministic,
    // collision-free session card to search for below.
    await page.getByRole("textbox", { name: "Session Name" }).fill(sessionTitle);

    await expect(page.getByTestId("remote-selector")).toBeVisible({ timeout: 10000 });
    await page.getByTestId("remote-selector").selectOption(REMOTE_NAME);

    // Two "Create Session"-labeled buttons are in the DOM at once: Omnibar.tsx's own footer
    // shortcut-bar button and OmnibarCreationPanel's form submit button (data-testid'd) --
    // target the testid'd one to avoid a strict-mode ambiguity.
    await expect(page.getByTestId("omnibar-create-session-button")).toBeEnabled({ timeout: 5000 });
    await page.getByTestId("omnibar-create-session-button").click();

    // Filter the sidebar down to just this session — the freshly-created card can land in any
    // grouping bucket (Category/Tag/etc.), so searching by its unique title is more robust than
    // scrolling to find it.
    await page.getByLabel("Search sessions").fill(sessionTitle);

    // The session list's default display mode is "row" (SessionList.tsx's `displayMode` prop),
    // which renders SessionRow.tsx (data-testid="session-row"), NOT SessionCard.tsx
    // (data-testid="session-card", "card" mode only, reached via the list header's view
    // toggle). The host badge lives on the row in both modes.
    const card = page.getByTestId("session-row").filter({ hasText: sessionTitle });
    await expect(card).toBeVisible({ timeout: 30000 });
    await expect(card.getByTestId("host-badge")).toBeVisible();
    await expect(card.getByTestId("host-badge")).toHaveText(new RegExp(REMOTE_NAME));
  });
});
