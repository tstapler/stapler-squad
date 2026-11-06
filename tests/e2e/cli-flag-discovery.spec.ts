// @feature program_config:probe, settings-programs
import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { ProgramsSettingsPage } from "./pages/ProgramsSettingsPage";

const FIXTURE_SRC = path.join(__dirname, "fixtures", "probe-fixture.sh");
const MIN_TARGET = 44;

let dir: string;
let fixture: string;
let marker: string;
const ran = () => fs.existsSync(marker);

// A per-test copy owned by the test user and not world-writable, as the probe requires.
test.beforeEach(() => {
  dir = fs.mkdtempSync(path.join(os.tmpdir(), "probe-fixture-"));
  fixture = path.join(dir, "probe-fixture.sh");
  marker = path.join(dir, "probe-fixture.ran");
  fs.copyFileSync(FIXTURE_SRC, fixture);
  fs.chmodSync(fixture, 0o755);
});

test.afterEach(() => {
  fs.rmSync(dir, { recursive: true, force: true });
});

test.describe("cli-flag-discovery: desktop", () => {
  test("programs_should_NotRunScriptOnBlurOrEnterAndRunAfterCheck_When_FixtureIsShebang", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();

    await programs.enterCommandAndBlur(fixture);
    await expect(programs.status).toContainText("Found:");
    await expect(programs.status).toContainText("Not checked for flags yet");
    expect(ran()).toBe(false);

    await programs.commandInput.focus();
    await programs.commandInput.press("Enter");
    await expect(programs.status).toContainText("Not checked for flags yet");
    await expect(programs.form).toBeVisible();
    await expect(page.getByTestId("programs-success")).toHaveCount(0);
    expect(ran()).toBe(false);

    await programs.check();
    await expect.poll(ran).toBe(true);
    await expect(programs.status).toContainText("flags detected");
    await expect(programs.checkButton).toBeFocused();

    await programs.flagsInput.fill("--alp");
    await expect(page.getByRole("option", { name: /--alpha/ })).toBeVisible();
  });

  test("programs_should_ShowNotFound_When_PathDoesNotExist", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.enterCommandAndBlur(path.join(dir, "does-not-exist"));
    await expect(programs.status).toContainText("Not found");
  });

  test("programs_should_WarnSoftlyAndKeepSaveEnabled_When_UnknownFlagTyped", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.checkFixtureAndAwaitFlags(fixture);

    await programs.flagsInput.fill("--alpha --bogus");
    await programs.commandInput.focus();
    await expect(programs.flagsWarning).toContainText("--bogus is not listed in probe-fixture.sh --help");
    await expect(programs.flagsInput).not.toHaveAttribute("aria-invalid", /.*/);
    await expect(programs.flagsInput).toHaveAttribute("aria-describedby", /prog-flags-warning/);
    await expect(programs.saveButton).toBeEnabled();
  });

  test("programs_should_RejectProbeWithForeignHost_When_HostHeaderIsNotLoopback", async ({ request, baseURL }) => {
    const res = await request.post(`${baseURL}/api/session.v1.SessionService/ProbeProgram`, {
      headers: { Host: "evil.example", "Content-Type": "application/json" },
      data: { command: fixture },
    });
    expect(res.status()).toBe(403);
    expect(ran()).toBe(false);
  });

  test("forms_should_PassAxeWcagAA_When_FoundAndWarningStates", async ({ page }) => {
    test.setTimeout(120_000);
    const programs = new ProgramsSettingsPage(page);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await programs.gotoAndOpenForm();
    await programs.checkFixtureAndAwaitFlags(fixture);
    await programs.flagsInput.fill("--alpha --bogus");
    await programs.commandInput.focus();
    await expect(programs.flagsWarning).toBeVisible();
    await programs.availableFlagsToggle.click();
    await programs.flagInfoButtons.first().click();

    const results = await new AxeBuilder({ page })
      .include('[data-testid="program-form"]')
      .withTags(["wcag2a", "wcag2aa"])
      .analyze();
    const blocking = results.violations.filter((v) => v.impact === "critical" || v.impact === "serious");
    expect(
      blocking.map((v) => `${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} ${n.any[0]?.message ?? ""}`).join(" | ")}`),
    ).toEqual([]);
    expect(results.violations.map((v) => v.id)).not.toContain("nested-interactive");
  });
});

test.describe("cli-flag-discovery: touch 375x667", () => {
  test.use({ viewport: { width: 375, height: 667 }, hasTouch: true, isMobile: true });

  test("mobile_should_TapInfoButtonAndShowDescription44px_When_Viewport375x667HasTouch", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.checkFixtureAndAwaitFlags(fixture);

    await programs.flagsInput.fill("--alp");
    await programs.availableFlagsToggle.tap();
    const info = programs.flagInfoButtons.first();
    await expect(info).toHaveAttribute("aria-expanded", "false");
    const box = await info.boundingBox();
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(MIN_TARGET);
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TARGET);

    await info.tap();
    await expect(info).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByTestId("prog-flag-info-description").first()).toBeVisible();
    await expect(page.getByTestId("prog-flag-info-description").first()).toContainText("first flag");

    await info.tap();
    await expect(info).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByTestId("prog-flag-info-description")).toHaveCount(0);
  });

  test("check_should_BeFullWidth44pxRow_When_NeedsConfirmAt375", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.enterCommandAndBlur(fixture);
    await expect(programs.status).toContainText("Not checked for flags yet");
    const box = await programs.checkButton.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TARGET);
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(MIN_TARGET);
    expect(await programs.horizontalOverflow()).toBe(false);
  });
});

test.describe("cli-flag-discovery: on-screen keyboard (375x300 visual viewport)", () => {
  test.use({ viewport: { width: 375, height: 300 }, hasTouch: true, isMobile: true });

  test("keyboard_should_KeepFlagListAndSaveReachable_When_ViewportShrinks", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.checkFixtureAndAwaitFlags(fixture);

    await programs.flagsInput.fill("--a");
    await programs.flagsInput.press("ArrowDown");
    const active = page.getByRole("listbox", { name: "Flag suggestions" }).getByRole("option", { selected: true });
    await expect(active).toBeVisible();
    await expect(active).toBeInViewport();
    await expect(page.getByRole("listbox")).toBeVisible();

    await programs.flagsInput.press("Escape");
    await programs.saveButton.scrollIntoViewIfNeeded();
    await expect(programs.saveButton).toBeInViewport();
    await expect(programs.saveButton).toBeEnabled();
  });
});

test.describe("cli-flag-discovery: mid width 800x1024", () => {
  test.use({ viewport: { width: 800, height: 1024 } });

  test("check_should_SitInlineAfterTextWithoutHScroll_When_Viewport800", async ({ page }) => {
    const programs = new ProgramsSettingsPage(page);
    await programs.gotoAndOpenForm();
    await programs.enterCommandAndBlur(fixture);
    await expect(programs.status).toContainText("Not checked for flags yet");
    expect(await programs.horizontalOverflow()).toBe(false);

    const check = await programs.checkButton.boundingBox();
    const input = await programs.commandInput.boundingBox();
    expect(check && input && check.y < input.y + input.height + 1).toBe(true);
    expect(check?.width ?? 0).toBeLessThan(400);

    await programs.check();
    await expect(programs.status).toContainText("flags detected");
    await programs.flagsInput.fill("--a");
    const listbox = page.getByRole("listbox");
    await expect(listbox).toBeVisible();
    const lb = await listbox.boundingBox();
    expect((lb?.x ?? -1) >= 0 && (lb?.x ?? 0) + (lb?.width ?? 0) <= 800).toBe(true);
  });
});
