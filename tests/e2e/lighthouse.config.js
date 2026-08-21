// Lighthouse CI configuration for Stapler Squad
// Performance score below 70 triggers a warning (non-blocking)
module.exports = {
  ci: {
    collect: {
      url: [
        'http://localhost:8543',
        'http://localhost:8543/review-queue',
      ],
      numberOfRuns: 1,
      settings: {
        // Desktop preset for terminal-heavy UI
        preset: 'desktop',
        // Skip PWA checks (not applicable for internal tool)
        skipAudits: ['installable-manifest', 'splash-screen', 'themed-address-bar'],
      },
    },
    assert: {
      preset: 'lighthouse:recommended',
      assertions: {
        // Performance: warn below 70 (non-blocking)
        'categories:performance': ['warn', { minScore: 0.7 }],
        // Accessibility: enforced by Axe Core (Lighthouse check is advisory)
        'categories:accessibility': ['warn', { minScore: 0.8 }],
        // Best practices: warn
        'categories:best-practices': ['warn', { minScore: 0.8 }],
        // SEO: not relevant for internal tool
        'categories:seo': 'off',
        // PWA: not applicable
        'categories:pwa': 'off',
        // Allow terminal/heavy UI specifics
        'uses-webp-images': 'off',
        'offscreen-images': 'off',
      },
    },
    upload: {
      target: 'temporary-public-storage',
    },
  },
};
