# Tagging rules

Tags organize sessions: they show as pills on session cards, and you can group or filter the session list by tag. Tagging rules apply tags to sessions automatically instead of by hand.

Rules are managed through the `list_tagging_rules`, `upsert_tagging_rule` and `delete_tagging_rule` MCP tools. Sessions that the rules cannot confidently tag fall back to a model-based classifier, configured under [Tagging classifier](/settings/tagging-classifier). Each session is classified once; to re-run it, use the session's tag editor.

Keep tags short (one or two words) and consistent, for example `Frontend`, `Backend` or `Urgent`, so grouping stays readable.
