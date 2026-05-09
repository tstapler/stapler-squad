/** @type {import('stylelint').Config} */
module.exports = {
  extends: [
    'stylelint-config-standard',
    'stylelint-config-css-modules',
  ],
  rules: {
    // ── STRUCTURAL RULES (errors) ──────────────────────────────────────────

    // Catch --foo: value used without var() wrapper
    'custom-property-no-missing-var-function': true,

    // No duplicate custom property declarations in a single block
    'declaration-block-no-duplicate-custom-properties': true,

    // Allow :global() and :local() — CSS Modules pseudo-classes
    'selector-pseudo-class-no-unknown': [true, {
      ignorePseudoClasses: ['global', 'local'],
    }],

    // Allow 'composes' — CSS Modules keyword
    'property-no-unknown': [true, {
      ignoreProperties: ['composes'],
    }],

    // Allow @layer, @property, @value (CSS Modules)
    'at-rule-no-unknown': [true, {
      ignoreAtRules: ['value', 'layer', 'property'],
    }],

    // ── STYLISTIC OVERRIDES (disabled — existing codebase uses legacy syntax)
    // Re-enable these as the codebase is progressively modernised.

    // CSS Modules use camelCase class names (e.g. .buttonPrimary) —
    // disable the kebab-case enforcement from stylelint-config-standard.
    'selector-class-pattern': null,

    // Existing CSS uses rgba() and full hex (#ffffff, #333333).
    // These are cosmetic-only; disable until a separate cleanup pass.
    'color-function-notation': null,
    'alpha-value-notation': null,
    'color-hex-length': null,
    'color-function-alias-notation': null,

    // Existing CSS uses longhand (top/right/bottom/left) — disable shorthand rule.
    'declaration-block-no-redundant-longhand-properties': null,

    // Existing CSS uses max-width media queries — disable range notation rule.
    'media-feature-range-notation': null,

    // Font-family quote style — cosmetic.
    'font-family-name-quotes': null,

    // Shorthand redundant values — cosmetic.
    'shorthand-property-no-redundant-values': null,

    // camelCase keyframe names (fadeIn, slideIn) — same rationale as class names.
    'keyframes-name-pattern': null,

    // Empty line formatting rules — cosmetic whitespace only.
    'rule-empty-line-before': null,
    'comment-empty-line-before': null,

    // CSS keyword case — currentColor vs currentcolor is cosmetic.
    'value-keyword-case': null,

    // Single-line blocks with multiple declarations — formatting only.
    'declaration-block-single-line-max-declarations': null,

    // Modern :not() notation — cosmetic.
    'selector-not-notation': null,

    // Vendor prefixes — some are still needed; disable the blanket ban.
    'property-no-vendor-prefix': null,

    // Deprecated keyword values (word-break: break-word) — cosmetic.
    'declaration-property-value-keyword-no-deprecated': null,

    // The existing pattern of .button:disabled after .button:hover:not(:disabled)
    // triggers this rule, but those pseudo-states are mutually exclusive so there
    // is no real-world cascade bug. Disable to keep a clean baseline.
    'no-descending-specificity': null,

    // ── MOBILE UX REGRESSION PREVENTION ────────────────────────────────────
    // These rules prevent regressions of bugs fixed in the UX overhaul.
    // See project_plans/ux-overhaul/ for context.

    // Ban bare 100vh in height properties — iOS virtual keyboard does not
    // shrink 100vh, causing content to be hidden behind the keyboard.
    // Use var(--viewport-height) which ViewportProvider updates via the
    // visualViewport API. Also bans 100vh in calc() via substring match.
    //
    // Ban bare env(safe-area-inset-*) in component CSS — use the
    // var(--safe-area-top/bottom/left/right) tokens defined in globals.css
    // :root instead. Exception: globals.css itself defines the tokens
    // (see overrides section at the bottom of this file).
    'declaration-property-value-disallowed-list': {
      '/^(?:height|min-height|max-height|block-size|min-block-size)$/': [
        '/100vh/',
      ],
      '/.+/': [
        '/env\\(safe-area-inset-/',
      ],
    },

    // ── NOTE ───────────────────────────────────────────────────────────────
    // Cross-file undefined variable detection is handled by
    // scripts/check-css-vars.mjs (npm run lint:css-vars).
    // stylelint's no-unknown-custom-properties is per-file only — it would
    // produce false positives for CSS Modules consuming tokens from globals.css.
  },
  ignoreFiles: [
    'node_modules/**',
    '.next/**',
    'out/**',
    'src/generated/**',
  ],

  // globals.css is allowed to use env(safe-area-inset-*) directly because it
  // is the file that DEFINES the --safe-area-* custom properties. All other
  // files must consume those variables instead of calling env() directly.
  overrides: [
    {
      files: ['src/app/globals.css'],
      rules: {
        'declaration-property-value-disallowed-list': {
          '/^(?:height|min-height|max-height|block-size|min-block-size)$/': [
            '/100vh/',
          ],
          // No safe-area restriction — this file defines the tokens
        },
      },
    },
  ],
};
