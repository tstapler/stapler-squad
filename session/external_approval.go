package session

import (
	"context"
	"github.com/tstapler/stapler-squad/session/detection"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ExternalApprovalSource identifies the source of an external approval.
type ExternalApprovalSource string

const (
	SourceIntelliJ ExternalApprovalSource = "IntelliJ"
	SourceTerminal ExternalApprovalSource = "Terminal"
	SourceVSCode   ExternalApprovalSource = "VS Code"
	SourceMux      ExternalApprovalSource = "mux"
	SourceUnknown  ExternalApprovalSource = "Unknown"
)

// ExternalApprovalEvent represents an approval detected in an external session.
type ExternalApprovalEvent struct {
	Request      *detection.ApprovalRequest
	SessionID    string // Socket path or unique identifier
	SessionTitle string
	Source       ExternalApprovalSource
	Cwd          string
	Command      string
}

// ExternalApprovalCallback is called when an approval is detected.
type ExternalApprovalCallback func(*ExternalApprovalEvent)

// ExternalApprovalMonitor monitors external sessions for approval requests.
type ExternalApprovalMonitor struct {
	detector *detection.ApprovalDetector

	// Active monitoring sessions
	sessions   map[string]*monitoredSession
	sessionsMu sync.RWMutex

	// Callbacks for approval events
	callbacks   []ExternalApprovalCallback
	callbacksMu sync.RWMutex

	// Context for lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// monitoredSession tracks approval monitoring state for one external session.
type monitoredSession struct {
	tmuxStreamer    *ExternalTmuxStreamer // Tmux-based streamer
	title           string
	source          ExternalApprovalSource
	tmuxConsumer    func(content string) // For tmux-based
	tmuxConsumerKey string               // Token for removing tmux consumer
	lastDetect      time.Time
	pending         []*detection.ApprovalRequest
}

// NewExternalApprovalMonitor creates a new external approval monitor.
func NewExternalApprovalMonitor() *ExternalApprovalMonitor {
	return &ExternalApprovalMonitor{
		detector: detection.NewApprovalDetector(),
		sessions: make(map[string]*monitoredSession),
	}
}

// Start begins monitoring for approvals.
func (m *ExternalApprovalMonitor) Start() {
	m.ctx, m.cancel = context.WithCancel(context.Background())
	log.Info("external approval monitor started")
}

// OnApproval registers a callback for approval events.
func (m *ExternalApprovalMonitor) OnApproval(callback ExternalApprovalCallback) {
	m.callbacksMu.Lock()
	m.callbacks = append(m.callbacks, callback)
	m.callbacksMu.Unlock()
}

// StopMonitoringSession stops monitoring a specific session.
func (m *ExternalApprovalMonitor) StopMonitoringSession(socketPath string) {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	if session, exists := m.sessions[socketPath]; exists {
		if session.tmuxStreamer != nil && session.tmuxConsumerKey != "" {
			session.tmuxStreamer.RemoveConsumer(session.tmuxConsumerKey)
		}
		delete(m.sessions, socketPath)
		log.Info("stopped approval monitoring", "path", socketPath)
	}
}

// GetPendingApprovals returns all pending approval requests for a session.
func (m *ExternalApprovalMonitor) GetPendingApprovals(socketPath string) []*detection.ApprovalRequest {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	if session, exists := m.sessions[socketPath]; exists {
		result := make([]*detection.ApprovalRequest, len(session.pending))
		copy(result, session.pending)
		return result
	}

	return nil
}

// GetAllPendingApprovals returns pending approvals across all monitored sessions.
func (m *ExternalApprovalMonitor) GetAllPendingApprovals() map[string][]*detection.ApprovalRequest {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	result := make(map[string][]*detection.ApprovalRequest)
	for socketPath, session := range m.sessions {
		if len(session.pending) > 0 {
			pending := make([]*detection.ApprovalRequest, len(session.pending))
			copy(pending, session.pending)
			result[socketPath] = pending
		}
	}

	return result
}

// notifyCallbacks sends an event to all registered callbacks.
func (m *ExternalApprovalMonitor) notifyCallbacks(event *ExternalApprovalEvent) {
	m.callbacksMu.RLock()
	callbacks := make([]ExternalApprovalCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.callbacksMu.RUnlock()

	for _, callback := range callbacks {
		go func(cb ExternalApprovalCallback) {
			defer func() {
				if r := recover(); r != nil {
					log.Warn("approval callback panic", "err", r)
				}
			}()
			cb(event)
		}(callback)
	}

	log.Info("approval detected in external session", "title", event.SessionTitle, "text", event.Request.DetectedText, "type", event.Request.Type, "confidence", event.Request.Confidence)
}

// MarkApprovalHandled marks an approval request as handled.
func (m *ExternalApprovalMonitor) MarkApprovalHandled(socketPath, requestID string, approved bool) error {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	session, exists := m.sessions[socketPath]
	if !exists {
		return nil
	}

	// Find and update the request
	for i, req := range session.pending {
		if req.ID == requestID {
			if approved {
				req.Status = detection.ApprovalApproved
			} else {
				req.Status = detection.ApprovalRejected
			}
			req.Response = &detection.ApprovalResponse{
				Approved:  approved,
				Timestamp: time.Now(),
			}

			// Remove from pending
			session.pending = append(session.pending[:i], session.pending[i+1:]...)

			return nil
		}
	}

	return nil
}

// guessSource attempts to identify the source terminal from metadata.
func guessSource(sourceTerminal string) ExternalApprovalSource {
	switch sourceTerminal {
	case "IntelliJ":
		return SourceIntelliJ
	case "VS Code":
		return SourceVSCode
	case "Terminal", "Terminal.app":
		return SourceTerminal
	case "iTerm":
		return SourceTerminal
	default:
		return SourceUnknown
	}
}

// IntegrateWithDiscoveryTmux connects the approval monitor to external session discovery
// using tmux-based streaming instead of socket-based streaming.
func (m *ExternalApprovalMonitor) IntegrateWithDiscoveryTmux(
	discovery *ExternalSessionDiscovery,
	tmuxStreamerManager *ExternalTmuxStreamerManager,
) {
	discovery.OnSessionAdded(func(instance *Instance) {
		if instance.ExternalMetadata == nil {
			return
		}

		tmuxSessionName := instance.ExternalMetadata.TmuxSessionName
		if tmuxSessionName == "" {
			return
		}

		// Get or create tmux streamer
		streamer, err := tmuxStreamerManager.GetOrCreate(tmuxSessionName)
		if err != nil {
			log.Warn("failed to create tmux streamer for approval monitoring", "err", err)
			return
		}

		// Determine source
		source := guessSource(instance.ExternalMetadata.SourceTerminal)

		// Start monitoring
		if err := m.MonitorSessionTmux(streamer, tmuxSessionName, instance.Title, source); err != nil {
			log.Warn("failed to start tmux approval monitoring", "title", instance.Title, "err", err)
		}
	})

	discovery.OnSessionRemoved(func(instance *Instance) {
		if instance.ExternalMetadata == nil {
			return
		}

		tmuxSessionName := instance.ExternalMetadata.TmuxSessionName
		if tmuxSessionName != "" {
			m.StopMonitoringSession(tmuxSessionName)
		}
	})
}

// MonitorSessionTmux starts monitoring an external session using tmux-based streaming.
func (m *ExternalApprovalMonitor) MonitorSessionTmux(
	streamer *ExternalTmuxStreamer,
	tmuxSessionName string,
	title string,
	source ExternalApprovalSource,
) error {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	// Check if already monitoring
	if _, exists := m.sessions[tmuxSessionName]; exists {
		return nil // Already monitoring
	}

	// Create monitored session
	monitored := &monitoredSession{
		tmuxStreamer: streamer,
		title:        title,
		source:       source,
		pending:      make([]*detection.ApprovalRequest, 0),
	}

	// Create consumer that processes content for approvals
	monitored.tmuxConsumer = m.createTmuxConsumer(tmuxSessionName, monitored)

	// Register consumer with tmux streamer; store key for reliable removal
	monitored.tmuxConsumerKey = streamer.AddConsumer(monitored.tmuxConsumer)

	m.sessions[tmuxSessionName] = monitored

	log.Info("started tmux approval monitoring for external session", "title", title, "source", source)

	return nil
}

// createTmuxConsumer creates a consumer function for tmux-based streaming.
// Unlike socket streaming, tmux provides full terminal content on each update.
func (m *ExternalApprovalMonitor) createTmuxConsumer(sessionID string, session *monitoredSession) func(content string) {
	return func(content string) {
		// For tmux streaming, we get full terminal content each time
		// Detect approvals in the content
		requests := m.detector.Detect(content)

		if len(requests) > 0 {
			session.lastDetect = time.Now()

			for _, request := range requests {
				// Check if we already have this request pending (avoid duplicates)
				isDuplicate := false
				for _, existing := range session.pending {
					if existing.DetectedText == request.DetectedText {
						isDuplicate = true
						break
					}
				}

				if !isDuplicate {
					// Track as pending
					session.pending = append(session.pending, request)

					// Create event
					event := &ExternalApprovalEvent{
						Request:      request,
						SessionID:    sessionID,
						SessionTitle: session.title,
						Source:       session.source,
					}

					// Notify callbacks
					m.notifyCallbacks(event)
				}
			}
		}
	}
}
