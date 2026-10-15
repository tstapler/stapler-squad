# Validation
| AC | Evidence |
|---|---|
| 1,2,3,5,6 | upstream tests in config/background_models_test.go, server/services/backlog_service_background_models*_test.go, session/headless/feature_model_test.go; `TestBackgroundModels_NoDefaultIsOpus` |
| 4 | server/services/background_models_service_test.go (persist + live read), web-app BackgroundModelsSettings.test.tsx |
| 7 | TestBackgroundModels_InvalidValuesFallBackToDefault |
| 8 | docs/reference/background-model-defaults.md (effort in code upstream; evaluation documented) |
| 9 | config.HeadlessPoolDefaultModel used in server/dependencies.go; TestBackgroundModels_NoDefaultIsOpus asserts non-empty non-opus |
Pre-mortem: panel replaces all overrides (UI round-trips full state); regex too strict (covers real ID forms). Gate: PASS.
