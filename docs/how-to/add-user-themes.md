# Add a custom theme

Drop a JSON file into `~/.stapler-squad/themes/` (the active config dir; see `docs/reference/state-isolation.md`). The server lists it at `GET /api/themes` and Settings → Appearance shows it next to the built-in themes. Reload the page after adding or editing a file.

```json
{
  "label": "My theme",
  "description": "Shown under the swatch",
  "base": "dark",
  "tokens": {
    "color.primary": "#e50914",
    "color.statusDot.running": "#46d369"
  }
}
```

- The file name (without `.json`) is the theme id: lowercase letters, digits and `-`, up to 40 characters.
- `base` is a built-in theme (`matrix`, `cyberpunk77`, `wh40k`, `clean`, `light`, `dark`); omitted or unknown falls back to `clean`. Tokens you don't set come from the base.
- `tokens` keys are dotted paths into the theme contract, `web-app/src/styles/theme-contract.css.ts` (for example `color.background`, `color.statusDot.running`, `shadow.sm`). Unknown paths are ignored.
- Values may use letters, digits and `# % . , ( ) / ' " _ + * -` only, and only these CSS functions: `rgb rgba hsl hsla hwb lab lch oklab oklch color-mix var calc min max clamp`. Anything else (`url()`, `image-set()`, `;`, `{}`) drops that token and logs a warning.
- Limits: 64 KB per file, 50 files, 300 tokens per theme. Symlinked files work, so a dotfiles repo can link its theme in.
