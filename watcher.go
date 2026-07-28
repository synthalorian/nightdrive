package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultWatchDebounce is the quiet period before a batch of filesystem
// events triggers a library scan.
const DefaultWatchDebounce = 3 * time.Second

// Watcher watches the music library recursively and triggers debounced
// library scans when files change.
type Watcher struct {
	scanner  *Scanner
	root     string
	debounce time.Duration

	fw   *fsnotify.Watcher
	done chan struct{}

	scanMu    sync.Mutex // serializes scans
	scanCount atomic.Int64
}

// NewWatcher creates a watcher for the given music root. The scanner is
// reused as-is; the watcher only decides when to scan.
func NewWatcher(scanner *Scanner, root string, debounce time.Duration) (*Watcher, error) {
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Expand home directory, mirroring Scanner.Scan
	if strings.HasPrefix(root, "~/") {
		if home, herr := os.UserHomeDir(); herr == nil {
			root = filepath.Join(home, root[2:])
		}
	}

	return &Watcher{
		scanner:  scanner,
		root:     root,
		debounce: debounce,
		fw:       fw,
		done:     make(chan struct{}),
	}, nil
}

// Start adds recursive watches and begins the event loop in a goroutine.
func (w *Watcher) Start() error {
	if err := w.addRecursive(w.root); err != nil {
		w.fw.Close()
		return err
	}
	go w.run()
	log.Printf("Watching music library for changes: %s (debounce %s)", w.root, w.debounce)
	return nil
}

// Close stops the event loop, waits for any in-flight scan to finish,
// and releases the fsnotify watcher.
func (w *Watcher) Close() error {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	// Block until a running scan (if any) completes.
	w.scanMu.Lock()
	w.scanMu.Unlock()
	return w.fw.Close()
}

// ScanCount returns how many scans the watcher has triggered.
func (w *Watcher) ScanCount() int64 {
	return w.scanCount.Load()
}

// addRecursive adds watches for root and all its subdirectories.
func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return w.fw.Add(path)
		}
		return nil
	})
}

// run is the event loop. Filesystem events reset a debounce timer; when the
// library has been quiet for the debounce period, a scan is triggered. Bursts
// of events therefore coalesce into a single scan.
func (w *Watcher) run() {
	var timer *time.Timer
	var timerC <-chan time.Time

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(w.debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)
		}
		timerC = timer.C
	}

	for {
		select {
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			// Watch newly created directories so nested drops are caught.
			// Walk recursively: children created before the watch lands would
			// otherwise be missed (inotify does not replay events).
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if err := w.addRecursive(ev.Name); err != nil {
						log.Printf("watch: failed to watch new dir %s: %v", ev.Name, err)
					}
				}
			}
			if w.relevant(ev) {
				resetTimer()
			}
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Printf("watch: error: %v", err)
		case <-timerC:
			timerC = nil
			w.triggerScan()
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// relevant reports whether an event should schedule a scan: audio file
// changes, or directory create/remove/rename events.
func (w *Watcher) relevant(ev fsnotify.Event) bool {
	ext := strings.ToLower(filepath.Ext(ev.Name))
	if audioExtensions[ext] {
		return true
	}
	// Non-audio path: only care if it might be a directory (create/remove/rename).
	if ext == "" && ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	return false
}

// triggerScan runs a library scan, serialized so bursts never overlap.
func (w *Watcher) triggerScan() {
	w.scanMu.Lock()
	defer w.scanMu.Unlock()

	log.Printf("watch: change detected, scanning library")
	if _, err := w.scanner.Scan(); err != nil {
		log.Printf("watch: scan failed: %v", err)
		return
	}
	w.scanCount.Add(1)
}
