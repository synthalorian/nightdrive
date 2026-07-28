package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// stubLyrics disables network lyrics fetching for hermetic tests.
func stubLyrics(t *testing.T) {
	t.Helper()
	orig := lyricsFetcher
	lyricsFetcher = func(artist, title, album string) (*lrclibResponse, error) {
		return nil, errors.New("lyrics fetch disabled in tests")
	}
	t.Cleanup(func() { lyricsFetcher = orig })
}

// writeFakeAudio creates a file with garbage bytes. Tag parsing fails
// cleanly and the scanner falls back to path-based metadata.
func writeFakeAudio(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := []byte("not really audio, but the scanner only needs a path")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fake audio: %v", err)
	}
}

func TestScannerScan(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	root := t.TempDir()
	writeFakeAudio(t, filepath.Join(root, "Test Artist", "Test Album", "01 - First Song.mp3"))
	writeFakeAudio(t, filepath.Join(root, "Test Artist", "Test Album", "02 - Second Song.flac"))
	writeFakeAudio(t, filepath.Join(root, "Other Artist", "Other Album", "01 - Only Song.mp3"))
	// Non-audio files must be ignored.
	if err := os.WriteFile(filepath.Join(root, "Test Artist", "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewScanner(d, root)
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.TracksAdded != 3 {
		t.Fatalf("expected 3 tracks added, got %d (errors: %v)", result.TracksAdded, result.Errors)
	}

	artists, err := d.GetAllArtists()
	if err != nil {
		t.Fatalf("GetAllArtists: %v", err)
	}
	if len(artists) != 2 {
		t.Fatalf("expected 2 artists, got %d: %+v", len(artists), artists)
	}

	var testArtist *Artist
	for _, a := range artists {
		if a.Name == "Test Artist" {
			testArtist = a
		}
	}
	if testArtist == nil {
		t.Fatalf("Test Artist not found in %+v", artists)
	}

	albums, err := d.GetAlbumsByArtist(testArtist.ID)
	if err != nil {
		t.Fatalf("GetAlbumsByArtist: %v", err)
	}
	if len(albums) != 1 || albums[0].Title != "Test Album" {
		t.Fatalf("albums mismatch: %+v", albums)
	}

	tracks, err := d.GetTracksByAlbum(albums[0].ID)
	if err != nil {
		t.Fatalf("GetTracksByAlbum: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks on Test Album, got %d", len(tracks))
	}
	// Track numbers should come from the "NN - Title" filename pattern.
	if tracks[0].TrackNum != 1 || tracks[0].Title != "First Song" {
		t.Fatalf("unexpected first track: %+v", tracks[0])
	}

	// Rescanning must not duplicate rows (upsert on path).
	// Note: TracksAdded/TracksUpdated is a time-based heuristic in
	// processTrack, so an immediate rescan may still count rows as "added";
	// what matters is that the library contents stay stable.
	result2, err := s.Scan()
	if err != nil {
		t.Fatalf("second Scan: %v", err)
	}
	if result2.TracksAdded+result2.TracksUpdated != 3 {
		t.Fatalf("expected 3 tracks processed on rescan, got %d added + %d updated",
			result2.TracksAdded, result2.TracksUpdated)
	}
	all, err := d.GetAllTracks(100, 0)
	if err != nil {
		t.Fatalf("GetAllTracks: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tracks after rescan, got %d", len(all))
	}
}

func TestScanMissingDir(t *testing.T) {
	stubLyrics(t)
	d := newTestDB(t)

	s := NewScanner(d, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := s.Scan(); err == nil {
		t.Fatal("expected error scanning missing directory, got nil")
	}
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		relPath  string
		artist   string
		album    string
		track    string
		trackNum int
	}{
		{filepath.Join("Artist", "Album", "01 - Song.mp3"), "Artist", "Album", "Song", 1},
		{filepath.Join("Artist", "Album", "Song.mp3"), "Artist", "Album", "Song", 0},
		{filepath.Join("Artist", "03. Numbered.mp3"), "Artist", "Unknown Album", "Numbered", 3},
		{"bare.mp3", "Unknown Artist", "Unknown Album", "bare", 0},
	}
	for _, c := range cases {
		artist, album, track, num := parsePath(c.relPath)
		if artist != c.artist || album != c.album || track != c.track || num != c.trackNum {
			t.Errorf("parsePath(%q) = (%q, %q, %q, %d), want (%q, %q, %q, %d)",
				c.relPath, artist, album, track, num, c.artist, c.album, c.track, c.trackNum)
		}
	}
}
