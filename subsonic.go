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
	AlbumList  *AlbumList  `xml:"albumList" json:"albumList,omitempty"`
	Starred    *Starred    `xml:"starred" json:"starred,omitempty"`
	Playlists  *Playlists  `xml:"playlists" json:"playlists,omitempty"`
	License    *License    `xml:"license" json:"license,omitempty"`
	Error      *SubError   `xml:"error" json:"error,omitempty"`
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

	// Return empty lists (no starring implemented yet in scanner)
	// Could be expanded to query stars table
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

	playlists, err := db.GetAllPlaylists(0) // Get all playlists for Subsonic compat
	if err != nil {
		writeSubsonicError(w, r, 0, fmt.Sprintf("Failed to get playlists: %v", err))
		return
	}

	subPlaylists := make([]SubsonicPlaylist, len(playlists))
	for i, p := range playlists {
		subPlaylists[i] = SubsonicPlaylist{
			ID:        strconv.FormatInt(p.ID, 10),
			Name:      p.Name,
			SongCount: 0,
			Created:   p.CreatedAt.Format(time.RFC3339),
		}
	}

	writeSubsonicResponse(w, r, &SubsonicResponse{
		Playlists: &Playlists{Playlist: subPlaylists},
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
