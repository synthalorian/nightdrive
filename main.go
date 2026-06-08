package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

var (
	db         *DB
	scanner    *Scanner
	stream     *StreamHandler
	transcoder *Transcoder
)

type Config struct {
	MusicPath  string `json:"music_path"`
	DBPath     string `json:"db_path"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
}

func main() {
	cfg := Config{
		MusicPath:  "~/music",
		DBPath:     "nightdrive.db",
		Host:       "0.0.0.0",
		Port:       8080,
	}

	// Override music path from env if set
	if envPath := os.Getenv("NIGHTDRIVE_MUSIC"); envPath != "" {
		cfg.MusicPath = envPath
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			cfg.Port = port
		}
	}

	// Initialize database
	var err error
	db, err = NewDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize scanner
	scanner = NewScanner(db, cfg.MusicPath)

	// Initialize stream handler
	stream = NewStreamHandler(cfg.MusicPath)

	// Initialize transcoder
	transcoder = NewTranscoder(cfg.MusicPath)

	// Create default admin user if none exist
	admin, err := initDefaultUser()
	if err != nil {
		log.Printf("Warning: failed to create default user: %v", err)
	}
	if admin != nil {
		log.Printf("Default admin API key: %s", admin.APIKey)
	}

	mux := http.NewServeMux()

	// API endpoints (wrapped with auth middleware)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/scan", apiKeyAuth(handleScan))
	mux.HandleFunc("/api/stats", apiKeyAuth(handleStats))
	mux.HandleFunc("/api/artists", apiKeyAuth(handleArtists))
	mux.HandleFunc("/api/artists/", apiKeyAuth(handleArtistDetail))
	mux.HandleFunc("/api/albums/", apiKeyAuth(handleAlbumDetail))
	mux.HandleFunc("/api/tracks", apiKeyAuth(handleTracks))
	mux.HandleFunc("/api/tracks/", apiKeyAuth(handleTrackDetail))
	mux.HandleFunc("/api/search", apiKeyAuth(handleSearch))
	mux.Handle("/api/stream/", stream)
	mux.HandleFunc("/api/art/", handleAlbumArt)
	mux.Handle("/api/transcode/", transcoder)
	mux.HandleFunc("/api/playlists", apiKeyAuth(handlePlaylists))
	mux.HandleFunc("/api/playlists/", apiKeyAuth(handlePlaylistDetail))

	// Auth endpoints
	mux.HandleFunc("/api/auth/register", handleRegister)
	mux.HandleFunc("/api/auth/me", apiKeyAuth(handleMe))

	// Star endpoints
	mux.HandleFunc("/api/stars", apiKeyAuth(handleGetStars))

	// Subsonic API
	mux.HandleFunc("/subsonic/rest/", handleSubsonic)

	// Static files
	mux.HandleFunc("/", handleIndex)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("NightDrive running on http://%s", addr)
	log.Printf("Music library: %s", cfg.MusicPath)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, err := scanner.Scan()
	if err != nil {
		log.Printf("Scan error: %v", err)
		http.Error(w, fmt.Sprintf("Scan failed: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, result)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := db.GetStats()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, stats)
}

func handleArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := db.GetAllArtists()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get artists: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{"artists": artists})
}

func handleArtistDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/artists/")
	if path == "" {
		http.Error(w, "Artist ID required", http.StatusBadRequest)
		return
	}

	artistID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid artist ID", http.StatusBadRequest)
		return
	}

	albums, err := db.GetAlbumsByArtist(artistID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get albums: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{"albums": albums})
}

func handleAlbumDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/albums/")
	if path == "" {
		http.Error(w, "Album ID required", http.StatusBadRequest)
		return
	}

	albumID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid album ID", http.StatusBadRequest)
		return
	}

	tracks, err := db.GetTracksByAlbum(albumID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get tracks: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{"tracks": tracks})
}

func handleTracks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tracks, err := db.GetAllTracks(limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get tracks: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{"tracks": tracks})
}

func handleTrackDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tracks/")
	if path == "" {
		http.Error(w, "Track ID required", http.StatusBadRequest)
		return
	}

	// Check for star action
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 && parts[1] == "star" {
		// Let auth wrapper handle this, then star track
		user := getUserFromRequest(r)
		if user == nil {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		trackID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "Invalid track ID", http.StatusBadRequest)
			return
		}
		starred, err := db.ToggleStar(user.ID, trackID, "track")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to star track: %v", err), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]interface{}{"starred": starred})
		return
	}

	trackID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid track ID", http.StatusBadRequest)
		return
	}

	track, err := db.GetTrackByID(trackID)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, track)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		jsonResponse(w, map[string]interface{}{"tracks": []interface{}{}})
		return
	}

	tracks, err := db.SearchTracks(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{"tracks": tracks})
}

func handleAlbumArt(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/art/")
	if path == "" {
		http.Error(w, "Album ID required", http.StatusBadRequest)
		return
	}

	albumID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "Invalid album ID", http.StatusBadRequest)
		return
	}

	data, err := db.GetAlbumArt(albumID)
	if err != nil || data == nil || len(data) == 0 {
		http.NotFound(w, r)
		return
	}

	// Detect content type from first few bytes
	contentType := detectImageType(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func detectImageType(data []byte) string {
	if len(data) < 4 {
		return "image/jpeg"
	}
	// PNG magic: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	// JPEG magic: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	return "image/jpeg"
}

func handlePlaylists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user := getUserFromRequest(r)
		userID := int64(0)
		if user != nil {
			userID = user.ID
		}
		playlists, err := db.GetAllPlaylists(userID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get playlists: %v", err), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]interface{}{"playlists": playlists})

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			http.Error(w, "Name required", http.StatusBadRequest)
			return
		}
		user := getUserFromRequest(r)
		userID := int64(0)
		if user != nil {
			userID = user.ID
		}
		playlist, err := db.CreatePlaylist(userID, req.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create playlist: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, playlist)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePlaylistDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/playlists/")
	if path == "" {
		http.Error(w, "Playlist ID required", http.StatusBadRequest)
		return
	}

	// Parse ID and optional action
	parts := strings.SplitN(path, "/", 2)
	playlistID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist ID", http.StatusBadRequest)
		return
	}

	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		playlist, err := db.GetPlaylistByID(playlistID)
		if err != nil {
			http.Error(w, "Playlist not found", http.StatusNotFound)
			return
		}

		tracks, err := db.GetPlaylistTracks(playlistID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get tracks: %v", err), http.StatusInternalServerError)
			return
		}

		jsonResponse(w, map[string]interface{}{
			"playlist": playlist,
			"tracks":   tracks,
		})

	case http.MethodPut:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		playlist, err := db.UpdatePlaylist(playlistID, req.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to update playlist: %v", err), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, playlist)

	case http.MethodDelete:
		if action != "" {
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}
		if err := db.DeletePlaylist(playlistID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete playlist: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPost:
		// Handle /api/playlists/{id}/tracks
		if action != "tracks" {
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}
		var req struct {
			TrackID int64 `json:"track_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if err := db.AddTrackToPlaylist(playlistID, req.TrackID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to add track: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, map[string]string{"status": "added"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		log.Printf("Error reading index.html: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}
