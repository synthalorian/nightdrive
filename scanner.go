package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
)

// Supported audio formats
var audioExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".ogg":  true,
	".opus": true,
	".m4a":  true,
	".wav":  true,
	".wma":  true,
	".aac":  true,
}

// Scanner handles music library scanning
type Scanner struct {
	db        *DB
	musicPath string
}

// NewScanner creates a new scanner instance
func NewScanner(db *DB, musicPath string) *Scanner {
	return &Scanner{
		db:        db,
		musicPath: musicPath,
	}
}

// ScanResult contains the results of a library scan
type ScanResult struct {
	ArtistsAdded  int       `json:"artists_added"`
	AlbumsAdded   int       `json:"albums_added"`
	TracksAdded   int       `json:"tracks_added"`
	TracksUpdated int       `json:"tracks_updated"`
	ArtExtracted  int       `json:"art_extracted"`
	Errors        []string  `json:"errors"`
	Duration      float64   `json:"duration_seconds"`
	ScannedAt     time.Time `json:"scanned_at"`
}

// Scan walks the music directory and populates the database
func (s *Scanner) Scan() (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{
		ScannedAt: time.Now(),
		Errors:    []string{},
	}

	// Expand home directory
	musicPath := s.musicPath
	if strings.HasPrefix(musicPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			musicPath = filepath.Join(home, musicPath[2:])
		}
	}

	// Check if directory exists
	info, err := os.Stat(musicPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("music directory does not exist: %s", musicPath)
		}
		return nil, fmt.Errorf("stat music directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("music path is not a directory: %s", musicPath)
	}

	log.Printf("Scanning music library: %s", musicPath)

	// Walk the directory tree
	err = filepath.WalkDir(musicPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("walk error at %s: %v", path, err))
			return nil // continue walking
		}

		if d.IsDir() {
			return nil
		}

		// Check if it's an audio file
		ext := strings.ToLower(filepath.Ext(path))
		if !audioExtensions[ext] {
			return nil
		}

		if err := s.processTrack(path, musicPath, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("process %s: %v", path, err))
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	result.Duration = time.Since(start).Seconds()
	log.Printf("Scan complete: %d artists, %d albums, %d tracks, %d art extracted in %.2fs",
		result.ArtistsAdded, result.AlbumsAdded, result.TracksAdded, result.ArtExtracted, result.Duration)

	return result, nil
}

func (s *Scanner) processTrack(path, basePath string, result *ScanResult) error {
	// Get relative path for cleaner display
	relPath, err := filepath.Rel(basePath, path)
	if err != nil {
		relPath = path
	}

	// Parse directory structure: artist/album/track.ext or artist - album/track.ext
	artistName, albumTitle, trackTitle, trackNum := parsePath(relPath)

	// Read metadata tags
	metadata := readMetadata(path)
	if metadata.Title != "" {
		trackTitle = metadata.Title
	}
	if metadata.Artist != "" {
		artistName = metadata.Artist
	}
	if metadata.Album != "" {
		albumTitle = metadata.Album
	}
	if metadata.TrackNum > 0 {
		trackNum = metadata.TrackNum
	}

	// Get or create artist
	artist, err := s.db.GetOrCreateArtist(artistName)
	if err != nil {
		return fmt.Errorf("create artist: %w", err)
	}

	// Get or create album
	album, err := s.db.GetOrCreateAlbum(artist.ID, albumTitle, metadata.Year)
	if err != nil {
		return fmt.Errorf("create album: %w", err)
	}

	// Extract album art if available and album doesn't have art yet
	if metadata.CoverArt != nil && len(metadata.CoverArt) > 0 {
		existingArt, _ := s.db.GetAlbumArt(album.ID)
		if existingArt == nil || len(existingArt) == 0 {
			if err := s.db.UpdateAlbumCoverArt(album.ID, metadata.CoverArt); err == nil {
				result.ArtExtracted++
			}
		}
	}

	// Determine format from extension
	format := strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))

	// Insert/update track
	track, err := s.db.InsertTrack(album.ID, artist.ID, trackTitle, path, trackNum, metadata.Duration, format, metadata.Bitrate)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}

	if track.CreatedAt.After(result.ScannedAt.Add(-time.Second)) {
		result.TracksAdded++
	} else {
		result.TracksUpdated++
	}

	if metadata.Lyrics != "" {
		s.db.SaveLyrics(track.ID, metadata.Lyrics, "embedded", false)
	}

	_, err = s.db.GetLyrics(track.ID)
	if err != nil {
		lrclibData, err := lyricsFetcher(artistName, trackTitle, albumTitle)
		if err == nil && lrclibData != nil && lrclibData.PlainLyrics != "" {
			s.db.SaveLyrics(track.ID, lrclibData.PlainLyrics, "lrclib", lrclibData.SyncedLyrics != "")
		}
	}

	result.ArtistsAdded = 0
	result.AlbumsAdded = 0

	return nil
}

// TrackMetadata holds extracted metadata from audio files
type TrackMetadata struct {
	Title    string
	Artist   string
	Album    string
	TrackNum int
	Year     int
	Duration int
	Bitrate  int
	CoverArt []byte
	Lyrics   string
}

// readMetadata reads metadata tags from an audio file
func readMetadata(path string) TrackMetadata {
	var meta TrackMetadata

	f, err := os.Open(path)
	if err != nil {
		return meta
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return meta
	}

	meta.Title = m.Title()
	meta.Artist = m.Artist()
	meta.Album = m.Album()

	trackNum, _ := m.Track()
	if trackNum > 0 {
		meta.TrackNum = trackNum
	}

	meta.Year = m.Year()

	// Extract cover art
	pic := m.Picture()
	if pic != nil {
		meta.CoverArt = pic.Data
	}

	meta.Lyrics = extractLyricsFromTags(m)

	return meta
}

func extractLyricsFromTags(m tag.Metadata) string {
	// dhowden/tag doesn't expose lyrics directly
	return ""
}

type lrclibResponse struct {
	ID           int     `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

// lyricsFetcher fetches lyrics from the network. It is a variable so tests
// can stub it out and stay hermetic.
var lyricsFetcher = fetchLyricsFromLRCLIB

func fetchLyricsFromLRCLIB(artist, title, album string) (*lrclibResponse, error) {
	baseURL := "https://lrclib.net/api/get"
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", title)
	if album != "" {
		params.Set("album_name", album)
	}

	reqURL := baseURL + "?" + params.Encode()
	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LRCLIB returned status %d", resp.StatusCode)
	}

	var result lrclibResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// parsePath extracts artist, album, and track info from a file path
// Expected structures:
//
//	artist/album/01 - track.mp3
//	artist/album/track.mp3
//	artist - album/track.mp3
func parsePath(relPath string) (artist, album, track string, trackNum int) {
	dir := filepath.Dir(relPath)
	base := filepath.Base(relPath)

	// Remove extension from track name
	track = strings.TrimSuffix(base, filepath.Ext(base))

	// Try to extract track number: "01 - Track Name" or "01. Track Name"
	trackNum = 0
	if idx := strings.Index(track, " - "); idx > 0 {
		if num, err := strconv.Atoi(strings.TrimSpace(track[:idx])); err == nil {
			trackNum = num
			track = strings.TrimSpace(track[idx+3:])
		}
	}
	if idx := strings.Index(track, ". "); idx > 0 {
		if num, err := strconv.Atoi(strings.TrimSpace(track[:idx])); err == nil {
			trackNum = num
			track = strings.TrimSpace(track[idx+2:])
		}
	}

	// Parse directory structure
	dirParts := strings.Split(dir, string(filepath.Separator))
	switch len(dirParts) {
	case 0, 1:
		// Just a file or single directory - use directory as artist
		if len(dirParts) == 1 && dirParts[0] != "." {
			artist = dirParts[0]
			album = "Unknown Album"
		} else {
			artist = "Unknown Artist"
			album = "Unknown Album"
		}
	case 2:
		// artist/album/
		artist = dirParts[0]
		album = dirParts[1]
	default:
		// Handle "artist - album" pattern in directory names
		artist = dirParts[0]
		album = dirParts[1]

		// Check if first directory contains " - " separator
		if idx := strings.Index(dirParts[0], " - "); idx > 0 {
			artist = strings.TrimSpace(dirParts[0][:idx])
			album = strings.TrimSpace(dirParts[0][idx+3:])
		}
	}

	// Clean up names
	artist = cleanName(artist)
	album = cleanName(album)
	track = cleanName(track)

	return artist, album, track, trackNum
}

// cleanName removes common noise from names
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	// Remove common prefixes/suffixes
	name = strings.TrimPrefix(name, "[")
	name = strings.TrimSuffix(name, "]")
	name = strings.TrimPrefix(name, "(")
	name = strings.TrimSuffix(name, ")")
	return strings.TrimSpace(name)
}

// QuickScan performs a fast scan that only adds new files
func (s *Scanner) QuickScan() (*ScanResult, error) {
	return s.Scan() // For now, same as full scan (upsert handles updates)
}
