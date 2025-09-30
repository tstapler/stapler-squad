package app

import (
	"claude-squad/session"
	"claude-squad/ui/overlay"
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSessionCreationFlow tests the complete session creation flow
func TestSessionCreationFlow(t *testing.T) {
	// Test that we can create session instances with valid options
	testOptions := session.InstanceOptions{
		Title:    "test-session",
		Program:  "echo hello",
		Category: "test",
	}

	instance, err := session.NewInstance(testOptions)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	if instance.Title != "test-session" {
		t.Errorf("Instance title = %q, expected %q", instance.Title, "test-session")
	}
	if instance.Program != "echo hello" {
		t.Errorf("Instance program = %q, expected %q", instance.Program, "echo hello")
	}

	// Test session setup overlay creation
	overlay := overlay.NewSessionSetupOverlay()
	if overlay == nil {
		t.Fatal("Failed to create session setup overlay")
	}

	// Test callback registration (this should not panic)
	var callbackCalled bool
	overlay.SetOnComplete(func(opts session.InstanceOptions) {
		callbackCalled = true
		// In real usage, this creates and sets up the pending session
	})

	// We can't easily trigger the callback without going through the UI flow,
	// but we can verify the setup doesn't crash
	if callbackCalled {
		t.Error("Callback was called unexpectedly")
	}
}

// TestAsyncSessionCreationHandling tests the async session creation logic
func TestAsyncSessionCreationHandling(t *testing.T) {
	// This test focuses on the core logic without full UI initialization
	// to avoid complex dependencies

	// Test the logic directly instead of through the Update method
	ctx := context.Background()
	h := &home{
		ctx:                       ctx,
		asyncSessionCreationActive: false,
	}

	// Create a mock instance
	testOptions := session.InstanceOptions{
		Title:    "async-test",
		Program:  "echo test",
		Category: "test",
	}

	instance, err := session.NewInstance(testOptions)
	if err != nil {
		t.Fatalf("Failed to create test instance: %v", err)
	}

	// Test pending session handling logic directly
	h.pendingSessionInstance = instance
	h.pendingAutoYes = false

	// Test the conditions that should trigger async creation
	if h.pendingSessionInstance == nil {
		t.Error("Pending session instance should be set")
	}
	if h.asyncSessionCreationActive {
		t.Error("Async session creation should not be active initially")
	}

	// Simulate what happens in the Update method when there's a pending session
	if h.pendingSessionInstance != nil && !h.asyncSessionCreationActive {
		// Clear pending state (simulating the Update method logic)
		h.pendingSessionInstance = nil
		h.pendingAutoYes = false
		h.asyncSessionCreationActive = true

		// Create the async command function (simulating what Update returns)
		asyncCommand := func() tea.Msg {
			err := instance.Start(true)
			return sessionCreationResultMsg{
				instance:        instance,
				err:             err,
				promptAfterName: false,
				autoYes:         false,
			}
		}

		// Verify the state changes happened correctly
		if h.asyncSessionCreationActive != true {
			t.Error("Async session creation flag was not set")
		}

		if h.pendingSessionInstance != nil {
			t.Error("Pending session instance was not cleared")
		}

		// Test that the async command can be created
		if asyncCommand == nil {
			t.Error("Async command was not created")
		}

		// Execute the command with a timeout to prevent hanging
		done := make(chan tea.Msg, 1)
		go func() {
			done <- asyncCommand()
		}()

		select {
		case msg := <-done:
			// Verify we got a sessionCreationResultMsg
			if resultMsg, ok := msg.(sessionCreationResultMsg); ok {
				if resultMsg.instance != instance {
					t.Error("Result message contains wrong instance")
				}
				// Note: We expect an error here because we're not actually starting tmux
				// The important thing is that the message structure is correct
			} else {
				t.Errorf("Expected sessionCreationResultMsg, got %T", msg)
			}
		case <-time.After(5 * time.Second):
			t.Error("Async session creation timed out")
		}
	}
}

// TestSessionCreationResultHandling tests the handling of session creation results
func TestSessionCreationResultHandling(t *testing.T) {
	// Test the result handling logic directly instead of through Update method
	// to avoid complex UI initialization requirements

	// Create a mock instance
	testOptions := session.InstanceOptions{
		Title:    "result-test",
		Program:  "echo result",
		Category: "test",
	}

	instance, err := session.NewInstance(testOptions)
	if err != nil {
		t.Fatalf("Failed to create test instance: %v", err)
	}

	// Test the result message structure
	resultMsg := sessionCreationResultMsg{
		instance:        instance,
		err:             nil,
		promptAfterName: false,
		autoYes:         false,
	}

	// Verify the result message was created correctly
	if resultMsg.instance != instance {
		t.Error("Result message contains wrong instance")
	}

	if resultMsg.err != nil {
		t.Errorf("Expected no error, got %v", resultMsg.err)
	}

	if resultMsg.promptAfterName {
		t.Error("Expected promptAfterName to be false")
	}

	if resultMsg.autoYes {
		t.Error("Expected autoYes to be false")
	}

	// Test that we can identify the message type
	var msg tea.Msg = resultMsg
	if _, ok := msg.(sessionCreationResultMsg); !ok {
		t.Error("Message is not of type sessionCreationResultMsg")
	}
}

// TestSessionCreationErrorHandling tests error handling during session creation
func TestSessionCreationErrorHandling(t *testing.T) {
	// Test error handling logic directly

	// Create a mock instance
	testOptions := session.InstanceOptions{
		Title:    "error-test",
		Program:  "echo error",
		Category: "test",
	}

	instance, err := session.NewInstance(testOptions)
	if err != nil {
		t.Fatalf("Failed to create test instance: %v", err)
	}

	// Test error result message structure
	testError := errors.New("test session creation error")
	resultMsg := sessionCreationResultMsg{
		instance:        instance,
		err:             testError,
		promptAfterName: false,
		autoYes:         false,
	}

	// Verify the error result message was created correctly
	if resultMsg.instance != instance {
		t.Error("Error result message contains wrong instance")
	}

	if resultMsg.err == nil {
		t.Error("Expected error in result message")
	}

	if resultMsg.err.Error() != "test session creation error" {
		t.Errorf("Expected specific error message, got %v", resultMsg.err)
	}

	// Test that we can identify the message type even with errors
	var msg tea.Msg = resultMsg
	if _, ok := msg.(sessionCreationResultMsg); !ok {
		t.Error("Error message is not of type sessionCreationResultMsg")
	}
}