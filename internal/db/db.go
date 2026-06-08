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
	return nil
}

func (d *DB) migrateAddColumn(table, col, def string) error {
	_, err := d.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, def))
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}
