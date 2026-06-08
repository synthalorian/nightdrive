package db

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestOpen(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "open.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Errorf("ping failed: %v", err)
	}
}

func TestMigrate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Verify tables exist by querying sqlite_master
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	expectedTables := map[string]bool{
		"artists":           false,
		"albums":            false,
		"tracks":            false,
		"playlists":         false,
		"playlist_tracks":   false,
		"users":             false,
		"sessions":          false,
		"playlist_shares":   false,
		"plays":             false,
		"playback_sessions": false,
		"playback_queue":    false,
		"sync_rooms":        false,
		"sync_room_members": false,
		"cast_devices":      false,
		"dsp_presets":       false,
		"gapless_reports":   false,
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		if _, ok := expectedTables[name]; ok {
			expectedTables[name] = true
		}
	}

	for name, found := range expectedTables {
		if !found {
			t.Errorf("table %s not found after migration", name)
		}
	}
}

func TestInsertArtist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertArtist("Test Artist")
	if err != nil {
		t.Fatalf("insert artist: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero artist id")
	}

	// Duplicate should return existing id
	id2, err := db.InsertArtist("Test Artist")
	if err != nil {
		t.Fatalf("insert duplicate artist: %v", err)
	}
	if id != id2 {
		t.Errorf("expected same id for duplicate, got %d and %d", id, id2)
	}
}

func TestInsertArtist_EmptyName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertArtist("")
	if err != nil {
		t.Fatalf("insert empty artist: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id for empty name")
	}
}

func TestInsertAlbum(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Test Artist")
	id, err := db.InsertAlbum(artistID, "Test Album", 2024, "cover.jpg")
	if err != nil {
		t.Fatalf("insert album: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero album id")
	}

	// Duplicate should return existing id
	id2, err := db.InsertAlbum(artistID, "Test Album", 2024, "cover.jpg")
	if err != nil {
		t.Fatalf("insert duplicate album: %v", err)
	}
	if id != id2 {
		t.Errorf("expected same id for duplicate, got %d and %d", id, id2)
	}
}

func TestInsertTrack(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Test Artist")
	albumID, _ := db.InsertAlbum(artistID, "Test Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Test Track",
		Path:     "/music/test.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Duration: 180,
		Bitrate:  320,
	}

	id, err := db.InsertTrack(track)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero track id")
	}

	// Update on conflict
	track.Title = "Updated Track"
	id2, err := db.InsertTrack(track)
	if err != nil {
		t.Fatalf("insert duplicate track: %v", err)
	}
	if id != id2 {
		t.Errorf("expected same id on conflict, got %d and %d", id, id2)
	}

	// Verify update
	retrieved, err := db.TrackByID(id)
	if err != nil {
		t.Fatalf("get track: %v", err)
	}
	if retrieved.Title != "Updated Track" {
		t.Errorf("expected title 'Updated Track', got %s", retrieved.Title)
	}
}

func TestTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	for i := 0; i < 5; i++ {
		track := &Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    "Track",
			Path:     "/music/track" + string(rune('0'+i)) + ".mp3",
			Mtime:    time.Now().Unix(),
			Size:     1024,
			Format:   "mp3",
		}
		db.InsertTrack(track)
	}

	tracks, err := db.Tracks(0, 10)
	if err != nil {
		t.Fatalf("get tracks: %v", err)
	}
	if len(tracks) != 5 {
		t.Errorf("expected 5 tracks, got %d", len(tracks))
	}

	// Test pagination
	tracks, err = db.Tracks(0, 2)
	if err != nil {
		t.Fatalf("get tracks paginated: %v", err)
	}
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(tracks))
	}
}

func TestSearchTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Search Artist")
	albumID, _ := db.InsertAlbum(artistID, "Search Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Search Track",
		Path:     "/music/search.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	db.InsertTrack(track)

	tracks, err := db.SearchTracks("Search", 0, 10)
	if err != nil {
		t.Fatalf("search tracks: %v", err)
	}
	if len(tracks) == 0 {
		t.Error("expected search results")
	}
}

func TestAlbums(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	db.InsertAlbum(artistID, "Album 1", 2023, "")
	db.InsertAlbum(artistID, "Album 2", 2024, "")

	albums, err := db.Albums()
	if err != nil {
		t.Fatalf("get albums: %v", err)
	}
	if len(albums) != 2 {
		t.Errorf("expected 2 albums, got %d", len(albums))
	}
}

func TestArtists(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	db.InsertArtist("Artist 1")
	db.InsertArtist("Artist 2")

	artists, err := db.Artists()
	if err != nil {
		t.Fatalf("get artists: %v", err)
	}
	if len(artists) != 2 {
		t.Errorf("expected 2 artists, got %d", len(artists))
	}
}

func TestAlbumTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track 1",
		Path:     "/music/track1.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	db.InsertTrack(track)

	tracks, err := db.AlbumTracks(albumID)
	if err != nil {
		t.Fatalf("get album tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(tracks))
	}
}

func TestSmartTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Smart Artist")
	albumID, _ := db.InsertAlbum(artistID, "Smart Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Smart Track",
		Path:     "/music/smart.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Duration: 200,
	}
	db.InsertTrack(track)

	tracks, err := db.SmartTracks("artist:Smart")
	if err != nil {
		t.Fatalf("smart tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(tracks))
	}

	tracks, err = db.SmartTracks("year:2024")
	if err != nil {
		t.Fatalf("smart tracks by year: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track by year, got %d", len(tracks))
	}

	tracks, err = db.SmartTracks("minDuration:100 maxDuration:300")
	if err != nil {
		t.Fatalf("smart tracks by duration: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track by duration, got %d", len(tracks))
	}
}

func TestPlaylistCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create playlist
	id, err := db.InsertPlaylist("Test Playlist", "")
	if err != nil {
		t.Fatalf("insert playlist: %v", err)
	}

	// Get playlists
	playlists, err := db.Playlists()
	if err != nil {
		t.Fatalf("get playlists: %v", err)
	}
	if len(playlists) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(playlists))
	}

	// Create smart playlist
	smartID, err := db.InsertPlaylist("Smart Playlist", "artist:test")
	if err != nil {
		t.Fatalf("insert smart playlist: %v", err)
	}

	playlists, _ = db.Playlists()
	if len(playlists) != 2 {
		t.Errorf("expected 2 playlists, got %d", len(playlists))
	}

	// Delete playlist
	if err := db.DeletePlaylist(id); err != nil {
		t.Fatalf("delete playlist: %v", err)
	}
	playlists, _ = db.Playlists()
	if len(playlists) != 1 {
		t.Errorf("expected 1 playlist after delete, got %d", len(playlists))
	}
	_ = smartID
}

func TestPlaylistTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := db.InsertTrack(track)

	playlistID, _ := db.InsertPlaylist("My Playlist", "")
	if err := db.AddPlaylistTrack(playlistID, trackID); err != nil {
		t.Fatalf("add playlist track: %v", err)
	}

	tracks, err := db.PlaylistTracks(playlistID)
	if err != nil {
		t.Fatalf("get playlist tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(tracks))
	}

	// Remove track
	if err := db.RemovePlaylistTrack(playlistID, trackID); err != nil {
		t.Fatalf("remove playlist track: %v", err)
	}
	tracks, _ = db.PlaylistTracks(playlistID)
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks after remove, got %d", len(tracks))
	}
}

func TestUserCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertUser("testuser", "hash123", "user")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero user id")
	}

	// Get by username
	u, err := db.UserByUsername("testuser")
	if err != nil {
		t.Fatalf("get user by username: %v", err)
	}
	if u.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", u.Username)
	}

	// Get by ID
	u2, err := db.UserByID(id)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if u2.ID != id {
		t.Errorf("expected id %d, got %d", id, u2.ID)
	}

	// Update password
	if err := db.UpdateUserPassword(id, "newhash"); err != nil {
		t.Fatalf("update password: %v", err)
	}

	// List users
	users, err := db.Users()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}

	// Delete user
	if err := db.DeleteUser(id); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	users, _ = db.Users()
	if len(users) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(users))
	}
}

func TestSessionCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("sessionuser", "hash", "user")
	expiresAt := time.Now().Add(24 * time.Hour)

	sessionID, err := db.CreateSession(userID, "token123", expiresAt)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_ = sessionID

	// Get by token
	s, err := db.SessionByToken("token123")
	if err != nil {
		t.Fatalf("get session by token: %v", err)
	}
	if s.Token != "token123" {
		t.Errorf("expected token token123, got %s", s.Token)
	}

	// Delete session
	if err := db.DeleteSession("token123"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	// Should not find expired/deleted
	_, err = db.SessionByToken("token123")
	if err == nil {
		t.Error("expected error for deleted session")
	}
}

func TestTrackPosition(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := db.InsertTrack(track)

	if err := db.UpdateTrackPosition(trackID, 123.45); err != nil {
		t.Fatalf("update position: %v", err)
	}

	retrieved, err := db.TrackByID(trackID)
	if err != nil {
		t.Fatalf("get track: %v", err)
	}
	if retrieved.PlaybackPosition != 123.45 {
		t.Errorf("expected position 123.45, got %f", retrieved.PlaybackPosition)
	}
}

func TestTrackLyrics(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := db.InsertTrack(track)

	if err := db.UpdateTrackLyrics(trackID, "These are lyrics"); err != nil {
		t.Fatalf("update lyrics: %v", err)
	}

	lyrics, err := db.TrackLyrics(trackID)
	if err != nil {
		t.Fatalf("get lyrics: %v", err)
	}
	if lyrics != "These are lyrics" {
		t.Errorf("expected lyrics, got %s", lyrics)
	}
}

func TestTracksByMediaType(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:   &albumID,
		ArtistID:  &artistID,
		Title:     "Podcast Episode",
		Path:      "/music/podcast.mp3",
		Mtime:     time.Now().Unix(),
		Size:      1024,
		Format:    "mp3",
		MediaType: "podcast",
	}
	db.InsertTrack(track)

	tracks, err := db.TracksByMediaType("podcast", 0, 10)
	if err != nil {
		t.Fatalf("get tracks by media type: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 podcast track, got %d", len(tracks))
	}
}

func TestPlayCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("playuser", "hash", "user")
	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := db.InsertTrack(track)

	playID, err := db.InsertPlay(userID, trackID, 120.5, 0.8)
	if err != nil {
		t.Fatalf("insert play: %v", err)
	}
	_ = playID

	plays, err := db.PlaysByUser(userID, 0, 10)
	if err != nil {
		t.Fatalf("get plays: %v", err)
	}
	if len(plays) != 1 {
		t.Errorf("expected 1 play, got %d", len(plays))
	}

	stats, err := db.StatsForUser(userID)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalPlays != 1 {
		t.Errorf("expected 1 total play, got %d", stats.TotalPlays)
	}
}

func TestPlaylistVisibility(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("owner", "hash", "user")
	id, err := db.InsertPlaylistWithOwner("Private Playlist", "", userID, "private")
	if err != nil {
		t.Fatalf("insert playlist with owner: %v", err)
	}

	ownerID, err := db.PlaylistOwner(id)
	if err != nil {
		t.Fatalf("get playlist owner: %v", err)
	}
	if ownerID != userID {
		t.Errorf("expected owner %d, got %d", userID, ownerID)
	}

	if err := db.UpdatePlaylistVisibility(id, "public"); err != nil {
		t.Fatalf("update visibility: %v", err)
	}
}

func TestSharePlaylist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ownerID, _ := db.InsertUser("owner", "hash", "user")
	otherID, _ := db.InsertUser("other", "hash", "user")
	playlistID, _ := db.InsertPlaylistWithOwner("Shared", "", ownerID, "shared")

	if err := db.SharePlaylist(playlistID, otherID); err != nil {
		t.Fatalf("share playlist: %v", err)
	}

	// other user should see it
	pls, err := db.PlaylistsForUser(otherID)
	if err != nil {
		t.Fatalf("get playlists for user: %v", err)
	}
	found := false
	for _, p := range pls {
		if p.ID == playlistID {
			found = true
			break
		}
	}
	if !found {
		t.Error("shared playlist not found for other user")
	}

	// Unshare
	if err := db.UnsharePlaylist(playlistID, otherID); err != nil {
		t.Fatalf("unshare playlist: %v", err)
	}
}

func TestSimilarAlbums(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	album1, _ := db.InsertAlbum(artistID, "Album 1", 2023, "")
	album2, _ := db.InsertAlbum(artistID, "Album 2", 2024, "")

	similar, err := db.SimilarAlbums(artistID, album1, 10)
	if err != nil {
		t.Fatalf("get similar albums: %v", err)
	}
	if len(similar) != 1 {
		t.Errorf("expected 1 similar album, got %d", len(similar))
	}
	if similar[0].ID != album2 {
		t.Errorf("expected album %d, got %d", album2, similar[0].ID)
	}
}

func TestGenres(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Genre:    "Rock",
	}
	db.InsertTrack(track)

	genres, err := db.Genres()
	if err != nil {
		t.Fatalf("get genres: %v", err)
	}
	if len(genres) != 1 || genres[0] != "Rock" {
		t.Errorf("expected genres [Rock], got %v", genres)
	}
}

func TestPlaybackSessionCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("sessionuser", "hash", "user")
	_, err := db.Exec(
		`INSERT INTO playback_sessions(user_id, token, device_name, state, volume, repeat_mode)
		 VALUES (?, ?, ?, 'stopped', 1.0, 'none')`,
		userID, "token1", "Device",
	)
	if err != nil {
		t.Fatalf("insert playback session: %v", err)
	}

	sessions, err := db.PlaybackSessions()
	if err != nil {
		t.Fatalf("get playback sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	session, err := db.PlaybackSessionByToken("token1")
	if err != nil {
		t.Fatalf("get session by token: %v", err)
	}
	if session.Token != "token1" {
		t.Errorf("expected token token1, got %s", session.Token)
	}
}

func TestCastDeviceCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertCastDevice("Living Room", "chromecast", "192.168.1.100", 8009, "castv2", `["audio"]`)
	if err != nil {
		t.Fatalf("insert cast device: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero id")
	}

	devices, err := db.CastDevices()
	if err != nil {
		t.Fatalf("get cast devices: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}

	if err := db.UpdateCastDeviceActivity(id, true); err != nil {
		t.Fatalf("update cast device activity: %v", err)
	}

	devices, _ = db.CastDevices()
	if len(devices) == 1 && !devices[0].IsActive {
		t.Error("expected device to be active")
	}

	if err := db.DeleteCastDevice(id); err != nil {
		t.Fatalf("delete cast device: %v", err)
	}
	devices, _ = db.CastDevices()
	if len(devices) != 0 {
		t.Errorf("expected 0 devices after delete, got %d", len(devices))
	}
}

func TestDSPPresetCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("dspuser", "hash", "user")
	id, err := db.InsertDSPPreset("Flat", userID, 0, 0, 0, -20, 4, -14, false)
	if err != nil {
		t.Fatalf("insert dsp preset: %v", err)
	}

	presets, err := db.DSPPresets(userID)
	if err != nil {
		t.Fatalf("get dsp presets: %v", err)
	}
	if len(presets) != 1 {
		t.Errorf("expected 1 preset, got %d", len(presets))
	}

	preset, err := db.DSPPresetByID(id)
	if err != nil {
		t.Fatalf("get preset by id: %v", err)
	}
	if preset.Name != "Flat" {
		t.Errorf("expected name Flat, got %s", preset.Name)
	}

	if err := db.DeleteDSPPreset(id); err != nil {
		t.Fatalf("delete preset: %v", err)
	}
}

func TestSyncRoomCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO sync_rooms(name, token, state) VALUES (?, ?, 'stopped')`, "Test Room", "token123")
	if err != nil {
		t.Fatalf("insert sync room: %v", err)
	}

	room, err := db.SyncRoomByToken("token123")
	if err != nil {
		t.Fatalf("get room by token: %v", err)
	}
	if room.Name != "Test Room" {
		t.Errorf("expected name Test Room, got %s", room.Name)
	}

	rooms, err := db.SyncRooms()
	if err != nil {
		t.Fatalf("get rooms: %v", err)
	}
	if len(rooms) != 1 {
		t.Errorf("expected 1 room, got %d", len(rooms))
	}
}

func TestNullInt64(t *testing.T) {
	var n NullInt64
	err := n.Scan(int64(42))
	if err != nil {
		t.Fatalf("scan int64: %v", err)
	}
	if !n.Valid || n.Int64 != 42 {
		t.Errorf("expected Valid=true, Int64=42, got Valid=%v, Int64=%d", n.Valid, n.Int64)
	}

	err = n.Scan(nil)
	if err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if n.Valid {
		t.Error("expected Valid=false after scanning nil")
	}

	err = n.Scan("string")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestRecommendedTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("recuser", "hash", "user")
	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
	}
	trackID, _ := db.InsertTrack(track)
	db.InsertPlay(userID, trackID, 100, 0.5)

	// Should return empty since no collaborative filtering data
	_, err := db.RecommendedTracks(userID, 10)
	if err != nil {
		t.Fatalf("get recommended tracks: %v", err)
	}
}

func TestRadioTracks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	artistID, _ := db.InsertArtist("Artist")
	albumID, _ := db.InsertAlbum(artistID, "Album", 2024, "")
	track := &Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track",
		Path:     "/music/track.mp3",
		Mtime:    time.Now().Unix(),
		Size:     1024,
		Format:   "mp3",
		Genre:    "Rock",
	}
	db.InsertTrack(track)

	tracks, err := db.RadioTracksRandom(10)
	if err != nil {
		t.Fatalf("get random tracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 random track, got %d", len(tracks))
	}

	tracks, err = db.RadioTracksByGenre("Rock", 10)
	if err != nil {
		t.Fatalf("get tracks by genre: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track by genre, got %d", len(tracks))
	}

	tracks, err = db.RadioTracksByArtist(artistID, 10)
	if err != nil {
		t.Fatalf("get tracks by artist: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("expected 1 track by artist, got %d", len(tracks))
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	userID, _ := db.InsertUser("cleanuser", "hash", "user")
	// Insert expired session
	_, err := db.Exec(
		"INSERT INTO sessions(user_id, token, expires_at) VALUES (?, ?, datetime('now', '-1 day'))",
		userID, "expired_token",
	)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	if err := db.CleanExpiredSessions(); err != nil {
		t.Fatalf("clean expired sessions: %v", err)
	}

	_, err = db.SessionByToken("expired_token")
	if err == nil {
		t.Error("expected expired session to be deleted")
	}
}

func TestMigrateAddColumn(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Adding a column that already exists should not error
	err := db.migrateAddColumn("tracks", "title", "TEXT")
	if err != nil {
		t.Fatalf("migrate add existing column: %v", err)
	}
}
