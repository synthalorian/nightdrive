package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherTriggersScan(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	root := t.TempDir()
	writeFakeAudio(t, filepath.Join(root, "Artist", "Album", "01 - Existing.mp3"))

	s := NewScanner(d, root)
	w, err := NewWatcher(s, root, 150*time.Millisecond)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { w.Close() })

	// Drop a new audio file into an existing watched dir.
	writeFakeAudio(t, filepath.Join(root, "Artist", "Album", "02 - New Drop.mp3"))

	deadline := time.Now().Add(10 * time.Second)
	for w.ScanCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if w.ScanCount() == 0 {
		t.Fatal("watcher did not trigger a scan within 10s")
	}

	tracks, err := d.GetAllTracks(100, 0)
	if err != nil {
		t.Fatalf("GetAllTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks after watch scan, got %d", len(tracks))
	}
}

func TestWatcherNewDirectory(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	root := t.TempDir()
	s := NewScanner(d, root)
	w, err := NewWatcher(s, root, 150*time.Millisecond)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { w.Close() })

	// Create a brand-new nested directory, then drop a file into it. The
	// watcher must pick up the new dir dynamically and scan.
	newDir := filepath.Join(root, "Fresh Artist", "Fresh Album")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Give the watcher a moment to register the new dir watch.
	time.Sleep(300 * time.Millisecond)
	writeFakeAudio(t, filepath.Join(newDir, "01 - Fresh Song.mp3"))

	deadline := time.Now().Add(10 * time.Second)
	for {
		tracks, _ := d.GetAllTracks(100, 0)
		if len(tracks) >= 1 || time.Now().After(deadline) {
			if len(tracks) < 1 {
				t.Fatal("watcher did not scan file dropped into newly created directory")
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWatcherDebounceCoalesces(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	root := t.TempDir()
	s := NewScanner(d, root)
	w, err := NewWatcher(s, root, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { w.Close() })

	// Burst of events: many files in rapid succession should coalesce into
	// very few scans (ideally one).
	for i := 0; i < 5; i++ {
		writeFakeAudio(t, filepath.Join(root, "Burst", "Album",
			"0"+string(rune('1'+i))+" - Song.mp3"))
	}

	deadline := time.Now().Add(10 * time.Second)
	for w.ScanCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if w.ScanCount() == 0 {
		t.Fatal("no scan triggered after burst")
	}

	// Allow any trailing debounce to fire, then check coalescing.
	time.Sleep(600 * time.Millisecond)
	if got := w.ScanCount(); got > 2 {
		t.Fatalf("expected burst to coalesce into <=2 scans, got %d", got)
	}

	tracks, err := d.GetAllTracks(100, 0)
	if err != nil {
		t.Fatalf("GetAllTracks: %v", err)
	}
	if len(tracks) != 5 {
		t.Fatalf("expected 5 tracks after burst scan, got %d", len(tracks))
	}
}

func TestWatcherCloseIsClean(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	root := t.TempDir()
	s := NewScanner(d, root)
	w, err := NewWatcher(s, root, time.Second)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close must not panic or error.
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
