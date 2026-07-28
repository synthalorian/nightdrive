package main

import (
	"path/filepath"
	"testing"
)

// newTestDB opens a fresh database in a temp dir and registers cleanup.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// seedTrack inserts an artist/album/track and returns the track.
func seedTrack(t *testing.T, d *DB, artistName, albumTitle, trackTitle, path string) *Track {
	t.Helper()
	artist, err := d.GetOrCreateArtist(artistName)
	if err != nil {
		t.Fatalf("GetOrCreateArtist: %v", err)
	}
	album, err := d.GetOrCreateAlbum(artist.ID, albumTitle, 2024)
	if err != nil {
		t.Fatalf("GetOrCreateAlbum: %v", err)
	}
	track, err := d.InsertTrack(album.ID, artist.ID, trackTitle, path, 1, 180, "MP3", 320)
	if err != nil {
		t.Fatalf("InsertTrack: %v", err)
	}
	return track
}

func TestSchemaInit(t *testing.T) {
	d := newTestDB(t)

	// Schema should be idempotent: re-running initSchema must succeed.
	if err := d.initSchema(); err != nil {
		t.Fatalf("initSchema second run: %v", err)
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats["tracks"] != 0 || stats["artists"] != 0 || stats["albums"] != 0 {
		t.Fatalf("expected empty stats, got %v", stats)
	}
}

func TestSmartPlaylistCRUD(t *testing.T) {
	d := newTestDB(t)

	sp, err := d.CreateSmartPlaylist(0, "Fresh Finds", "recently_added", "", 25)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}
	if sp.ID == 0 {
		t.Fatal("expected non-zero smart playlist ID")
	}
	if sp.Name != "Fresh Finds" || sp.RuleType != "recently_added" || sp.LimitNum != 25 {
		t.Fatalf("unexpected smart playlist: %+v", sp)
	}

	got, err := d.GetSmartPlaylistByID(sp.ID)
	if err != nil {
		t.Fatalf("GetSmartPlaylistByID: %v", err)
	}
	if got.Name != sp.Name || got.RuleType != sp.RuleType {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	all, err := d.GetAllSmartPlaylists(0)
	if err != nil {
		t.Fatalf("GetAllSmartPlaylists: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 smart playlist, got %d", len(all))
	}

	updated, err := d.UpdateSmartPlaylist(sp.ID, "Most Played", "most_played", "", 10)
	if err != nil {
		t.Fatalf("UpdateSmartPlaylist: %v", err)
	}
	if updated.Name != "Most Played" || updated.RuleType != "most_played" || updated.LimitNum != 10 {
		t.Fatalf("update mismatch: %+v", updated)
	}

	if err := d.DeleteSmartPlaylist(sp.ID); err != nil {
		t.Fatalf("DeleteSmartPlaylist: %v", err)
	}
	if _, err := d.GetSmartPlaylistByID(sp.ID); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestSmartPlaylistTracks(t *testing.T) {
	d := newTestDB(t)
	seedTrack(t, d, "Artist A", "Album A", "Song One", "/music/a/1.mp3")
	seedTrack(t, d, "Artist A", "Album A", "Song Two", "/music/a/2.mp3")

	sp, err := d.CreateSmartPlaylist(0, "Random", "random", "", 10)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}
	tracks, err := d.GetSmartPlaylistTracks(sp)
	if err != nil {
		t.Fatalf("GetSmartPlaylistTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	// Limit should be respected.
	spLimited, err := d.CreateSmartPlaylist(0, "One Only", "recently_added", "", 1)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}
	tracks, err = d.GetSmartPlaylistTracks(spLimited)
	if err != nil {
		t.Fatalf("GetSmartPlaylistTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track with limit 1, got %d", len(tracks))
	}
}

func TestPlaylistCRUDAndTracks(t *testing.T) {
	d := newTestDB(t)
	track := seedTrack(t, d, "Artist B", "Album B", "Playlist Song", "/music/b/1.mp3")

	pl, err := d.CreatePlaylist(0, "Night Mix")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if pl.ID == 0 || pl.Name != "Night Mix" {
		t.Fatalf("unexpected playlist: %+v", pl)
	}

	got, err := d.GetPlaylistByID(pl.ID)
	if err != nil {
		t.Fatalf("GetPlaylistByID: %v", err)
	}
	if got.Name != "Night Mix" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	all, err := d.GetAllPlaylists(0)
	if err != nil {
		t.Fatalf("GetAllPlaylists: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 playlist, got %d", len(all))
	}

	updated, err := d.UpdatePlaylist(pl.ID, "Day Mix")
	if err != nil {
		t.Fatalf("UpdatePlaylist: %v", err)
	}
	if updated.Name != "Day Mix" {
		t.Fatalf("update mismatch: %+v", updated)
	}

	// Add and list tracks.
	if err := d.AddTrackToPlaylist(pl.ID, track.ID); err != nil {
		t.Fatalf("AddTrackToPlaylist: %v", err)
	}
	tracks, err := d.GetPlaylistTracks(pl.ID)
	if err != nil {
		t.Fatalf("GetPlaylistTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 playlist track, got %d", len(tracks))
	}
	if tracks[0].TrackID != track.ID {
		t.Fatalf("expected track %d, got %d", track.ID, tracks[0].TrackID)
	}
	if tracks[0].Track == nil || tracks[0].Track.Title != "Playlist Song" {
		t.Fatalf("expected joined track data, got %+v", tracks[0].Track)
	}

	// Duplicate add should fail (UNIQUE constraint).
	if err := d.AddTrackToPlaylist(pl.ID, track.ID); err == nil {
		t.Fatal("expected error on duplicate playlist track, got nil")
	}

	if err := d.RemoveTrackFromPlaylist(pl.ID, track.ID); err != nil {
		t.Fatalf("RemoveTrackFromPlaylist: %v", err)
	}
	tracks, err = d.GetPlaylistTracks(pl.ID)
	if err != nil {
		t.Fatalf("GetPlaylistTracks: %v", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("expected 0 playlist tracks after removal, got %d", len(tracks))
	}

	if err := d.DeletePlaylist(pl.ID); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}
	if _, err := d.GetPlaylistByID(pl.ID); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestTrackInsertAndSearch(t *testing.T) {
	d := newTestDB(t)

	track := seedTrack(t, d, "Synth Master", "Neon Nights", "Midnight Drive", "/music/sm/nn/01 - Midnight Drive.flac")
	if track.ID == 0 {
		t.Fatal("expected non-zero track ID")
	}
	if track.Title != "Midnight Drive" || track.Format != "MP3" || track.Bitrate != 320 {
		t.Fatalf("unexpected track: %+v", track)
	}

	// Upsert on same path should not duplicate.
	again, err := d.InsertTrack(track.AlbumID, track.ArtistID, "Midnight Drive", track.Path, 1, 180, "FLAC", 320)
	if err != nil {
		t.Fatalf("InsertTrack upsert: %v", err)
	}
	if again.ID != track.ID {
		t.Fatalf("expected upsert to keep ID %d, got %d", track.ID, again.ID)
	}

	got, err := d.GetTrackByID(track.ID)
	if err != nil {
		t.Fatalf("GetTrackByID: %v", err)
	}
	if got.Path != track.Path {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Search by title.
	results, err := d.SearchTracks("Midnight")
	if err != nil {
		t.Fatalf("SearchTracks: %v", err)
	}
	if len(results) != 1 || results[0].ID != track.ID {
		t.Fatalf("search mismatch: %+v", results)
	}

	// Search should miss unrelated terms.
	results, err = d.SearchTracks("nonexistent-xyzzy")
	if err != nil {
		t.Fatalf("SearchTracks: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}

	// Listing helpers.
	tracks, err := d.GetAllTracks(10, 0)
	if err != nil {
		t.Fatalf("GetAllTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}

	artists, err := d.GetAllArtists()
	if err != nil {
		t.Fatalf("GetAllArtists: %v", err)
	}
	if len(artists) != 1 || artists[0].Name != "Synth Master" {
		t.Fatalf("artists mismatch: %+v", artists)
	}

	albums, err := d.GetAlbumsByArtist(artists[0].ID)
	if err != nil {
		t.Fatalf("GetAlbumsByArtist: %v", err)
	}
	if len(albums) != 1 || albums[0].Title != "Neon Nights" || albums[0].Year != 2024 {
		t.Fatalf("albums mismatch: %+v", albums)
	}

	albumTracks, err := d.GetTracksByAlbum(albums[0].ID)
	if err != nil {
		t.Fatalf("GetTracksByAlbum: %v", err)
	}
	if len(albumTracks) != 1 || albumTracks[0].ID != track.ID {
		t.Fatalf("album tracks mismatch: %+v", albumTracks)
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats["tracks"] != 1 || stats["artists"] != 1 || stats["albums"] != 1 {
		t.Fatalf("unexpected stats: %v", stats)
	}
}

func TestUserCreateAndLookup(t *testing.T) {
	d := newTestDB(t)

	user, err := d.CreateUser("testuser", "test-api-key")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected non-zero user ID")
	}

	byKey, err := d.GetUserByAPIKey("test-api-key")
	if err != nil {
		t.Fatalf("GetUserByAPIKey: %v", err)
	}
	if byKey.Username != "testuser" {
		t.Fatalf("user mismatch: %+v", byKey)
	}

	count, err := d.GetUsersCount()
	if err != nil {
		t.Fatalf("GetUsersCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	// Duplicate username must fail.
	if _, err := d.CreateUser("testuser", "other-key"); err == nil {
		t.Fatal("expected error on duplicate username, got nil")
	}
}
