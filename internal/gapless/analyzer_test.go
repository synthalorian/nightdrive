package gapless

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nightdrive/internal/db"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return database
}

func TestNewAnalyzer(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)
	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
}

func TestIsPCMFormat(t *testing.T) {
	tests := []struct {
		format   string
		expected bool
	}{
		{"wav", true},
		{"WAV", true},
		{"flac", true},
		{"FLAC", true},
		{"aiff", true},
		{"mp3", false},
		{"ogg", false},
		{"m4a", false},
	}

	for _, tt := range tests {
		result := isPCMFormat(tt.format)
		if result != tt.expected {
			t.Errorf("isPCMFormat(%q) = %v, want %v", tt.format, result, tt.expected)
		}
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("expected boolToInt(true) = 1")
	}
	if boolToInt(false) != 0 {
		t.Error("expected boolToInt(false) = 0")
	}
}

func TestEstimateEncoderDelay(t *testing.T) {
	tests := []struct {
		format   string
		expected int
	}{
		{"mp3", 529},
		{"aac", 2112},
		{"m4a", 2112},
		{"ogg", 0},
		{"opus", 0},
		{"flac", 0},
		{"wav", 0},
		{"unknown", 0},
	}

	for _, tt := range tests {
		result := EstimateEncoderDelay(tt.format)
		if result != tt.expected {
			t.Errorf("EstimateEncoderDelay(%q) = %d, want %d", tt.format, result, tt.expected)
		}
	}
}

func TestAnalyzeAlbum_SingleTrack(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")
	track := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Only Track",
		Path:     "/music/track.mp3",
		Format:   "mp3",
	}
	database.InsertTrack(track)

	reports, err := analyzer.AnalyzeAlbum(albumID)
	if err != nil {
		t.Fatalf("analyze album: %v", err)
	}
	if reports != nil && len(reports) != 0 {
		t.Errorf("expected nil/empty reports for single track album, got %d", len(reports))
	}
}

func TestGetAlbumReport_Empty(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")

	reports, err := analyzer.GetAlbumReport(albumID)
	if err != nil {
		t.Fatalf("get album report: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestVerifyGapless(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album", 2024, "")

	verified, issues, err := analyzer.VerifyGapless(albumID)
	if err != nil {
		t.Fatalf("verify gapless: %v", err)
	}
	if verified {
		t.Error("expected not verified for empty album")
	}
	if issues != 0 {
		t.Errorf("expected 0 issues, got %d", issues)
	}
}

func TestBatchAnalyze(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)

	artistID, _ := database.InsertArtist("Artist")
	albumID, _ := database.InsertAlbum(artistID, "Album 1", 2024, "")
	track1 := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track 1",
		Path:     "/music/track1.mp3",
		Format:   "mp3",
	}
	track2 := &db.Track{
		AlbumID:  &albumID,
		ArtistID: &artistID,
		Title:    "Track 2",
		Path:     "/music/track2.mp3",
		Format:   "mp3",
	}
	database.InsertTrack(track1)
	database.InsertTrack(track2)

	progressCalled := false
	err := analyzer.BatchAnalyze(func(title string, done, total int) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("batch analyze: %v", err)
	}
	if !progressCalled {
		t.Error("expected progress callback to be called")
	}
}

func TestWriteWAVHeader(t *testing.T) {
	var buf bytes.Buffer
	err := WriteWAVHeader(&buf, 44100, 2, 16, 176400)
	if err != nil {
		t.Fatalf("write WAV header: %v", err)
	}

	header := buf.Bytes()
	if len(header) != 44 {
		t.Errorf("expected 44 byte header, got %d", len(header))
	}
	if string(header[0:4]) != "RIFF" {
		t.Errorf("expected RIFF signature, got %s", string(header[0:4]))
	}
	if string(header[8:12]) != "WAVE" {
		t.Errorf("expected WAVE signature, got %s", string(header[8:12]))
	}
}

func TestStreamCrossfadeBuffer(t *testing.T) {
	data1 := bytes.NewReader(make([]byte, 4096))
	data2 := bytes.NewReader(make([]byte, 4096))

	result, err := StreamCrossfadeBuffer(data1, data2, 44100, 2, 1000)
	if err != nil {
		t.Fatalf("stream crossfade: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestStreamCrossfadeBuffer_ZeroDuration(t *testing.T) {
	data1 := bytes.NewReader(make([]byte, 4096))
	data2 := bytes.NewReader(make([]byte, 4096))

	result, err := StreamCrossfadeBuffer(data1, data2, 44100, 2, 0)
	if err != nil {
		t.Fatalf("stream crossfade: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for zero duration")
	}
}

func TestCreateGaplessStream(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "track1.mp3")
	file2 := filepath.Join(tmpDir, "track2.mp3")
	os.WriteFile(file1, []byte("audio data 1"), 0644)
	os.WriteFile(file2, []byte("audio data 2"), 0644)

	var buf bytes.Buffer
	err := CreateGaplessStream(file1, file2, &buf)
	if err != nil {
		t.Fatalf("create gapless stream: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestAnalyzeWAVSilence(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	analyzer := NewAnalyzer(database)

	// Create a minimal valid WAV file
	tmpDir := t.TempDir()
	wavFile := filepath.Join(tmpDir, "test.wav")

	f, err := os.Create(wavFile)
	if err != nil {
		t.Fatalf("create wav: %v", err)
	}

	// Write WAV header
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	header[16] = 16 // fmt chunk size
	header[20] = 1  // PCM
	header[22] = 2  // channels
	header[24] = 0x44 // sample rate 44100 (little endian)
	header[25] = 0xAC
	header[26] = 0
	header[27] = 0
	header[32] = 4  // bytes per frame
	header[34] = 16 // bits per sample
	copy(header[36:40], "data")
	header[40] = 0x00 // data size
	header[41] = 0x10
	header[42] = 0
	header[43] = 0

	f.Write(header)
	// Write some silent PCM data
	f.Write(make([]byte, 4096))
	f.Close()

	// Test analyzing start silence
	ms, err := analyzer.analyzeWAVSilence(wavFile, false)
	if err != nil {
		t.Fatalf("analyze start silence: %v", err)
	}
	// Should detect silence
	_ = ms
}

func TestIsSilentSample(t *testing.T) {
	// 8-bit sample at center (128 = 0)
	data8 := []byte{128}
	if !isSilentSample(data8, 1, 64) {
		t.Error("expected 8-bit center sample to be silent")
	}

	// 16-bit sample at 0
	data16 := []byte{0, 0}
	if !isSilentSample(data16, 2, 64) {
		t.Error("expected 16-bit zero sample to be silent")
	}

	// 16-bit loud sample
	dataLoud := []byte{0xFF, 0x7F}
	if isSilentSample(dataLoud, 2, 64) {
		t.Error("expected loud sample to not be silent")
	}

	// Unsupported sample size
	data24 := []byte{0, 0, 0}
	if isSilentSample(data24, 3, 64) {
		t.Error("expected unsupported sample size to not be silent")
	}
}
