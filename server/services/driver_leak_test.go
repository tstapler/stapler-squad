package services

// A SessionDriver left running after its test polls every 2s, and each poll's
// config.LoadConfig() can write config.json into whichever test's
// STAPLER_SQUAD_TEST_DIR is current, racing that test's TempDir RemoveAll.

// Tests must start drivers they own with session.StartSessionDriverForTest; the
// nosessiondrivertest analyzer rejects a raw session.StartSessionDriver.
// Drivers started by production code under test (CreateSession etc.) are
// covered by stopSessionDriverOnCleanup / setupForkTestFixture below.
