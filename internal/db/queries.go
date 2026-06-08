package db

import (
	"database/sql"
	"fmt"
	"strings"
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
		`INSERT INTO tracks(album_id, artist_id, title, disc_number, track_number, duration, bitrate, format, path, mtime, size, lyrics, media_type, playback_position)
		 VALUES (:aid, :arid, :title, :disc, :track, :dur, :bit, :fmt, :path, :mtime, :size, :lyrics, :mediaType, :pos)
		 ON CONFLICT(path) DO UPDATE SET
		   album_id=excluded.album_id, artist_id=excluded.artist_id, title=excluded.title,
		   disc_number=excluded.disc_number, track_number=excluded.track_number,
		   duration=excluded.duration, bitrate=excluded.bitrate, format=excluded.format,
		   mtime=excluded.mtime, size=excluded.size, lyrics=excluded.lyrics, media_type=excluded.media_type,
		   playback_position=excluded.playback_position`,
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
		       t.lyrics, t.media_type, t.playback_position
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
		       t.lyrics, t.media_type, t.playback_position
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
		       t.lyrics, t.media_type, t.playback_position
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
		       t.lyrics, t.media_type, t.playback_position
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
	err := scanner.Scan(
		&t.ID, &albumID, &artistID, &t.Title,
		&t.ArtistName, &t.AlbumTitle,
		&t.DiscNumber, &t.TrackNumber, &t.Duration, &t.Bitrate, &t.Format,
		&t.Path, &t.Mtime, &t.Size,
		&lyrics, &mediaType, &t.PlaybackPosition,
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
		       t.lyrics, t.media_type, t.playback_position
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
		       t.lyrics, t.media_type, t.playback_position
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
		       t.lyrics, t.media_type, t.playback_position
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
