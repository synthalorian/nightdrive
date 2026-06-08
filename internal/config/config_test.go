package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Listen != "0.0.0.0:4040" {
		t.Errorf("expected listen 0.0.0.0:4040, got %s", cfg.Listen)
	}
	if cfg.Database != "data/nightdrive.db" {
		t.Errorf("expected database data/nightdrive.db, got %s", cfg.Database)
	}
	if cfg.ScanOnStart != true {
		t.Error("expected ScanOnStart true")
	}
	if cfg.Transcode.Format != "mp3" {
		t.Errorf("expected transcode format mp3, got %s", cfg.Transcode.Format)
	}
	if cfg.Transcode.Bitrate != 320 {
		t.Errorf("expected transcode bitrate 320, got %d", cfg.Transcode.Bitrate)
	}
	if !cfg.Playback.CrossfadeEnabled {
		t.Error("expected CrossfadeEnabled true")
	}
	if cfg.Playback.CrossfadeDuration != 5.0 {
		t.Errorf("expected CrossfadeDuration 5.0, got %f", cfg.Playback.CrossfadeDuration)
	}
}

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")
	content := `listen = "127.0.0.1:8080"
database = "/tmp/test.db"
music_dir = "/music"
scan_on_start = false
crossfade_enabled = false
crossfade_duration = 2.5
gapless_enabled = false
dsp_pipeline_enabled = false
multi_room_enabled = false
casting_enabled = false
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Errorf("expected listen 127.0.0.1:8080, got %s", cfg.Listen)
	}
	if cfg.Database != "/tmp/test.db" {
		t.Errorf("expected database /tmp/test.db, got %s", cfg.Database)
	}
	if len(cfg.MusicDir) != 1 || cfg.MusicDir[0] != "/music" {
		t.Errorf("expected music_dir [/music], got %v", cfg.MusicDir)
	}
	if cfg.ScanOnStart {
		t.Error("expected ScanOnStart false")
	}
	if cfg.Playback.CrossfadeEnabled {
		t.Error("expected CrossfadeEnabled false")
	}
	if cfg.Playback.CrossfadeDuration != 2.5 {
		t.Errorf("expected CrossfadeDuration 2.5, got %f", cfg.Playback.CrossfadeDuration)
	}
	if cfg.Playback.GaplessEnabled {
		t.Error("expected GaplessEnabled false")
	}
	if cfg.Playback.DSPPipelineEnabled {
		t.Error("expected DSPPipelineEnabled false")
	}
	if cfg.Playback.MultiRoomEnabled {
		t.Error("expected MultiRoomEnabled false")
	}
	if cfg.Playback.CastingEnabled {
		t.Error("expected CastingEnabled false")
	}
}

func TestLoad_MultipleMusicDirs(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")
	content := `music_dir = "/music1"
music_dir = "/music2"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.MusicDir) != 2 {
		t.Errorf("expected 2 music dirs, got %d", len(cfg.MusicDir))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.toml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Should return defaults
	if cfg.Listen != "0.0.0.0:4040" {
		t.Errorf("expected defaults, got listen=%s", cfg.Listen)
	}
}

func TestLoad_CommentsAndBlankLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")
	content := `# This is a comment
; This is also a comment

listen = "127.0.0.1:9090"

# Another comment
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Listen != "127.0.0.1:9090" {
		t.Errorf("expected listen 127.0.0.1:9090, got %s", cfg.Listen)
	}
}

func TestLoad_SectionsIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")
	content := `[transcode]
enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Section headers should be ignored, transcode.enabled not parsed
	if cfg.Transcode.Enabled {
		t.Error("expected transcode enabled to remain default (false)")
	}
}
