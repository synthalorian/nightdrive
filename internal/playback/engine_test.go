package playback

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nightdrive/internal/db"
)

func setupTestEngine(tb testing.TB) (*Engine, *db.DB) {
	tb.Helper()
	tmpDir := tb.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		tb.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		tb.Fatalf("migrate db: %v", err)
	}

	engine := NewEngine(database)
	return engine, database
}

func TestNewEngine(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.db == nil {
		t.Error("expected non-nil db")
	}
}

func TestCreateSession(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	userID := int64(1)
	session, err := engine.CreateSession(userID, "Test Device", "testtoken")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.UserID != userID {
		t.Errorf("expected user ID %d, got %d", userID, session.UserID)
	}
	if session.Token != "testtoken" {
		t.Errorf("expected token 'testtoken', got %s", session.Token)
	}
	if session.DeviceName != "Test Device" {
		t.Errorf("expected device name 'Test Device', got %s", session.DeviceName)
	}
	if session.State != StateStopped {
		t.Errorf("expected state 'stopped', got %s", session.State)
	}
}

func TestGetSession(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, err := engine.CreateSession(1, "Device", "token1")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	retrieved, err := engine.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("expected ID %d, got %d", session.ID, retrieved.ID)
	}
	if retrieved.Token != "token1" {
		t.Errorf("expected token 'token1', got %s", retrieved.Token)
	}
}

func TestGetSessionByToken(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token2")

	retrieved, err := engine.GetSessionByToken("token2")
	if err != nil {
		t.Fatalf("GetSessionByToken failed: %v", err)
	}
	if retrieved.ID != session.ID {
		t.Errorf("expected ID %d, got %d", session.ID, retrieved.ID)
	}
}

func TestUpdateState(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token3")

	err := engine.UpdateState(session.ID, StatePlaying, 30.0)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	retrieved, _ := engine.GetSession(session.ID)
	if retrieved.State != StatePlaying {
		t.Errorf("expected state 'playing', got %s", retrieved.State)
	}
	if retrieved.Position != 30.0 {
		t.Errorf("expected position 30.0, got %f", retrieved.Position)
	}
}

func TestSetTrack(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token4")

	err := engine.SetTrack(session.ID, 42, 15.5)
	if err != nil {
		t.Fatalf("SetTrack failed: %v", err)
	}

	retrieved, _ := engine.GetSession(session.ID)
	if retrieved.CurrentTrackID == nil {
		t.Fatal("expected non-nil current track ID")
	}
	if *retrieved.CurrentTrackID != 42 {
		t.Errorf("expected track ID 42, got %d", *retrieved.CurrentTrackID)
	}
	if retrieved.Position != 15.5 {
		t.Errorf("expected position 15.5, got %f", retrieved.Position)
	}
	if retrieved.State != StatePlaying {
		t.Errorf("expected state 'playing', got %s", retrieved.State)
	}
}

func TestSetVolume(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token5")

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.5, 0.5},
		{1.5, 1.0},  // clamped to max
		{-0.5, 0.0}, // clamped to min
		{0.0, 0.0},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		err := engine.SetVolume(session.ID, tt.input)
		if err != nil {
			t.Fatalf("SetVolume(%f) failed: %v", tt.input, err)
		}

		retrieved, _ := engine.GetSession(session.ID)
		if retrieved.Volume != tt.expected {
			t.Errorf("SetVolume(%f): expected %f, got %f", tt.input, tt.expected, retrieved.Volume)
		}
	}
}

func TestSetRepeatMode(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token6")

	err := engine.SetRepeatMode(session.ID, RepeatAll)
	if err != nil {
		t.Fatalf("SetRepeatMode failed: %v", err)
	}

	retrieved, _ := engine.GetSession(session.ID)
	if retrieved.RepeatMode != RepeatAll {
		t.Errorf("expected repeat mode 'all', got %s", retrieved.RepeatMode)
	}
}

func TestSetShuffle(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token7")

	err := engine.SetShuffle(session.ID, true)
	if err != nil {
		t.Fatalf("SetShuffle failed: %v", err)
	}

	retrieved, _ := engine.GetSession(session.ID)
	if !retrieved.Shuffle {
		t.Error("expected shuffle to be true")
	}
}

func TestQueueOperations(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token8")

	// Insert tracks into database first
	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")

	for i := 0; i < 3; i++ {
		track := &db.Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    "Track",
			Path:     "/music/track" + string(rune('0'+i)) + ".mp3",
			Format:   "mp3",
		}
		trackID, _ := database.InsertTrack(track)

		err := engine.AddToQueue(session.ID, trackID)
		if err != nil {
			t.Fatalf("AddToQueue failed: %v", err)
		}
	}

	queue, err := engine.GetQueue(session.ID)
	if err != nil {
		t.Fatalf("GetQueue failed: %v", err)
	}
	if len(queue) != 3 {
		t.Errorf("expected 3 queue items, got %d", len(queue))
	}

	// Test removing from queue
	err = engine.RemoveFromQueue(session.ID, 2)
	if err != nil {
		t.Fatalf("RemoveFromQueue failed: %v", err)
	}

	queue, _ = engine.GetQueue(session.ID)
	if len(queue) != 2 {
		t.Errorf("expected 2 queue items after removal, got %d", len(queue))
	}

	// Test clearing queue
	err = engine.ClearQueue(session.ID)
	if err != nil {
		t.Fatalf("ClearQueue failed: %v", err)
	}

	queue, _ = engine.GetQueue(session.ID)
	if len(queue) != 0 {
		t.Errorf("expected 0 queue items after clear, got %d", len(queue))
	}
}

func TestDeleteSession(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token9")

	err := engine.DeleteSession(session.ID)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, err = engine.GetSession(session.ID)
	if err == nil {
		t.Error("expected error for deleted session")
	}
}

func TestAdvanceTrack(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token10")

	// Insert tracks
	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")

	for i := 0; i < 3; i++ {
		track := &db.Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    "Track " + string(rune('0'+i)),
			Path:     "/music/track" + string(rune('0'+i)) + ".mp3",
			Format:   "mp3",
		}
		trackID, _ := database.InsertTrack(track)
		engine.AddToQueue(session.ID, trackID)
	}

	// Set first track as current
	queue, _ := engine.GetQueue(session.ID)
	if len(queue) == 0 {
		t.Fatal("expected non-empty queue")
	}
	engine.SetTrack(session.ID, queue[0].TrackID, 0)

	// Advance to next track
	nextTrack, err := engine.AdvanceTrack(session.ID)
	if err != nil {
		t.Fatalf("AdvanceTrack failed: %v", err)
	}
	if nextTrack == nil {
		t.Fatal("expected next track")
	}

	// Verify session was updated
	retrieved, _ := engine.GetSession(session.ID)
	if retrieved.CurrentTrackID == nil || *retrieved.CurrentTrackID != nextTrack.ID {
		t.Error("expected session to be updated to next track")
	}
}

func TestPreviousTrack(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token11")

	// Insert tracks
	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")

	var trackIDs []int64
	for i := 0; i < 3; i++ {
		track := &db.Track{
			AlbumID:  &albumID,
			ArtistID: &artistID,
			Title:    "Track " + string(rune('0'+i)),
			Path:     "/music/track" + string(rune('0'+i)) + ".mp3",
			Format:   "mp3",
		}
		trackID, _ := database.InsertTrack(track)
		trackIDs = append(trackIDs, trackID)
		engine.AddToQueue(session.ID, trackID)
	}

	// Set second track as current
	engine.SetTrack(session.ID, trackIDs[1], 0)

	// Go to previous
	prevTrack, err := engine.PreviousTrack(session.ID)
	if err != nil {
		t.Fatalf("PreviousTrack failed: %v", err)
	}
	if prevTrack == nil {
		t.Fatal("expected previous track")
	}

	if prevTrack.ID != trackIDs[0] {
		t.Errorf("expected track ID %d, got %d", trackIDs[0], prevTrack.ID)
	}
}

func TestCrossfadeParams(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token12")

	enabled, duration, err := engine.CrossfadeParams(session.ID)
	if err != nil {
		t.Fatalf("CrossfadeParams failed: %v", err)
	}
	if !enabled {
		t.Error("expected crossfade enabled by default")
	}
	if duration != 5.0 {
		t.Errorf("expected duration 5.0, got %f", duration)
	}
}

func TestCleanupOldSessions(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	// Create a session
	session, _ := engine.CreateSession(1, "Device", "token13")

	// Set it to stopped and update to old timestamp
	engine.UpdateState(session.ID, StateStopped, 0)

	// Clean up sessions older than 0 seconds (should remove all stopped)
	// Note: This test might be flaky depending on timing
	// In a real scenario we'd mock time
	err := engine.CleanupOldSessions(0)
	if err != nil {
		t.Fatalf("CleanupOldSessions failed: %v", err)
	}
}

func TestStartCleanupRoutine(t *testing.T) {
	engine, database := setupTestEngine(t)
	defer database.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic
	engine.StartCleanupRoutine(ctx, 100*time.Millisecond)

	// Give it a moment to run
	time.Sleep(50 * time.Millisecond)
}

func BenchmarkCreateSession(b *testing.B) {
	engine, database := setupTestEngine(b)
	defer database.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.CreateSession(int64(i), "Device", "token")
	}
}

func BenchmarkGetSession(b *testing.B) {
	engine, database := setupTestEngine(b)
	defer database.Close()

	session, _ := engine.CreateSession(1, "Device", "token")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.GetSession(session.ID)
	}
}
