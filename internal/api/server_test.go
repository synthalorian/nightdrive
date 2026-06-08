package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nightdrive/internal/config"
	"nightdrive/internal/db"
	"nightdrive/internal/scanner"
)

func setupTestServer(t *testing.T) (*Server, *db.DB, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	cfg := config.Default()
	scan := scanner.New(cfg, database)
	srv := NewServer(cfg, database, scan)

	cleanup := func() {
		database.Close()
	}
	return srv, database, cleanup
}

func TestHealthHandler(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	srv.healthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestTracksHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	// Insert test data
	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Test Track",
		Path:     "/music/test.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	database.InsertTrack(track)

	req := httptest.NewRequest(http.MethodGet, "/api/tracks", nil)
	rr := httptest.NewRecorder()
	srv.tracksHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var tracks []db.Track
	if err := json.Unmarshal(rr.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("unmarshal tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(tracks))
	}
}

func TestTrackHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Test Track",
		Path:     "/music/test.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+string(rune('0'+int(trackID))), nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	srv.trackHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestTrackHandler_NotFound(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/999", nil)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()
	srv.trackHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestAlbumsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	database.InsertAlbum(artistID, "Album", 2024, "")

	req := httptest.NewRequest(http.MethodGet, "/api/albums", nil)
	rr := httptest.NewRecorder()
	srv.albumsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var albums []db.Album
	if err := json.Unmarshal(rr.Body.Bytes(), &albums); err != nil {
		t.Fatalf("unmarshal albums: %v", err)
	}
	if len(albums) != 1 {
		t.Errorf("expected 1 album, got %d", len(albums))
	}
}

func TestArtistsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	database.InsertArtist("Artist")

	req := httptest.NewRequest(http.MethodGet, "/api/artists", nil)
	rr := httptest.NewRecorder()
	srv.artistsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var artists []db.Artist
	if err := json.Unmarshal(rr.Body.Bytes(), &artists); err != nil {
		t.Fatalf("unmarshal artists: %v", err)
	}
	if len(artists) != 1 {
		t.Errorf("expected 1 artist, got %d", len(artists))
	}
}

func TestSearchHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Search Me",
		Path:     "/music/search.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	database.InsertTrack(track)

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=Search", nil)
	rr := httptest.NewRecorder()
	srv.searchHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var tracks []db.Track
	if err := json.Unmarshal(rr.Body.Bytes(), &tracks); err != nil {
		t.Fatalf("unmarshal search results: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 result, got %d", len(tracks))
	}
}

func TestPlaylistsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	// Create public playlist so it's visible to anonymous users
	database.InsertPlaylistWithOwner("Test Playlist", "", 0, "public")

	req := httptest.NewRequest(http.MethodGet, "/api/playlists", nil)
	rr := httptest.NewRecorder()
	srv.playlistsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var playlists []db.Playlist
	if err := json.Unmarshal(rr.Body.Bytes(), &playlists); err != nil {
		t.Fatalf("unmarshal playlists: %v", err)
	}
	if len(playlists) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(playlists))
	}
}

func TestCreatePlaylistHandler(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/playlists?name=New%20Playlist", nil)
	rr := httptest.NewRecorder()
	srv.createPlaylistHandler(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
}

func TestCreatePlaylistHandler_JSON(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	body := `{"name":"JSON Playlist","smartQuery":"artist:test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/playlists", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.createPlaylistHandler(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
}

func TestCreatePlaylistHandler_NoName(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/playlists", nil)
	rr := httptest.NewRecorder()
	srv.createPlaylistHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestScanHandler(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Create admin user for auth
	req := httptest.NewRequest(http.MethodPost, "/api/scan", nil)
	req.SetBasicAuth("admin", "admin")
	rr := httptest.NewRecorder()
	srv.scanHandler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
}

func TestLoginHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	// Create user
	hash, _ := hashPassword("password123")
	database.InsertUser("testuser", hash, "user")

	body := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.loginHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if resp["token"] == nil {
		t.Error("expected token in response")
	}
}

func TestLoginHandler_InvalidCredentials(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	body := `{"username":"nobody","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.loginHandler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestMeHandler_Unauthenticated(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	rr := httptest.NewRecorder()
	srv.meHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal me response: %v", err)
	}
	if resp["authenticated"] != false {
		t.Error("expected authenticated=false")
	}
}

func TestGenresHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Genre:    "Rock",
	}
	database.InsertTrack(track)

	req := httptest.NewRequest(http.MethodGet, "/api/genres", nil)
	rr := httptest.NewRecorder()
	srv.genresHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var genres []string
	if err := json.Unmarshal(rr.Body.Bytes(), &genres); err != nil {
		t.Fatalf("unmarshal genres: %v", err)
	}
	if len(genres) != 1 || genres[0] != "Rock" {
		t.Errorf("expected genres [Rock], got %v", genres)
	}
}

func TestRadioHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Genre:    "Rock",
	}
	database.InsertTrack(track)

	// Test random radio
	req := httptest.NewRequest(http.MethodGet, "/api/radio", nil)
	rr := httptest.NewRecorder()
	srv.radioHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestSubsonicPing(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/rest/ping", nil)
	rr := httptest.NewRecorder()

	// The subsonicHandler wraps the handler
	handler := srv.subsonicHandler(srv.subPing)
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal subsonic response: %v", err)
	}
	inner, ok := resp["subsonic-response"].(map[string]interface{})
	if !ok {
		t.Fatal("expected subsonic-response object")
	}
	if inner["status"] != "ok" {
		t.Errorf("expected status ok, got %v", inner["status"])
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestServeFile(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.mp3")
	if err := os.WriteFile(testFile, []byte("fake audio data"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rr := httptest.NewRecorder()
	srv.serveFile(rr, req, testFile, "mp3")

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("expected Content-Type audio/mpeg, got %s", ct)
	}
}

func TestServeFile_FLAC(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.flac")
	if err := os.WriteFile(testFile, []byte("fake flac data"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rr := httptest.NewRecorder()
	srv.serveFile(rr, req, testFile, "flac")

	if ct := rr.Header().Get("Content-Type"); ct != "audio/flac" {
		t.Errorf("expected Content-Type audio/flac, got %s", ct)
	}
}

func TestServeFile_NotFound(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rr := httptest.NewRecorder()
	srv.serveFile(rr, req, "/nonexistent/file.mp3", "mp3")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestCoverHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	tmpDir := t.TempDir()
	musicDir := filepath.Join(tmpDir, "music")
	os.MkdirAll(musicDir, 0755)

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     filepath.Join(musicDir, "track.mp3"),
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)

	// Create cover file
	coverFile := filepath.Join(musicDir, "cover.jpg")
	os.WriteFile(coverFile, []byte("fake image"), 0644)

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/"+string(rune('0'+int(trackID)))+"/cover", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	srv.coverHandler(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Errorf("unexpected status %d", rr.Code)
	}
}

func TestLyricsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)
	database.UpdateTrackLyrics(trackID, "These are the lyrics")

	req := httptest.NewRequest(http.MethodGet, "/api/tracks/1/lyrics", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	srv.lyricsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal lyrics: %v", err)
	}
	if resp["lyrics"] != "These are the lyrics" {
		t.Errorf("expected lyrics, got %q", resp["lyrics"])
	}
}

func TestPositionHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)

	// Create user for auth
	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "testtoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	body := `{"position":45.5}`
	req := httptest.NewRequest(http.MethodPost, "/api/tracks/1/position", strings.NewReader(body))
	req.SetPathValue("id", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.positionHandler)(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	// Verify position was updated
	tr, _ := database.TrackByID(trackID)
	if tr.PlaybackPosition != 45.5 {
		t.Errorf("expected position 45.5, got %f", tr.PlaybackPosition)
	}
}

func TestRecordPlayHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	database.InsertTrack(track)

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "testtoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	body := `{"trackId":1,"durationListened":120,"completionPct":0.75}`
	req := httptest.NewRequest(http.MethodPost, "/api/plays", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.recordPlayHandler)(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
}

func TestListUsersHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	// Create admin user
	hash, _ := hashPassword("admin")
	adminID, _ := database.InsertUser("admin", hash, "admin")
	token := "admintoken"
	database.CreateSession(adminID, token, time.Now().Add(24*time.Hour))

	// Create regular user
	database.InsertUser("user1", hash, "user")

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAdmin(srv.listUsersHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var users []db.User
	if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal users: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestDeleteUserHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("admin")
	adminID, _ := database.InsertUser("admin", hash, "admin")
	token := "admintoken"
	database.CreateSession(adminID, token, time.Now().Add(24*time.Hour))

	userID, _ := database.InsertUser("todelete", hash, "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/users/2", nil)
	req.SetPathValue("id", "2")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAdmin(srv.deleteUserHandler)(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	_, err := database.UserByID(userID)
	if err == nil {
		t.Error("expected user to be deleted")
	}
}

func TestRequireAuth(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	handler := srv.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// No auth
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected %d without auth, got %d", http.StatusUnauthorized, rr.Code)
	}

	// With valid token
	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "validtoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Auth-Token", token)
	rr = httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected %d with valid token, got %d", http.StatusOK, rr.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	handler := srv.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Regular user
	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("regular", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected %d for regular user, got %d", http.StatusForbidden, rr.Code)
	}

	// Admin user
	adminID, _ := database.InsertUser("admin2", hash, "admin")
	adminToken := "admintoken"
	database.CreateSession(adminID, adminToken, time.Now().Add(24*time.Hour))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Auth-Token", adminToken)
	rr = httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected %d for admin, got %d", http.StatusOK, rr.Code)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestGenerateToken(t *testing.T) {
	token1 := generateToken()
	token2 := generateToken()
	if token1 == "" {
		t.Error("expected non-empty token")
	}
	if token1 == token2 {
		t.Error("expected unique tokens")
	}
}

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if hash == "password123" {
		t.Error("expected hash to differ from password")
	}

	// Verify password check
	if !checkPassword("password123", hash) {
		t.Error("expected password to match")
	}
	if checkPassword("wrongpassword", hash) {
		t.Error("expected wrong password to fail")
	}
}

func TestAlbumTracksHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	database.InsertTrack(track)

	req := httptest.NewRequest(http.MethodGet, "/api/albums/1/tracks", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	srv.albumTracksHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestCreateUserHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	// Create admin
	hash, _ := hashPassword("admin")
	adminID, _ := database.InsertUser("admin", hash, "admin")
	token := "admintoken"
	database.CreateSession(adminID, token, time.Now().Add(24*time.Hour))

	body := `{"username":"newuser","password":"newpass","role":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAdmin(srv.createUserHandler)(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
}

func TestCreateSessionHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	body := `{"deviceName":"Test Device"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.createSessionHandler)(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
}

func TestSaveLyricsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	body := `{"lyrics":"New lyrics"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tracks/1/lyrics", strings.NewReader(body))
	req.SetPathValue("id", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.saveLyricsHandler)(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	lyrics, _ := database.TrackLyrics(trackID)
	if lyrics != "New lyrics" {
		t.Errorf("expected lyrics 'New lyrics', got %q", lyrics)
	}
}

func TestDSPPresetsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/dsp/presets", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.dspPresetsHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestDSPProcessHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	body := `{"samples":[0.1,0.2,0.3,0.4],"sampleRate":44100,"presetId":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/dsp/process", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.dspProcessHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestCastDevicesHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/cast/devices", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.authMiddleware(srv.castDevicesHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestListSyncRoomsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/sync/rooms", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.authMiddleware(srv.listSyncRoomsHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestStatsHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := database.InsertTrack(track)
	database.InsertPlay(userID, trackID, 100, 0.5)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.statsHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestHistoryHandler(t *testing.T) {
	srv, database, cleanup := setupTestServer(t)
	defer cleanup()

	hash, _ := hashPassword("password")
	userID, _ := database.InsertUser("testuser", hash, "user")
	token := "usertoken"
	database.CreateSession(userID, token, time.Now().Add(24*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	req.Header.Set("X-Auth-Token", token)
	rr := httptest.NewRecorder()
	srv.requireAuth(srv.historyHandler)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestSubSong(t *testing.T) {
	track := db.Track{
		ID:         1,
		Title:      "Test Song",
		ArtistName: "Test Artist",
		AlbumTitle: "Test Album",
		Duration:   180,
		Bitrate:    320,
		Format:     "mp3",
		Path:       "/music/test.mp3",
	}

	song := subSong(track)
	if song["title"] != "Test Song" {
		t.Errorf("expected title 'Test Song', got %v", song["title"])
	}
	if song["artist"] != "Test Artist" {
		t.Errorf("expected artist 'Test Artist', got %v", song["artist"])
	}
	if song["album"] != "Test Album" {
		t.Errorf("expected album 'Test Album', got %v", song["album"])
	}
	if song["duration"] != 180 {
		t.Errorf("expected duration 180, got %v", song["duration"])
	}
	if song["bitRate"] != 320 {
		t.Errorf("expected bitRate 320, got %v", song["bitRate"])
	}
	if song["suffix"] != "mp3" {
		t.Errorf("expected suffix 'mp3', got %v", song["suffix"])
	}
}

func TestRegister(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	srv.Register(mux)

	// Test that routes are registered by making requests
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/tracks"},
		{http.MethodGet, "/api/albums"},
		{http.MethodGet, "/api/artists"},
		{http.MethodGet, "/api/search"},
		{http.MethodGet, "/api/playlists"},
		{http.MethodGet, "/rest/ping"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		// Should not be 404 (handler exists)
		if rr.Code == http.StatusNotFound {
			t.Errorf("route %s %s not registered", tt.method, tt.path)
		}
	}
}
