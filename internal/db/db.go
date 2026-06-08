package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection.
type DB struct {
	*sql.DB
}

// Open opens the SQLite database.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Hour)
	return &DB{db}, nil
}

// Migrate creates the schema.
func (d *DB) Migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS artists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS albums (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	artist_id INTEGER REFERENCES artists(id),
	title TEXT NOT NULL,
	year INTEGER,
	cover_art TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(artist_id, title)
);

CREATE TABLE IF NOT EXISTS tracks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	album_id INTEGER REFERENCES albums(id),
	artist_id INTEGER REFERENCES artists(id),
	title TEXT NOT NULL,
	disc_number INTEGER DEFAULT 1,
	track_number INTEGER DEFAULT 0,
	duration REAL DEFAULT 0,
	bitrate INTEGER DEFAULT 0,
	format TEXT,
	path TEXT NOT NULL UNIQUE,
	mtime INTEGER,
	size INTEGER,
	lyrics TEXT,
	media_type TEXT DEFAULT 'music',
	playback_position REAL DEFAULT 0,
	genre TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playlists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	visibility TEXT DEFAULT 'private',
	smart_query TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playlist_tracks (
	playlist_id INTEGER REFERENCES playlists(id) ON DELETE CASCADE,
	track_id INTEGER REFERENCES tracks(id) ON DELETE CASCADE,
	position INTEGER NOT NULL,
	PRIMARY KEY (playlist_id, track_id)
);

CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'user',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token TEXT NOT NULL UNIQUE,
	expires_at DATETIME NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playlist_shares (
	playlist_id INTEGER REFERENCES playlists(id) ON DELETE CASCADE,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (playlist_id, user_id)
);

CREATE TABLE IF NOT EXISTS plays (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
	played_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	duration_listened REAL DEFAULT 0,
	completion_pct REAL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path);
CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_plays_user ON plays(user_id);
CREATE INDEX IF NOT EXISTS idx_plays_track ON plays(track_id);
CREATE INDEX IF NOT EXISTS idx_plays_played_at ON plays(played_at);
`
	if _, err := d.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	_ = d.migrateAddColumn("tracks", "lyrics", "TEXT")
	_ = d.migrateAddColumn("tracks", "media_type", "TEXT DEFAULT 'music'")
	_ = d.migrateAddColumn("tracks", "playback_position", "REAL DEFAULT 0")
	_ = d.migrateAddColumn("tracks", "genre", "TEXT")
	_ = d.migrateAddColumn("playlists", "owner_id", "INTEGER REFERENCES users(id) ON DELETE SET NULL")
	_ = d.migrateAddColumn("playlists", "visibility", "TEXT DEFAULT 'private'")

	// v0.6.0: Advanced playback schema
	v06Schema := `
CREATE TABLE IF NOT EXISTS playback_sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	token TEXT NOT NULL UNIQUE,
	device_name TEXT,
	current_track_id INTEGER REFERENCES tracks(id),
	position REAL DEFAULT 0,
	state TEXT DEFAULT 'stopped',
	volume REAL DEFAULT 1.0,
	shuffle INTEGER DEFAULT 0,
	repeat_mode TEXT DEFAULT 'none',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playback_queue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
	track_id INTEGER NOT NULL REFERENCES tracks(id),
	position INTEGER NOT NULL,
	UNIQUE(session_id, position)
);

CREATE TABLE IF NOT EXISTS sync_rooms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	token TEXT NOT NULL UNIQUE,
	current_track_id INTEGER REFERENCES tracks(id),
	position REAL DEFAULT 0,
	state TEXT DEFAULT 'stopped',
	master_session_id INTEGER REFERENCES playback_sessions(id),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sync_room_members (
	room_id INTEGER REFERENCES sync_rooms(id) ON DELETE CASCADE,
	session_id INTEGER REFERENCES playback_sessions(id) ON DELETE CASCADE,
	joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (room_id, session_id)
);

CREATE TABLE IF NOT EXISTS cast_devices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	host TEXT,
	port INTEGER,
	protocol TEXT,
	capabilities TEXT,
	is_active INTEGER DEFAULT 0,
	last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dsp_presets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	eq_low REAL DEFAULT 0,
	eq_mid REAL DEFAULT 0,
	eq_high REAL DEFAULT 0,
	compressor_threshold REAL DEFAULT -20,
	compressor_ratio REAL DEFAULT 4,
	loudness_target REAL DEFAULT -14,
	is_default INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gapless_reports (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	album_id INTEGER REFERENCES albums(id) ON DELETE CASCADE,
	track_id INTEGER REFERENCES tracks(id),
	next_track_id INTEGER REFERENCES tracks(id),
	gap_detected INTEGER DEFAULT 0,
	gap_duration_ms INTEGER DEFAULT 0,
	analyzed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_playback_sessions_token ON playback_sessions(token);
CREATE INDEX IF NOT EXISTS idx_playback_queue_session ON playback_queue(session_id);
CREATE INDEX IF NOT EXISTS idx_sync_rooms_token ON sync_rooms(token);
	CREATE INDEX IF NOT EXISTS idx_cast_devices_type ON cast_devices(type);
	`
	if _, err := d.Exec(v06Schema); err != nil {
		return fmt.Errorf("migrate v0.6.0: %w", err)
	}

	// v0.8.0: Feedback and crash reporting schema
	v08Schema := `
	CREATE TABLE IF NOT EXISTS feedback (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
		type TEXT NOT NULL DEFAULT 'general',
		message TEXT NOT NULL,
		rating INTEGER DEFAULT 0,
		metadata TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_feedback_type ON feedback(type);
	CREATE INDEX IF NOT EXISTS idx_feedback_created ON feedback(created_at);
	`
	if _, err := d.Exec(v08Schema); err != nil {
		return fmt.Errorf("migrate v0.8.0: %w", err)
	}
	return nil
}

func (d *DB) migrateAddColumn(table, col, def string) error {
	_, err := d.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, def))
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}
