You are ready to ship your work as a pull request.

Before shipping, confirm all acceptance criteria are marked complete (`/backlog/status`).

Steps:
1. Create the pull request:
   Run `/github:pr-ship` — this drives the PR through local CI, code review, remote CI, and
   merge-conflict resolution. It will stop short of actually merging; the final merge is left to
   the human reviewer.

2. Once `/github:pr-ship` reports all gates green, request the automated review:
   Run `/backlog/review` with a 2-3 sentence summary of what was built and the PR number.

Note: if the repository has no GitHub remote, use `gh pr create --fill` to create the PR manually,
then run `/backlog/review`.
