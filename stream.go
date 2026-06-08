package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StreamHandler handles audio streaming with range request support
type StreamHandler struct {
	musicPath string
}

// NewStreamHandler creates a new stream handler
func NewStreamHandler(musicPath string) *StreamHandler {
	return &StreamHandler{musicPath: musicPath}
}

// ServeHTTP handles streaming requests
func (sh *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trackIDStr := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	if trackIDStr == "" {
		http.Error(w, "Track ID required", http.StatusBadRequest)
		return
	}

	trackID, err := strconv.ParseInt(trackIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid track ID", http.StatusBadRequest)
		return
	}

	// Check if transcoding is requested
	format := r.URL.Query().Get("format")
	if format != "" {
		// Redirect to transcode endpoint
		bitrate := r.URL.Query().Get("bitrate")
		if bitrate == "" {
			bitrate = "128"
		}
		targetURL := fmt.Sprintf("/api/transcode/%s?format=%s&bitrate=%s", trackIDStr, format, bitrate)
		http.Redirect(w, r, targetURL, http.StatusFound)
		return
	}

	// Get track from database
	track, err := db.GetTrackByID(trackID)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}
	if apiKey != "" {
		if user, err := db.GetUserByAPIKey(apiKey); err == nil {
			db.RecordScrobble(user.ID, track.ID)
			db.IncrementPlayCount(track.ID)
			if scrobbler != nil {
				scrobbler.Scrobble(user.ID, track.ID)
			}
		}
	}

	// Expand home directory in path
	path := track.Path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// If path is relative, prepend music directory
	if !filepath.IsAbs(path) {
		musicPath := sh.musicPath
		if strings.HasPrefix(musicPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				musicPath = filepath.Join(home, musicPath[2:])
			}
		}
		path = filepath.Join(musicPath, path)
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		log.Printf("Error opening file %s: %v", path, err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Get file info
	stat, err := file.Stat()
	if err != nil {
		log.Printf("Error stating file %s: %v", path, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fileSize := stat.Size()

	// Set content type based on file extension
	contentType := getContentType(path)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	// Handle range requests
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		sh.serveRange(w, r, file, fileSize, rangeHeader)
		return
	}

	// Serve full file
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, file)
}

func (sh *StreamHandler) serveRange(w http.ResponseWriter, r *http.Request, file *os.File, fileSize int64, rangeHeader string) {
	// Parse range header: "bytes=start-end"
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		http.Error(w, "Invalid range header", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		http.Error(w, "Invalid range header", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	var start, end int64

	// Parse start
	if parts[0] != "" {
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "Invalid range start", http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}

	// Parse end
	if parts[1] != "" {
		var err error
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.Error(w, "Invalid range end", http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}

	// Determine actual range
	if parts[0] == "" {
		// Suffix range: "-500" means last 500 bytes
		suffixLength := end
		start = fileSize - suffixLength
		end = fileSize - 1
	} else if parts[1] == "" {
		// Open-ended range: "500-" means from 500 to end
		end = fileSize - 1
	}

	// Validate range
	if start < 0 || start >= fileSize || end < start || end >= fileSize {
		http.Error(w, "Range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Seek to start position
	_, err := file.Seek(start, io.SeekStart)
	if err != nil {
		http.Error(w, "Seek error", http.StatusInternalServerError)
		return
	}

	contentLength := end - start + 1

	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.WriteHeader(http.StatusPartialContent)

	// Copy the requested range
	io.CopyN(w, file, contentLength)
}

func getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".wma":
		return "audio/x-ms-wma"
	case ".aac":
		return "audio/aac"
	default:
		return "audio/mpeg"
	}
}
