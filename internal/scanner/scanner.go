package scanner

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"nightdrive/internal/config"
	"nightdrive/internal/db"
)

// Scanner walks music directories, watches for changes, and updates metadata.
type Scanner struct {
	cfg    *config.Config
	db     *db.DB
	roots  []string
	mu     sync.RWMutex
	watcher *fsnotify.Watcher
	stop   chan struct{}
}

// New creates a Scanner.
func New(cfg *config.Config, database *db.DB) *Scanner {
	return &Scanner{
		cfg:  cfg,
		db:   database,
		stop: make(chan struct{}),
	}
}

// AddRoot adds a music directory root.
func (s *Scanner) AddRoot(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roots = append(s.roots, path)
	return nil
}

// Run starts the watcher and optional initial scan.
func (s *Scanner) Run() {
	if s.cfg.ScanOnStart {
		s.ScanNow()
	}
	if err := s.startWatcher(); err != nil {
		log.Printf("[scanner] watcher error: %v", err)
		return
	}
	defer s.watcher.Close()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if isMusicFile(event.Name) {
					s.handleFile(event.Name)
				} else if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					_ = s.watcher.Add(event.Name)
				}
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[scanner] watcher error: %v", err)
		case <-ticker.C:
			s.ScanNow()
		}
	}
}

// ScanNow performs a full scan of all roots.
func (s *Scanner) ScanNow() {
	s.mu.RLock()
	roots := append([]string{}, s.roots...)
	s.mu.RUnlock()
	for _, r := range roots {
		log.Printf("[scanner] scanning %s", r)
		err := filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if s.watcher != nil {
					_ = s.watcher.Add(path)
				}
				return nil
			}
			if isMusicFile(path) {
				s.handleFile(path)
			}
			return nil
		})
		if err != nil {
			log.Printf("[scanner] walk error: %v", err)
		}
		log.Printf("[scanner] finished %s", r)
	}
}

func (s *Scanner) startWatcher() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.watcher = w
	s.mu.RLock()
	roots := append([]string{}, s.roots...)
	s.mu.RUnlock()
	for _, r := range roots {
		_ = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			return s.watcher.Add(path)
		})
	}
	return nil
}

func (s *Scanner) handleFile(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// Minimal metadata extraction: filename parsing.
	track := parseMetadata(path, info)
	artistID, err := s.db.InsertArtist(track.ArtistName)
	if err != nil {
		log.Printf("[scanner] artist insert %s: %v", path, err)
		return
	}
	var albumID int64
	if track.AlbumTitle != "" {
		albumID, err = s.db.InsertAlbum(artistID, track.AlbumTitle, 0, "")
		if err != nil {
			log.Printf("[scanner] album insert %s: %v", path, err)
			return
		}
	}
	track.ArtistID = &artistID
	if albumID != 0 {
		track.AlbumID = &albumID
	}
	if _, err := s.db.InsertTrack(track); err != nil {
		log.Printf("[scanner] track insert %s: %v", path, err)
	}
}

func isMusicFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3", ".flac", ".ogg", ".m4a", ".mp4", ".opus", ".wav", ".wma", ".aac":
		return true
	}
	return false
}

func parseMetadata(path string, info os.FileInfo) *db.Track {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	artist := "Unknown Artist"
	title := base
	album := ""
	// naive "Artist - Title" or "Artist - Album - Title" parsing
	parts := strings.Split(base, " - ")
	if len(parts) >= 2 {
		artist = strings.TrimSpace(parts[0])
		title = strings.TrimSpace(parts[len(parts)-1])
		if len(parts) >= 3 {
			album = strings.TrimSpace(parts[1])
		}
	}
	return &db.Track{
		Title:      title,
		ArtistName: artist,
		AlbumTitle: album,
		Path:       path,
		Mtime:      info.ModTime().Unix(),
		Size:       info.Size(),
		Format:     strings.TrimPrefix(filepath.Ext(path), "."),
	}
}

// Stop halts the watcher.
func (s *Scanner) Stop() {
	close(s.stop)
}
