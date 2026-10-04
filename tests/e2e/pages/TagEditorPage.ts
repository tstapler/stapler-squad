import { Page, Locator, expect } from "@playwright/test";

/**
 * Page object for the session-tag surfaces added by the session-classifier-pipeline
 * feature (design/ux.md Surfaces 1–3): the provenance-aware tag pills rendered on
 * `SessionCard` (web-app/src/components/sessions/SessionCard.tsx) and the removal
 * confirmation flow inside the `TagEditor` modal
 * (web-app/src/components/sessions/TagEditor.tsx) it opens.
 *
 * A new page object rather than an extension of `SessionDetailPage` — tag editing is
 * triggered from a `SessionCard` on the dashboard's session list, not from the session
 * detail view, so it shares no navigation with `SessionDetailPage`.
 *
 * `SessionCard` only renders in **Board** view — the default **List** view renders
 * `SessionRow` instead, which has no tag-pill/Edit-Tags UI at all (confirmed by reading
 * `web-app/src/components/sessions/SessionRow.tsx`: it accepts an `onUpdateTags` callback
 * prop but never renders a tag or a button that calls it). `switchToBoardView()` must be
 * called once per page before any other method here will find anything.
 *
 * All locators use `data-testid`/ARIA roles only, per `e2e-test-conventions`: the tag
 * pills have `role="listitem"` inside a `role="list"` (`aria-label="Session tags"`), the
 * `TagEditor` modal is `role="dialog"` with accessible name "Edit Tags", and every button
 * inside it has a stable accessible name (`Remove tag ${tag}`, `Keep`, `Remove anyway`,
 * `Save Tags`, `Cancel`). The one exception is the in-progress confirmation row, which
 * carries `data-testid="tag-confirm-row-${tag}"` because it has no single accessible name
 * of its own (it's a compound row: caption + two buttons).
 */
export class TagEditorPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /**
   * Switches the session list from the default List view (`SessionRow`, no tag UI) to
   * Board view (`SessionCard` via `BoardCard`), where the tag/provenance UI lives.
   */
  async switchToBoardView() {
    // Proof the SPA has hydrated and the initial ListSessions fetch has landed before
    // touching the view-mode toggle — same wait bulk-select.spec.ts uses.
    await this.page.waitForSelector('input[aria-label="Search sessions"]', { timeout: 15000 });
    await this.page.getByTestId("session-view-mode-board").click();
    await expect(this.page.getByTestId("session-view-mode-board")).toHaveAttribute("aria-pressed", "true");
  }

  /** A `SessionCard` (Board view only — see class doc). */
  getCard(sessionTitle: string): Locator {
    return this.page.getByTestId("session-card").filter({ hasText: sessionTitle });
  }

  /** The tag-pill row (`role="list"`, `aria-label="Session tags"`) on a card. */
  getTagList(sessionTitle: string): Locator {
    return this.getCard(sessionTitle).getByRole("list", { name: "Session tags" });
  }

  /** A single tag pill (`role="listitem"`) by its visible text. */
  getTagPill(sessionTitle: string, tag: string): Locator {
    return this.getTagList(sessionTitle).getByRole("listitem").filter({ hasText: tag });
  }

  /**
   * The "Edit Tags"/"Add Tags" button on a card. Its accessible name comes from an
   * explicit `aria-label` (`"${Edit|Add} tags for ${session.title}"`), not its visible
   * "Edit Tags"/"Add Tags" text — see SessionCard.tsx.
   */
  getEditTagsButton(sessionTitle: string): Locator {
    return this.getCard(sessionTitle).getByRole("button", { name: new RegExp(`tags for ${sessionTitle}$`) });
  }

  /** Opens the TagEditor modal for `sessionTitle` and returns its dialog locator. */
  async open(sessionTitle: string): Promise<Locator> {
    await this.getEditTagsButton(sessionTitle).click();
    const dialog = this.getDialog();
    await dialog.waitFor({ state: "visible" });
    return dialog;
  }

  getDialog(): Locator {
    return this.page.getByRole("dialog", { name: "Edit Tags" });
  }

  /** The "x" remove button for a non-confirming tag row. */
  getRemoveButton(tag: string): Locator {
    return this.getDialog().getByRole("button", { name: `Remove tag ${tag}` });
  }

  /** The expanded removal-confirmation row for `tag` (ux.md Surface 3). */
  getConfirmRow(tag: string): Locator {
    return this.getDialog().locator(`[data-testid="tag-confirm-row-${tag}"]`);
  }

  getKeepButton(): Locator {
    return this.getDialog().getByRole("button", { name: "Keep" });
  }

  getRemoveAnywayButton(): Locator {
    return this.getDialog().getByRole("button", { name: "Remove anyway" });
  }

  /** The non-removable Unclassified row's explanatory caption text. */
  getUnclassifiedCaption(): Locator {
    return this.getDialog().getByText("Removed automatically once classification succeeds");
  }

  getSaveButton(): Locator {
    return this.getDialog().getByRole("button", { name: "Save Tags" });
  }

  getCancelButton(): Locator {
    return this.getDialog().getByRole("button", { name: "Cancel" });
  }
}
