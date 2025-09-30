package state

import (
	"testing"
)

func TestGetViewDirective(t *testing.T) {
	tests := []struct {
		name           string
		state          State
		context        TransitionContext
		expectedType   ViewDirectiveType
		expectedOverlay string
		expectedCentered bool
		expectedBordered bool
		expectedShouldResetOnNil bool
	}{
		{
			name:           "Default state returns main view",
			state:          Default,
			context:        TransitionContext{},
			expectedType:   ViewMain,
			expectedOverlay: "",
			expectedCentered: false,
			expectedBordered: false,
			expectedShouldResetOnNil: false,
		},
		{
			name:           "New state returns main view",
			state:          New,
			context:        TransitionContext{},
			expectedType:   ViewMain,
			expectedOverlay: "",
			expectedCentered: false,
			expectedBordered: false,
			expectedShouldResetOnNil: false,
		},
		{
			name:           "Prompt state with search context returns search overlay",
			state:          Prompt,
			context:        TransitionContext{MenuState: "Search"},
			expectedType:   ViewOverlay,
			expectedOverlay: "liveSearchOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Prompt state without search context returns text input overlay",
			state:          Prompt,
			context:        TransitionContext{MenuState: "Prompt"},
			expectedType:   ViewOverlay,
			expectedOverlay: "textInputOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Help state with messages context returns messages overlay",
			state:          Help,
			context:        TransitionContext{OverlayName: "messages"},
			expectedType:   ViewOverlay,
			expectedOverlay: "messagesOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Help state without messages context returns text overlay",
			state:          Help,
			context:        TransitionContext{OverlayName: "help"},
			expectedType:   ViewText,
			expectedOverlay: "textOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Confirm state returns confirmation overlay",
			state:          Confirm,
			context:        TransitionContext{},
			expectedType:   ViewOverlay,
			expectedOverlay: "confirmationOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "CreatingSession state returns spinner",
			state:          CreatingSession,
			context:        TransitionContext{},
			expectedType:   ViewSpinner,
			expectedOverlay: "",
			expectedCentered: true,
			expectedBordered: false,
			expectedShouldResetOnNil: false,
		},
		{
			name:           "AdvancedNew state returns session setup overlay",
			state:          AdvancedNew,
			context:        TransitionContext{},
			expectedType:   ViewOverlay,
			expectedOverlay: "sessionSetupOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Git state returns git status overlay",
			state:          Git,
			context:        TransitionContext{},
			expectedType:   ViewOverlay,
			expectedOverlay: "gitStatusOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "ClaudeSettings state returns claude settings overlay",
			state:          ClaudeSettings,
			context:        TransitionContext{},
			expectedType:   ViewOverlay,
			expectedOverlay: "claudeSettingsOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "ZFSearch state returns zf search overlay",
			state:          ZFSearch,
			context:        TransitionContext{},
			expectedType:   ViewOverlay,
			expectedOverlay: "zfSearchOverlay",
			expectedCentered: true,
			expectedBordered: true,
			expectedShouldResetOnNil: true,
		},
		{
			name:           "Invalid state returns main view",
			state:          State(999),
			context:        TransitionContext{},
			expectedType:   ViewMain,
			expectedOverlay: "",
			expectedCentered: false,
			expectedBordered: false,
			expectedShouldResetOnNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &manager{
				current:           tt.state,
				transitionContext: tt.context,
			}

			directive := manager.GetViewDirective()

			if directive.Type != tt.expectedType {
				t.Errorf("Expected directive type %v, got %v", tt.expectedType, directive.Type)
			}

			if directive.OverlayComponent != tt.expectedOverlay {
				t.Errorf("Expected overlay component %s, got %s", tt.expectedOverlay, directive.OverlayComponent)
			}

			if directive.Centered != tt.expectedCentered {
				t.Errorf("Expected centered %v, got %v", tt.expectedCentered, directive.Centered)
			}

			if directive.Bordered != tt.expectedBordered {
				t.Errorf("Expected bordered %v, got %v", tt.expectedBordered, directive.Bordered)
			}

			if directive.ShouldResetOnNil != tt.expectedShouldResetOnNil {
				t.Errorf("Expected shouldResetOnNil %v, got %v", tt.expectedShouldResetOnNil, directive.ShouldResetOnNil)
			}
		})
	}
}

func TestShouldRenderOverlay(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		context  TransitionContext
		expected bool
	}{
		{
			name:     "Default state should not render overlay",
			state:    Default,
			context:  TransitionContext{},
			expected: false,
		},
		{
			name:     "New state should not render overlay",
			state:    New,
			context:  TransitionContext{},
			expected: false,
		},
		{
			name:     "Prompt state should render overlay",
			state:    Prompt,
			context:  TransitionContext{MenuState: "Search"},
			expected: true,
		},
		{
			name:     "Help state should render overlay",
			state:    Help,
			context:  TransitionContext{OverlayName: "messages"},
			expected: true,
		},
		{
			name:     "Confirm state should render overlay",
			state:    Confirm,
			context:  TransitionContext{},
			expected: true,
		},
		{
			name:     "CreatingSession state should render overlay (spinner)",
			state:    CreatingSession,
			context:  TransitionContext{},
			expected: true,
		},
		{
			name:     "AdvancedNew state should render overlay",
			state:    AdvancedNew,
			context:  TransitionContext{},
			expected: true,
		},
		{
			name:     "Git state should render overlay",
			state:    Git,
			context:  TransitionContext{},
			expected: true,
		},
		{
			name:     "ClaudeSettings state should render overlay",
			state:    ClaudeSettings,
			context:  TransitionContext{},
			expected: true,
		},
		{
			name:     "ZFSearch state should render overlay",
			state:    ZFSearch,
			context:  TransitionContext{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &manager{
				current:           tt.state,
				transitionContext: tt.context,
			}

			result := manager.ShouldRenderOverlay()

			if result != tt.expected {
				t.Errorf("Expected ShouldRenderOverlay %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetOverlayComponent(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		context  TransitionContext
		expected string
	}{
		{
			name:     "Default state returns empty component",
			state:    Default,
			context:  TransitionContext{},
			expected: "",
		},
		{
			name:     "Prompt with search returns live search overlay",
			state:    Prompt,
			context:  TransitionContext{MenuState: "Search"},
			expected: "liveSearchOverlay",
		},
		{
			name:     "Prompt without search returns text input overlay",
			state:    Prompt,
			context:  TransitionContext{MenuState: "Prompt"},
			expected: "textInputOverlay",
		},
		{
			name:     "Help with messages returns messages overlay",
			state:    Help,
			context:  TransitionContext{OverlayName: "messages"},
			expected: "messagesOverlay",
		},
		{
			name:     "Help without messages returns text overlay",
			state:    Help,
			context:  TransitionContext{OverlayName: "help"},
			expected: "textOverlay",
		},
		{
			name:     "Confirm returns confirmation overlay",
			state:    Confirm,
			context:  TransitionContext{},
			expected: "confirmationOverlay",
		},
		{
			name:     "CreatingSession returns empty (spinner mode)",
			state:    CreatingSession,
			context:  TransitionContext{},
			expected: "",
		},
		{
			name:     "AdvancedNew returns session setup overlay",
			state:    AdvancedNew,
			context:  TransitionContext{},
			expected: "sessionSetupOverlay",
		},
		{
			name:     "Git returns git status overlay",
			state:    Git,
			context:  TransitionContext{},
			expected: "gitStatusOverlay",
		},
		{
			name:     "ClaudeSettings returns claude settings overlay",
			state:    ClaudeSettings,
			context:  TransitionContext{},
			expected: "claudeSettingsOverlay",
		},
		{
			name:     "ZFSearch returns zf search overlay",
			state:    ZFSearch,
			context:  TransitionContext{},
			expected: "zfSearchOverlay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &manager{
				current:           tt.state,
				transitionContext: tt.context,
			}

			result := manager.GetOverlayComponent()

			if result != tt.expected {
				t.Errorf("Expected overlay component %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestViewDirectiveTypes(t *testing.T) {
	// Test that view directive types have expected values
	expectedTypes := map[ViewDirectiveType]string{
		ViewMain:    "ViewMain",
		ViewOverlay: "ViewOverlay",
		ViewText:    "ViewText",
		ViewSpinner: "ViewSpinner",
	}

	for directiveType, expectedName := range expectedTypes {
		// Basic validation that the constants are defined
		if int(directiveType) < 0 {
			t.Errorf("ViewDirectiveType %s has invalid value %d", expectedName, int(directiveType))
		}
	}

	// Test that the types are distinct
	types := []ViewDirectiveType{ViewMain, ViewOverlay, ViewText, ViewSpinner}
	for i, typeA := range types {
		for j, typeB := range types {
			if i != j && typeA == typeB {
				t.Errorf("ViewDirectiveType values are not distinct: %d == %d", int(typeA), int(typeB))
			}
		}
	}
}

func TestViewDirectiveContextSensitivity(t *testing.T) {
	manager := NewManager().(*manager)

	// Test that Prompt state directive changes based on context
	manager.current = Prompt
	manager.transitionContext = TransitionContext{MenuState: "Search"}
	directive1 := manager.GetViewDirective()

	manager.transitionContext = TransitionContext{MenuState: "Prompt"}
	directive2 := manager.GetViewDirective()

	if directive1.OverlayComponent == directive2.OverlayComponent {
		t.Error("Prompt state should return different overlay components based on MenuState context")
	}

	if directive1.OverlayComponent != "liveSearchOverlay" {
		t.Errorf("Expected liveSearchOverlay for Search context, got %s", directive1.OverlayComponent)
	}

	if directive2.OverlayComponent != "textInputOverlay" {
		t.Errorf("Expected textInputOverlay for Prompt context, got %s", directive2.OverlayComponent)
	}

	// Test that Help state directive changes based on context
	manager.current = Help
	manager.transitionContext = TransitionContext{OverlayName: "messages"}
	directive3 := manager.GetViewDirective()

	manager.transitionContext = TransitionContext{OverlayName: "help"}
	directive4 := manager.GetViewDirective()

	if directive3.Type == directive4.Type && directive3.OverlayComponent == directive4.OverlayComponent {
		t.Error("Help state should return different directives based on OverlayName context")
	}

	if directive3.OverlayComponent != "messagesOverlay" {
		t.Errorf("Expected messagesOverlay for messages context, got %s", directive3.OverlayComponent)
	}

	if directive4.OverlayComponent != "textOverlay" {
		t.Errorf("Expected textOverlay for help context, got %s", directive4.OverlayComponent)
	}
}

func TestViewDirectiveMessage(t *testing.T) {
	manager := NewManager().(*manager)
	manager.current = CreatingSession

	directive := manager.GetViewDirective()

	if directive.Type != ViewSpinner {
		t.Errorf("Expected ViewSpinner for CreatingSession state, got %v", directive.Type)
	}

	expectedMessage := "Creating session...\n\nThis may take up to 60 seconds if starting Claude or Aider for the first time.\nPress Ctrl+C to cancel if needed."
	if directive.Message != expectedMessage {
		t.Errorf("Expected specific creation message, got: %s", directive.Message)
	}

	if !directive.Centered {
		t.Error("Expected CreatingSession spinner to be centered")
	}

	if directive.Bordered {
		t.Error("Expected CreatingSession spinner to not be bordered")
	}
}