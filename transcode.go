package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TranscodeCacheEntry holds cached transcoded data
type TranscodeCacheEntry struct {
	Data      []byte
	CreatedAt time.Time
}

// Transcoder handles on-the-fly audio transcoding
type Transcoder struct {
	musicPath string
	cache     map[string]*TranscodeCacheEntry
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
}

// NewTranscoder creates a new transcoder
func NewTranscoder(musicPath string) *Transcoder {
	return &Transcoder{
		musicPath: musicPath,
		cache:     make(map[string]*TranscodeCacheEntry),
		cacheTTL:  24 * time.Hour,
	}
}

// cacheKey generates a cache key for a transcode request
func cacheKey(trackID int64, format string, bitrate int) string {
	return fmt.Sprintf("%d_%s_%d", trackID, format, bitrate)
}

// getCached returns cached transcoded data if available
func (t *Transcoder) getCached(key string) ([]byte, bool) {
	t.cacheMu.RLock()
	defer t.cacheMu.RUnlock()

	entry, ok := t.cache[key]
	if !ok {
		return nil, false
	}

	if time.Since(entry.CreatedAt) > t.cacheTTL {
		return nil, false
	}

	return entry.Data, true
}

// setCached stores transcoded data in cache
func (t *Transcoder) setCached(key string, data []byte) {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()

	t.cache[key] = &TranscodeCacheEntry{
		Data:      data,
		CreatedAt: time.Now(),
	}
}

// ServeHTTP handles transcode requests at /api/transcode/{id}
func (t *Transcoder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trackIDStr := strings.TrimPrefix(r.URL.Path, "/api/transcode/")
	if trackIDStr == "" {
		http.Error(w, "Track ID required", http.StatusBadRequest)
		return
	}

	trackID, err := strconv.ParseInt(trackIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid track ID", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "mp3"
	}
	format = strings.ToLower(format)

	if format != "mp3" && format != "opus" {
		http.Error(w, "Unsupported format. Use mp3 or opus", http.StatusBadRequest)
		return
	}

	bitrateStr := r.URL.Query().Get("bitrate")
	bitrate := 128
	if bitrateStr != "" {
		b, err := strconv.Atoi(bitrateStr)
		if err == nil && b > 0 {
			bitrate = b
		}
	}

	// Check cache
	key := cacheKey(trackID, format, bitrate)
	if cached, ok := t.getCached(key); ok {
		setTranscodeContentType(w, format)
		w.Header().Set("Content-Length", strconv.Itoa(len(cached)))
		w.WriteHeader(http.StatusOK)
		w.Write(cached)
		return
	}

	// Get track from database
	track, err := db.GetTrackByID(trackID)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	// Resolve path
	path := track.Path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		musicPath := t.musicPath
		if strings.HasPrefix(musicPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				musicPath = filepath.Join(home, musicPath[2:])
			}
		}
		path = filepath.Join(musicPath, path)
	}

	// Check ffmpeg availability
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		http.Error(w, "ffmpeg not found in PATH", http.StatusInternalServerError)
		return
	}

	// Build ffmpeg command
	var mimeType string
	var ffmpegFormat string
	if format == "mp3" {
		ffmpegFormat = "mp3"
		mimeType = "audio/mpeg"
	} else {
		ffmpegFormat = "opus"
		mimeType = "audio/opus"
	}

	cmd := exec.Command(ffmpegPath,
		"-i", path,
		"-f", ffmpegFormat,
		"-b:a", fmt.Sprintf("%dk", bitrate),
		"-map_metadata", "-1",
		"-vn",
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("ffmpeg stdout pipe error: %v", err)
		http.Error(w, "Transcoding failed", http.StatusInternalServerError)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("ffmpeg stderr pipe error: %v", err)
		http.Error(w, "Transcoding failed", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("ffmpeg start error: %v", err)
		http.Error(w, "Transcoding failed", http.StatusInternalServerError)
		return
	}

	// Read all output into memory for caching
	data, err := io.ReadAll(stdout)
	if err != nil {
		log.Printf("ffmpeg read error: %v", err)
		http.Error(w, "Transcoding failed", http.StatusInternalServerError)
		return
	}

	// Drain stderr and wait for completion
	stderrData, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		log.Printf("ffmpeg error: %v, stderr: %s", err, string(stderrData))
		http.Error(w, "Transcoding failed", http.StatusInternalServerError)
		return
	}

	// Cache the result
	t.setCached(key, data)

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func setTranscodeContentType(w http.ResponseWriter, format string) {
	if format == "mp3" {
		w.Header().Set("Content-Type", "audio/mpeg")
	} else {
		w.Header().Set("Content-Type", "audio/opus")
	}
}

// HasFfmpeg checks if ffmpeg is available
func HasFfmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
