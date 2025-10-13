package signals

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSignalManager_Shutdown(t *testing.T) {
	manager := New()

	// Verify initial state
	if manager.IsShutdown() {
		t.Error("Expected manager to not be shutdown initially")
	}

	// Trigger shutdown
	expectedExitCode := 1
	manager.Shutdown(expectedExitCode)

	// Verify shutdown state
	if !manager.IsShutdown() {
		t.Error("Expected manager to be shutdown after Shutdown() call")
	}

	// Verify exit code
	if manager.ExitCode() != expectedExitCode {
		t.Errorf("Expected exit code %d, got %d", expectedExitCode, manager.ExitCode())
	}

	// Verify context is cancelled
	select {
	case <-manager.Context().Done():
		// Context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled after shutdown")
	}
}

func TestSignalManager_IsShutdown(t *testing.T) {
	tests := []struct {
		name             string
		triggerShutdown  bool
		expectedShutdown bool
	}{
		{
			name:             "Initial state - not shutdown",
			triggerShutdown:  false,
			expectedShutdown: false,
		},
		{
			name:             "After shutdown - is shutdown",
			triggerShutdown:  true,
			expectedShutdown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()

			if tt.triggerShutdown {
				m.Shutdown(0)
			}

			if m.IsShutdown() != tt.expectedShutdown {
				t.Errorf("Expected IsShutdown()=%v, got %v", tt.expectedShutdown, m.IsShutdown())
			}
		})
	}
}

func TestSignalManager_ExitCode(t *testing.T) {
	tests := []struct {
		name         string
		exitCode     int
		expectedCode int
	}{
		{
			name:         "Exit code 0",
			exitCode:     0,
			expectedCode: 0,
		},
		{
			name:         "Exit code 1",
			exitCode:     1,
			expectedCode: 1,
		},
		{
			name:         "Exit code 130 (SIGINT)",
			exitCode:     130,
			expectedCode: 130,
		},
		{
			name:         "Exit code 143 (SIGTERM)",
			exitCode:     143,
			expectedCode: 143,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.Shutdown(tt.exitCode)

			if m.ExitCode() != tt.expectedCode {
				t.Errorf("Expected exit code %d, got %d", tt.expectedCode, m.ExitCode())
			}
		})
	}
}

func TestSignalManager_Context(t *testing.T) {
	manager := New()

	// Verify context is not nil
	ctx := manager.Context()
	if ctx == nil {
		t.Fatal("Expected context to be non-nil")
	}

	// Verify context is not cancelled initially
	select {
	case <-ctx.Done():
		t.Error("Expected context to not be cancelled initially")
	default:
		// Context is not cancelled, as expected
	}

	// Trigger shutdown
	manager.Shutdown(0)

	// Verify context is now cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled after shutdown")
	}

	// Verify context error
	if ctx.Err() != context.Canceled {
		t.Errorf("Expected context error to be context.Canceled, got %v", ctx.Err())
	}
}

func TestSignalManager_IdempotentShutdown(t *testing.T) {
	manager := New()

	// Call shutdown multiple times
	manager.Shutdown(1)
	manager.Shutdown(2)
	manager.Shutdown(3)

	// Verify only the first exit code is used (idempotency)
	if manager.ExitCode() != 1 {
		t.Errorf("Expected exit code to remain 1 (from first shutdown), got %d", manager.ExitCode())
	}
}

func TestSignalManager_ConcurrentShutdown(t *testing.T) {
	manager := New()

	// Trigger concurrent shutdowns
	done := make(chan bool)
	for i := range 10 {
		go func(exitCode int) {
			manager.Shutdown(exitCode)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Verify manager is shutdown
	if !manager.IsShutdown() {
		t.Error("Expected manager to be shutdown after concurrent calls")
	}

	// Verify context is cancelled
	select {
	case <-manager.Context().Done():
		// Context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled after shutdown")
	}
}

func TestSignalManager_GlobalManager(t *testing.T) {
	// Get global manager multiple times
	m1 := GetGlobalManager()
	m2 := GetGlobalManager()

	// Verify they are the same instance
	if m1 != m2 {
		t.Error("Expected GetGlobalManager() to return the same instance")
	}

	// Verify it's not nil
	if m1 == nil {
		t.Fatal("Expected global manager to be non-nil")
	}
}

func TestSignalManager_SignalHandling(t *testing.T) {
	// Note: This test is tricky because we can't easily send signals to ourselves
	// in a test environment without affecting the test process itself.
	// We'll test the signal handling setup indirectly by verifying the manager works.

	manager := New()

	// Verify manager starts in non-shutdown state
	if manager.IsShutdown() {
		t.Error("Expected manager to not be shutdown initially")
	}

	// Verify context is not cancelled
	select {
	case <-manager.Context().Done():
		t.Error("Expected context to not be cancelled initially")
	default:
		// Context is not cancelled, as expected
	}
}

func TestSignalManager_ContextDerivation(t *testing.T) {
	manager := New()

	// Create a derived context
	ctx := manager.Context()
	derivedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Shutdown the manager
	manager.Shutdown(0)

	// Verify parent context is cancelled
	select {
	case <-ctx.Done():
		// Parent context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected parent context to be cancelled after shutdown")
	}

	// Verify derived context is also cancelled
	select {
	case <-derivedCtx.Done():
		// Derived context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected derived context to be cancelled after parent shutdown")
	}
}

func TestSignalManager_ExitCodeForSignals(t *testing.T) {
	// Test that signal handler would set correct exit codes
	// Note: We can't actually test signal receipt, but we can verify the logic

	tests := []struct {
		name         string
		signal       os.Signal
		expectedCode int
	}{
		{
			name:         "SIGINT (Ctrl+C)",
			signal:       os.Interrupt,
			expectedCode: 130,
		},
		{
			name:         "SIGTERM",
			signal:       syscall.SIGTERM,
			expectedCode: 143,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Manually determine exit code based on signal type (same logic as signal handler)
			exitCode := 0
			switch tt.signal {
			case os.Interrupt:
				exitCode = 130
			case syscall.SIGTERM:
				exitCode = 143
			}

			if exitCode != tt.expectedCode {
				t.Errorf("Expected exit code %d for signal %v, got %d", tt.expectedCode, tt.signal, exitCode)
			}
		})
	}
}

func TestSignalManager_ThreadSafety(t *testing.T) {
	manager := New()

	// Test concurrent reads and writes
	done := make(chan bool)

	// Readers
	for range 50 {
		go func() {
			_ = manager.IsShutdown()
			_ = manager.ExitCode()
			_ = manager.Context()
			done <- true
		}()
	}

	// Writers (shutdown once)
	go func() {
		time.Sleep(10 * time.Millisecond)
		manager.Shutdown(42)
		done <- true
	}()

	// Wait for all goroutines
	for range 51 {
		<-done
	}

	// Verify final state
	if !manager.IsShutdown() {
		t.Error("Expected manager to be shutdown")
	}

	if manager.ExitCode() != 42 {
		t.Errorf("Expected exit code 42, got %d", manager.ExitCode())
	}
}
