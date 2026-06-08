package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nightdrive/internal/config"
	"nightdrive/internal/db"
	"nightdrive/internal/scanner"
)

// Server holds API dependencies.
type Server struct {
	cfg     *config.Config
	db      *db.DB
	scanner *scanner.Scanner
}

// NewServer creates an API server.
func NewServer(cfg *config.Config, database *db.DB, s *scanner.Scanner) *Server {
	return &Server{cfg: cfg, db: database, scanner: s}
}

// Register mounts all API and Subsonic routes.
func (s *Server) Register(mux *http.ServeMux) {
	// REST API
	mux.HandleFunc("GET /api/health", s.healthHandler)
	mux.HandleFunc("GET /api/tracks", s.tracksHandler)
	mux.HandleFunc("GET /api/tracks/{id}", s.trackHandler)
	mux.HandleFunc("GET /api/tracks/{id}/stream", s.streamHandler)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.coverHandler)
	mux.HandleFunc("GET /api/albums", s.albumsHandler)
	mux.HandleFunc("GET /api/albums/{id}/tracks", s.albumTracksHandler)
	mux.HandleFunc("GET /api/artists", s.artistsHandler)
	mux.HandleFunc("GET /api/search", s.searchHandler)
	mux.HandleFunc("GET /api/playlists", s.playlistsHandler)
	mux.HandleFunc("POST /api/playlists", s.createPlaylistHandler)
	mux.HandleFunc("GET /api/playlists/{id}/tracks", s.playlistTracksHandler)
	mux.HandleFunc("POST /api/playlists/{id}/tracks", s.addPlaylistTrackHandler)
	mux.HandleFunc("DELETE /api/playlists/{id}/tracks/{trackId}", s.removePlaylistTrackHandler)
	mux.HandleFunc("DELETE /api/playlists/{id}", s.deletePlaylistHandler)
	mux.HandleFunc("POST /api/scan", s.scanHandler)

	// Subsonic API compatibility
	mux.HandleFunc("GET /rest/ping", s.subsonicHandler(s.subPing))
	mux.HandleFunc("GET /rest/getLicense", s.subsonicHandler(s.subLicense))
	mux.HandleFunc("GET /rest/getMusicFolders", s.subsonicHandler(s.subMusicFolders))
	mux.HandleFunc("GET /rest/getIndexes", s.subsonicHandler(s.subIndexes))
	mux.HandleFunc("GET /rest/getArtists", s.subsonicHandler(s.subArtists))
	mux.HandleFunc("GET /rest/getArtist", s.subsonicHandler(s.subArtist))
	mux.HandleFunc("GET /rest/getAlbum", s.subsonicHandler(s.subAlbum))
	mux.HandleFunc("GET /rest/getSong", s.subsonicHandler(s.subSong))
	mux.HandleFunc("GET /rest/getRandomSongs", s.subsonicHandler(s.subRandomSongs))
	mux.HandleFunc("GET /rest/search3", s.subsonicHandler(s.subSearch3))
	mux.HandleFunc("GET /rest/stream", s.subStream)
	mux.HandleFunc("GET /rest/getCoverArt", s.subCoverArt)
	mux.HandleFunc("GET /rest/getPlaylists", s.subsonicHandler(s.subPlaylists))
	mux.HandleFunc("GET /rest/getPlaylist", s.subsonicHandler(s.subPlaylist))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

func (s *Server) tracksHandler(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tracks, err := s.db.Tracks(offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) trackHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	track, err := s.db.TrackByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Server) streamHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	track, err := s.db.TrackByID(id)
	if err != nil || track == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.serveFile(w, r, track.Path, track.Format)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, path, format string) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	contentType := "audio/mpeg"
	switch strings.ToLower(format) {
	case "flac":
		contentType = "audio/flac"
	case "ogg", "oga":
		contentType = "audio/ogg"
	case "m4a", "mp4":
		contentType = "audio/mp4"
	case "opus":
		contentType = "audio/opus"
	case "wav":
		contentType = "audio/wav"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	// Hint gapless clients by serving contiguous files with consistent content-type.
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Server) coverHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	track, err := s.db.TrackByID(id)
	if err != nil || track == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	dir := filepath.Dir(track.Path)
	candidates := []string{"cover.jpg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "front.png", "album.jpg", "album.png"}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
	}
	http.Error(w, "no cover", http.StatusNotFound)
}

func (s *Server) albumsHandler(w http.ResponseWriter, r *http.Request) {
	albums, err := s.db.Albums()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Server) albumTracksHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	tracks, err := s.db.AlbumTracks(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) artistsHandler(w http.ResponseWriter, r *http.Request) {
	artists, err := s.db.Artists()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, artists)
}

func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tracks, err := s.db.SearchTracks(q, offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) playlistsHandler(w http.ResponseWriter, r *http.Request) {
	pls, err := s.db.Playlists()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, pls)
}

func (s *Server) createPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		var body struct {
			Name       string `json:"name"`
			SmartQuery string `json:"smartQuery"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name = body.Name
		if sq := body.SmartQuery; sq != "" {
			r.URL.Query().Set("smartQuery", sq)
		}
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	sq := r.URL.Query().Get("smartQuery")
	id, err := s.db.InsertPlaylist(name, sq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) playlistTracksHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	tracks, err := s.db.PlaylistTracks(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) addPlaylistTrackHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tid, _ := strconv.ParseInt(r.URL.Query().Get("trackId"), 10, 64)
	if tid == 0 {
		var body struct{ TrackID int64 `json:"trackId"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		tid = body.TrackID
	}
	if pid == 0 || tid == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "playlist and track required"})
		return
	}
	if err := s.db.AddPlaylistTrack(pid, tid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) removePlaylistTrackHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tid, _ := strconv.ParseInt(r.PathValue("trackId"), 10, 64)
	if err := s.db.RemovePlaylistTrack(pid, tid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) deletePlaylistHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.db.DeletePlaylist(pid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) scanHandler(w http.ResponseWriter, r *http.Request) {
	if s.scanner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no scanner configured"})
		return
	}
	go s.scanner.ScanNow()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scanning"})
}

// Subsonic helpers
func (s *Server) subsonicHandler(fn func(w http.ResponseWriter, r *http.Request) interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := fn(w, r)
		resp := map[string]interface{}{
			"subsonic-response": map[string]interface{}{
				"status":        "ok",
				"version":       "1.16.1",
				"type":          "nightdrive",
				"serverVersion": "0.1.0",
				"openSubsonic":  true,
			},
		}
		if data != nil {
			resp["subsonic-response"].(map[string]interface{})[r.URL.Query().Get("f")] = data
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (s *Server) subPing(w http.ResponseWriter, r *http.Request) interface{} { return nil }
func (s *Server) subLicense(w http.ResponseWriter, r *http.Request) interface{} {
	return map[string]interface{}{"valid": true}
}
func (s *Server) subMusicFolders(w http.ResponseWriter, r *http.Request) interface{} {
	var folders []map[string]interface{}
	for i, d := range s.cfg.MusicDir {
		folders = append(folders, map[string]interface{}{"id": i + 1, "name": filepath.Base(d)})
	}
	return map[string]interface{}{"musicFolder": folders}
}
func (s *Server) subIndexes(w http.ResponseWriter, r *http.Request) interface{} {
	artists, _ := s.db.Artists()
	index := map[string][]map[string]interface{}{}
	for _, a := range artists {
		key := "#"
		if len(a.Name) > 0 {
			key = strings.ToUpper(a.Name[:1])
			if key < "A" || key > "Z" {
				key = "#"
			}
		}
		index[key] = append(index[key], map[string]interface{}{"id": fmt.Sprintf("ar-%d", a.ID), "name": a.Name})
	}
	var out []map[string]interface{}
	for k, v := range index {
		out = append(out, map[string]interface{}{"name": k, "artist": v})
	}
	return map[string]interface{}{"indexes": map[string]interface{}{"index": out}}
}
func (s *Server) subArtists(w http.ResponseWriter, r *http.Request) interface{} {
	return s.subIndexes(w, r)
}
func (s *Server) subArtist(w http.ResponseWriter, r *http.Request) interface{} {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "ar-"), 10, 64)
	albums, _ := s.db.Albums()
	var out []map[string]interface{}
	for _, al := range albums {
		if al.ArtistID == id {
			out = append(out, map[string]interface{}{"id": fmt.Sprintf("al-%d", al.ID), "name": al.Title, "artistId": id})
		}
	}
	return map[string]interface{}{"album": out}
}
func (s *Server) subAlbum(w http.ResponseWriter, r *http.Request) interface{} {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "al-"), 10, 64)
	tracks, _ := s.db.AlbumTracks(id)
	var songs []map[string]interface{}
	for _, t := range tracks {
		songs = append(songs, subSong(t))
	}
	return map[string]interface{}{"song": songs}
}
func (s *Server) subSong(w http.ResponseWriter, r *http.Request) interface{} {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "tr-"), 10, 64)
	t, _ := s.db.TrackByID(id)
	if t == nil {
		return nil
	}
	return subSong(*t)
}
func (s *Server) subRandomSongs(w http.ResponseWriter, r *http.Request) interface{} {
	tracks, _ := s.db.Tracks(0, 50)
	var songs []map[string]interface{}
	for _, t := range tracks {
		songs = append(songs, subSong(t))
	}
	return map[string]interface{}{"song": songs}
}
func (s *Server) subSearch3(w http.ResponseWriter, r *http.Request) interface{} {
	q := r.URL.Query().Get("query")
	tracks, _ := s.db.SearchTracks(q, 0, 50)
	var songs []map[string]interface{}
	for _, t := range tracks {
		songs = append(songs, subSong(t))
	}
	return map[string]interface{}{"searchResult3": map[string]interface{}{"song": songs}}
}
func (s *Server) subPlaylists(w http.ResponseWriter, r *http.Request) interface{} {
	pls, _ := s.db.Playlists()
	var out []map[string]interface{}
	for _, p := range pls {
		out = append(out, map[string]interface{}{"id": fmt.Sprintf("pl-%d", p.ID), "name": p.Name})
	}
	return map[string]interface{}{"playlist": out}
}
func (s *Server) subPlaylist(w http.ResponseWriter, r *http.Request) interface{} {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "pl-"), 10, 64)
	p := db.Playlist{ID: id}
	tracks, _ := s.db.PlaylistTracks(id)
	var songs []map[string]interface{}
	for _, t := range tracks {
		songs = append(songs, subSong(t))
	}
	return map[string]interface{}{"id": fmt.Sprintf("pl-%d", p.ID), "entry": songs}
}

func (s *Server) subStream(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "tr-"), 10, 64)
	track, err := s.db.TrackByID(id)
	if err != nil || track == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"subsonic-response": map[string]interface{}{
				"status": "failed",
				"error":  map[string]interface{}{"code": 70, "message": "not found"},
			},
		})
		return
	}
	s.serveFile(w, r, track.Path, track.Format)
}

func (s *Server) subCoverArt(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(strings.TrimPrefix(idStr, "al-"), 10, 64)
	// Find first track in album and use folder cover heuristic.
	tracks, _ := s.db.AlbumTracks(id)
	if len(tracks) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	dir := filepath.Dir(tracks[0].Path)
	candidates := []string{"cover.jpg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "front.png", "album.jpg", "album.png"}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
	}
	http.Error(w, "no cover", http.StatusNotFound)
}

func subSong(t db.Track) map[string]interface{} {
	artistID := ""
	if t.ArtistID != nil {
		artistID = fmt.Sprintf("ar-%d", *t.ArtistID)
	}
	albumID := ""
	if t.AlbumID != nil {
		albumID = fmt.Sprintf("al-%d", *t.AlbumID)
	}
	return map[string]interface{}{
		"id":          fmt.Sprintf("tr-%d", t.ID),
		"parent":      albumID,
		"title":       t.Title,
		"album":       t.AlbumTitle,
		"artist":      t.ArtistName,
		"isDir":       false,
		"type":        "music",
		"albumId":     albumID,
		"artistId":    artistID,
		"coverArt":    albumID,
		"duration":    int(t.Duration),
		"bitRate":     t.Bitrate,
		"suffix":      t.Format,
		"contentType": "audio/mpeg",
		"path":        t.Path,
		"isVideo":     false,
		"created":     time.Now().UTC().Format(time.RFC3339),
	}
}

// Logging middleware wrapper.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// NopCloser wraps an io.Reader as an io.ReadCloser.
type NopCloser struct {
	io.Reader
}

func (NopCloser) Close() error { return nil }
