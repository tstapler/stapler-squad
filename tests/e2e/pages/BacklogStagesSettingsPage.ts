import type { Locator, Page } from "@playwright/test";
import { baseUrl } from '../helpers/base-url';

const BASE_URL = baseUrl();

/** /settings/backlog-stages: the engine-mode banner, the stage list and the stage form. */
export class BacklogStagesSettingsPage {
  readonly page: Page;
  readonly banner: Locator;
  readonly bannerText: Locator;
  /** The banner's single nested live region; empty until a resolved value changes. */
  readonly bannerLive: Locator;
  readonly bannerRetry: Locator;
  readonly bannerFlagsLink: Locator;
  readonly review: Locator;
  readonly saveStatus: Locator;
  readonly form: Locator;
  readonly formReminder: Locator;
  readonly formSubmit: Locator;
  readonly formCancel: Locator;

  constructor(page: Page) {
    this.page = page;
    this.banner = page.getByTestId("engine-mode-banner");
    this.bannerText = page.getByTestId("engine-mode-banner-text");
    this.bannerLive = page.getByTestId("engine-mode-banner-live");
    this.bannerRetry = page.getByTestId("engine-mode-banner-retry");
    this.bannerFlagsLink = page.getByTestId("engine-mode-banner-flags-link");
    this.review = page.getByTestId("engine-mode-review");
    this.saveStatus = page.getByTestId("stage-save-status");
    this.form = page.getByTestId("stage-form");
    this.formReminder = page.getByTestId("stage-form-mode-reminder");
    this.formSubmit = page.getByTestId("stage-form-submit");
    this.formCancel = page.getByTestId("stage-form-cancel");
  }

  async goto(): Promise<void> {
    await this.page.goto(`${BASE_URL}/settings/backlog-stages`, { waitUntil: "domcontentloaded" });
  }

  stageRow(slug: string): Locator {
    return this.page.getByTestId(`backlog-stage-row-${slug}`);
  }

  editButton(slug: string): Locator {
    return this.page.getByTestId(`backlog-stage-edit-${slug}`);
  }

  async openStageForm(slug: string): Promise<void> {
    await this.editButton(slug).click();
  }

  /** True when the first stage row follows the banner in DOM order (UX-01). */
  async bannerPrecedesStageList(): Promise<boolean> {
    return this.page.evaluate(() => {
      const banner = document.querySelector('[data-testid="engine-mode-banner"]');
      const row = document.querySelector('[data-testid^="backlog-stage-row-"]');
      return Boolean(banner && row && banner.compareDocumentPosition(row) & Node.DOCUMENT_POSITION_FOLLOWING);
    });
  }

  /** True when neither the page nor the banner scrolls horizontally. */
  async hasNoHorizontalScroll(): Promise<boolean> {
    return this.page.evaluate(() => {
      const root = document.scrollingElement as HTMLElement;
      const banner = document.querySelector<HTMLElement>('[data-testid="engine-mode-banner"]');
      return root.scrollWidth <= root.clientWidth && (!banner || banner.scrollWidth <= banner.clientWidth);
    });
  }
}
