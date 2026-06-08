package db

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkInsertArtist(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.InsertArtist(fmt.Sprintf("Artist %d", i))
	}
}

func BenchmarkInsertTrack(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		db.InsertTrack(track)
	}
}

func BenchmarkTracks(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	for i := 0; i < 1000; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		db.InsertTrack(track)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.Tracks(0, 50)
	}
}

func BenchmarkSearchTracks(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	artistID, _ := db.InsertArtist("Search Artist")
	albumID, _ := db.InsertAlbum(artistID, "Search Album", 2024, "")
	for i := 0; i < 1000; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Search Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		db.InsertTrack(track)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.SearchTracks("Search", 0, 50)
	}
}

func BenchmarkAlbumTracks(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	for i := 0; i < 20; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		db.InsertTrack(track)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.AlbumTracks(albumID)
	}
}

func BenchmarkSmartTracks(b *testing.B) {
	tmpDir := b.TempDir()
	db, _ := Open(filepath.Join(tmpDir, "bench.db"))
	defer db.Close()
	db.Migrate()

	artistID, _ := db.InsertArtist("Smart Artist")
	albumID, _ := db.InsertAlbum(artistID, "Smart Album", 2024, "")
	for i := 0; i < 500; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Smart Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
			Duration: 180,
		}
		db.InsertTrack(track)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.SmartTracks("artist:Smart year:2024")
	}
}
