package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"nightdrive/internal/config"
	"nightdrive/internal/db"
)

func TestIsMusicFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"song.mp3", true},
		{"song.flac", true},
		{"song.ogg", true},
		{"song.m4a", true},
		{"song.mp4", true},
		{"song.opus", true},
		{"song.wav", true},
		{"song.wma", true},
		{"song.aac", true},
		{"song.txt", false},
		{"song.jpg", false},
		{"song", false},
		{"SONG.MP3", true},
	}

	for _, tt := range tests {
		result := isMusicFile(tt.path)
		if result != tt.expected {
			t.Errorf("isMusicFile(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantArtist string
		wantTitle  string
		wantAlbum  string
	}{
		{
			name:       "simple",
			path:       "/music/Artist - Title.mp3",
			wantArtist: "Artist",
			wantTitle:  "Title",
			wantAlbum:  "",
		},
		{
			name:       "with album",
			path:       "/music/Artist - Album - Title.mp3",
			wantArtist: "Artist",
			wantTitle:  "Title",
			wantAlbum:  "Album",
		},
		{
			name:       "no artist",
			path:       "/music/JustATitle.mp3",
			wantArtist: "Unknown Artist",
			wantTitle:  "JustATitle",
			wantAlbum:  "",
		},
		{
			name:       "extra spaces",
			path:       "/music/Artist  -  Title.mp3",
			wantArtist: "Artist",
			wantTitle:  "Title",
			wantAlbum:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &mockFileInfo{size: 1024, modTime: time.Now()}
			track := parseMetadata(tt.path, info)
			if track.ArtistName != tt.wantArtist {
				t.Errorf("artist = %q, want %q", track.ArtistName, tt.wantArtist)
			}
			if track.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", track.Title, tt.wantTitle)
			}
			if track.AlbumTitle != tt.wantAlbum {
				t.Errorf("album = %q, want %q", track.AlbumTitle, tt.wantAlbum)
			}
			if track.Path != tt.path {
				t.Errorf("path = %q, want %q", track.Path, tt.path)
			}
			if track.Format != "mp3" {
				t.Errorf("format = %q, want mp3", track.Format)
			}
		})
	}
}

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/music/song.mp3", "music"},
		{"/music/podcast/episode1.mp3", "podcast"},
		{"/audiobooks/book1.mp3", "audiobook"},
		{"/audio book/chapter1.mp3", "audiobook"},
		{"/books/audio/chapter1.mp3", "audiobook"},
	}

	for _, tt := range tests {
		result := detectMediaType(tt.path)
		if result != tt.expected {
			t.Errorf("detectMediaType(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestFindLyrics(t *testing.T) {
	tmpDir := t.TempDir()
	musicPath := filepath.Join(tmpDir, "song.mp3")

	// Test: no lyrics file
	result := findLyrics(musicPath)
	if result != "" {
		t.Errorf("expected empty lyrics, got %q", result)
	}

	// Test: .lrc file
	lrcPath := filepath.Join(tmpDir, "song.lrc")
	if err := os.WriteFile(lrcPath, []byte("[00:00] Hello"), 0644); err != nil {
		t.Fatalf("write lrc: %v", err)
	}
	result = findLyrics(musicPath)
	if result != "[00:00] Hello" {
		t.Errorf("expected lrc content, got %q", result)
	}

	// Test: .txt file (after removing lrc)
	os.Remove(lrcPath)
	txtPath := filepath.Join(tmpDir, "song.txt")
	if err := os.WriteFile(txtPath, []byte("Lyrics here"), 0644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	result = findLyrics(musicPath)
	if result != "Lyrics here" {
		t.Errorf("expected txt content, got %q", result)
	}
}

func TestFindLyrics_DirectorySearch(t *testing.T) {
	tmpDir := t.TempDir()
	musicPath := filepath.Join(tmpDir, "My Song.mp3")

	// Create lyrics with matching prefix
	lyricsPath := filepath.Join(tmpDir, "My Song.lrc")
	if err := os.WriteFile(lyricsPath, []byte("Found by prefix"), 0644); err != nil {
		t.Fatalf("write lyrics: %v", err)
	}

	result := findLyrics(musicPath)
	if result != "Found by prefix" {
		t.Errorf("expected prefix match content, got %q", result)
	}
}

func TestScannerAddRoot(t *testing.T) {
	tmpDir := t.TempDir()
	musicDir := filepath.Join(tmpDir, "music")
	if err := os.MkdirAll(musicDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	database := setupTestDB(t)
	defer database.Close()

	cfg := config.Default()
	scan := New(cfg, database)

	// Add valid directory
	if err := scan.AddRoot(musicDir); err != nil {
		t.Fatalf("add root: %v", err)
	}

	// Add file (should fail)
	tmpFile := filepath.Join(tmpDir, "notadir.txt")
	os.WriteFile(tmpFile, []byte(""), 0644)
	if err := scan.AddRoot(tmpFile); err == nil {
		t.Error("expected error for non-directory")
	}

	// Add nonexistent (should fail)
	if err := scan.AddRoot("/nonexistent/path"); err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestScannerScanNow(t *testing.T) {
	tmpDir := t.TempDir()
	musicDir := filepath.Join(tmpDir, "music")
	artistDir := filepath.Join(musicDir, "Artist")
	if err := os.MkdirAll(artistDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a fake music file
	musicFile := filepath.Join(artistDir, "Artist - Song.mp3")
	if err := os.WriteFile(musicFile, []byte("fake mp3"), 0644); err != nil {
		t.Fatalf("write music file: %v", err)
	}

	database := setupTestDB(t)
	defer database.Close()

	cfg := config.Default()
	scan := New(cfg, database)
	if err := scan.AddRoot(musicDir); err != nil {
		t.Fatalf("add root: %v", err)
	}

	scan.ScanNow()

	// Check that track was inserted
	tracks, err := database.Tracks(0, 10)
	if err != nil {
		t.Fatalf("get tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track after scan, got %d", len(tracks))
	}
	if len(tracks) > 0 {
		if tracks[0].Title != "Song" {
			t.Errorf("expected title 'Song', got %q", tracks[0].Title)
		}
		if tracks[0].ArtistName != "Artist" {
			t.Errorf("expected artist 'Artist', got %q", tracks[0].ArtistName)
		}
	}
}

func TestScannerStop(t *testing.T) {
	tmpDir := t.TempDir()
	musicDir := filepath.Join(tmpDir, "music")
	os.MkdirAll(musicDir, 0755)

	database := setupTestDB(t)
	defer database.Close()

	cfg := config.Default()
	cfg.ScanOnStart = false
	scan := New(cfg, database)
	scan.AddRoot(musicDir)

	// Stop should not panic
	scan.Stop()
}

// Mock types

type mockFileInfo struct {
	size    int64
	modTime time.Time
}

func (m *mockFileInfo) Name() string       { return "mock" }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return database
}
