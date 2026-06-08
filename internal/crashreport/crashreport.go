package crashreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nightdrive/internal/logging"
	"nightdrive/internal/recovery"
)

// Report summarizes a crash event.
type Report struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Summary     string    `json:"summary"`
	Path        string    `json:"path"`
	Method      string    `json:"method"`
	StackHash   string    `json:"stack_hash"`
	Occurrences int       `json:"occurrences"`
}

// Reporter collects and serves crash reports.
type Reporter struct {
	mu       sync.RWMutex
	reports  []Report
	byHash   map[string]*Report
	maxKeep  int
}

// NewReporter creates a crash reporter.
func NewReporter() *Reporter {
	return &Reporter{
		byHash:  make(map[string]*Report),
		maxKeep: 100,
	}
}

// ObserveRecovery should be called by the recovery manager whenever a panic is recovered.
func (r *Reporter) ObserveRecovery(state recovery.State) {
	hash := hashStack(state.StackTrace)

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.byHash[hash]; ok {
		existing.Occurrences++
		existing.Timestamp = state.Timestamp
		return
	}

	report := Report{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp:   state.Timestamp,
		Summary:     truncate(state.PanicValue, 200),
		Path:        state.Path,
		Method:      state.Method,
		StackHash:   hash,
		Occurrences: 1,
	}
	r.reports = append(r.reports, report)
	r.byHash[hash] = &r.reports[len(r.reports)-1]

	if len(r.reports) > r.maxKeep {
		// Remove oldest.
		oldest := r.reports[0]
		delete(r.byHash, oldest.StackHash)
		r.reports = r.reports[1:]
	}
}

// Reports returns all crash reports sorted by newest first.
func (r *Reporter) Reports(limit int) []Report {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Report, len(r.reports))
	copy(out, r.reports)
	r.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}

// Export writes all reports to a JSON file.
func (r *Reporter) Export(path string) error {
	reports := r.Reports(0)
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Import reads reports from a JSON file.
func (r *Reporter) Import(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var reports []Report
	if err := json.Unmarshal(data, &reports); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rep := range reports {
		if existing, ok := r.byHash[rep.StackHash]; ok {
			existing.Occurrences += rep.Occurrences
			if rep.Timestamp.After(existing.Timestamp) {
				existing.Timestamp = rep.Timestamp
			}
		} else {
			r.reports = append(r.reports, rep)
			r.byHash[rep.StackHash] = &r.reports[len(r.reports)-1]
		}
	}
	return nil
}

// ScanCrashDumps reads any crash dump JSON files from the given directory and imports them.
func (r *Reporter) ScanCrashDumps(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			logging.Warn("failed to read crash dump", map[string]interface{}{"path": path, "error": err.Error()})
			continue
		}
		var state recovery.State
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		r.ObserveRecovery(state)
	}
	return nil
}

func hashStack(stack string) string {
	// Simple hash: first 5 non-empty lines of the stack.
	lines := strings.Split(stack, "\n")
	var relevant []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "goroutine") {
			relevant = append(relevant, trimmed)
			if len(relevant) >= 5 {
				break
			}
		}
	}
	h := fmt.Sprintf("%x", relevant)
	if len(h) > 16 {
		return h[:16]
	}
	return h
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
