package signals

import (
	"context"
	"sync"
)

// Manager owns cancellation and the requested process exit code. OS signal
// registration belongs to main; interactive commands invoke Shutdown through
// a callback stored in their context.
type Manager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	shutdownMu sync.RWMutex
	isShutdown bool
	exitCode   int
}

func New(parent ...context.Context) *Manager {
	base := context.Background()
	if len(parent) > 0 && parent[0] != nil {
		base = parent[0]
	}
	ctx, cancel := context.WithCancel(base)
	return &Manager{ctx: ctx, cancel: cancel}
}

func (m *Manager) Context() context.Context { return m.ctx }

func (m *Manager) IsShutdown() bool {
	m.shutdownMu.RLock()
	defer m.shutdownMu.RUnlock()
	return m.isShutdown
}

func (m *Manager) ExitCode() int {
	m.shutdownMu.RLock()
	defer m.shutdownMu.RUnlock()
	return m.exitCode
}

func (m *Manager) Shutdown(exitCode int) {
	m.once.Do(func() {
		m.shutdownMu.Lock()
		m.isShutdown = true
		m.exitCode = exitCode
		m.shutdownMu.Unlock()
		m.cancel()
	})
}

type shutdownKey struct{}

// WithShutdown attaches an explicit process shutdown callback to a command
// context without introducing process-global state.
func WithShutdown(ctx context.Context, shutdown func(int)) context.Context {
	if shutdown == nil {
		shutdown = func(int) {}
	}
	return context.WithValue(ctx, shutdownKey{}, shutdown)
}

// Shutdown requests process cancellation when a callback was provided.
func Shutdown(ctx context.Context, exitCode int) {
	if shutdown, ok := ctx.Value(shutdownKey{}).(func(int)); ok {
		shutdown(exitCode)
	}
}
