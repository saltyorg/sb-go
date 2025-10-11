package signals

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Manager handles graceful shutdown of the application via signal handling.
// It provides a centralized, thread-safe mechanism for managing OS signals
// and canceling operations across the application.
type Manager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	shutdownMu sync.RWMutex
	isShutdown bool
	exitCode   int
}

var (
	// globalManager is the singleton instance of the signal manager
	globalManager *Manager
	// initOnce ensures the global manager is initialized only once
	initOnce sync.Once
)

// New creates a new signal manager with proper signal handling setup.
// It returns a Manager instance that can be used to manage graceful shutdowns.
func New() *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		ctx:      ctx,
		cancel:   cancel,
		exitCode: 0,
	}

	// Start signal handling
	m.startSignalHandler()

	return m
}

// GetGlobalManager returns the singleton global signal manager instance.
// It initializes the manager on the first call using sync.Once for thread-safety.
func GetGlobalManager() *Manager {
	initOnce.Do(func() {
		globalManager = New()
	})
	return globalManager
}

// Context returns the context that will be canceled when a signal is received.
// This context should be passed to all long-running operations to enable graceful shutdown.
func (m *Manager) Context() context.Context {
	return m.ctx
}

// IsShutdown returns true if a shutdown signal has been received.
// This method is thread-safe and can be used to check shutdown state without blocking.
func (m *Manager) IsShutdown() bool {
	m.shutdownMu.RLock()
	defer m.shutdownMu.RUnlock()
	return m.isShutdown
}

// ExitCode returns the exit code that should be used when the application exits.
// Returns 130 for SIGINT (Ctrl+C), 143 for SIGTERM, or 0 for normal termination.
func (m *Manager) ExitCode() int {
	m.shutdownMu.RLock()
	defer m.shutdownMu.RUnlock()
	return m.exitCode
}

// Shutdown initiates a graceful shutdown by canceling the context.
// It can be called manually or is automatically triggered by OS signals.
// This method is idempotent - subsequent calls have no effect.
func (m *Manager) Shutdown(exitCode int) {
	m.once.Do(func() {
		m.shutdownMu.Lock()
		m.isShutdown = true
		m.exitCode = exitCode
		m.shutdownMu.Unlock()
		m.cancel()
	})
}

// startSignalHandler sets up the signal handler goroutine.
// It listens for SIGINT and SIGTERM and initiates shutdown when received.
func (m *Manager) startSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan

		// Determine exit code based on signal type
		exitCode := 0
		switch sig {
		case os.Interrupt:
			exitCode = 130 // Standard exit code for SIGINT (128 + 2)
		case syscall.SIGTERM:
			exitCode = 143 // Standard exit code for SIGTERM (128 + 15)
		}

		// Clean up signal notification
		signal.Stop(sigChan)
		close(sigChan)

		// Initiate shutdown
		m.Shutdown(exitCode)
	}()
}
