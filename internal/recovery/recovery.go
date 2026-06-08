package recovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"nightdrive/internal/logging"
)

// State represents a snapshot of runtime state saved during a panic.
type State struct {
	Timestamp   time.Time              `json:"timestamp"`
	Path        string                 `json:"path"`
	Method      string                 `json:"method"`
	RemoteAddr  string                 `json:"remote_addr"`
	PanicValue  string                 `json:"panic_value"`
	StackTrace  string                 `json:"stack_trace"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// Manager handles panic recovery and crash state persistence.
type Manager struct {
	dumpDir     string
	mu          sync.Mutex
	recent      []State
	maxRecent   int
	onRecovered func(State)
}

// NewManager creates a recovery manager.
func NewManager(dumpDir string) *Manager {
	_ = os.MkdirAll(dumpDir, 0o755)
	return &Manager{
		dumpDir:   dumpDir,
		maxRecent: 50,
	}
}

// SetOnRecovered sets a callback invoked after a panic is recovered.
func (m *Manager) SetOnRecovered(fn func(State)) {
	m.onRecovered = fn
}

// Middleware returns HTTP middleware that recovers panics, saves state, and returns 500.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				state := State{
					Timestamp:  time.Now().UTC(),
					Path:       r.URL.Path,
					Method:     r.Method,
					RemoteAddr: r.RemoteAddr,
					PanicValue: fmt.Sprintf("%v", rec),
					StackTrace: string(debug.Stack()),
					Extra: map[string]interface{}{
						"user_agent": r.UserAgent(),
						"host":       r.Host,
					},
				}
				m.saveState(state)
				logging.Error("panic recovered", map[string]interface{}{
					"path":  state.Path,
					"panic": state.PanicValue,
				})
				if m.onRecovered != nil {
					m.onRecovered(state)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":      "internal server error",
					"request_id": state.Timestamp.Format(time.RFC3339Nano),
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// saveState persists the crash state to disk and keeps an in-memory buffer.
func (m *Manager) saveState(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep in-memory recent list.
	m.recent = append(m.recent, s)
	if len(m.recent) > m.maxRecent {
		m.recent = m.recent[len(m.recent)-m.maxRecent:]
	}

	// Write to timestamped file.
	fname := fmt.Sprintf("crash_%s.json", s.Timestamp.Format("20060102_150405.000"))
	path := filepath.Join(m.dumpDir, fname)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		logging.Error("failed to marshal crash state", map[string]interface{}{"error": err.Error()})
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		logging.Error("failed to write crash state", map[string]interface{}{"error": err.Error()})
	}
}

// Recent returns the most recent recovered panic states (newest first).
func (m *Manager) Recent(limit int) []State {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > len(m.recent) {
		limit = len(m.recent)
	}
	// Return newest first.
	out := make([]State, limit)
	for i := 0; i < limit; i++ {
		out[i] = m.recent[len(m.recent)-1-i]
	}
	return out
}

// DumpDir returns the directory where crash dumps are stored.
func (m *Manager) DumpDir() string {
	return m.dumpDir
}
