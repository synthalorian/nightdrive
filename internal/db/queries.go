package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// InsertArtist inserts or returns an existing artist.
func (d *DB) InsertArtist(name string) (int64, error) {
	if name == "" {
		name = "Unknown Artist"
	}
	res, err := d.Exec("INSERT OR IGNORE INTO artists(name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	if id, _ := res.LastInsertId(); id != 0 {
		return id, nil
	}
	var id int64
	err = d.QueryRow("SELECT id FROM artists WHERE name = ?", name).Scan(&id)
	return id, err
}

// InsertAlbum inserts or returns an existing album.
func (d *DB) InsertAlbum(artistID int64, title string, year int, cover string) (int64, error) {
	if title == "" {
		title = "Unknown Album"
	}
	res, err := d.Exec(
		"INSERT OR IGNORE INTO albums(artist_id, title, year, cover_art) VALUES (?,?,?,?)",
		artistID, title, year, cover)
	if err != nil {
		return 0, err
	}
	if id, _ := res.LastInsertId(); id != 0 {
		return id, nil
	}
	var id int64
	err = d.QueryRow("SELECT id FROM albums WHERE artist_id = ? AND title = ?", artistID, title).Scan(&id)
	return id, err
}

// InsertTrack inserts or updates a track.
func (d *DB) InsertTrack(t *Track) (int64, error) {
	res, err := d.Exec(
		`INSERT INTO tracks(album_id, artist_id, title, disc_number, track_number, duration, bitrate, format, path, mtime, size, lyrics, media_type, playback_position, genre)
		 VALUES (:aid, :arid, :title, :disc, :track, :dur, :bit, :fmt, :path, :mtime, :size, :lyrics, :mediaType, :pos, :genre)
		 ON CONFLICT(path) DO UPDATE SET
		   album_id=excluded.album_id, artist_id=excluded.artist_id, title=excluded.title,
		   disc_number=excluded.disc_number, track_number=excluded.track_number,
		   duration=excluded.duration, bitrate=excluded.bitrate, format=excluded.format,
		   mtime=excluded.mtime, size=excluded.size, lyrics=excluded.lyrics, media_type=excluded.media_type,
		   playback_position=excluded.playback_position, genre=excluded.genre`,
		sql.Named("aid", t.AlbumID),
		sql.Named("arid", t.ArtistID),
		sql.Named("title", t.Title),
		sql.Named("disc", t.DiscNumber),
		sql.Named("track", t.TrackNumber),
		sql.Named("dur", t.Duration),
		sql.Named("bit", t.Bitrate),
		sql.Named("fmt", t.Format),
		sql.Named("path", t.Path),
		sql.Named("mtime", t.Mtime),
		sql.Named("size", t.Size),
		sql.Named("lyrics", t.Lyrics),
		sql.Named("mediaType", t.MediaType),
		sql.Named("pos", t.PlaybackPosition),
		sql.Named("genre", t.Genre),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// TrackByID returns a single track by ID.
func (d *DB) TrackByID(id int64) (*Track, error) {
	row := d.QueryRow(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.id = ?`, id)
	return scanTrack(row)
}

// Tracks returns a paginated list of tracks.
func (d *DB) Tracks(offset, limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		ORDER BY a.name, al.title, t.disc_number, t.track_number
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// SearchTracks searches tracks, artists, and albums.
func (d *DB) SearchTracks(q string, offset, limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	pattern := "%" + strings.ReplaceAll(q, "%", "\\%") + "%"
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.title LIKE ? ESCAPE '\' OR a.name LIKE ? ESCAPE '\' OR al.title LIKE ? ESCAPE '\'
		ORDER BY a.name, al.title, t.disc_number, t.track_number
		LIMIT ? OFFSET ?`, pattern, pattern, pattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// Albums returns all albums.
func (d *DB) Albums() ([]Album, error) {
	rows, err := d.Query(`
		SELECT al.id, al.artist_id, al.title, al.year, al.cover_art
		FROM albums al
		ORDER BY al.title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var a Album
		var cover sql.NullString
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.Title, &a.Year, &cover); err != nil {
			return nil, err
		}
		if cover.Valid {
			a.CoverArt = &cover.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Artists returns all artists.
func (d *DB) Artists() ([]Artist, error) {
	rows, err := d.Query(`SELECT id, name, created_at FROM artists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artist
	for rows.Next() {
		var a Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AlbumTracks returns tracks for an album.
func (d *DB) AlbumTracks(albumID int64) ([]Track, error) {
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.album_id = ?
		ORDER BY t.disc_number, t.track_number`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func scanTrack(scanner interface{ Scan(...interface{}) error }) (*Track, error) {
	var t Track
	var albumID, artistID sql.NullInt64
	var lyrics sql.NullString
	var mediaType sql.NullString
	var genre sql.NullString
	err := scanner.Scan(
		&t.ID, &albumID, &artistID, &t.Title,
		&t.ArtistName, &t.AlbumTitle,
		&t.DiscNumber, &t.TrackNumber, &t.Duration, &t.Bitrate, &t.Format,
		&t.Path, &t.Mtime, &t.Size,
		&lyrics, &mediaType, &t.PlaybackPosition, &genre,
	)
	if err != nil {
		return nil, err
	}
	if albumID.Valid {
		v := albumID.Int64
		t.AlbumID = &v
	}
	if artistID.Valid {
		v := artistID.Int64
		t.ArtistID = &v
	}
	if lyrics.Valid {
		t.Lyrics = lyrics.String
	}
	if mediaType.Valid {
		t.MediaType = mediaType.String
	}
	if genre.Valid {
		t.Genre = genre.String
	}
	return &t, nil
}

func scanTracks(rows *sql.Rows) ([]Track, error) {
	var out []Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// SmartTracks evaluates a simple smart query.
// Supported: artist:foo, album:foo, title:foo, year:YYYY, minDuration:SS, maxDuration:SS
func (d *DB) SmartTracks(query string) ([]Track, error) {
	parts := strings.Fields(query)
	where := []string{"1=1"}
	args := []interface{}{}
	for _, p := range parts {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := kv[0], kv[1]
		switch k {
		case "artist":
			where = append(where, "a.name LIKE ?")
			args = append(args, "%"+v+"%")
		case "album":
			where = append(where, "al.title LIKE ?")
			args = append(args, "%"+v+"%")
		case "title":
			where = append(where, "t.title LIKE ?")
			args = append(args, "%"+v+"%")
		case "year":
			where = append(where, "al.year = ?")
			args = append(args, v)
		case "minDuration":
			where = append(where, "t.duration >= ?")
			args = append(args, v)
		case "maxDuration":
			where = append(where, "t.duration <= ?")
			args = append(args, v)
		}
	}
	q := fmt.Sprintf(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE %s
		ORDER BY a.name, al.title, t.disc_number, t.track_number`, strings.Join(where, " AND "))
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// Playlists returns all playlists.
func (d *DB) Playlists() ([]Playlist, error) {
	rows, err := d.Query(`SELECT id, name, smart_query, created_at, updated_at FROM playlists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Playlist
	for rows.Next() {
		var p Playlist
		var sq sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &sq, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if sq.Valid {
			p.SmartQuery = &sq.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InsertPlaylist creates a playlist.
func (d *DB) InsertPlaylist(name, smartQuery string) (int64, error) {
	var sq interface{}
	if smartQuery != "" {
		sq = smartQuery
	} else {
		sq = nil
	}
	res, err := d.Exec(`INSERT INTO playlists(name, smart_query) VALUES (?,?)`, name, sq)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PlaylistTracks returns tracks for a playlist (smart or static).
func (d *DB) PlaylistTracks(pid int64) ([]Track, error) {
	var sq sql.NullString
	if err := d.QueryRow(`SELECT smart_query FROM playlists WHERE id = ?`, pid).Scan(&sq); err != nil {
		return nil, err
	}
	if sq.Valid && sq.String != "" {
		return d.SmartTracks(sq.String)
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM playlist_tracks pt
		JOIN tracks t ON t.id = pt.track_id
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE pt.playlist_id = ?
		ORDER BY pt.position`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

// AddPlaylistTrack appends a track to a static playlist.
func (d *DB) AddPlaylistTrack(pid, tid int64) error {
	var pos int
	d.QueryRow(`SELECT COALESCE(MAX(position),0)+1 FROM playlist_tracks WHERE playlist_id = ?`, pid).Scan(&pos)
	_, err := d.Exec(`INSERT OR IGNORE INTO playlist_tracks(playlist_id, track_id, position) VALUES (?,?,?)`, pid, tid, pos)
	return err
}

// RemovePlaylistTrack removes a track from a static playlist.
func (d *DB) RemovePlaylistTrack(pid, tid int64) error {
	_, err := d.Exec(`DELETE FROM playlist_tracks WHERE playlist_id = ? AND track_id = ?`, pid, tid)
	return err
}

// DeletePlaylist removes a playlist.
func (d *DB) DeletePlaylist(pid int64) error {
	_, err := d.Exec(`DELETE FROM playlists WHERE id = ?`, pid)
	return err
}

// UpdateTrackPosition saves the playback position for a track.
func (d *DB) UpdateTrackPosition(id int64, pos float64) error {
	_, err := d.Exec(`UPDATE tracks SET playback_position = ? WHERE id = ?`, pos, id)
	return err
}

// TrackLyrics returns lyrics for a track.
func (d *DB) TrackLyrics(id int64) (string, error) {
	var lyrics sql.NullString
	err := d.QueryRow(`SELECT lyrics FROM tracks WHERE id = ?`, id).Scan(&lyrics)
	if err != nil {
		return "", err
	}
	if lyrics.Valid {
		return lyrics.String, nil
	}
	return "", nil
}

// UpdateTrackLyrics saves lyrics for a track.
func (d *DB) UpdateTrackLyrics(id int64, lyrics string) error {
	_, err := d.Exec(`UPDATE tracks SET lyrics = ? WHERE id = ?`, lyrics, id)
	return err
}

// TracksByMediaType returns tracks filtered by media type.
func (d *DB) TracksByMediaType(mediaType string, offset, limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.media_type = ?
		ORDER BY a.name, al.title, t.disc_number, t.track_number
		LIMIT ? OFFSET ?`, mediaType, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (d *DB) InsertUser(username, passwordHash, role string) (int64, error) {
	res, err := d.Exec("INSERT INTO users(username, password_hash, role) VALUES (?,?,?)", username, passwordHash, role)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UserByUsername(username string) (*User, error) {
	var u User
	err := d.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?", username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) UserByID(id int64) (*User, error) {
	var u User
	err := d.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?", id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) UpdateUserPassword(id int64, hash string) error {
	_, err := d.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	return err
}

func (d *DB) DeleteUser(id int64) error {
	_, err := d.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (d *DB) Users() ([]User, error) {
	rows, err := d.Query("SELECT id, username, password_hash, role, created_at FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (d *DB) CreateSession(userID int64, token string, expiresAt time.Time) (int64, error) {
	res, err := d.Exec("INSERT INTO sessions(user_id, token, expires_at) VALUES (?,?,?)", userID, token, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) SessionByToken(token string) (*Session, error) {
	var s Session
	err := d.QueryRow("SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE token = ? AND expires_at > datetime('now')", token).Scan(
		&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) DeleteSession(token string) error {
	_, err := d.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func (d *DB) CleanExpiredSessions() error {
	_, err := d.Exec("DELETE FROM sessions WHERE expires_at <= datetime('now')")
	return err
}

func (d *DB) PlaylistsForUser(userID int64) ([]Playlist, error) {
	rows, err := d.Query(`
		SELECT p.id, p.name, p.owner_id, p.visibility, p.smart_query, p.created_at, p.updated_at
		FROM playlists p
		LEFT JOIN playlist_shares ps ON ps.playlist_id = p.id
		WHERE p.owner_id = ? OR p.visibility = 'public' OR (p.visibility = 'shared' AND ps.user_id = ?)
		GROUP BY p.id
		ORDER BY p.name`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaylists(rows)
}

func (d *DB) InsertPlaylistWithOwner(name, smartQuery string, ownerID int64, visibility string) (int64, error) {
	var sq interface{}
	if smartQuery != "" {
		sq = smartQuery
	}
	res, err := d.Exec(`INSERT INTO playlists(name, owner_id, visibility, smart_query) VALUES (?,?,?,?)`, name, ownerID, visibility, sq)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdatePlaylistVisibility(pid int64, visibility string) error {
	_, err := d.Exec("UPDATE playlists SET visibility = ? WHERE id = ?", visibility, pid)
	return err
}

func (d *DB) PlaylistOwner(pid int64) (int64, error) {
	var ownerID sql.NullInt64
	err := d.QueryRow("SELECT owner_id FROM playlists WHERE id = ?", pid).Scan(&ownerID)
	if err != nil {
		return 0, err
	}
	if ownerID.Valid {
		return ownerID.Int64, nil
	}
	return 0, nil
}

func (d *DB) SharePlaylist(pid, userID int64) error {
	_, err := d.Exec("INSERT OR IGNORE INTO playlist_shares(playlist_id, user_id) VALUES (?,?)", pid, userID)
	return err
}

func (d *DB) UnsharePlaylist(pid, userID int64) error {
	_, err := d.Exec("DELETE FROM playlist_shares WHERE playlist_id = ? AND user_id = ?", pid, userID)
	return err
}

func scanPlaylists(rows *sql.Rows) ([]Playlist, error) {
	var out []Playlist
	for rows.Next() {
		var p Playlist
		var sq sql.NullString
		var ownerID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &ownerID, &p.Visibility, &sq, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if sq.Valid {
			p.SmartQuery = &sq.String
		}
		if ownerID.Valid {
			p.OwnerID = &ownerID.Int64
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) InsertPlay(userID, trackID int64, durationListened, completionPct float64) (int64, error) {
	res, err := d.Exec(
		"INSERT INTO plays(user_id, track_id, duration_listened, completion_pct) VALUES (?,?,?,?)",
		userID, trackID, durationListened, completionPct)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) PlaysByUser(userID int64, offset, limit int) ([]Play, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(
		"SELECT id, user_id, track_id, played_at, duration_listened, completion_pct FROM plays WHERE user_id = ? ORDER BY played_at DESC LIMIT ? OFFSET ?",
		userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Play
	for rows.Next() {
		var p Play
		if err := rows.Scan(&p.ID, &p.UserID, &p.TrackID, &p.PlayedAt, &p.DurationListened, &p.CompletionPct); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) StatsForUser(userID int64) (*Stats, error) {
	var s Stats
	err := d.QueryRow("SELECT COUNT(*), COALESCE(SUM(duration_listened),0), COUNT(DISTINCT track_id) FROM plays WHERE user_id = ?", userID).Scan(
		&s.TotalPlays, &s.TotalDuration, &s.UniqueTracks)
	if err != nil {
		return nil, err
	}
	err = d.QueryRow(`
		SELECT COUNT(DISTINCT t.artist_id) FROM plays p
		JOIN tracks t ON t.id = p.track_id WHERE p.user_id = ?`, userID).Scan(&s.UniqueArtists)
	if err != nil {
		return nil, err
	}
	trackRows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM plays p
		JOIN tracks t ON t.id = p.track_id
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE p.user_id = ?
		GROUP BY t.id
		ORDER BY COUNT(*) DESC
		LIMIT 10`, userID)
	if err != nil {
		return nil, err
	}
	defer trackRows.Close()
	s.TopTracks, err = scanTracks(trackRows)
	if err != nil {
		return nil, err
	}
	artRows, err := d.Query(`
		SELECT a.id, a.name, a.created_at, COUNT(*) as cnt
		FROM plays p
		JOIN tracks t ON t.id = p.track_id
		JOIN artists a ON a.id = t.artist_id
		WHERE p.user_id = ?
		GROUP BY a.id
		ORDER BY cnt DESC
		LIMIT 10`, userID)
	if err != nil {
		return nil, err
	}
	defer artRows.Close()
	for artRows.Next() {
		var ac ArtistCount
		if err := artRows.Scan(&ac.Artist.ID, &ac.Artist.Name, &ac.Artist.CreatedAt, &ac.Count); err != nil {
			return nil, err
		}
		s.TopArtists = append(s.TopArtists, ac)
	}
	return &s, nil
}

func (d *DB) SimilarAlbums(artistID int64, excludeAlbumID int64, limit int) ([]Album, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := d.Query(`
		SELECT id, artist_id, title, year, cover_art FROM albums
		WHERE artist_id = ? AND id != ?
		ORDER BY year DESC
		LIMIT ?`, artistID, excludeAlbumID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Album
	for rows.Next() {
		var a Album
		var cover sql.NullString
		if err := rows.Scan(&a.ID, &a.ArtistID, &a.Title, &a.Year, &cover); err != nil {
			return nil, err
		}
		if cover.Valid {
			a.CoverArt = &cover.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) SimilarArtists(artistID int64, limit int) ([]Artist, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := d.Query(`
		SELECT DISTINCT a.id, a.name, a.created_at FROM artists a
		JOIN tracks t ON t.artist_id = a.id
		JOIN playlist_tracks pt ON pt.track_id = t.id
		WHERE pt.playlist_id IN (
			SELECT DISTINCT playlist_id FROM playlist_tracks pt2
			JOIN tracks t2 ON t2.id = pt2.track_id
			WHERE t2.artist_id = ?
		) AND a.id != ?
		LIMIT ?`, artistID, artistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Artist
	for rows.Next() {
		var a Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) RecommendedTracks(userID int64, limit int) ([]Track, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM plays p
		JOIN tracks t ON t.id = p.track_id
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE p.user_id IN (
			SELECT DISTINCT p2.user_id FROM plays p2
			WHERE p2.track_id IN (SELECT track_id FROM plays WHERE user_id = ?)
			AND p2.user_id != ?
		)
		AND t.id NOT IN (SELECT track_id FROM plays WHERE user_id = ?)
		GROUP BY t.id
		ORDER BY COUNT(*) DESC
		LIMIT ?`, userID, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (d *DB) RadioTracksByGenre(genre string, limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.genre = ?
		ORDER BY RANDOM()
		LIMIT ?`, genre, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (d *DB) RadioTracksByArtist(artistID int64, limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		WHERE t.artist_id = ?
		ORDER BY RANDOM()
		LIMIT ?`, artistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (d *DB) RadioTracksRandom(limit int) ([]Track, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT t.id, t.album_id, t.artist_id, t.title,
		       COALESCE(a.name,'') AS artist_name,
		       COALESCE(al.title,'') AS album_title,
		       t.disc_number, t.track_number, t.duration, t.bitrate, t.format, t.path, t.mtime, t.size,
		       t.lyrics, t.media_type, t.playback_position, t.genre
		FROM tracks t
		LEFT JOIN artists a ON a.id = t.artist_id
		LEFT JOIN albums al ON al.id = t.album_id
		ORDER BY RANDOM()
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (d *DB) Genres() ([]string, error) {
	rows, err := d.Query("SELECT DISTINCT genre FROM tracks WHERE genre IS NOT NULL AND genre != '' ORDER BY genre")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type NullInt64 struct {
	Int64 int64
	Valid bool
}

func (n *NullInt64) Scan(value interface{}) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case int64:
		n.Int64 = v
		n.Valid = true
		return nil
	default:
		return fmt.Errorf("cannot scan %T into NullInt64", value)
	}
}

type PlaybackSession struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"userId"`
	Token          string    `json:"token"`
	DeviceName     string    `json:"deviceName"`
	CurrentTrackID *int64    `json:"currentTrackId,omitempty"`
	Position       float64   `json:"position"`
	State          string    `json:"state"`
	Volume         float64   `json:"volume"`
	Shuffle        bool      `json:"shuffle"`
	RepeatMode     string    `json:"repeatMode"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (d *DB) PlaybackSessionByToken(token string) (*PlaybackSession, error) {
	var s PlaybackSession
	var currentTrackID NullInt64
	err := d.QueryRow(
		`SELECT id, user_id, token, device_name, current_track_id, position, state, volume, shuffle, repeat_mode, created_at, updated_at
		 FROM playback_sessions WHERE token = ?`, token,
	).Scan(&s.ID, &s.UserID, &s.Token, &s.DeviceName, &currentTrackID, &s.Position, &s.State, &s.Volume, &s.Shuffle, &s.RepeatMode, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if currentTrackID.Valid {
		s.CurrentTrackID = &currentTrackID.Int64
	}
	return &s, nil
}

func (d *DB) PlaybackSessions() ([]PlaybackSession, error) {
	rows, err := d.Query(
		`SELECT id, user_id, token, device_name, current_track_id, position, state, volume, shuffle, repeat_mode, created_at, updated_at
		 FROM playback_sessions ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []PlaybackSession
	for rows.Next() {
		var s PlaybackSession
		var currentTrackID NullInt64
		err := rows.Scan(&s.ID, &s.UserID, &s.Token, &s.DeviceName, &currentTrackID, &s.Position, &s.State, &s.Volume, &s.Shuffle, &s.RepeatMode, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if currentTrackID.Valid {
			s.CurrentTrackID = &currentTrackID.Int64
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (d *DB) DeletePlaybackSession(id int64) error {
	_, err := d.Exec(`DELETE FROM playback_sessions WHERE id = ?`, id)
	return err
}

type CastDevice struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Protocol     string    `json:"protocol"`
	Capabilities string    `json:"capabilities"`
	IsActive     bool      `json:"isActive"`
	LastSeen     time.Time `json:"lastSeen"`
}

func (d *DB) InsertCastDevice(name, devType, host string, port int, protocol, capabilities string) (int64, error) {
	res, err := d.Exec(
		`INSERT INTO cast_devices(name, type, host, port, protocol, capabilities, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(name) DO UPDATE SET
		   host=excluded.host, port=excluded.port, last_seen=excluded.last_seen`,
		name, devType, host, port, protocol, capabilities,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) CastDevices() ([]CastDevice, error) {
	rows, err := d.Query(`SELECT id, name, type, host, port, protocol, capabilities, is_active, last_seen FROM cast_devices ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []CastDevice
	for rows.Next() {
		var dev CastDevice
		var isActive int
		err := rows.Scan(&dev.ID, &dev.Name, &dev.Type, &dev.Host, &dev.Port, &dev.Protocol, &dev.Capabilities, &isActive, &dev.LastSeen)
		if err != nil {
			return nil, err
		}
		dev.IsActive = isActive != 0
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

func (d *DB) UpdateCastDeviceActivity(id int64, active bool) error {
	val := 0
	if active {
		val = 1
	}
	_, err := d.Exec(`UPDATE cast_devices SET is_active = ? WHERE id = ?`, val, id)
	return err
}

func (d *DB) DeleteCastDevice(id int64) error {
	_, err := d.Exec(`DELETE FROM cast_devices WHERE id = ?`, id)
	return err
}

type DSPPreset struct {
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	UserID              int64   `json:"userId"`
	EQLow               float64 `json:"eqLow"`
	EQMid               float64 `json:"eqMid"`
	EQHigh              float64 `json:"eqHigh"`
	CompressorThreshold float64 `json:"compressorThreshold"`
	CompressorRatio     float64 `json:"compressorRatio"`
	LoudnessTarget      float64 `json:"loudnessTarget"`
	IsDefault           bool    `json:"isDefault"`
}

func (d *DB) InsertDSPPreset(name string, userID int64, eqLow, eqMid, eqHigh, compThreshold, compRatio, loudnessTarget float64, isDefault bool) (int64, error) {
	defaultVal := 0
	if isDefault {
		defaultVal = 1
		_, _ = d.Exec(`UPDATE dsp_presets SET is_default = 0 WHERE user_id = ?`, userID)
	}
	res, err := d.Exec(
		`INSERT INTO dsp_presets(name, user_id, eq_low, eq_mid, eq_high, compressor_threshold, compressor_ratio, loudness_target, is_default)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, userID, eqLow, eqMid, eqHigh, compThreshold, compRatio, loudnessTarget, defaultVal,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) DSPPresets(userID int64) ([]DSPPreset, error) {
	rows, err := d.Query(
		`SELECT id, name, user_id, eq_low, eq_mid, eq_high, compressor_threshold, compressor_ratio, loudness_target, is_default
		 FROM dsp_presets WHERE user_id = ? OR user_id = 0 ORDER BY is_default DESC, name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var presets []DSPPreset
	for rows.Next() {
		var p DSPPreset
		var isDefault int
		err := rows.Scan(&p.ID, &p.Name, &p.UserID, &p.EQLow, &p.EQMid, &p.EQHigh, &p.CompressorThreshold, &p.CompressorRatio, &p.LoudnessTarget, &isDefault)
		if err != nil {
			return nil, err
		}
		p.IsDefault = isDefault != 0
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func (d *DB) DSPPresetByID(id int64) (*DSPPreset, error) {
	var p DSPPreset
	var isDefault int
	err := d.QueryRow(
		`SELECT id, name, user_id, eq_low, eq_mid, eq_high, compressor_threshold, compressor_ratio, loudness_target, is_default
		 FROM dsp_presets WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.UserID, &p.EQLow, &p.EQMid, &p.EQHigh, &p.CompressorThreshold, &p.CompressorRatio, &p.LoudnessTarget, &isDefault)
	if err != nil {
		return nil, err
	}
	p.IsDefault = isDefault != 0
	return &p, nil
}

func (d *DB) DeleteDSPPreset(id int64) error {
	_, err := d.Exec(`DELETE FROM dsp_presets WHERE id = ?`, id)
	return err
}

type SyncRoom struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	OwnerID         *int64    `json:"ownerId,omitempty"`
	Token           string    `json:"token"`
	CurrentTrackID  *int64    `json:"currentTrackId,omitempty"`
	Position        float64   `json:"position"`
	State           string    `json:"state"`
	MasterSessionID *int64    `json:"masterSessionId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (d *DB) SyncRoomByToken(token string) (*SyncRoom, error) {
	var r SyncRoom
	var ownerID NullInt64
	var currentTrackID NullInt64
	var masterSessionID NullInt64

	err := d.QueryRow(
		`SELECT id, name, owner_id, token, current_track_id, position, state, master_session_id, created_at
		 FROM sync_rooms WHERE token = ?`, token,
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
	return &r, nil
}

func (d *DB) SyncRooms() ([]SyncRoom, error) {
	rows, err := d.Query(
		`SELECT id, name, owner_id, token, current_track_id, position, state, master_session_id, created_at
		 FROM sync_rooms ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []SyncRoom
	for rows.Next() {
		var r SyncRoom
		var ownerID NullInt64
		var currentTrackID NullInt64
		var masterSessionID NullInt64

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

func (d *DB) DeleteSyncRoom(id int64) error {
	_, err := d.Exec(`DELETE FROM sync_rooms WHERE id = ?`, id)
	return err
}

