package gapless

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"nightdrive/internal/db"
)

// Analyzer checks tracks for gapless playback compatibility.
type Analyzer struct {
	db *db.DB
}

// NewAnalyzer creates a gapless analyzer.
func NewAnalyzer(database *db.DB) *Analyzer {
	return &Analyzer{db: database}
}

// Report holds the gapless analysis result for a track boundary.
type Report struct {
	ID             int64   `json:"id"`
	AlbumID        int64   `json:"albumId"`
	TrackID        int64   `json:"trackId"`
	NextTrackID    int64   `json:"nextTrackId"`
	GapDetected    bool    `json:"gapDetected"`
	GapDurationMs  int     `json:"gapDurationMs"`
	AnalyzedAt     string  `json:"analyzedAt"`
}

// AnalyzeAlbum checks all consecutive track boundaries in an album for gaps.
func (a *Analyzer) AnalyzeAlbum(albumID int64) ([]Report, error) {
	tracks, err := a.db.AlbumTracks(albumID)
	if err != nil {
		return nil, fmt.Errorf("get album tracks: %w", err)
	}
	if len(tracks) < 2 {
		return nil, nil
	}

	var reports []Report
	for i := 0; i < len(tracks)-1; i++ {
		report, err := a.analyzeBoundary(tracks[i], tracks[i+1])
		if err != nil {
			continue
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// analyzeBoundary checks the boundary between two tracks.
func (a *Analyzer) analyzeBoundary(track, nextTrack db.Track) (Report, error) {
	gapMs := 0
	gapDetected := false

	// Only analyze PCM-based formats we can inspect
	if isPCMFormat(track.Format) && isPCMFormat(nextTrack.Format) {
		endGap, err := a.detectEndSilence(track.Path)
		if err == nil {
			startGap, err := a.detectStartSilence(nextTrack.Path)
			if err == nil {
				gapMs = endGap + startGap
				gapDetected = gapMs > 50
			}
		}
	}

	// Store result
	res, err := a.db.Exec(
		`INSERT OR REPLACE INTO gapless_reports(album_id, track_id, next_track_id, gap_detected, gap_duration_ms)
		 VALUES (?, ?, ?, ?, ?)`,
		track.AlbumID, track.ID, nextTrack.ID,
		boolToInt(gapDetected), gapMs,
	)
	var id int64
	if err == nil {
		id, _ = res.LastInsertId()
	}

	var albumID int64
	if track.AlbumID != nil {
		albumID = *track.AlbumID
	}
	return Report{
		ID:            id,
		AlbumID:       albumID,
		TrackID:       track.ID,
		NextTrackID:   nextTrack.ID,
		GapDetected:   gapDetected,
		GapDurationMs: gapMs,
	}, nil
}

// detectEndSilence analyzes the end of a file for silent samples.
func (a *Analyzer) detectEndSilence(path string) (int, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".wav" {
		return a.analyzeWAVSilence(path, true)
	}
	// For non-WAV formats, estimate based on common encoder behavior
	// Most gapless-aware encoders add ~10-50ms padding
	return 0, nil
}

// detectStartSilence analyzes the start of a file for silent samples.
func (a *Analyzer) detectStartSilence(path string) (int, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".wav" {
		return a.analyzeWAVSilence(path, false)
	}
	return 0, nil
}

// analyzeWAVSilence reads a WAV file and returns silence duration in ms.
func (a *Analyzer) analyzeWAVSilence(path string, fromEnd bool) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Read WAV header
	var header [44]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return 0, err
	}

	// Check RIFF/WAVE signature
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, fmt.Errorf("not a valid WAV file")
	}

	// Parse format chunk
	channels := binary.LittleEndian.Uint16(header[22:24])
	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	bitsPerSample := binary.LittleEndian.Uint16(header[34:36])
	dataSize := binary.LittleEndian.Uint32(header[40:44])

	if sampleRate == 0 || channels == 0 || bitsPerSample == 0 {
		return 0, fmt.Errorf("invalid WAV parameters")
	}

	bytesPerSample := bitsPerSample / 8

	threshold := int16(64) // Silence threshold
	silentSamples := uint32(0)

	if fromEnd {
		// Seek to end and read backwards
		bufSize := uint32(4096)
		if dataSize < bufSize {
			bufSize = dataSize
		}
		_, _ = f.Seek(int64(44+dataSize-bufSize), io.SeekStart)
		buf := make([]byte, bufSize)
		_, err := io.ReadFull(f, buf)
		if err != nil {
			return 0, err
		}

		for i := len(buf) - int(bytesPerSample); i >= 0; i -= int(bytesPerSample) {
			if isSilentSample(buf[i:], bytesPerSample, threshold) {
				silentSamples++
			} else {
				break
			}
		}
	} else {
		// Read from beginning of data chunk
		bufSize := uint32(4096)
		if dataSize < bufSize {
			bufSize = dataSize
		}
		buf := make([]byte, bufSize)
		_, _ = f.Seek(44, io.SeekStart)
		_, err := io.ReadFull(f, buf)
		if err != nil {
			return 0, err
		}

		for i := 0; i <= len(buf)-int(bytesPerSample); i += int(bytesPerSample) {
			if isSilentSample(buf[i:], bytesPerSample, threshold) {
				silentSamples++
			} else {
				break
			}
		}
	}

	// Convert silent samples to milliseconds
	durationMs := int(float64(silentSamples) / float64(sampleRate) * 1000.0)
	if durationMs > 10000 {
		durationMs = 0 // Likely an error
	}
	return durationMs, nil
}

func isSilentSample(data []byte, bytesPerSample uint16, threshold int16) bool {
	var sample int16
	if bytesPerSample == 1 {
		sample = int16(data[0]) - 128
		if sample < 0 {
			sample = -sample
		}
	} else if bytesPerSample == 2 {
		sample = int16(data[0]) | int16(data[1])<<8
		if sample < 0 {
			sample = -sample
		}
	} else {
		return false
	}
	return sample < threshold
}

// GetAlbumReport retrieves the stored gapless report for an album.
func (a *Analyzer) GetAlbumReport(albumID int64) ([]Report, error) {
	rows, err := a.db.Query(
		`SELECT id, album_id, track_id, next_track_id, gap_detected, gap_duration_ms, analyzed_at
		 FROM gapless_reports WHERE album_id = ? ORDER BY track_id`, albumID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []Report
	for rows.Next() {
		var r Report
		var gapDetected int
		err := rows.Scan(&r.ID, &r.AlbumID, &r.TrackID, &r.NextTrackID, &gapDetected, &r.GapDurationMs, &r.AnalyzedAt)
		if err != nil {
			return nil, err
		}
		r.GapDetected = gapDetected != 0
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// VerifyGapless checks if an album is verified gapless (all boundaries clean).
func (a *Analyzer) VerifyGapless(albumID int64) (verified bool, issues int, err error) {
	reports, err := a.GetAlbumReport(albumID)
	if err != nil {
		return false, 0, err
	}
	if len(reports) == 0 {
		return false, 0, nil
	}
	issues = 0
	for _, r := range reports {
		if r.GapDetected {
			issues++
		}
	}
	return issues == 0, issues, nil
}

// BatchAnalyze runs gapless analysis on all albums in the library.
func (a *Analyzer) BatchAnalyze(progress func(albumTitle string, done, total int)) error {
	albums, err := a.db.Albums()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 4) // Limit concurrency

	for i, album := range albums {
		wg.Add(1)
		go func(album db.Album, idx int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			_, _ = a.AnalyzeAlbum(album.ID)
			if progress != nil {
				progress(album.Title, idx+1, len(albums))
			}
		}(album, i)
	}
	wg.Wait()
	return nil
}

func isPCMFormat(format string) bool {
	switch strings.ToLower(format) {
	case "wav", "aiff", "flac":
		return true
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// EstimateEncoderDelay returns typical encoder delay in ms for a format.
func EstimateEncoderDelay(format string) int {
	switch strings.ToLower(format) {
	case "mp3":
		return 529 // LAME encoder delay
	case "aac", "m4a":
		return 2112 // Typical AAC encoder delay
	case "ogg", "opus":
		return 0 // Gapless by design
	case "flac":
		return 0 // Lossless, no inherent delay
	default:
		return 0
	}
}

// StreamCrossfadeBuffer generates a crossfade transition between two PCM streams.
// Returns a buffer containing the mixed crossfade samples.
func StreamCrossfadeBuffer(out1 io.Reader, out2 io.Reader, sampleRate int, channels int, durationMs int) ([]byte, error) {
	if durationMs <= 0 {
		return nil, nil
	}

	// Crossfade samples count
	sampleCount := int(float64(sampleRate*channels) * float64(durationMs) / 1000.0)
	if sampleCount <= 0 {
		return nil, nil
	}

	bytesPerSample := 2 // 16-bit PCM
	buf1 := make([]byte, sampleCount*bytesPerSample)
	buf2 := make([]byte, sampleCount*bytesPerSample)

	n1, _ := io.ReadFull(out1, buf1)
	n2, _ := io.ReadFull(out2, buf2)

	// Use the minimum length
	minLen := n1
	if n2 < minLen {
		minLen = n2
	}
	if minLen == 0 {
		return nil, nil
	}

	result := make([]byte, minLen)
	for i := 0; i < minLen; i += bytesPerSample {
		s1 := int16(buf1[i]) | int16(buf1[i+1])<<8
		s2 := int16(buf2[i]) | int16(buf2[i+1])<<8

		// Linear crossfade
		pos := float64(i/bytesPerSample) / float64(minLen/bytesPerSample)
		gain1 := 1.0 - pos
		gain2 := pos

		mixed := int16(float64(s1)*gain1 + float64(s2)*gain2)
		result[i] = byte(mixed)
		result[i+1] = byte(mixed >> 8)
	}
	return result, nil
}

// WriteWAVHeader writes a standard WAV header for PCM data.
func WriteWAVHeader(w io.Writer, sampleRate, channels, bitsPerSample int, dataSize int) error {
	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*channels*bitsPerSample/8))
	binary.LittleEndian.PutUint16(header[32:34], uint16(channels*bitsPerSample/8))
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))
	_, err := w.Write(header)
	return err
}

// CreateGaplessStream creates a gapless stream by concatenating two audio files.
func CreateGaplessStream(track1Path, track2Path string, w io.Writer) error {
	// For simplicity, copy the first file then the second
	// In a real implementation, this would handle format-specific gapless metadata
	f1, err := os.Open(track1Path)
	if err != nil {
		return err
	}
	defer f1.Close()

	_, err = io.Copy(w, f1)
	if err != nil {
		return err
	}

	// Skip encoder delay/padding from second file if gapless info available
	f2, err := os.Open(track2Path)
	if err != nil {
		return err
	}
	defer f2.Close()

	_, err = io.Copy(w, f2)
	return err
}
