package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

	"golang.org/x/crypto/bcrypt"

	"nightdrive/internal/cast"
	"nightdrive/internal/config"
	"nightdrive/internal/crashreport"
	"nightdrive/internal/db"
	"nightdrive/internal/dsp"
	"nightdrive/internal/gapless"
	"nightdrive/internal/playback"
	"nightdrive/internal/scanner"
	syncpkg "nightdrive/internal/sync"
)

// Server holds API dependencies.
type Server struct {
	cfg          *config.Config
	db           *db.DB
	scanner      *scanner.Scanner
	playback     *playback.Engine
	gapless      *gapless.Analyzer
	castMgr      *cast.Manager
	syncCoord    *syncpkg.Coordinator
	crashReporter *crashreport.Reporter
}

// castStoreAdapter adapts *db.DB to cast.DeviceStore.
type castStoreAdapter struct{ db *db.DB }

func (a *castStoreAdapter) InsertCastDevice(name, devType, host string, port int, protocol, capabilities string) (int64, error) {
	return a.db.InsertCastDevice(name, devType, host, port, protocol, capabilities)
}

func (a *castStoreAdapter) UpdateCastDeviceActivity(id int64, active bool) error {
	return a.db.UpdateCastDeviceActivity(id, active)
}

func (a *castStoreAdapter) CastDevices() ([]cast.CastDeviceRecord, error) {
	devs, err := a.db.CastDevices()
	if err != nil {
		return nil, err
	}
	out := make([]cast.CastDeviceRecord, len(devs))
	for i, d := range devs {
		out[i] = cast.CastDeviceRecord{
			ID:           d.ID,
			Name:         d.Name,
			Type:         d.Type,
			Host:         d.Host,
			Port:         d.Port,
			Protocol:     d.Protocol,
			Capabilities: d.Capabilities,
			IsActive:     d.IsActive,
			LastSeen:     d.LastSeen,
		}
	}
	return out, nil
}

func (a *castStoreAdapter) DeleteCastDevice(id int64) error {
	return a.db.DeleteCastDevice(id)
}

// NewServer creates an API server.
func NewServer(cfg *config.Config, database *db.DB, s *scanner.Scanner, reporter *crashreport.Reporter) *Server {
	eng := playback.NewEngine(database)
	return &Server{
		cfg:           cfg,
		db:            database,
		scanner:       s,
		playback:      eng,
		gapless:       gapless.NewAnalyzer(database),
		castMgr:       cast.NewManager(&castStoreAdapter{db: database}),
		syncCoord:     syncpkg.NewCoordinator(database, eng),
		crashReporter: reporter,
	}
}

type contextKey int

const userContextKey contextKey = iota

func userFromContext(ctx context.Context) *db.User {
	u, _ := ctx.Value(userContextKey).(*db.User)
	return u
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(pw, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.URLEncoding.EncodeToString(b)
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user *db.User
		if token := r.Header.Get("X-Auth-Token"); token != "" {
			if sess, err := s.db.SessionByToken(token); err == nil {
				if u, err := s.db.UserByID(sess.UserID); err == nil {
					user = u
				}
			}
		}
		if user == nil {
			if u, p, ok := r.BasicAuth(); ok {
				if udb, err := s.db.UserByUsername(u); err == nil && checkPassword(p, udb.PasswordHash) {
					user = udb
				}
			}
		}
		if user != nil {
			r = r.WithContext(context.WithValue(r.Context(), userContextKey, user))
		}
		next(w, r)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if userFromContext(r.Context()) == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	})
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFromContext(r.Context())
		if u == nil || u.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin required"})
			return
		}
		next(w, r)
	})
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.healthHandler)
	mux.HandleFunc("POST /api/feedback", s.feedbackHandler)
	mux.HandleFunc("GET /api/admin/crashes", s.requireAdmin(s.crashesHandler))
	mux.HandleFunc("GET /api/tracks", s.tracksHandler)
	mux.HandleFunc("GET /api/tracks/{id}", s.trackHandler)
	mux.HandleFunc("GET /api/tracks/{id}/stream", s.streamHandler)
	mux.HandleFunc("GET /api/tracks/{id}/cover", s.coverHandler)
	mux.HandleFunc("GET /api/tracks/{id}/lyrics", s.lyricsHandler)
	mux.HandleFunc("POST /api/tracks/{id}/lyrics", s.requireAuth(s.saveLyricsHandler))
	mux.HandleFunc("POST /api/tracks/{id}/position", s.requireAuth(s.positionHandler))
	mux.HandleFunc("GET /api/albums", s.albumsHandler)
	mux.HandleFunc("GET /api/albums/{id}/tracks", s.albumTracksHandler)
	mux.HandleFunc("GET /api/artists", s.artistsHandler)
	mux.HandleFunc("GET /api/search", s.searchHandler)
	mux.HandleFunc("GET /api/playlists", s.authMiddleware(s.playlistsHandler))
	mux.HandleFunc("POST /api/playlists", s.requireAuth(s.createPlaylistHandler))
	mux.HandleFunc("GET /api/playlists/{id}/tracks", s.playlistTracksHandler)
	mux.HandleFunc("POST /api/playlists/{id}/tracks", s.requireAuth(s.addPlaylistTrackHandler))
	mux.HandleFunc("DELETE /api/playlists/{id}/tracks/{trackId}", s.requireAuth(s.removePlaylistTrackHandler))
	mux.HandleFunc("DELETE /api/playlists/{id}", s.requireAuth(s.deletePlaylistHandler))
	mux.HandleFunc("POST /api/playlists/{id}/share", s.requireAuth(s.sharePlaylistHandler))
	mux.HandleFunc("DELETE /api/playlists/{id}/share/{userId}", s.requireAuth(s.unsharePlaylistHandler))
	mux.HandleFunc("POST /api/scan", s.requireAdmin(s.scanHandler))

	mux.HandleFunc("POST /api/auth/login", s.loginHandler)
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.logoutHandler))
	mux.HandleFunc("GET /api/auth/me", s.authMiddleware(s.meHandler))

	mux.HandleFunc("GET /api/users", s.requireAdmin(s.listUsersHandler))
	mux.HandleFunc("POST /api/users", s.requireAdmin(s.createUserHandler))
	mux.HandleFunc("DELETE /api/users/{id}", s.requireAdmin(s.deleteUserHandler))

	mux.HandleFunc("POST /api/plays", s.requireAuth(s.recordPlayHandler))
	mux.HandleFunc("GET /api/history", s.requireAuth(s.historyHandler))
	mux.HandleFunc("GET /api/stats", s.requireAuth(s.statsHandler))

	mux.HandleFunc("GET /api/recommendations/similar-albums", s.similarAlbumsHandler)
	mux.HandleFunc("GET /api/recommendations/similar-artists", s.similarArtistsHandler)
	mux.HandleFunc("GET /api/recommendations/tracks", s.requireAuth(s.recommendedTracksHandler))

	mux.HandleFunc("GET /api/radio", s.radioHandler)
	mux.HandleFunc("GET /api/genres", s.genresHandler)

	mux.HandleFunc("POST /api/sessions", s.requireAuth(s.createSessionHandler))
	mux.HandleFunc("GET /api/sessions", s.requireAuth(s.listSessionsHandler))
	mux.HandleFunc("GET /api/sessions/{id}", s.requireAuth(s.getSessionHandler))
	mux.HandleFunc("DELETE /api/sessions/{id}", s.requireAuth(s.deleteSessionHandler))
	mux.HandleFunc("POST /api/sessions/{id}/play", s.requireAuth(s.sessionPlayHandler))
	mux.HandleFunc("POST /api/sessions/{id}/pause", s.requireAuth(s.sessionPauseHandler))
	mux.HandleFunc("POST /api/sessions/{id}/next", s.requireAuth(s.sessionNextHandler))
	mux.HandleFunc("POST /api/sessions/{id}/prev", s.requireAuth(s.sessionPrevHandler))
	mux.HandleFunc("POST /api/sessions/{id}/queue", s.requireAuth(s.sessionAddQueueHandler))
	mux.HandleFunc("GET /api/sessions/{id}/queue", s.requireAuth(s.sessionGetQueueHandler))
	mux.HandleFunc("DELETE /api/sessions/{id}/queue", s.requireAuth(s.sessionClearQueueHandler))
	mux.HandleFunc("POST /api/sessions/{id}/volume", s.requireAuth(s.sessionVolumeHandler))
	mux.HandleFunc("POST /api/sessions/{id}/crossfade", s.requireAuth(s.sessionCrossfadeHandler))

	mux.HandleFunc("POST /api/gapless/analyze/{albumId}", s.requireAuth(s.gaplessAnalyzeHandler))
	mux.HandleFunc("GET /api/gapless/report/{albumId}", s.gaplessReportHandler)
	mux.HandleFunc("POST /api/gapless/batch", s.requireAdmin(s.gaplessBatchHandler))

	mux.HandleFunc("GET /api/dsp/presets", s.requireAuth(s.dspPresetsHandler))
	mux.HandleFunc("POST /api/dsp/presets", s.requireAuth(s.dspCreatePresetHandler))
	mux.HandleFunc("GET /api/dsp/presets/{id}", s.requireAuth(s.dspPresetHandler))
	mux.HandleFunc("DELETE /api/dsp/presets/{id}", s.requireAuth(s.dspDeletePresetHandler))
	mux.HandleFunc("POST /api/dsp/process", s.requireAuth(s.dspProcessHandler))

	mux.HandleFunc("GET /api/sync/rooms", s.authMiddleware(s.listSyncRoomsHandler))
	mux.HandleFunc("POST /api/sync/rooms", s.requireAuth(s.createSyncRoomHandler))
	mux.HandleFunc("GET /api/sync/rooms/{id}", s.authMiddleware(s.getSyncRoomHandler))
	mux.HandleFunc("DELETE /api/sync/rooms/{id}", s.requireAuth(s.deleteSyncRoomHandler))
	mux.HandleFunc("POST /api/sync/rooms/{id}/join", s.requireAuth(s.joinSyncRoomHandler))
	mux.HandleFunc("POST /api/sync/rooms/{id}/leave", s.requireAuth(s.leaveSyncRoomHandler))
	mux.HandleFunc("POST /api/sync/rooms/{id}/play", s.requireAuth(s.syncRoomPlayHandler))
	mux.HandleFunc("POST /api/sync/rooms/{id}/pause", s.requireAuth(s.syncRoomPauseHandler))
	mux.HandleFunc("POST /api/sync/rooms/{id}/seek", s.requireAuth(s.syncRoomSeekHandler))

	mux.HandleFunc("GET /api/cast/devices", s.authMiddleware(s.castDevicesHandler))
	mux.HandleFunc("POST /api/cast/discover", s.requireAuth(s.castDiscoverHandler))
	mux.HandleFunc("POST /api/cast/devices", s.requireAuth(s.castRegisterHandler))
	mux.HandleFunc("POST /api/cast/devices/{id}/play", s.requireAuth(s.castPlayHandler))
	mux.HandleFunc("POST /api/cast/devices/{id}/pause", s.requireAuth(s.castPauseHandler))
	mux.HandleFunc("POST /api/cast/devices/{id}/stop", s.requireAuth(s.castStopHandler))
	mux.HandleFunc("DELETE /api/cast/devices/{id}", s.requireAuth(s.castDeleteHandler))

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

func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type     string `json:"type"`
		Message  string `json:"message"`
		Rating   int    `json:"rating"`
		Metadata string `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}
	if body.Type == "" {
		body.Type = "general"
	}
	var userID *int64
	if u := userFromContext(r.Context()); u != nil {
		userID = &u.ID
	}
	id, err := s.db.InsertFeedback(userID, body.Type, body.Message, body.Rating, body.Metadata)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "status": "received"})
}

func (s *Server) crashesHandler(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if s.crashReporter == nil {
		writeJSON(w, http.StatusOK, []crashreport.Report{})
		return
	}
	reports := s.crashReporter.Reports(limit)
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) tracksHandler(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	mediaType := r.URL.Query().Get("mediaType")
	var tracks []db.Track
	var err error
	if mediaType != "" {
		tracks, err = s.db.TracksByMediaType(mediaType, offset, limit)
	} else {
		tracks, err = s.db.Tracks(offset, limit)
	}
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

func (s *Server) lyricsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	lyrics, err := s.db.TrackLyrics(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lyrics": lyrics})
}

func (s *Server) saveLyricsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Lyrics string `json:"lyrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.db.UpdateTrackLyrics(id, body.Lyrics); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) positionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.db.UpdateTrackPosition(id, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
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
	var pls []db.Playlist
	var err error
	if u := userFromContext(r.Context()); u != nil {
		pls, err = s.db.PlaylistsForUser(u.ID)
	} else {
		pls, err = s.db.PlaylistsForUser(0)
	}
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
	u := userFromContext(r.Context())
	ownerID := int64(0)
	if u != nil {
		ownerID = u.ID
	}
	id, err := s.db.InsertPlaylistWithOwner(name, sq, ownerID, "private")
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
	ownerID, _ := s.db.PlaylistOwner(pid)
	u := userFromContext(r.Context())
	if u != nil && ownerID != 0 && ownerID != u.ID && u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
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
	ownerID, _ := s.db.PlaylistOwner(pid)
	u := userFromContext(r.Context())
	if u != nil && ownerID != 0 && ownerID != u.ID && u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
		return
	}
	if err := s.db.RemovePlaylistTrack(pid, tid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) deletePlaylistHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ownerID, _ := s.db.PlaylistOwner(pid)
	u := userFromContext(r.Context())
	if u != nil && ownerID != 0 && ownerID != u.ID && u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
		return
	}
	if err := s.db.DeletePlaylist(pid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) sharePlaylistHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		UserID int64 `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	ownerID, _ := s.db.PlaylistOwner(pid)
	u := userFromContext(r.Context())
	if u != nil && ownerID != 0 && ownerID != u.ID && u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
		return
	}
	if err := s.db.SharePlaylist(pid, body.UserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) unsharePlaylistHandler(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	uid, _ := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	ownerID, _ := s.db.PlaylistOwner(pid)
	u := userFromContext(r.Context())
	if u != nil && ownerID != 0 && ownerID != u.ID && u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not owner"})
		return
	}
	if err := s.db.UnsharePlaylist(pid, uid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u, err := s.db.UserByUsername(body.Username)
	if err != nil || !checkPassword(body.Password, u.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	token := generateToken()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if _, err := s.db.CreateSession(u.ID, token, expiresAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": u})
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Auth-Token")
	if token != "" {
		_ = s.db.DeleteSession(token)
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) meHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": true, "user": u})
}

func (s *Server) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.Users()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
		return
	}
	if body.Role == "" {
		body.Role = "user"
	}
	hash, err := hashPassword(body.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, err := s.db.InsertUser(body.Username, string(hash), body.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	u := userFromContext(r.Context())
	if u != nil && u.ID == id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete self"})
		return
	}
	if err := s.db.DeleteUser(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) recordPlayHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TrackID          int64   `json:"trackId"`
		DurationListened float64 `json:"durationListened"`
		CompletionPct    float64 `json:"completionPct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u := userFromContext(r.Context())
	if _, err := s.db.InsertPlay(u.ID, body.TrackID, body.DurationListened, body.CompletionPct); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

func (s *Server) historyHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	plays, err := s.db.PlaysByUser(u.ID, offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, plays)
}

func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	stats, err := s.db.StatsForUser(u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) similarAlbumsHandler(w http.ResponseWriter, r *http.Request) {
	artistID, _ := strconv.ParseInt(r.URL.Query().Get("artistId"), 10, 64)
	excludeID, _ := strconv.ParseInt(r.URL.Query().Get("excludeAlbumId"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	albums, err := s.db.SimilarAlbums(artistID, excludeID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Server) similarArtistsHandler(w http.ResponseWriter, r *http.Request) {
	artistID, _ := strconv.ParseInt(r.URL.Query().Get("artistId"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	artists, err := s.db.SimilarArtists(artistID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, artists)
}

func (s *Server) recommendedTracksHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tracks, err := s.db.RecommendedTracks(u.ID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) radioHandler(w http.ResponseWriter, r *http.Request) {
	radioType := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var tracks []db.Track
	var err error
	switch radioType {
	case "genre":
		genre := r.URL.Query().Get("genre")
		tracks, err = s.db.RadioTracksByGenre(genre, limit)
	case "artist":
		artistID, _ := strconv.ParseInt(r.URL.Query().Get("artistId"), 10, 64)
		tracks, err = s.db.RadioTracksByArtist(artistID, limit)
	default:
		tracks, err = s.db.RadioTracksRandom(limit)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Server) genresHandler(w http.ResponseWriter, r *http.Request) {
	genres, err := s.db.Genres()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, genres)
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
		username := r.URL.Query().Get("u")
		password := r.URL.Query().Get("p")
		if username != "" && password != "" {
			if strings.HasPrefix(password, "enc:") {
				password = strings.TrimPrefix(password, "enc:")
			}
			if u, err := s.db.UserByUsername(username); err == nil && checkPassword(password, u.PasswordHash) {
				r = r.WithContext(context.WithValue(r.Context(), userContextKey, u))
			}
		}
		data := fn(w, r)
		resp := map[string]interface{}{
			"subsonic-response": map[string]interface{}{
				"status":        "ok",
				"version":       "1.16.1",
				"type":          "nightdrive",
				"serverVersion": "0.5.0",
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
	var pls []db.Playlist
	var err error
	if u := userFromContext(r.Context()); u != nil {
		pls, err = s.db.PlaylistsForUser(u.ID)
	} else {
		pls, err = s.db.PlaylistsForUser(0)
	}
	if err != nil {
		pls = []db.Playlist{}
	}
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

func (s *Server) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceName string `json:"deviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.DeviceName = "Unknown Device"
	}
	u := userFromContext(r.Context())
	userID := int64(0)
	if u != nil {
		userID = u.ID
	}
	token := generateToken()
	session, err := s.playback.CreateSession(userID, body.DeviceName, token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) listSessionsHandler(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.db.PlaybackSessions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) getSessionHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	session, err := s.playback.GetSession(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) deleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.playback.DeleteSession(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) sessionPlayHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		TrackID  int64   `json:"trackId"`
		Position float64 `json:"position"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.TrackID > 0 {
		if err := s.playback.SetTrack(id, body.TrackID, body.Position); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := s.playback.UpdateState(id, playback.StatePlaying, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "playing"})
}

func (s *Server) sessionPauseHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Position float64 `json:"position"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.playback.UpdateState(id, playback.StatePaused, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) sessionNextHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	track, err := s.playback.AdvanceTrack(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Server) sessionPrevHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	track, err := s.playback.PreviousTrack(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Server) sessionAddQueueHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		TrackID int64 `json:"trackId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.playback.AddToQueue(id, body.TrackID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

func (s *Server) sessionGetQueueHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	queue, err := s.playback.GetQueue(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

func (s *Server) sessionClearQueueHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.playback.ClearQueue(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) sessionVolumeHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Volume float64 `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.playback.SetVolume(id, body.Volume); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]float64{"volume": body.Volume})
}

func (s *Server) sessionCrossfadeHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Enabled     bool    `json:"enabled"`
		DurationSec float64 `json:"durationSec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	session, err := s.playback.GetSession(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	session.CrossfadeConfig.Enabled = body.Enabled
	session.CrossfadeConfig.DurationSec = body.DurationSec
	writeJSON(w, http.StatusOK, session.CrossfadeConfig)
}

func (s *Server) gaplessAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	albumID, _ := strconv.ParseInt(r.PathValue("albumId"), 10, 64)
	reports, err := s.gapless.AnalyzeAlbum(albumID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) gaplessReportHandler(w http.ResponseWriter, r *http.Request) {
	albumID, _ := strconv.ParseInt(r.PathValue("albumId"), 10, 64)
	reports, err := s.gapless.GetAlbumReport(albumID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	verified, issues, _ := s.gapless.VerifyGapless(albumID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"verified": verified,
		"issues":   issues,
		"reports":  reports,
	})
}

func (s *Server) gaplessBatchHandler(w http.ResponseWriter, r *http.Request) {
	go s.gapless.BatchAnalyze(nil)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "analyzing"})
}

func (s *Server) dspPresetsHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromContext(r.Context())
	userID := int64(0)
	if u != nil {
		userID = u.ID
	}
	presets, err := s.db.DSPPresets(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, presets)
}

func (s *Server) dspCreatePresetHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name                string  `json:"name"`
		EQLow               float64 `json:"eqLow"`
		EQMid               float64 `json:"eqMid"`
		EQHigh              float64 `json:"eqHigh"`
		CompressorThreshold float64 `json:"compressorThreshold"`
		CompressorRatio     float64 `json:"compressorRatio"`
		LoudnessTarget      float64 `json:"loudnessTarget"`
		IsDefault           bool    `json:"isDefault"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u := userFromContext(r.Context())
	userID := int64(0)
	if u != nil {
		userID = u.ID
	}
	id, err := s.db.InsertDSPPreset(body.Name, userID, body.EQLow, body.EQMid, body.EQHigh, body.CompressorThreshold, body.CompressorRatio, body.LoudnessTarget, body.IsDefault)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) dspPresetHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	preset, err := s.db.DSPPresetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "preset not found"})
		return
	}
	writeJSON(w, http.StatusOK, preset)
}

func (s *Server) dspDeletePresetHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.db.DeleteDSPPreset(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) dspProcessHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Samples    []float32 `json:"samples"`
		SampleRate int       `json:"sampleRate"`
		PresetID   int64     `json:"presetId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	pipeline := dsp.NewPipeline()
	if body.PresetID > 0 {
		preset, err := s.db.DSPPresetByID(body.PresetID)
		if err == nil {
			pipeline.ApplyPreset((*dsp.Preset)(preset))
		}
	}
	processed := pipeline.Process(body.Samples, body.SampleRate)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"samples":  processed,
		"loudness": dsp.MeasureLoudness(processed),
	})
}

func (s *Server) listSyncRoomsHandler(w http.ResponseWriter, r *http.Request) {
	rooms, err := s.syncCoord.ListRooms()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rooms)
}

func (s *Server) createSyncRoomHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	u := userFromContext(r.Context())
	ownerID := int64(0)
	if u != nil {
		ownerID = u.ID
	}
	token := generateToken()
	room, err := s.syncCoord.CreateRoom(body.Name, ownerID, token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, room)
}

func (s *Server) getSyncRoomHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	room, err := s.syncCoord.GetRoom(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "room not found"})
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (s *Server) deleteSyncRoomHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.syncCoord.DeleteRoom(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) joinSyncRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		SessionID int64 `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId required"})
		return
	}
	if err := s.syncCoord.JoinRoom(roomID, body.SessionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}

func (s *Server) leaveSyncRoomHandler(w http.ResponseWriter, r *http.Request) {
	roomID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		SessionID int64 `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SessionID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId required"})
		return
	}
	if err := s.syncCoord.LeaveRoom(roomID, body.SessionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
}

func (s *Server) syncRoomPlayHandler(w http.ResponseWriter, r *http.Request) {
	roomID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		TrackID int64 `json:"trackId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.syncCoord.StartRoomPlayback(roomID, body.TrackID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "playing"})
}

func (s *Server) syncRoomPauseHandler(w http.ResponseWriter, r *http.Request) {
	roomID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Position float64 `json:"position"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.syncCoord.PauseRoomPlayback(roomID, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) syncRoomSeekHandler(w http.ResponseWriter, r *http.Request) {
	roomID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Position float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "position required"})
		return
	}
	if err := s.syncCoord.SeekRoom(roomID, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]float64{"position": body.Position})
}

func (s *Server) castDevicesHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := s.castMgr.GetDevices()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) castDiscoverHandler(w http.ResponseWriter, r *http.Request) {
	devices, err := s.castMgr.Discover()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) castRegisterHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	device, err := s.castMgr.RegisterDevice(body.Name, cast.DeviceType(body.Type), body.Host, body.Port)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, device)
}

func (s *Server) castPlayHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		StreamURL string  `json:"streamUrl"`
		Title     string  `json:"title"`
		Position  float64 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.castMgr.CastToDevice(deviceID, body.StreamURL, body.Title, body.Position); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "casting"})
}

func (s *Server) castPauseHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.castMgr.Pause(deviceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) castStopHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.castMgr.StopPlayback(deviceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) castDeleteHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.castMgr.UnregisterDevice(deviceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
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
