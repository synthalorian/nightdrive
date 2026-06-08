package playback

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"nightdrive/internal/db"
)

// State represents the playback state.
type State string

const (
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
)

// RepeatMode represents the repeat mode.
type RepeatMode string

const (
	RepeatNone  RepeatMode = "none"
	RepeatOne   RepeatMode = "one"
	RepeatAll   RepeatMode = "all"
)

// Session represents an active playback session.
type Session struct {
	ID              int64       `json:"id"`
	UserID          int64       `json:"userId"`
	Token           string      `json:"token"`
	DeviceName      string      `json:"deviceName"`
	CurrentTrackID  *int64      `json:"currentTrackId,omitempty"`
	Position        float64     `json:"position"`
	State           State       `json:"state"`
	Volume          float64     `json:"volume"`
	Shuffle         bool        `json:"shuffle"`
	RepeatMode      RepeatMode  `json:"repeatMode"`
	CrossfadeConfig Crossfade   `json:"crossfade"`
	DSPPresetID     *int64      `json:"dspPresetId,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	Queue           []QueueItem `json:"queue,omitempty"`
}

// QueueItem represents a track in the playback queue.
type QueueItem struct {
	ID       int64     `json:"id"`
	TrackID  int64     `json:"trackId"`
	Position int       `json:"position"`
	Track    *db.Track `json:"track,omitempty"`
}

// Crossfade holds crossfade configuration.
type Crossfade struct {
	Enabled       bool    `json:"enabled"`
	DurationSec   float64 `json:"durationSec"`
	ApplyOnManual bool    `json:"applyOnManual"`
}

// Engine manages playback sessions and state.
type Engine struct {
	db *db.DB
	mu sync.RWMutex
}

// NewEngine creates a playback engine.
func NewEngine(database *db.DB) *Engine {
	return &Engine{db: database}
}

// CreateSession initializes a new playback session.
func (e *Engine) CreateSession(userID int64, deviceName, token string) (*Session, error) {
	res, err := e.db.Exec(
		`INSERT INTO playback_sessions(user_id, token, device_name, state, volume, repeat_mode)
		 VALUES (?, ?, ?, 'stopped', 1.0, 'none')`,
		userID, token, deviceName,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	id, _ := res.LastInsertId()
	return e.GetSession(id)
}

// GetSession retrieves a session by ID.
func (e *Engine) GetSession(id int64) (*Session, error) {
	var s Session
	var currentTrackID db.NullInt64
	var dspPresetID db.NullInt64
	err := e.db.QueryRow(
		`SELECT id, user_id, token, device_name, current_track_id, position, state, volume,
		        shuffle, repeat_mode, created_at, updated_at
		 FROM playback_sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.UserID, &s.Token, &s.DeviceName, &currentTrackID, &s.Position,
		&s.State, &s.Volume, &s.Shuffle, &s.RepeatMode, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if currentTrackID.Valid {
		s.CurrentTrackID = &currentTrackID.Int64
	}
	if dspPresetID.Valid {
		s.DSPPresetID = &dspPresetID.Int64
	}
	s.CrossfadeConfig = Crossfade{Enabled: true, DurationSec: 5.0}
	queue, _ := e.GetQueue(id)
	s.Queue = queue
	return &s, nil
}

// GetSessionByToken retrieves a session by token.
func (e *Engine) GetSessionByToken(token string) (*Session, error) {
	var id int64
	err := e.db.QueryRow(`SELECT id FROM playback_sessions WHERE token = ?`, token).Scan(&id)
	if err != nil {
		return nil, err
	}
	return e.GetSession(id)
}

// UpdateState updates the playback state.
func (e *Engine) UpdateState(sessionID int64, state State, position float64) error {
	_, err := e.db.Exec(
		`UPDATE playback_sessions SET state = ?, position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		state, position, sessionID,
	)
	return err
}

// SetTrack changes the current track.
func (e *Engine) SetTrack(sessionID, trackID int64, position float64) error {
	_, err := e.db.Exec(
		`UPDATE playback_sessions SET current_track_id = ?, position = ?, state = 'playing', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		trackID, position, sessionID,
	)
	return err
}

// SetVolume updates the volume.
func (e *Engine) SetVolume(sessionID int64, volume float64) error {
	volume = math.Max(0, math.Min(1, volume))
	_, err := e.db.Exec(
		`UPDATE playback_sessions SET volume = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		volume, sessionID,
	)
	return err
}

// SetRepeatMode updates the repeat mode.
func (e *Engine) SetRepeatMode(sessionID int64, mode RepeatMode) error {
	_, err := e.db.Exec(
		`UPDATE playback_sessions SET repeat_mode = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		mode, sessionID,
	)
	return err
}

// SetShuffle updates the shuffle state.
func (e *Engine) SetShuffle(sessionID int64, shuffle bool) error {
	val := 0
	if shuffle {
		val = 1
	}
	_, err := e.db.Exec(
		`UPDATE playback_sessions SET shuffle = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		val, sessionID,
	)
	return err
}

// AddToQueue appends a track to the session queue.
func (e *Engine) AddToQueue(sessionID, trackID int64) error {
	var pos int
	e.db.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM playback_queue WHERE session_id = ?`, sessionID).Scan(&pos)
	_, err := e.db.Exec(
		`INSERT INTO playback_queue(session_id, track_id, position) VALUES (?, ?, ?)`,
		sessionID, trackID, pos,
	)
	return err
}

// GetQueue returns the session queue.
func (e *Engine) GetQueue(sessionID int64) ([]QueueItem, error) {
	rows, err := e.db.Query(
		`SELECT q.id, q.track_id, q.position, t.id, t.title, COALESCE(a.name,'') AS artist_name, t.duration
		 FROM playback_queue q
		 JOIN tracks t ON t.id = q.track_id
		 LEFT JOIN artists a ON a.id = t.artist_id
		 WHERE q.session_id = ?
		 ORDER BY q.position`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var queue []QueueItem
	for rows.Next() {
		var qi QueueItem
		var track db.Track
		err := rows.Scan(&qi.ID, &qi.TrackID, &qi.Position, &track.ID, &track.Title, &track.ArtistName, &track.Duration)
		if err != nil {
			return nil, err
		}
		qi.Track = &track
		queue = append(queue, qi)
	}
	return queue, rows.Err()
}

// RemoveFromQueue removes a track from the queue.
func (e *Engine) RemoveFromQueue(sessionID int64, position int) error {
	_, err := e.db.Exec(
		`DELETE FROM playback_queue WHERE session_id = ? AND position = ?`,
		sessionID, position,
	)
	return err
}

// ClearQueue removes all tracks from the session queue.
func (e *Engine) ClearQueue(sessionID int64) error {
	_, err := e.db.Exec(`DELETE FROM playback_queue WHERE session_id = ?`, sessionID)
	return err
}

// DeleteSession removes a session.
func (e *Engine) DeleteSession(sessionID int64) error {
	_, err := e.db.Exec(`DELETE FROM playback_sessions WHERE id = ?`, sessionID)
	return err
}

// AdvanceTrack moves to the next track in the queue.
func (e *Engine) AdvanceTrack(sessionID int64) (*db.Track, error) {
	queue, err := e.GetQueue(sessionID)
	if err != nil || len(queue) == 0 {
		return nil, err
	}
	var session Session
	err = e.db.QueryRow(`SELECT current_track_id, repeat_mode FROM playback_sessions WHERE id = ?`, sessionID).Scan(&session.CurrentTrackID, &session.RepeatMode)
	if err != nil {
		return nil, err
	}

	nextPos := 0
	if session.CurrentTrackID != nil {
		for i, qi := range queue {
			if qi.TrackID == *session.CurrentTrackID {
				nextPos = i + 1
				break
			}
		}
	}

	if nextPos >= len(queue) {
		if session.RepeatMode == RepeatAll {
			nextPos = 0
		} else {
			return nil, nil
		}
	}

	nextTrack := queue[nextPos]
	if err := e.SetTrack(sessionID, nextTrack.TrackID, 0); err != nil {
		return nil, err
	}
	return nextTrack.Track, nil
}

// PreviousTrack moves to the previous track.
func (e *Engine) PreviousTrack(sessionID int64) (*db.Track, error) {
	queue, err := e.GetQueue(sessionID)
	if err != nil || len(queue) == 0 {
		return nil, err
	}
	var currentTrackID *int64
	err = e.db.QueryRow(`SELECT current_track_id FROM playback_sessions WHERE id = ?`, sessionID).Scan(&currentTrackID)
	if err != nil {
		return nil, err
	}

	prevPos := len(queue) - 1
	if currentTrackID != nil {
		for i, qi := range queue {
			if qi.TrackID == *currentTrackID {
				if i > 0 {
					prevPos = i - 1
				}
				break
			}
		}
	}

	prevTrack := queue[prevPos]
	if err := e.SetTrack(sessionID, prevTrack.TrackID, 0); err != nil {
		return nil, err
	}
	return prevTrack.Track, nil
}

// CrossfadeParams returns the active crossfade parameters for a session transition.
func (e *Engine) CrossfadeParams(sessionID int64) (enabled bool, duration float64, err error) {
	s, err := e.GetSession(sessionID)
	if err != nil {
		return false, 0, err
	}
	return s.CrossfadeConfig.Enabled, s.CrossfadeConfig.DurationSec, nil
}

// CleanupOldSessions removes inactive sessions older than the given duration.
func (e *Engine) CleanupOldSessions(olderThan time.Duration) error {
	_, err := e.db.Exec(
		`DELETE FROM playback_sessions WHERE updated_at < datetime('now', ?) AND state = 'stopped'`,
		fmt.Sprintf("-%d seconds", int(olderThan.Seconds())),
	)
	return err
}

// StartCleanupRoutine begins a background goroutine that cleans old sessions.
func (e *Engine) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = e.CleanupOldSessions(24 * time.Hour)
			}
		}
	}()
}
