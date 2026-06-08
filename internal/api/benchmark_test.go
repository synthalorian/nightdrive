package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nightdrive/internal/config"
	"nightdrive/internal/db"
	"nightdrive/internal/scanner"
)

func setupBenchmarkServer(b *testing.B) (*Server, *db.DB) {
	tmpDir := b.TempDir()
	database, _ := db.Open(filepath.Join(tmpDir, "bench.db"))
	database.Migrate()

	cfg := config.Default()
	scan := scanner.New(cfg, database)
	srv := NewServer(cfg, database, scan, nil)

	// Insert test data
	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	for i := 0; i < 100; i++ {
		track := &db.Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    fmt.Sprintf("Track %d", i),
			Path:     fmt.Sprintf("/music/track%d.mp3", i),
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		database.InsertTrack(track)
	}

	return srv, database
}

func BenchmarkHealthHandler(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		srv.healthHandler(rr, req)
	}
}

func BenchmarkTracksHandler(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/tracks", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		srv.tracksHandler(rr, req)
	}
}

func BenchmarkSearchHandler(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=Track", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		srv.searchHandler(rr, req)
	}
}

func BenchmarkAlbumsHandler(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/albums", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		srv.albumsHandler(rr, req)
	}
}

func BenchmarkArtistsHandler(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		srv.artistsHandler(rr, req)
	}
}

func BenchmarkSubsonicPing(b *testing.B) {
	srv, database := setupBenchmarkServer(b)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/rest/ping", nil)
	handler := srv.subsonicHandler(srv.subPing)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler(rr, req)
	}
}
