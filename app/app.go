package app

import (
	"claude-squad/config"
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 100

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
	// stateCreatingSession is the state when a session is being created asynchronously.
	stateCreatingSession
	// stateAdvancedNew is the state when the user is using the advanced session setup.
	stateAdvancedNew
	// stateGit is the state when the git status overlay is displayed.
	stateGit
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// asyncSessionCreation is used for tracking async session creation
	asyncSessionCreationActive bool

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// sessionSetupOverlay handles advanced session creation
	sessionSetupOverlay *overlay.SessionSetupOverlay
	// gitStatusOverlay provides fugitive-style git interface
	gitStatusOverlay *overlay.GitStatusOverlay
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state with built-in locking
	appState := config.LoadState()

	// Initialize storage with the state
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
	}
	h.list = ui.NewList(&h.spinner, autoYes, appState)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}
	
	// Restore the last selected index if it's still valid
	lastSelectedIdx := appState.GetSelectedIndex()
	if lastSelectedIdx >= 0 && lastSelectedIdx < h.list.NumInstances() {
		h.list.SetSelectedIdx(lastSelectedIdx)
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}
	if m.sessionSetupOverlay != nil {
		m.sessionSetupOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	if m.gitStatusOverlay != nil {
		m.gitStatusOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	cmds := []tea.Cmd{
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
	}
	
	// Add session detection ticker if enabled
	if m.appConfig.DetectNewSessions {
		cmds = append(cmds, tickSessionDetectionCmd(m.appConfig.SessionDetectionInterval))
	}
	
	return tea.Batch(cmds...)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ensure categories are organized whenever the model updates
	m.list.OrganizeByCategory()
	
	switch msg := msg.(type) {
	case sessionCreationResultMsg:
		m.asyncSessionCreationActive = false
		
		// Handle error if session creation failed
		if msg.err != nil {
			m.list.Kill()
			m.state = stateDefault
			return m, m.handleError(msg.err)
		}
		
		// Save after adding new instance
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		
		// Instance added successfully, call the finalizer.
		m.newInstanceFinalizer()
		if msg.autoYes {
			msg.instance.AutoYes = true
		}

		// Reset state
		m.state = stateDefault
		if msg.promptAfterName {
			m.state = statePrompt
			m.menu.SetState(ui.StatePrompt)
			// Initialize the text input overlay
			m.textInputOverlay = overlay.NewTextInputOverlay("Enter prompt", "")
			m.promptAfterName = false
		} else {
			m.menu.SetState(ui.StateDefault)
			m.showHelpScreen(helpStart(msg.instance), nil)
		}

		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		// First check if the selected instance exists and isn't paused before updating
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() {
			return m, func() tea.Msg {
				time.Sleep(500 * time.Millisecond) // Slower refresh for paused/empty instances
				return previewTickMsg{}
			}
		}

		// Update the UI with the selected instance
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(100 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		for _, instance := range m.list.GetInstances() {
			if !instance.Started() {
				continue
			}
			
			// Check if the instance is paused before updating
			if instance.Paused() {
				continue
			}
			
			// Track if the instance was already paused before updating diff stats
			wasPaused := instance.Paused()

			// Check if worktree path exists (if it doesn't, the UpdateDiffStats call will mark it as paused)
			if err := instance.UpdateDiffStats(); err != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
				continue // Skip the rest of the updates if we can't update diff stats
			}

			// If the instance was newly marked as paused by UpdateDiffStats, trigger UI refresh
			if !wasPaused && instance.Paused() {
				// Force refresh UI layout to handle the changed state
				return m, tea.Sequence(tea.WindowSize(), tickUpdateMetadataCmd)
			}

			// If the instance is now paused, skip the rest of the updates
			if instance.Paused() {
				continue
			}
			
			updated, prompt := instance.HasUpdated()
			if updated {
				instance.SetStatus(session.Running)
			} else {
				if prompt {
					if instance.AutoYes {
						instance.TapEnter()
					} else {
						instance.SetStatus(session.NeedsApproval)
					}
				} else {
					instance.SetStatus(session.Ready)
				}
			}
		}
		return m, tickUpdateMetadataCmd
	case tickSessionDetectionMessage:
		// Check for new sessions created by other instances
		if m.appConfig.DetectNewSessions {
			if err := m.detectAndLoadNewSessions(); err != nil {
				log.WarningLog.Printf("failed to detect new sessions: %v", err)
			}
		}
		return m, tea.Batch(tickSessionDetectionCmd(m.appConfig.SessionDetectionInterval), tea.WindowSize())
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case gitStatusLoadedMsg:
		// Handle git status data loaded
		if m.gitStatusOverlay != nil {
			m.gitStatusOverlay.SetFiles(msg.files, msg.branchName)
		}
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	// Save instances before quitting
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}
	
	// Release any locks held by the state manager
	if stateManager, ok := m.storage.GetStateManager().(config.StateManager); ok {
		if err := stateManager.Close(); err != nil {
			log.WarningLog.Printf("Failed to close state manager: %v", err)
			// Continue with quit anyway
		}
	}
	
	return m, tea.Quit
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	// Skip the menu highlighting if the key is not in the map or we are using the shift up and down keys.
	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}
	
	if m.state == stateAdvancedNew {
		return m.handleAdvancedSessionSetupUpdate(msg)
	}

	if m.state == stateGit {
		return m.handleGitState(msg)
	}

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Set state to creating session and update menu
			m.state = stateCreatingSession
			m.asyncSessionCreationActive = true
			m.menu.SetState(ui.StateCreatingInstance)
			
			// Start session creation in a goroutine
			// Store important values before the goroutine starts
			promptAfterName := m.promptAfterName
			autoYes := m.autoYes
			
			// Return the tick command to send a message after session is created
			return m, func() tea.Msg {
				// Start the instance in a blocking operation
				err := instance.Start(true)
				
				// Return a message with the result
				return sessionCreationResultMsg{
					instance:        instance,
					err:            err,
					promptAfterName: promptAfterName,
					autoYes:         autoYes,
				}
			}
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the new TextInputOverlay component to handle all key events
		shouldClose := m.textInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			// Get the current menu state
			// Handle search mode differently than prompt mode
			if m.menu.GetState() == ui.StateSearch {
				// Close the overlay and reset state
				m.textInputOverlay = nil
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
				return m, tea.WindowSize()
			}
			
			// Regular prompt handling
			selected := m.list.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if m.textInputOverlay.IsSubmitted() {
				if err := selected.SendPrompt(m.textInputOverlay.GetValue()); err != nil {
					// TODO: we probably end up in a bad state here.
					return m, m.handleError(err)
				}
			}

			// Close the overlay and reset state
			m.textInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					m.showHelpScreen(helpStart(selected), nil)
					return nil
				},
			)
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			m.state = stateDefault
			m.confirmationOverlay = nil
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyEsc:
		// Handle escape key for search mode
		if m.state == statePrompt && m.menu.GetState() == ui.StateSearch {
			m.list.ExitSearchMode()
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			m.textInputOverlay = nil
			return m, tea.WindowSize()
		}
		return m, nil
	case keys.KeyPrompt:
		// Use the advanced session setup overlay
		return m.handleAdvancedSessionSetup()
	case keys.KeyRight:
		// Expand the selected category
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			category := selected.Category
			if category == "" {
				category = "Uncategorized"
			}
			m.list.ExpandCategory(category)
		}
		return m, nil
	case keys.KeyLeft:
		// Collapse the selected category
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			category := selected.Category
			if category == "" {
				category = "Uncategorized"
			}
			m.list.CollapseCategory(category)
		}
		return m, nil
	case keys.KeyToggleGroup:
		// Toggle expand/collapse of the selected category
		selected := m.list.GetSelectedInstance()
		if selected != nil {
			category := selected.Category
			if category == "" {
				category = "Uncategorized"
			}
			m.list.ToggleCategory(category)
		}
		return m, nil
	case keys.KeySearch:
		// Create the text input overlay for search with previous query pre-populated
		_, previousQuery := m.list.GetSearchState()
		m.textInputOverlay = overlay.NewTextInputOverlay("Search Sessions", previousQuery)
		
		// Set callback for when search is submitted
		m.textInputOverlay.SetOnSubmitWithValue(func(query string) {
			if query == "" {
				// Exit search mode if query is empty
				m.list.ExitSearchMode()
			} else {
				// Search by title
				m.list.SearchByTitle(query)
			}
		})
		
		// Set callback for when search is canceled
		m.textInputOverlay.SetOnCancel(func() {
			m.list.ExitSearchMode()
		})
		
		// Set search mode state
		m.state = statePrompt
		m.menu.SetState(ui.StateSearch)
		return m, nil
	case keys.KeyFilterPaused:
		// Toggle paused session filter
		m.list.TogglePausedFilter()
		return m, nil
	case keys.KeyClearFilters:
		// Clear all filters and search
		m.list.ClearAllFilters()
		return m, tea.WindowSize()
	case keys.KeyGit:
		// Open git status interface
		return m.handleGitStatus()
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.list.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.list.Down()
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, m.instanceChanged()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the kill action as a tea.Cmd
		killAction := func() tea.Msg {
			// Get worktree and check if branch is checked out
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}

			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}

			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", selected.Title)
			}

			// Delete from storage first
			if err := m.storage.DeleteInstance(selected.Title); err != nil {
				return err
			}

			// Then kill the instance
			m.list.Kill()
			return instanceChangedMsg{}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout{}, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			ch, err := m.list.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	default:
		return m, nil
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	// Ensure categories are organized
	m.list.OrganizeByCategory()

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

type tickSessionDetectionMessage struct{}

type instanceChangedMsg struct{}

// sessionCreationResultMsg is sent when async session creation completes
type sessionCreationResultMsg struct {
	instance        *session.Instance
	err             error
	promptAfterName bool
	autoYes         bool
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return tickUpdateMetadataMessage{}
}

// tickSessionDetectionCmd creates a ticker for detecting new sessions created by other instances
func tickSessionDetectionCmd(intervalMs int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		return tickSessionDetectionMessage{}
	}
}

// detectAndLoadNewSessions checks for new sessions created by other instances and adds them to the current list
func (m *home) detectAndLoadNewSessions() error {
	// Load all instances from storage (this will get the latest state including new sessions)
	allStorageInstances, err := m.storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances from storage: %w", err)
	}

	// Create a map of current instances by title for quick lookup
	currentInstances := make(map[string]*session.Instance)
	for _, instance := range m.list.GetInstances() {
		currentInstances[instance.Title] = instance
	}

	// Check for new instances that aren't in our current list
	newInstancesFound := false
	for _, storageInstance := range allStorageInstances {
		if _, exists := currentInstances[storageInstance.Title]; !exists {
			// This is a new instance we don't have yet
			log.InfoLog.Printf("Detected new session created by another instance: %s", storageInstance.Title)
			
			// Add the new instance to our list
			finalizer := m.list.AddInstance(storageInstance)
			finalizer() // Call finalizer to complete setup
			
			newInstancesFound = true
		}
	}

	// If we found new instances, save our updated list and trigger UI refresh
	if newInstancesFound {
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			log.WarningLog.Printf("failed to save instances after detecting new sessions: %v", err)
		}
		log.InfoLog.Printf("Successfully loaded new sessions from other instances")
	}

	return nil
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		// Execute the action if it exists
		if action != nil {
			_ = action()
		}
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
	}

	return nil
}

// handleGitStatus opens the git status overlay interface
func (m *home) handleGitStatus() (tea.Model, tea.Cmd) {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for git operations"))
	}

	// Check if session has git integration
	if !selected.Started() || selected.Paused() {
		return m, m.handleError(fmt.Errorf("session must be started and active for git operations"))
	}

	// Create and configure git status overlay
	m.gitStatusOverlay = overlay.NewGitStatusOverlay()
	m.gitStatusOverlay.SetSize(int(float32(m.list.NumInstances())*0.8), int(float32(m.list.NumInstances())*0.8))
	
	// Set up git operation callbacks
	m.setupGitCallbacks(selected)
	
	// Change to git state
	m.state = stateGit
	
	// Load git status data
	return m, m.loadGitStatus(selected)
}

// setupGitCallbacks configures the git operation callbacks for the overlay
func (m *home) setupGitCallbacks(instance *session.Instance) {
	m.gitStatusOverlay.OnCancel = func() {
		m.state = stateDefault
		m.gitStatusOverlay = nil
	}
	
	// TODO: Implement these callbacks with actual git operations
	m.gitStatusOverlay.OnStageFile = func(path string) error {
		// Placeholder - will implement in next task
		return fmt.Errorf("staging not yet implemented")
	}
	
	m.gitStatusOverlay.OnUnstageFile = func(path string) error {
		// Placeholder - will implement in next task
		return fmt.Errorf("unstaging not yet implemented")
	}
	
	m.gitStatusOverlay.OnCommit = func() error {
		// Placeholder - will implement in next task
		return fmt.Errorf("commit not yet implemented")
	}
	
	m.gitStatusOverlay.OnPush = func() error {
		// Use existing push functionality
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", instance.Title, time.Now().Format(time.RFC822))
		return worktree.PushChanges(commitMsg, true)
	}
}

// handleGitState processes key events when in git status mode
func (m *home) handleGitState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.gitStatusOverlay == nil {
		m.state = stateDefault
		return m, nil
	}

	shouldClose := m.gitStatusOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.state = stateDefault
		m.gitStatusOverlay = nil
	}
	
	return m, nil
}

// loadGitStatus loads git status information for the overlay
func (m *home) loadGitStatus(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// TODO: Implement actual git status loading
		// For now, return some mock data to test the interface
		files := []overlay.GitFileStatus{
			{Path: "app/app.go", Staged: false, Modified: true, StatusChar: "M"},
			{Path: "keys/keys.go", Staged: true, Modified: false, StatusChar: "A"},
			{Path: "ui/overlay/gitStatusOverlay.go", Staged: false, Modified: false, Untracked: true, StatusChar: "??"},
		}
		
		return gitStatusLoadedMsg{
			files:      files,
			branchName: "feature/fugitive-git-integration",
		}
	}
}

// gitStatusLoadedMsg carries git status data
type gitStatusLoadedMsg struct {
	files      []overlay.GitFileStatus
	branchName string
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == statePrompt {
		if m.textInputOverlay == nil {
			log.ErrorLog.Printf("text input overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	} else if m.state == stateCreatingSession {
		// Show spinner or progress indicator when creating session
		creatingMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render(m.spinner.View() + " Creating session...")
		return overlay.PlaceOverlay(0, 0, creatingMsg, mainView, true, false)
	} else if m.state == stateAdvancedNew {
		if m.sessionSetupOverlay == nil {
			log.ErrorLog.Printf("session setup overlay is nil")
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.sessionSetupOverlay.View(), mainView, true, true)
	} else if m.state == stateGit {
		if m.gitStatusOverlay == nil {
			log.ErrorLog.Printf("git status overlay is nil")
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.gitStatusOverlay.View(), mainView, true, true)
	}

	return mainView
}
