package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB connection
type DB struct {
	conn *sql.DB
}

// Artist represents a music artist
type Artist struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Album represents a music album
type Album struct {
	ID        int64     `json:"id"`
	ArtistID  int64     `json:"artist_id"`
	Title     string    `json:"title"`
	Year      int       `json:"year"`
	CreatedAt time.Time `json:"created_at"`
}

// AlbumWithArtist represents an album with nested artist info
type AlbumWithArtist struct {
	Album
	ArtistName string `json:"artist_name"`
}

// Track represents a single music track
type Track struct {
	ID        int64     `json:"id"`
	AlbumID   int64     `json:"album_id"`
	ArtistID  int64     `json:"artist_id"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	TrackNum  int       `json:"track_num"`
	Duration  int       `json:"duration"` // seconds
	Format    string    `json:"format"`
	Bitrate   int       `json:"bitrate"`
	CreatedAt time.Time `json:"created_at"`
}

// Playlist represents a user playlist
type Playlist struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlaylistTrack represents a track in a playlist with ordering
type PlaylistTrack struct {
	ID         int64     `json:"id"`
	PlaylistID int64     `json:"playlist_id"`
	TrackID    int64     `json:"track_id"`
	Position   int       `json:"position"`
	CreatedAt  time.Time `json:"created_at"`
	Track      *Track    `json:"track,omitempty"`
}

// User represents a system user
type User struct {
	ID               int64     `json:"id"`
	Username         string    `json:"username"`
	APIKey           string    `json:"api_key"`
	LastFMUsername   string    `json:"lastfm_username"`
	LastFMSessionKey string    `json:"lastfm_session_key,omitempty"`
	LastFMAPIKey     string    `json:"lastfm_api_key,omitempty"`
	QuotaBytes       int64     `json:"quota_bytes"`
	QuotaUsedBytes   int64     `json:"quota_used_bytes"`
	CreatedAt        time.Time `json:"created_at"`
}

// Star represents a starred item
type Star struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ItemID    int64     `json:"item_id"`
	ItemType  string    `json:"item_type"`
	CreatedAt time.Time `json:"created_at"`
}

// SmartPlaylist represents a rules-based auto-updating playlist
type SmartPlaylist struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	RuleType  string    `json:"rule_type"`  // recently_added, most_played, genre, year, random
	RuleValue string    `json:"rule_value"` // e.g. "Rock", "2020", ""
	LimitNum  int       `json:"limit_num"`  // max tracks
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Lyrics represents track lyrics
type Lyrics struct {
	TrackID   int64     `json:"track_id"`
	Content   string    `json:"content"`
	Synced    bool      `json:"synced"`
	Source    string    `json:"source"` // embedded, lrclib
	UpdatedAt time.Time `json:"updated_at"`
}

// NewDB creates a new database connection and initializes the schema
func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	log.Println("Database initialized")
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS artists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS albums (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		artist_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		year INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE,
		UNIQUE(artist_id, title)
	);

	CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id);

	CREATE TABLE IF NOT EXISTS tracks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		album_id INTEGER NOT NULL,
		artist_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		path TEXT NOT NULL UNIQUE,
		track_num INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 0,
		format TEXT DEFAULT '',
		bitrate INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE,
		FOREIGN KEY (artist_id) REFERENCES artists(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id);
	CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id);

	CREATE TABLE IF NOT EXISTS playlists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS playlist_tracks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		playlist_id INTEGER NOT NULL,
		track_id INTEGER NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE,
		FOREIGN KEY (track_id) REFERENCES tracks(id) ON DELETE CASCADE,
		UNIQUE(playlist_id, track_id)
	);

	CREATE INDEX IF NOT EXISTS idx_playlist_tracks_playlist ON playlist_tracks(playlist_id);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		api_key TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS stars (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		item_id INTEGER NOT NULL,
		item_type TEXT NOT NULL CHECK(item_type IN ('track', 'album', 'artist')),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, item_id, item_type),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_stars_user ON stars(user_id);

	CREATE TABLE IF NOT EXISTS play_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		track_id INTEGER NOT NULL,
		played_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (track_id) REFERENCES tracks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_play_history_user ON play_history(user_id);
	CREATE INDEX IF NOT EXISTS idx_play_history_track ON play_history(track_id);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Migrate: add cover_art to albums if not exists
	if _, err := db.conn.Exec(`ALTER TABLE albums ADD COLUMN cover_art BLOB`); err != nil {
		// Column likely already exists, ignore error
	}

	// Migrate: add user_id to playlists if not exists
	if _, err := db.conn.Exec(`ALTER TABLE playlists ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`); err != nil {
		// Column likely already exists, ignore error
	}

	// Migrate: add lyrics table
	if _, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS lyrics (
			track_id INTEGER PRIMARY KEY,
			content TEXT,
			synced INTEGER DEFAULT 0,
			source TEXT DEFAULT '',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (track_id) REFERENCES tracks(id) ON DELETE CASCADE
		)`); err != nil {
		return err
	}

	// Migrate: add play_count to tracks
	if _, err := db.conn.Exec(`ALTER TABLE tracks ADD COLUMN play_count INTEGER DEFAULT 0`); err != nil {
		// Column likely already exists, ignore error
	}

	// Migrate: add smart_playlists table
	if _, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS smart_playlists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			rule_type TEXT NOT NULL DEFAULT 'random',
			rule_value TEXT DEFAULT '',
			limit_num INTEGER DEFAULT 50,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return err
	}

	// Migrate: add lastfm and quota fields to users
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN lastfm_username TEXT DEFAULT ''`); err != nil {
		// ignore
	}
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN lastfm_session_key TEXT DEFAULT ''`); err != nil {
		// ignore
	}
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN lastfm_api_key TEXT DEFAULT ''`); err != nil {
		// ignore
	}
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN quota_bytes INTEGER DEFAULT 0`); err != nil {
		// ignore
	}
	if _, err := db.conn.Exec(`ALTER TABLE users ADD COLUMN quota_used_bytes INTEGER DEFAULT 0`); err != nil {
		// ignore
	}

	return nil
}

// GetOrCreateArtist gets an existing artist or creates a new one
func (db *DB) GetOrCreateArtist(name string) (*Artist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Unknown Artist"
	}

	var artist Artist
	err := db.conn.QueryRow(
		"SELECT id, name, created_at FROM artists WHERE name = ?",
		name,
	).Scan(&artist.ID, &artist.Name, &artist.CreatedAt)

	if err == nil {
		return &artist, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	res, err := db.conn.Exec(
		"INSERT INTO artists (name) VALUES (?)",
		name,
	)
	if err != nil {
		return nil, err
	}

	artist.ID, _ = res.LastInsertId()
	artist.Name = name
	artist.CreatedAt = time.Now()
	return &artist, nil
}

// GetOrCreateAlbum gets an existing album or creates a new one
func (db *DB) GetOrCreateAlbum(artistID int64, title string, year int) (*Album, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Unknown Album"
	}

	var album Album
	err := db.conn.QueryRow(
		"SELECT id, artist_id, title, year, created_at FROM albums WHERE artist_id = ? AND title = ?",
		artistID, title,
	).Scan(&album.ID, &album.ArtistID, &album.Title, &album.Year, &album.CreatedAt)

	if err == nil {
		return &album, nil
	}

	if err != sql.ErrNoRows {
		return nil, err
	}

	res, err := db.conn.Exec(
		"INSERT INTO albums (artist_id, title, year) VALUES (?, ?, ?)",
		artistID, title, year,
	)
	if err != nil {
		return nil, err
	}

	album.ID, _ = res.LastInsertId()
	album.ArtistID = artistID
	album.Title = title
	album.Year = year
	album.CreatedAt = time.Now()
	return &album, nil
}

// InsertTrack inserts a new track into the database
func (db *DB) InsertTrack(albumID, artistID int64, title, path string, trackNum, duration int, format string, bitrate int) (*Track, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		// Use filename without extension as fallback
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	res, err := db.conn.Exec(
		`INSERT INTO tracks (album_id, artist_id, title, path, track_num, duration, format, bitrate)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
			album_id=excluded.album_id,
			artist_id=excluded.artist_id,
			title=excluded.title,
			track_num=excluded.track_num,
			duration=excluded.duration,
			format=excluded.format,
			bitrate=excluded.bitrate`,
		albumID, artistID, title, path, trackNum, duration, format, bitrate,
	)
	if err != nil {
		return nil, err
	}

	trackID, _ := res.LastInsertId()
	return db.GetTrackByID(trackID)
}

// GetTrackByID retrieves a track by its ID
func (db *DB) GetTrackByID(id int64) (*Track, error) {
	var track Track
	err := db.conn.QueryRow(
		"SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks WHERE id = ?",
		id,
	).Scan(&track.ID, &track.AlbumID, &track.ArtistID, &track.Title, &track.Path,
		&track.TrackNum, &track.Duration, &track.Format, &track.Bitrate, &track.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &track, nil
}

// GetAllArtists returns all artists
func (db *DB) GetAllArtists() ([]*Artist, error) {
	rows, err := db.conn.Query("SELECT id, name, created_at FROM artists ORDER BY name")
	if err != nil {
		return []*Artist{}, err
	}
	defer rows.Close()

	var artists []*Artist
	for rows.Next() {
		var a Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.CreatedAt); err != nil {
			return []*Artist{}, err
		}
		artists = append(artists, &a)
	}
	if artists == nil {
		artists = []*Artist{}
	}
	return artists, rows.Err()
}

// GetAlbumsByArtist returns all albums for an artist
func (db *DB) GetAlbumsByArtist(artistID int64) ([]*Album, error) {
	rows, err := db.conn.Query(
		"SELECT id, artist_id, title, year, created_at FROM albums WHERE artist_id = ? ORDER BY year, title",
		artistID,
	)
	if err != nil {
		return []*Album{}, err
	}
	defer rows.Close()

	var albums []*Album
	for rows.Next() {
		var a Album
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.Title, &a.Year, &a.CreatedAt); err != nil {
			return []*Album{}, err
		}
		albums = append(albums, &a)
	}
	if albums == nil {
		albums = []*Album{}
	}
	return albums, rows.Err()
}

// GetTracksByAlbum returns all tracks for an album
func (db *DB) GetTracksByAlbum(albumID int64) ([]*Track, error) {
	rows, err := db.conn.Query(
		"SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks WHERE album_id = ? ORDER BY track_num, title",
		albumID,
	)
	if err != nil {
		return []*Track{}, err
	}
	defer rows.Close()

	var tracks []*Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.Title, &t.Path,
			&t.TrackNum, &t.Duration, &t.Format, &t.Bitrate, &t.CreatedAt); err != nil {
			return []*Track{}, err
		}
		tracks = append(tracks, &t)
	}
	if tracks == nil {
		tracks = []*Track{}
	}
	return tracks, rows.Err()
}

// GetAllTracks returns all tracks in the library
func (db *DB) GetAllTracks(limit, offset int) ([]*Track, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.conn.Query(
		"SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks ORDER BY title LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return []*Track{}, err
	}
	defer rows.Close()

	var tracks []*Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.Title, &t.Path,
			&t.TrackNum, &t.Duration, &t.Format, &t.Bitrate, &t.CreatedAt); err != nil {
			return []*Track{}, err
		}
		tracks = append(tracks, &t)
	}
	if tracks == nil {
		tracks = []*Track{}
	}
	return tracks, rows.Err()
}

// SearchTracks searches tracks by title
func (db *DB) SearchTracks(query string) ([]*Track, error) {
	rows, err := db.conn.Query(
		"SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks WHERE title LIKE ? ORDER BY title",
		"%"+query+"%",
	)
	if err != nil {
		return []*Track{}, err
	}
	defer rows.Close()

	var tracks []*Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.Title, &t.Path,
			&t.TrackNum, &t.Duration, &t.Format, &t.Bitrate, &t.CreatedAt); err != nil {
			return []*Track{}, err
		}
		tracks = append(tracks, &t)
	}
	if tracks == nil {
		tracks = []*Track{}
	}
	return tracks, rows.Err()
}

// ClearLibrary removes all tracks, albums, and artists
func (db *DB) ClearLibrary() error {
	_, err := db.conn.Exec("DELETE FROM tracks; DELETE FROM albums; DELETE FROM artists;")
	return err
}

// CreatePlaylist creates a new playlist
func (db *DB) CreatePlaylist(userID int64, name string) (*Playlist, error) {
	res, err := db.conn.Exec("INSERT INTO playlists (user_id, name) VALUES (?, ?)", userID, name)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return db.GetPlaylistByID(id)
}

// GetPlaylistByID retrieves a playlist by ID
func (db *DB) GetPlaylistByID(id int64) (*Playlist, error) {
	var p Playlist
	err := db.conn.QueryRow(
		"SELECT id, user_id, name, created_at, updated_at FROM playlists WHERE id = ?",
		id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetAllPlaylists returns all playlists for a user (or all if userID == 0)
func (db *DB) GetAllPlaylists(userID int64) ([]*Playlist, error) {
	var rows *sql.Rows
	var err error
	if userID > 0 {
		rows, err = db.conn.Query("SELECT id, user_id, name, created_at, updated_at FROM playlists WHERE user_id = ? ORDER BY name", userID)
	} else {
		rows, err = db.conn.Query("SELECT id, user_id, name, created_at, updated_at FROM playlists ORDER BY name")
	}
	if err != nil {
		return []*Playlist{}, err
	}
	defer rows.Close()

	var playlists []*Playlist
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return []*Playlist{}, err
		}
		playlists = append(playlists, &p)
	}
	if playlists == nil {
		playlists = []*Playlist{}
	}
	return playlists, rows.Err()
}

// UpdatePlaylist updates a playlist's name
func (db *DB) UpdatePlaylist(id int64, name string) (*Playlist, error) {
	_, err := db.conn.Exec(
		"UPDATE playlists SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		name, id,
	)
	if err != nil {
		return nil, err
	}
	return db.GetPlaylistByID(id)
}

// DeletePlaylist deletes a playlist
func (db *DB) DeletePlaylist(id int64) error {
	_, err := db.conn.Exec("DELETE FROM playlists WHERE id = ?", id)
	return err
}

// AddTrackToPlaylist adds a track to a playlist
func (db *DB) AddTrackToPlaylist(playlistID, trackID int64) error {
	// Get max position
	var maxPos int
	err := db.conn.QueryRow(
		"SELECT COALESCE(MAX(position), 0) FROM playlist_tracks WHERE playlist_id = ?",
		playlistID,
	).Scan(&maxPos)
	if err != nil {
		return err
	}

	_, err = db.conn.Exec(
		"INSERT INTO playlist_tracks (playlist_id, track_id, position) VALUES (?, ?, ?)",
		playlistID, trackID, maxPos+1,
	)
	return err
}

// RemoveTrackFromPlaylist removes a track from a playlist
func (db *DB) RemoveTrackFromPlaylist(playlistID, trackID int64) error {
	_, err := db.conn.Exec(
		"DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?",
		playlistID, trackID,
	)
	return err
}

// ReorderPlaylistTrack updates the position of a track in a playlist
func (db *DB) ReorderPlaylistTrack(playlistID, trackID int64, newPosition int) error {
	_, err := db.conn.Exec(
		"UPDATE playlist_tracks SET position = ? WHERE playlist_id = ? AND track_id = ?",
		newPosition, playlistID, trackID,
	)
	return err
}

// GetPlaylistTracks returns all tracks in a playlist
func (db *DB) GetPlaylistTracks(playlistID int64) ([]*PlaylistTrack, error) {
	rows, err := db.conn.Query(
		`SELECT pt.id, pt.playlist_id, pt.track_id, pt.position, pt.created_at,
			t.id, t.album_id, t.artist_id, t.title, t.path, t.track_num, t.duration, t.format, t.bitrate, t.created_at
		 FROM playlist_tracks pt
		 JOIN tracks t ON pt.track_id = t.id
		 WHERE pt.playlist_id = ?
		 ORDER BY pt.position`,
		playlistID,
	)
	if err != nil {
		return []*PlaylistTrack{}, err
	}
	defer rows.Close()

	var pts []*PlaylistTrack
	for rows.Next() {
		var pt PlaylistTrack
		pt.Track = &Track{}
		if err := rows.Scan(
			&pt.ID, &pt.PlaylistID, &pt.TrackID, &pt.Position, &pt.CreatedAt,
			&pt.Track.ID, &pt.Track.AlbumID, &pt.Track.ArtistID, &pt.Track.Title,
			&pt.Track.Path, &pt.Track.TrackNum, &pt.Track.Duration, &pt.Track.Format,
			&pt.Track.Bitrate, &pt.Track.CreatedAt,
		); err != nil {
			return []*PlaylistTrack{}, err
		}
		pts = append(pts, &pt)
	}
	if pts == nil {
		pts = []*PlaylistTrack{}
	}
	return pts, rows.Err()
}

// GetStats returns library statistics
func (db *DB) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	var count int64
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM artists").Scan(&count); err == nil {
		stats["artists"] = count
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM albums").Scan(&count); err == nil {
		stats["albums"] = count
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&count); err == nil {
		stats["tracks"] = count
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM playlists").Scan(&count); err == nil {
		stats["playlists"] = count
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM smart_playlists").Scan(&count); err == nil {
		stats["smart_playlists"] = count
	}

	return stats, nil
}

// --- User Methods ---

// CreateUser creates a new user
func (db *DB) CreateUser(username, apiKey string) (*User, error) {
	res, err := db.conn.Exec("INSERT INTO users (username, api_key) VALUES (?, ?)", username, apiKey)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetUserByID(id)
}

// GetUserByID retrieves a user by ID
func (db *DB) GetUserByID(id int64) (*User, error) {
	var u User
	err := db.conn.QueryRow(
		"SELECT id, username, api_key, lastfm_username, lastfm_session_key, lastfm_api_key, quota_bytes, quota_used_bytes, created_at FROM users WHERE id = ?", id).Scan(
		&u.ID, &u.Username, &u.APIKey, &u.LastFMUsername, &u.LastFMSessionKey, &u.LastFMAPIKey, &u.QuotaBytes, &u.QuotaUsedBytes, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByAPIKey retrieves a user by API key
func (db *DB) GetUserByAPIKey(apiKey string) (*User, error) {
	var u User
	err := db.conn.QueryRow(
		"SELECT id, username, api_key, lastfm_username, lastfm_session_key, lastfm_api_key, quota_bytes, quota_used_bytes, created_at FROM users WHERE api_key = ?", apiKey).Scan(
		&u.ID, &u.Username, &u.APIKey, &u.LastFMUsername, &u.LastFMSessionKey, &u.LastFMAPIKey, &u.QuotaBytes, &u.QuotaUsedBytes, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByUsername retrieves a user by username
func (db *DB) GetUserByUsername(username string) (*User, error) {
	var u User
	err := db.conn.QueryRow(
		"SELECT id, username, api_key, lastfm_username, lastfm_session_key, lastfm_api_key, quota_bytes, quota_used_bytes, created_at FROM users WHERE username = ?", username).Scan(
		&u.ID, &u.Username, &u.APIKey, &u.LastFMUsername, &u.LastFMSessionKey, &u.LastFMAPIKey, &u.QuotaBytes, &u.QuotaUsedBytes, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUsersCount returns the number of users
func (db *DB) GetUsersCount() (int64, error) {
	var count int64
	err := db.conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// --- Star Methods ---

// ToggleStar stars or unstars an item
func (db *DB) ToggleStar(userID, itemID int64, itemType string) (bool, error) {
	// Check if already starred
	var id int64
	err := db.conn.QueryRow(
		"SELECT id FROM stars WHERE user_id = ? AND item_id = ? AND item_type = ?",
		userID, itemID, itemType,
	).Scan(&id)

	if err == nil {
		// Unstar
		_, err := db.conn.Exec(
			"DELETE FROM stars WHERE user_id = ? AND item_id = ? AND item_type = ?",
			userID, itemID, itemType,
		)
		return false, err
	}

	if err != sql.ErrNoRows {
		return false, err
	}

	// Star
	_, err = db.conn.Exec(
		"INSERT INTO stars (user_id, item_id, item_type) VALUES (?, ?, ?)",
		userID, itemID, itemType,
	)
	return true, err
}

// GetStars returns starred items for a user by type
func (db *DB) GetStars(userID int64, itemType string) ([]*Star, error) {
	rows, err := db.conn.Query(
		"SELECT id, user_id, item_id, item_type, created_at FROM stars WHERE user_id = ? AND item_type = ? ORDER BY created_at DESC",
		userID, itemType,
	)
	if err != nil {
		return []*Star{}, err
	}
	defer rows.Close()

	var stars []*Star
	for rows.Next() {
		var s Star
		if err := rows.Scan(&s.ID, &s.UserID, &s.ItemID, &s.ItemType, &s.CreatedAt); err != nil {
			return []*Star{}, err
		}
		stars = append(stars, &s)
	}
	if stars == nil {
		stars = []*Star{}
	}
	return stars, rows.Err()
}

// GetAllStars returns all starred items for a user
func (db *DB) GetAllStars(userID int64) (map[string][]*Star, error) {
	result := make(map[string][]*Star)
	for _, t := range []string{"artist", "album", "track"} {
		s, err := db.GetStars(userID, t)
		if err != nil {
			return nil, err
		}
		result[t] = s
	}
	return result, nil
}

// --- Play History / Scrobble ---

// RecordScrobble records a play event
func (db *DB) RecordScrobble(userID, trackID int64) error {
	_, err := db.conn.Exec(
		"INSERT INTO play_history (user_id, track_id) VALUES (?, ?)",
		userID, trackID,
	)
	return err
}

// GetLyrics retrieves lyrics for a track
func (db *DB) GetLyrics(trackID int64) (*Lyrics, error) {
	var l Lyrics
	err := db.conn.QueryRow(
		"SELECT track_id, content, synced, source, updated_at FROM lyrics WHERE track_id = ?",
		trackID,
	).Scan(&l.TrackID, &l.Content, &l.Synced, &l.Source, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// SaveLyrics saves or updates lyrics for a track
func (db *DB) SaveLyrics(trackID int64, content, source string, synced bool) error {
	_, err := db.conn.Exec(
		`INSERT INTO lyrics (track_id, content, source, synced, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(track_id) DO UPDATE SET
			content=excluded.content,
			source=excluded.source,
			synced=excluded.synced,
			updated_at=CURRENT_TIMESTAMP`,
		trackID, content, source, synced,
	)
	return err
}

// IncrementPlayCount increments the play count for a track
func (db *DB) IncrementPlayCount(trackID int64) error {
	_, err := db.conn.Exec(
		"UPDATE tracks SET play_count = play_count + 1 WHERE id = ?",
		trackID,
	)
	return err
}

// CreateSmartPlaylist creates a new smart playlist
func (db *DB) CreateSmartPlaylist(userID int64, name, ruleType, ruleValue string, limitNum int) (*SmartPlaylist, error) {
	res, err := db.conn.Exec(
		"INSERT INTO smart_playlists (user_id, name, rule_type, rule_value, limit_num) VALUES (?, ?, ?, ?, ?)",
		userID, name, ruleType, ruleValue, limitNum,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.GetSmartPlaylistByID(id)
}

// GetSmartPlaylistByID retrieves a smart playlist by ID
func (db *DB) GetSmartPlaylistByID(id int64) (*SmartPlaylist, error) {
	var sp SmartPlaylist
	err := db.conn.QueryRow(
		"SELECT id, user_id, name, rule_type, rule_value, limit_num, created_at, updated_at FROM smart_playlists WHERE id = ?",
		id,
	).Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.RuleType, &sp.RuleValue, &sp.LimitNum, &sp.CreatedAt, &sp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// GetAllSmartPlaylists returns all smart playlists for a user (or all if userID == 0)
func (db *DB) GetAllSmartPlaylists(userID int64) ([]*SmartPlaylist, error) {
	var rows *sql.Rows
	var err error
	if userID > 0 {
		rows, err = db.conn.Query("SELECT id, user_id, name, rule_type, rule_value, limit_num, created_at, updated_at FROM smart_playlists WHERE user_id = ? ORDER BY name", userID)
	} else {
		rows, err = db.conn.Query("SELECT id, user_id, name, rule_type, rule_value, limit_num, created_at, updated_at FROM smart_playlists ORDER BY name")
	}
	if err != nil {
		return []*SmartPlaylist{}, err
	}
	defer rows.Close()

	var playlists []*SmartPlaylist
	for rows.Next() {
		var sp SmartPlaylist
		if err := rows.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.RuleType, &sp.RuleValue, &sp.LimitNum, &sp.CreatedAt, &sp.UpdatedAt); err != nil {
			return []*SmartPlaylist{}, err
		}
		playlists = append(playlists, &sp)
	}
	if playlists == nil {
		playlists = []*SmartPlaylist{}
	}
	return playlists, rows.Err()
}

// UpdateSmartPlaylist updates a smart playlist
func (db *DB) UpdateSmartPlaylist(id int64, name, ruleType, ruleValue string, limitNum int) (*SmartPlaylist, error) {
	_, err := db.conn.Exec(
		"UPDATE smart_playlists SET name = ?, rule_type = ?, rule_value = ?, limit_num = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		name, ruleType, ruleValue, limitNum, id,
	)
	if err != nil {
		return nil, err
	}
	return db.GetSmartPlaylistByID(id)
}

// DeleteSmartPlaylist deletes a smart playlist
func (db *DB) DeleteSmartPlaylist(id int64) error {
	_, err := db.conn.Exec("DELETE FROM smart_playlists WHERE id = ?", id)
	return err
}

// GetSmartPlaylistTracks returns tracks matching a smart playlist's rules
func (db *DB) GetSmartPlaylistTracks(sp *SmartPlaylist) ([]*Track, error) {
	var query string
	var args []interface{}

	limit := sp.LimitNum
	if limit <= 0 {
		limit = 50
	}

	switch sp.RuleType {
	case "recently_added":
		query = "SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)
	case "most_played":
		query = "SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks ORDER BY play_count DESC, title LIMIT ?"
		args = append(args, limit)
	case "genre":
		query = "SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks WHERE title LIKE ? ORDER BY RANDOM() LIMIT ?"
		args = append(args, "%"+sp.RuleValue+"%", limit)
	case "year":
		query = `SELECT t.id, t.album_id, t.artist_id, t.title, t.path, t.track_num, t.duration, t.format, t.bitrate, t.created_at
				 FROM tracks t
				 JOIN albums a ON t.album_id = a.id
				 WHERE a.year = ?
				 ORDER BY t.title LIMIT ?`
		args = append(args, sp.RuleValue, limit)
	default:
		query = "SELECT id, album_id, artist_id, title, path, track_num, duration, format, bitrate, created_at FROM tracks ORDER BY RANDOM() LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return []*Track{}, err
	}
	defer rows.Close()

	var tracks []*Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.AlbumID, &t.ArtistID, &t.Title, &t.Path,
			&t.TrackNum, &t.Duration, &t.Format, &t.Bitrate, &t.CreatedAt); err != nil {
			return []*Track{}, err
		}
		tracks = append(tracks, &t)
	}
	if tracks == nil {
		tracks = []*Track{}
	}
	return tracks, rows.Err()
}

// UpdateUserLastFM updates a user's Last.fm credentials
func (db *DB) UpdateUserLastFM(userID int64, username, sessionKey string) error {
	_, err := db.conn.Exec(
		"UPDATE users SET lastfm_username = ?, lastfm_session_key = ? WHERE id = ?",
		username, sessionKey, userID,
	)
	return err
}

// UpdateUserQuota updates a user's storage quota
func (db *DB) UpdateUserQuota(userID int64, quotaBytes int64) error {
	_, err := db.conn.Exec(
		"UPDATE users SET quota_bytes = ? WHERE id = ?",
		quotaBytes, userID,
	)
	return err
}

// UpdateUserQuotaUsed updates a user's used storage quota
func (db *DB) UpdateUserQuotaUsed(userID int64, usedBytes int64) error {
	_, err := db.conn.Exec(
		"UPDATE users SET quota_used_bytes = ? WHERE id = ?",
		usedBytes, userID,
	)
	return err
}

// --- Album Art ---

// UpdateAlbumCoverArt updates the cover art for an album
func (db *DB) UpdateAlbumCoverArt(albumID int64, data []byte) error {
	_, err := db.conn.Exec("UPDATE albums SET cover_art = ? WHERE id = ?", data, albumID)
	return err
}

// GetAlbumArt retrieves cover art for an album
func (db *DB) GetAlbumArt(albumID int64) ([]byte, error) {
	var data []byte
	err := db.conn.QueryRow("SELECT cover_art FROM albums WHERE id = ?", albumID).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// --- Subsonic Helpers ---

// GetAlbumsSorted returns albums sorted by various criteria
func (db *DB) GetAlbumsSorted(sortType string, limit, offset int) ([]*AlbumWithArtist, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var orderBy string
	switch sortType {
	case "newest":
		orderBy = "a.created_at DESC"
	case "alphabeticalByArtist":
		orderBy = "ar.name COLLATE NOCASE, a.title COLLATE NOCASE"
	case "alphabeticalByName":
		orderBy = "a.title COLLATE NOCASE"
	case "byYear":
		orderBy = "a.year DESC, a.title COLLATE NOCASE"
	default:
		orderBy = "a.created_at DESC"
	}

	query := fmt.Sprintf(
		`SELECT a.id, a.artist_id, a.title, a.year, a.created_at, ar.name
		 FROM albums a
		 JOIN artists ar ON a.artist_id = ar.id
		 ORDER BY %s
		 LIMIT ? OFFSET ?`,
		orderBy,
	)

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return []*AlbumWithArtist{}, err
	}
	defer rows.Close()

	var albums []*AlbumWithArtist
	for rows.Next() {
		var a AlbumWithArtist
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.Title, &a.Year, &a.CreatedAt, &a.ArtistName); err != nil {
			return []*AlbumWithArtist{}, err
		}
		albums = append(albums, &a)
	}
	if albums == nil {
		albums = []*AlbumWithArtist{}
	}
	return albums, rows.Err()
}
