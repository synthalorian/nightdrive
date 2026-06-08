package sync

import (
	"fmt"
	"math"
	"sync"
	"time"

	"nightdrive/internal/db"
	"nightdrive/internal/playback"
)

// Room represents a synchronized multi-room playback group.
type Room struct {
	ID               int64       `json:"id"`
	Name             string      `json:"name"`
	OwnerID          *int64      `json:"ownerId,omitempty"`
	Token            string      `json:"token"`
	CurrentTrackID   *int64      `json:"currentTrackId,omitempty"`
	Position         float64     `json:"position"`
	State            string      `json:"state"`
	MasterSessionID  *int64      `json:"masterSessionId,omitempty"`
	Members          []Member    `json:"members,omitempty"`
	CreatedAt        time.Time   `json:"createdAt"`
}

// Member represents a device/session in a sync room.
type Member struct {
	SessionID  int64     `json:"sessionId"`
	DeviceName string    `json:"deviceName"`
	JoinedAt   time.Time `json:"joinedAt"`
	LatencyMs  int       `json:"latencyMs"`
}

// Coordinator manages multi-room synchronization.
type Coordinator struct {
	db       *db.DB
	engine   *playback.Engine
	mu       sync.RWMutex
	rooms    map[string]*Room // token -> room
	syncTick time.Duration
}

// NewCoordinator creates a sync coordinator.
func NewCoordinator(database *db.DB, engine *playback.Engine) *Coordinator {
	return &Coordinator{
		db:       database,
		engine:   engine,
		rooms:    make(map[string]*Room),
		syncTick: 100 * time.Millisecond,
	}
}

// CreateRoom creates a new sync room.
func (c *Coordinator) CreateRoom(name string, ownerID int64, token string) (*Room, error) {
	res, err := c.db.Exec(
		`INSERT INTO sync_rooms(name, owner_id, token, state) VALUES (?, ?, ?, 'stopped')`,
		name, ownerID, token,
	)
	if err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	id, _ := res.LastInsertId()
	return c.GetRoom(id)
}

// GetRoom retrieves a room by ID.
func (c *Coordinator) GetRoom(id int64) (*Room, error) {
	var r Room
	var ownerID db.NullInt64
	var currentTrackID db.NullInt64
	var masterSessionID db.NullInt64

	err := c.db.QueryRow(
		`SELECT id, name, owner_id, token, current_track_id, position, state, master_session_id, created_at
		 FROM sync_rooms WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &ownerID, &r.Token, &currentTrackID, &r.Position, &r.State, &masterSessionID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	if ownerID.Valid {
		r.OwnerID = &ownerID.Int64
	}
	if currentTrackID.Valid {
		r.CurrentTrackID = &currentTrackID.Int64
	}
	if masterSessionID.Valid {
		r.MasterSessionID = &masterSessionID.Int64
	}

	members, _ := c.GetRoomMembers(id)
	r.Members = members

	c.mu.Lock()
	c.rooms[r.Token] = &r
	c.mu.Unlock()

	return &r, nil
}

// GetRoomByToken retrieves a room by its token.
func (c *Coordinator) GetRoomByToken(token string) (*Room, error) {
	var id int64
	err := c.db.QueryRow(`SELECT id FROM sync_rooms WHERE token = ?`, token).Scan(&id)
	if err != nil {
		return nil, err
	}
	return c.GetRoom(id)
}

// JoinRoom adds a session to a sync room.
func (c *Coordinator) JoinRoom(roomID, sessionID int64) error {
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO sync_room_members(room_id, session_id) VALUES (?, ?)`,
		roomID, sessionID,
	)
	if err != nil {
		return err
	}

	// Sync the new member to current state
	room, err := c.GetRoom(roomID)
	if err != nil {
		return err
	}

	// Apply room state to the joining session
	if room.CurrentTrackID != nil {
		_ = c.engine.SetTrack(sessionID, *room.CurrentTrackID, room.Position)
	}
	_ = c.engine.UpdateState(sessionID, playback.State(room.State), room.Position)

	return nil
}

// LeaveRoom removes a session from a sync room.
func (c *Coordinator) LeaveRoom(roomID, sessionID int64) error {
	_, err := c.db.Exec(
		`DELETE FROM sync_room_members WHERE room_id = ? AND session_id = ?`,
		roomID, sessionID,
	)
	return err
}

// GetRoomMembers returns all members of a room.
func (c *Coordinator) GetRoomMembers(roomID int64) ([]Member, error) {
	rows, err := c.db.Query(
		`SELECT m.session_id, COALESCE(s.device_name, ''), m.joined_at
		 FROM sync_room_members m
		 JOIN playback_sessions s ON s.id = m.session_id
		 WHERE m.room_id = ?`, roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var m Member
		err := rows.Scan(&m.SessionID, &m.DeviceName, &m.JoinedAt)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// SetMaster designates a session as the room master.
func (c *Coordinator) SetMaster(roomID, sessionID int64) error {
	_, err := c.db.Exec(
		`UPDATE sync_rooms SET master_session_id = ? WHERE id = ?`,
		sessionID, roomID,
	)
	return err
}

// SyncState broadcasts state changes to all room members.
func (c *Coordinator) SyncState(roomID int64, trackID int64, position float64, state string) error {
	_, err := c.db.Exec(
		`UPDATE sync_rooms SET current_track_id = ?, position = ?, state = ? WHERE id = ?`,
		trackID, position, state, roomID,
	)
	if err != nil {
		return err
	}

	// Propagate to all member sessions
	members, err := c.GetRoomMembers(roomID)
	if err != nil {
		return err
	}

	for _, m := range members {
		// Don't update the master if it's the source
		room, _ := c.GetRoom(roomID)
		if room != nil && room.MasterSessionID != nil && *room.MasterSessionID == m.SessionID {
			continue
		}

		_ = c.engine.SetTrack(m.SessionID, trackID, position)
		_ = c.engine.UpdateState(m.SessionID, playback.State(state), position)
	}

	return nil
}

// CalculateSyncOffset computes the playback offset needed to synchronize a member.
func (c *Coordinator) CalculateSyncOffset(roomID, sessionID int64, localLatencyMs int) (offsetMs int, err error) {
	_, err = c.GetRoom(roomID)
	if err != nil {
		return 0, err
	}

	// Get member latencies
	members, err := c.GetRoomMembers(roomID)
	if err != nil {
		return 0, err
	}

	maxLatency := localLatencyMs
	for _, m := range members {
		if m.LatencyMs > maxLatency {
			maxLatency = m.LatencyMs
		}
	}

	// Offset = difference from max latency
	offsetMs = maxLatency - localLatencyMs
	if offsetMs < 0 {
		offsetMs = 0
	}
	return offsetMs, nil
}

// StartRoomPlayback begins synchronized playback in a room.
func (c *Coordinator) StartRoomPlayback(roomID int64, trackID int64) error {
	return c.SyncState(roomID, trackID, 0, string(playback.StatePlaying))
}

// PauseRoomPlayback pauses all devices in a room.
func (c *Coordinator) PauseRoomPlayback(roomID int64, position float64) error {
	room, err := c.GetRoom(roomID)
	if err != nil {
		return err
	}
	if room.CurrentTrackID == nil {
		return fmt.Errorf("no track loaded")
	}
	return c.SyncState(roomID, *room.CurrentTrackID, position, string(playback.StatePaused))
}

// SeekRoom seeks all devices in a room to a position.
func (c *Coordinator) SeekRoom(roomID int64, position float64) error {
	room, err := c.GetRoom(roomID)
	if err != nil {
		return err
	}
	if room.CurrentTrackID == nil {
		return fmt.Errorf("no track loaded")
	}
	return c.SyncState(roomID, *room.CurrentTrackID, position, room.State)
}

// SetRoomVolume adjusts volume for all members (relative to each device's volume).
func (c *Coordinator) SetRoomVolume(roomID int64, volume float64) error {
	volume = math.Max(0, math.Min(1, volume))
	members, err := c.GetRoomMembers(roomID)
	if err != nil {
		return err
	}
	for _, m := range members {
		_ = c.engine.SetVolume(m.SessionID, volume)
	}
	return nil
}

// DeleteRoom removes a sync room and all its members.
func (c *Coordinator) DeleteRoom(roomID int64) error {
	_, err := c.db.Exec(`DELETE FROM sync_rooms WHERE id = ?`, roomID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	for token, room := range c.rooms {
		if room.ID == roomID {
			delete(c.rooms, token)
			break
		}
	}
	c.mu.Unlock()
	return nil
}

// ListRooms returns all active sync rooms.
func (c *Coordinator) ListRooms() ([]Room, error) {
	rows, err := c.db.Query(
		`SELECT id, name, owner_id, token, current_track_id, position, state, master_session_id, created_at
		 FROM sync_rooms ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var r Room
		var ownerID db.NullInt64
		var currentTrackID db.NullInt64
		var masterSessionID db.NullInt64

		err := rows.Scan(&r.ID, &r.Name, &ownerID, &r.Token, &currentTrackID, &r.Position, &r.State, &masterSessionID, &r.CreatedAt)
		if err != nil {
			return nil, err
		}
		if ownerID.Valid {
			r.OwnerID = &ownerID.Int64
		}
		if currentTrackID.Valid {
			r.CurrentTrackID = &currentTrackID.Int64
		}
		if masterSessionID.Valid {
			r.MasterSessionID = &masterSessionID.Int64
		}
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}

// Heartbeat updates the last seen time for a member session.
func (c *Coordinator) Heartbeat(sessionID int64) {
	// In a real implementation, this would update a last_seen timestamp
	// and potentially trigger re-sync if a member was stale
}

// SyncLoop starts a background synchronization loop for all rooms.
func (c *Coordinator) SyncLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(c.syncTick)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.performSync()
		}
	}
}

func (c *Coordinator) performSync() {
	rooms, err := c.ListRooms()
	if err != nil {
		return
	}
	for _, room := range rooms {
		if room.State != string(playback.StatePlaying) {
			continue
		}
		// Update position based on elapsed time since last sync
		// In a real implementation, this would use NTP-synchronized clocks
		members, _ := c.GetRoomMembers(room.ID)
		for _, m := range members {
			c.Heartbeat(m.SessionID)
		}
	}
}
