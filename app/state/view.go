package state

// ViewDirectiveType defines the type of view rendering instruction
type ViewDirectiveType int

const (
	// ViewMain renders the main view without overlays
	ViewMain ViewDirectiveType = iota
	// ViewOverlay renders an overlay on top of the main view
	ViewOverlay
	// ViewText renders a text overlay with centering
	ViewText
	// ViewSpinner renders a progress/loading overlay
	ViewSpinner
)

// ViewDirective provides instructions for how the UI should render based on state
type ViewDirective struct {
	// Type specifies the rendering mode
	Type ViewDirectiveType

	// Overlay specifies which overlay component to render (if any)
	OverlayComponent string

	// Message provides text content for text-based overlays
	Message string

	// Centered indicates if overlay should be centered
	Centered bool

	// Bordered indicates if overlay should have a border
	Bordered bool

	// ShouldResetOnNil indicates if state should reset to default when overlay is nil
	ShouldResetOnNil bool
}

// ViewManager provides view rendering instructions based on application state
type ViewManager interface {
	// GetViewDirective returns the appropriate view directive for current state
	GetViewDirective() ViewDirective

	// ShouldRenderOverlay returns true if an overlay should be rendered
	ShouldRenderOverlay() bool

	// GetOverlayComponent returns the component name that should render the overlay
	GetOverlayComponent() string
}

// GetViewDirective returns the view directive for the current state
func (m *manager) GetViewDirective() ViewDirective {
	switch m.current {
	case Default:
		return ViewDirective{
			Type: ViewMain,
		}

	case New:
		return ViewDirective{
			Type:              ViewMain, // New state uses inline editing, not overlay
			OverlayComponent:  "",
			ShouldResetOnNil:  false,
		}

	case Prompt:
		// Check context to determine if this is search or text input
		if m.transitionContext.MenuState == "Search" {
			return ViewDirective{
				Type:              ViewOverlay,
				OverlayComponent:  "liveSearchOverlay",
				Centered:          true,
				Bordered:          true,
				ShouldResetOnNil:  true,
			}
		}
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "textInputOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case Help:
		// Check if this is messages overlay or text overlay
		if m.transitionContext.OverlayName == "messages" {
			return ViewDirective{
				Type:              ViewOverlay,
				OverlayComponent:  "messagesOverlay",
				Centered:          true,
				Bordered:          true,
				ShouldResetOnNil:  true,
			}
		}
		return ViewDirective{
			Type:              ViewText,
			OverlayComponent:  "textOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case Confirm:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "confirmationOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case CreatingSession:
		return ViewDirective{
			Type:     ViewSpinner,
			Message:  "Creating session...\n\nThis may take up to 60 seconds if starting Claude or Aider for the first time.\nPress Ctrl+C to cancel if needed.",
			Centered: true,
			Bordered: false,
		}

	case AdvancedNew:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "sessionSetupOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case Git:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "gitStatusOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case ClaudeSettings:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "claudeSettingsOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case ZFSearch:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "zfSearchOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case TagEditor:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "tagEditorOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case HistoryBrowser:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "historyBrowserOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case ConfigEditor:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "configEditorOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case Rename:
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  "renameInputOverlay",
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	case Workspace:
		// Check context to determine which workspace overlay is being shown
		overlayName := "workspaceSwitchOverlay"
		if m.transitionContext.OverlayName == "workspaceStatus" {
			overlayName = "workspaceStatusOverlay"
		}
		return ViewDirective{
			Type:              ViewOverlay,
			OverlayComponent:  overlayName,
			Centered:          true,
			Bordered:          true,
			ShouldResetOnNil:  true,
		}

	default:
		// Fallback to main view for unknown states
		return ViewDirective{
			Type: ViewMain,
		}
	}
}

// ShouldRenderOverlay returns true if current state requires overlay rendering
func (m *manager) ShouldRenderOverlay() bool {
	directive := m.GetViewDirective()
	return directive.Type == ViewOverlay || directive.Type == ViewText || directive.Type == ViewSpinner
}

// GetOverlayComponent returns the component name for overlay rendering
func (m *manager) GetOverlayComponent() string {
	return m.GetViewDirective().OverlayComponent
}