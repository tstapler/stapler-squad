# Omnibar: promote "Create Backlog Item" to the footer action row

- Finding: the button existed only as a listbox row in `OmnibarResultList` (discovery mode); the creation panel footer (`OmnibarCreationPanel`) had only Cancel / Create Session.
- Change: add `onCreateBacklog` prop to `OmnibarCreationPanel`; render a top-level footer button (`omnibar-create-backlog-button`) wired to `openBacklogCompose` (same handler as before). Discovery-mode list row is kept (keyboard navigation depends on it).
- Acceptance: button in footer row; visible in discovery (list row) and creation (footer) modes; same handler; Omnibar tests pass plus new footer test.
