import { readFileSync } from "fs";
import { join } from "path";

// The list form of SessionPullRequest omits `url` (K11); these files hold list-form PRs and must
// call prUrl(pr) instead of reading `pr.url` (an empty href opens about:blank).
const FILES = [
  "components/shared/PullRequestList.tsx",
  "components/shared/PullRequestChip.tsx",
  "components/shared/PullRequestRow.tsx",
  "components/backlog/detail/ItemPullRequestList.tsx",
];

describe("list-form SessionPullRequest url guard", () => {
  it.each(FILES)("%s_should_notReadRawPrUrl", (file) => {
    const src = readFileSync(join(__dirname, "..", "..", file), "utf8");
    expect(src).not.toMatch(/\b(?:pr|dialog\.pr|shown|tracked)\.url\b/);
  });
});
