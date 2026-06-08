package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds NightDrive configuration.
type Config struct {
	Listen      string   `toml:"listen"`
	Database    string   `toml:"database"`
	MusicDir    []string `toml:"music_dir"`
	ScanOnStart bool     `toml:"scan_on_start"`
	Transcode   struct {
		Enabled bool   `toml:"enabled"`
		Format  string `toml:"format"`
		Bitrate int    `toml:"bitrate"`
	} `toml:"transcode"`
}

// Default returns a default configuration.
func Default() *Config {
	return &Config{
		Listen:      "0.0.0.0:4040",
		Database:    "data/nightdrive.db",
		MusicDir:    []string{},
		ScanOnStart: true,
		Transcode: struct {
			Enabled bool   `toml:"enabled"`
			Format  string `toml:"format"`
			Bitrate int    `toml:"bitrate"`
		}{
			Enabled: false,
			Format:  "mp3",
			Bitrate: 320,
		},
	}
}

// Load reads a simple key=value / TOML-ish config file.
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		switch key {
		case "listen":
			cfg.Listen = val
		case "database":
			cfg.Database = val
		case "music_dir":
			cfg.MusicDir = append(cfg.MusicDir, val)
		case "scan_on_start":
			cfg.ScanOnStart = val == "true"
		}
	}
	return cfg, nil
}
