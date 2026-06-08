package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const subsonicVersion = "1.16.1"

// SubsonicResponse is the wrapper for all Subsonic API responses
type SubsonicResponse struct {
	XMLName    xml.Name    `xml:"subsonic-response" json:"-"`
	Xmlns      string      `xml:"xmlns,attr" json:"-"`
	Status     string      `xml:"status,attr" json:"status"`
	Version    string      `xml:"version,attr" json:"version"`
	AlbumList  *AlbumList      `xml:"albumList" json:"albumList,omitempty"`
	Starred    *Starred        `xml:"starred" json:"starred,omitempty"`
	Playlists  *Playlists      `xml:"playlists" json:"playlists,omitempty"`
	License    *License        `xml:"license" json:"license,omitempty"`
	Lyrics     *SubsonicLyrics `xml:"lyrics" json:"lyrics,omitempty"`
	Playlist   *SubsonicPlaylist `xml:"playlist" json:"playlist,omitempty"`
	Error      *SubError       `xml:"error" json:"error,omitempty"`
}

// AlbumList holds a list of albums
type AlbumList struct {
	Albums []SubsonicAlbum `xml:"album" json:"album"`
}

// SubsonicAlbum represents an album in Subsonic format
type SubsonicAlbum struct {
	ID       string `xml:"id,attr" json:"id"`
	Name     string `xml:"name,attr" json:"name"`
	Artist   string `xml:"artist,attr" json:"artist"`
	ArtistID string `xml:"artistId,attr" json:"artistId"`
	Year     int    `xml:"year,attr" json:"year"`
}

// Starred holds starred items
type Starred struct {
	Artists []SubsonicArtist `xml:"artist" json:"artist"`
	Albums  []SubsonicAlbum  `xml:"album" json:"album"`
	Tracks  []SubsonicTrack  `xml:"song" json:"song"`
}

// SubsonicArtist represents an artist in Subsonic format
type SubsonicArtist struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

// SubsonicTrack represents a track in Subsonic format
type SubsonicTrack struct {
	ID       string `xml:"id,attr" json:"id"`
	Parent   string `xml:"parent,attr" json:"parent"`
	Title    string `xml:"title,attr" json:"title"`
	Album    string `xml:"album,attr" json:"album"`
	Artist   string `xml:"artist,attr" json:"artist"`
	Track    int    `xml:"track,attr" json:"track"`
	Year     int    `xml:"year,attr" json:"year"`
	Duration int    `xml:"duration,attr" json:"duration"`
	Format   string `xml:"suffix,attr" json:"suffix"`
}

// Playlists holds playlists
type Playlists struct {
	Playlist []SubsonicPlaylist `xml:"playlist" json:"playlist"`
}

// SubsonicPlaylist represents a playlist in Subsonic format
type SubsonicPlaylist struct {
	ID        string          `xml:"id,attr" json:"id"`
	Name      string          `xml:"name,attr" json:"name"`
	SongCount int             `xml:"songCount,attr" json:"songCount"`
	Created   string          `xml:"created,attr" json:"created"`
	Entry     []SubsonicTrack `xml:"entry" json:"entry"`
}

// License holds license info
type License struct {
	Valid bool `xml:"valid,attr" json:"valid"`
}

type SubsonicLyrics struct {
	Artist string `xml:"artist,attr" json:"artist"`
	Title  string `xml:"title,attr" json:"title"`
	Value  string `xml:",chardata" json:"value"`
}

// SubError represents a Subsonic error
type SubError struct {
	Code    int    `xml:"code,attr" json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

// writeSubsonicResponse writes a SubsonicResponse in XML or JSON format
func writeSubsonicResponse(w http.ResponseWriter, r *http.Request, resp *SubsonicResponse) {
	resp.Status = "ok"
	resp.Version = subsonicVersion
	resp.Xmlns = "http://subsonic.org/restapi"

	format := r.URL.Query().Get("f")
	if format == "" {
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") {
			format = "json"
		}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"subsonic-response": resp,
		})
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(resp)
}

// writeSubsonicError writes a Subsonic error response
func writeSubsonicError(w http.ResponseWriter, r *http.Request, code int, message string) {
	resp := &SubsonicResponse{
		Status:  "failed",
		Version: subsonicVersion,
		Xmlns:   "http://subsonic.org/restapi",
		Error: &SubError{
			Code:    code,
			Message: message,
		},
	}

	format := r.URL.Query().Get("f")
	if format == "" {
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") {
			format = "json"
		}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"subsonic-response": resp,
		})
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(resp)
}

// handleSubsonic routes Subsonic API requests
func handleSubsonic(w http.ResponseWriter, r *http.Request) {
	// Extract action from path: /subsonic/rest/{action}.view
	path := strings.TrimPrefix(r.URL.Path, "/subsonic/rest/")
	action := strings.TrimSuffix(path, ".view")

	switch action {
	case "ping":
		handlePing(w, r)
	case "getLicense":
		handleGetLicense(w, r)
	case "getAlbumList":
		handleGetAlbumList(w, r)
	case "getStarred":
		handleGetStarred(w, r)
	case "getPlaylists":
		handleGetPlaylists(w, r)
	case "getPlaylist":
		handleGetPlaylist(w, r)
	case "createPlaylist":
		handleCreatePlaylist(w, r)
	case "updatePlaylist":
		handleUpdatePlaylist(w, r)
	case "deletePlaylist":
		handleDeletePlaylist(w, r)
	case "getLyrics":
		handleGetLyrics(w, r)
	case "scrobble":
		handleScrobble(w, r)
	default:
		writeSubsonicError(w, r, 0, fmt.Sprintf("Unknown action: %s", action))
	}
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	writeSubsonicResponse(w, r, &SubsonicResponse{})
}

func handleGetLicense(w http.ResponseWriter, r *http.Request) {
	writeSubsonicResponse(w, r, &SubsonicResponse{
		License: &License{Valid: true},
	})
}

func handleGetAlbumList(w http.ResponseWriter, r *http.Request) {
	// Verify auth for non-ping endpoints
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	sortType := r.URL.Query().Get("type")
	if sortType == "" {
		sortType = "newest"
	}

	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 {
		size = 10
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	albums, err := db.GetAlbumsSorted(sortType, size, offset)
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to get albums: %v", err))
		return
	}

	subAlbums := make([]SubsonicAlbum, len(albums))
	for i, a := range albums {
		subAlbums[i] = SubsonicAlbum{
			ID:       strconv.FormatInt(a.ID, 10),
			Name:     a.Title,
			Artist:   a.ArtistName,
			ArtistID: strconv.FormatInt(a.ArtistID, 10),
			Year:     a.Year,
		}
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		AlbumList: &AlbumList{Albums: subAlbums},
	})
}

func handleGetStarred(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	user, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	_ = user
	starred := &Starred{
		Artists: []SubsonicArtist{},
		Albums:  []SubsonicAlbum{},
		Tracks:  []SubsonicTrack{},
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Starred: starred,
	})
}

func handleGetPlaylists(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	playlists, err := db.GetAllPlaylists(0)
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to get playlists: %v", err))
		return
	}

	smartPlaylists, err := db.GetAllSmartPlaylists(0)
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to get smart playlists: %v", err))
		return
	}

	totalCount := len(playlists) + len(smartPlaylists)
	subPlaylists := make([]SubsonicPlaylist, 0, totalCount)

	for _, p := range playlists {
		tracks, _ := db.GetPlaylistTracks(p.ID)
		subPlaylists = append(subPlaylists, SubsonicPlaylist{
			ID:        strconv.FormatInt(p.ID, 10),
			Name:      p.Name,
			SongCount: len(tracks),
			Created:   p.CreatedAt.Format(time.RFC3339),
		})
	}

	for _, sp := range smartPlaylists {
		tracks, _ := db.GetSmartPlaylistTracks(sp)
		subPlaylists = append(subPlaylists, SubsonicPlaylist{
			ID:        "sp_" + strconv.FormatInt(sp.ID, 10),
			Name:      sp.Name + " (Smart)",
			SongCount: len(tracks),
			Created:   sp.CreatedAt.Format(time.RFC3339),
		})
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Playlists: &Playlists{Playlist: subPlaylists},
	})
}

func handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: id")
		return
	}

	playlistID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeSubsonicError(w, r, 70, "Invalid id")
		return
	}

	playlist, err := db.GetPlaylistByID(playlistID)
	if err != nil {
		writeSubsonicError(w, r, 70, "Playlist not found")
		return
	}

	tracks, err := db.GetPlaylistTracks(playlistID)
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to get tracks: %v", err))
		return
	}

	entries := make([]SubsonicTrack, len(tracks))
	for i, pt := range tracks {
		entries[i] = SubsonicTrack{
			ID:       strconv.FormatInt(pt.Track.ID, 10),
			Parent:   strconv.FormatInt(pt.Track.AlbumID, 10),
			Title:    pt.Track.Title,
			Track:    pt.Track.TrackNum,
			Duration: pt.Track.Duration,
			Format:   strings.ToLower(pt.Track.Format),
		}
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Playlist: &SubsonicPlaylist{
			ID:        strconv.FormatInt(playlist.ID, 10),
			Name:      playlist.Name,
			SongCount: len(entries),
			Created:   playlist.CreatedAt.Format(time.RFC3339),
			Entry:     entries,
		},
	})
}

func handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	user, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: name")
		return
	}

	playlist, err := db.CreatePlaylist(user.ID, name)
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to create playlist: %v", err))
		return
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Playlist: &SubsonicPlaylist{
			ID:        strconv.FormatInt(playlist.ID, 10),
			Name:      playlist.Name,
			SongCount: 0,
			Created:   playlist.CreatedAt.Format(time.RFC3339),
		},
	})
}

func handleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	playlistID, err := strconv.ParseInt(r.URL.Query().Get("playlistId"), 10, 64)
	if err != nil {
		writeSubsonicError(w, r, 70, "Invalid playlistId")
		return
	}

	name := r.URL.Query().Get("name")
	if name != "" {
		_, err = db.UpdatePlaylist(playlistID, name)
		if err != nil {
			writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to update playlist: %v", err))
			return
		}
	}

	songIds := r.URL.Query()["songId"]
	for _, songId := range songIds {
		trackID, err := strconv.ParseInt(songId, 10, 64)
		if err == nil {
			db.AddTrackToPlaylist(playlistID, trackID)
		}
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{})
}

func handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	playlistID, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		writeSubsonicError(w, r, 70, "Invalid id")
		return
	}

	if err := db.DeletePlaylist(playlistID); err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to delete playlist: %v", err))
		return
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{})
}

func handleGetLyrics(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	_, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	artist := r.URL.Query().Get("artist")
	title := r.URL.Query().Get("title")

	var trackID int64
	err = db.conn.QueryRow(
		"SELECT t.id FROM tracks t JOIN artists ar ON t.artist_id = ar.id WHERE ar.name = ? AND t.title = ? LIMIT 1",
		artist, title,
	).Scan(&trackID)

	var lyricsText string
	if err == nil {
		lyrics, err := db.GetLyrics(trackID)
		if err == nil && lyrics != nil {
			lyricsText = lyrics.Content
		}
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Lyrics: &SubsonicLyrics{
			Artist: artist,
			Title:  title,
			Value:  lyricsText,
		},
	})
}

func handleScrobble(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("p")
	if apiKey == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: p")
		return
	}
	user, err := db.GetUserByAPIKey(apiKey)
	if err != nil {
		writeSubsonicError(w, r, 40, "Wrong username or password")
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeSubsonicError(w, r, 10, "Required parameter is missing: id")
		return
	}

	trackID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeSubsonicError(w, r, 70, "Invalid id")
		return
	}

	// Record scrobble
	if err := db.RecordScrobble(user.ID, trackID); err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to record scrobble: %v", err))
		return
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{})
}
