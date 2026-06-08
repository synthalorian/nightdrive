package db

import (
	"database/sql"
	"fmt"
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
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS playlists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
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

CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id);
CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS idx_tracks_path ON tracks(path);
CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id);
`
	if _, err := d.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
