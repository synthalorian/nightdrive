package sync

import (
	"path/filepath"
	"testing"
	"time"

	"nightdrive/internal/db"
	"nightdrive/internal/playback"
)

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

func TestNewCoordinator(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)
	if coord == nil {
		t.Fatal("NewCoordinator returned nil")
	}
}

func TestCreateRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, err := coord.CreateRoom("Test Room", 0, "token123")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if room == nil {
		t.Fatal("expected room, got nil")
	}
	if room.Name != "Test Room" {
		t.Errorf("expected name 'Test Room', got %s", room.Name)
	}
	if room.Token != "token123" {
		t.Errorf("expected token 'token123', got %s", room.Token)
	}
}

func TestGetRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	created, _ := coord.CreateRoom("Test Room", 0, "token123")

	room, err := coord.GetRoom(created.ID)
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	if room.Name != "Test Room" {
		t.Errorf("expected name 'Test Room', got %s", room.Name)
	}
}

func TestGetRoomByToken(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	_, _ = coord.CreateRoom("Test Room", 0, "token123")

	room, err := coord.GetRoomByToken("token123")
	if err != nil {
		t.Fatalf("get room by token: %v", err)
	}
	if room.Name != "Test Room" {
		t.Errorf("expected name 'Test Room', got %s", room.Name)
	}
}

func TestGetRoom_NotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	_, err := coord.GetRoom(999)
	if err == nil {
		t.Error("expected error for nonexistent room")
	}
}

func TestListRooms(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	coord.CreateRoom("Room 1", 0, "token1")
	coord.CreateRoom("Room 2", 0, "token2")

	rooms, err := coord.ListRooms()
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	if len(rooms) != 2 {
		t.Errorf("expected 2 rooms, got %d", len(rooms))
	}
}

func TestDeleteRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("To Delete", 0, "token123")

	if err := coord.DeleteRoom(room.ID); err != nil {
		t.Fatalf("delete room: %v", err)
	}

	rooms, _ := coord.ListRooms()
	if len(rooms) != 0 {
		t.Errorf("expected 0 rooms after delete, got %d", len(rooms))
	}
}

func TestJoinRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")

	// Create a playback session first
	session, _ := engine.CreateSession(0, "Test Device", "sessiontoken")

	if err := coord.JoinRoom(room.ID, session.ID); err != nil {
		t.Fatalf("join room: %v", err)
	}

	members, err := coord.GetRoomMembers(room.ID)
	if err != nil {
		t.Fatalf("get room members: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
}

func TestLeaveRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	session, _ := engine.CreateSession(0, "Test Device", "sessiontoken")
	coord.JoinRoom(room.ID, session.ID)

	if err := coord.LeaveRoom(room.ID, session.ID); err != nil {
		t.Fatalf("leave room: %v", err)
	}

	members, _ := coord.GetRoomMembers(room.ID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after leave, got %d", len(members))
	}
}

func TestSetMaster(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	session, _ := engine.CreateSession(0, "Test Device", "sessiontoken")

	if err := coord.SetMaster(room.ID, session.ID); err != nil {
		t.Fatalf("set master: %v", err)
	}
}

func TestSyncState(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	session, _ := engine.CreateSession(0, "Test Device", "sessiontoken")
	coord.JoinRoom(room.ID, session.ID)

	if err := coord.SyncState(room.ID, 1, 30.0, "playing"); err != nil {
		t.Fatalf("sync state: %v", err)
	}

	updated, _ := coord.GetRoom(room.ID)
	if updated.State != "playing" {
		t.Errorf("expected state 'playing', got %s", updated.State)
	}
	if updated.Position != 30.0 {
		t.Errorf("expected position 30.0, got %f", updated.Position)
	}
}

func TestStartRoomPlayback(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")

	if err := coord.StartRoomPlayback(room.ID, 1); err != nil {
		t.Fatalf("start room playback: %v", err)
	}

	updated, _ := coord.GetRoom(room.ID)
	if updated.State != "playing" {
		t.Errorf("expected state 'playing', got %s", updated.State)
	}
}

func TestPauseRoomPlayback(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	coord.StartRoomPlayback(room.ID, 1)

	if err := coord.PauseRoomPlayback(room.ID, 45.0); err != nil {
		t.Fatalf("pause room playback: %v", err)
	}

	updated, _ := coord.GetRoom(room.ID)
	if updated.State != "paused" {
		t.Errorf("expected state 'paused', got %s", updated.State)
	}
}

func TestPauseRoomPlayback_NoTrack(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")

	err := coord.PauseRoomPlayback(room.ID, 0)
	if err == nil {
		t.Error("expected error when no track loaded")
	}
}

func TestSeekRoom(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	coord.StartRoomPlayback(room.ID, 1)

	if err := coord.SeekRoom(room.ID, 60.0); err != nil {
		t.Fatalf("seek room: %v", err)
	}

	updated, _ := coord.GetRoom(room.ID)
	if updated.Position != 60.0 {
		t.Errorf("expected position 60.0, got %f", updated.Position)
	}
}

func TestSetRoomVolume(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")
	session, _ := engine.CreateSession(0, "Test Device", "sessiontoken")
	coord.JoinRoom(room.ID, session.ID)

	if err := coord.SetRoomVolume(room.ID, 0.5); err != nil {
		t.Fatalf("set room volume: %v", err)
	}
}

func TestCalculateSyncOffset(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	room, _ := coord.CreateRoom("Test Room", 0, "token123")

	offset, err := coord.CalculateSyncOffset(room.ID, 1, 50)
	if err != nil {
		t.Fatalf("calculate sync offset: %v", err)
	}
	if offset < 0 {
		t.Errorf("expected non-negative offset, got %d", offset)
	}
}

func TestCalculateSyncOffset_RoomNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	_, err := coord.CalculateSyncOffset(999, 1, 50)
	if err == nil {
		t.Error("expected error for nonexistent room")
	}
}

func TestHeartbeat(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	// Should not panic
	coord.Heartbeat(1)
}

func TestSyncLoop(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	engine := playback.NewEngine(database)
	coord := NewCoordinator(database, engine)

	stop := make(chan struct{})
	go coord.SyncLoop(stop)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	close(stop)
}
