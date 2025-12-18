package app

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanArchitectureExample demonstrates proper dependency injection and testing patterns
// This test shows how to use the clean architecture patterns we've implemented
func TestCleanArchitectureExample(t *testing.T) {
	t.Run("demonstrates production dependencies", func(t *testing.T) {
		// Use production dependencies through the builder pattern
		// This ensures tests use the same construction path as production
		h := NewTestHomeBuilder().
			WithProgram("claude").
			WithAutoYes(true).
			Build(t)

		// Test that dependencies are properly injected
		assert.NotNil(t, h.stateManager)
		assert.NotNil(t, h.list)
		assert.NotNil(t, h.storage)
		assert.Equal(t, "claude", h.program)
		assert.True(t, h.autoYes)

		// Test facade methods work correctly (no Law of Demeter violations)
		assert.True(t, h.isInState(state.Default))
	})

	t.Run("demonstrates mock dependencies for unit testing", func(t *testing.T) {
		// Use mock dependencies for isolated unit testing
		h := NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
			// Only set up the dependencies needed for this test
			mocks.program = "test-program"
			mocks.autoYes = false
			mocks.stateManager = state.NewManager()
			mocks.SetMockAppConfig(config.DefaultConfig())
		})

		// Test that mocked dependencies are used
		assert.Equal(t, "test-program", h.program)
		assert.False(t, h.autoYes)
		assert.NotNil(t, h.stateManager)
	})

	t.Run("demonstrates facade pattern prevents Law of Demeter violations", func(t *testing.T) {
		h := SetupMinimalTestHome(t)

		// Good: Using facade methods (no direct access to internal state)
		err := h.setState(state.Confirm)
		require.NoError(t, err)
		assert.True(t, h.isInState(state.Confirm))

		// Test state transitions work through facade
		err = h.transitionToDefault()
		require.NoError(t, err)
		assert.True(t, h.isInState(state.Default))

		// This demonstrates clean encapsulation - tests don't access
		// h.stateManager directly, they use the facade methods
	})

	t.Run("demonstrates proper error handling", func(t *testing.T) {
		h := SetupMinimalTestHome(t)

		// Test error handling through facade methods
		// The facade methods encapsulate error handling logic
		err := h.setState(state.Default)
		assert.NoError(t, err)

		// Test that invalid state transitions are handled properly
		// (Note: this would need actual validation logic in the state manager)
		assert.True(t, h.getState() == state.Default)
	})
}

// TestDependencyInjectionBenefits demonstrates the benefits of our DI approach
func TestDependencyInjectionBenefits(t *testing.T) {
	t.Run("allows easy component substitution", func(t *testing.T) {
		// We can easily substitute different implementations for testing
		customConfig := &config.Config{
			DefaultProgram: "custom-program",
			AutoYes:        true,
		}

		h := NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
			mocks.SetMockAppConfig(customConfig)
			mocks.program = "test"
			mocks.stateManager = state.NewManager()
		})

		// Verify custom config is used
		assert.Equal(t, customConfig, h.appConfig)
	})

	t.Run("enables isolated testing", func(t *testing.T) {
		// Each test can have completely isolated dependencies
		sm1 := state.NewManager()
		sm2 := state.NewManager()

		h1 := NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
			mocks.program = "program1"
			mocks.stateManager = sm1
		})

		h2 := NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
			mocks.program = "program2"
			mocks.stateManager = sm2
		})

		// Tests are completely isolated
		assert.Equal(t, "program1", h1.program)
		assert.Equal(t, "program2", h2.program)
		assert.NotSame(t, h1.stateManager, h2.stateManager)

		// Verify we're using the expected instances
		assert.Same(t, sm1, h1.stateManager)
		assert.Same(t, sm2, h2.stateManager)
	})

	t.Run("makes testing specific scenarios easy", func(t *testing.T) {
		// We can easily create specific test scenarios
		h := NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
			// Set up a specific state for testing
			mocks.stateManager = state.NewManager()
			// We could mock specific behavior here if needed
		})

		// Test specific scenario
		err := h.setState(state.Confirm)
		require.NoError(t, err)
		assert.True(t, h.isInState(state.Confirm))
	})
}

// TestArchitecturalPatterns demonstrates various architectural patterns we've implemented
func TestArchitecturalPatterns(t *testing.T) {
	t.Run("builder pattern for test setup", func(t *testing.T) {
		// Builder pattern allows fluent, readable test setup
		h := NewTestHomeBuilder().
			WithProgram("builder-test").
			WithAutoYes(false).
			WithBridge().
			Build(t)

		assert.Equal(t, "builder-test", h.program)
		assert.False(t, h.autoYes)
		assert.NotNil(t, h.bridge)
	})

	t.Run("factory pattern for different test types", func(t *testing.T) {
		// Different factory methods for different test needs
		sessionHome := SetupTestHomeWithSession(t, "test-session")
		assert.NotNil(t, sessionHome.list.GetSelectedInstance())

		multiSessionHome := SetupTestHomeWithMultipleSessions(t, []string{"s1", "s2"})
		assert.Equal(t, 2, multiSessionHome.list.NumInstances())

		minimalHome := SetupMinimalTestHome(t)
		assert.NotNil(t, minimalHome.stateManager)
	})

	t.Run("interface segregation principle", func(t *testing.T) {
		// Dependencies interface provides only what's needed
		// This follows Interface Segregation Principle
		deps := NewProductionDependencies(nil, "test", false)

		// Interface provides clean contract
		assert.Equal(t, "test", deps.GetProgram())
		assert.False(t, deps.GetAutoYes())
		assert.NotNil(t, deps.GetAppConfig())
	})
}
