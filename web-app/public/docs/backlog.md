# Backlog

The backlog is Stapler Squad's queue of work items. Each item has a title, a description, acceptance criteria and a status, and an item can be handed to an agent session that works on it in an isolated git worktree.

Open [Backlog](/backlog) for the list, or [the board](/backlog/board) for a column view by status. Items can be created by hand, imported from external trackers, or created by agents through the MCP tools.

## Where items come from and how they run

Import sources are configured under [Backlog sources](/settings/backlog-sources). The stages an item moves through are configured under [Backlog stages](/settings/backlog-stages), and the pipeline that runs for an item is chosen under [Pipeline modes](/settings/pipeline-modes).

While a session works an item it reports progress against each acceptance criterion, then requests a review. The review verdict decides whether the work moves on to a pull request or goes back for fixes.
