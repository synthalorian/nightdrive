package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Version is the current config schema version.
const CurrentVersion = 1

// Migrator handles upgrading older config files to the current schema.
type Migrator struct {
	version int
	steps   []MigrationStep
}

// MigrationStep defines a single config migration.
type MigrationStep struct {
	FromVersion int
	ToVersion   int
	Transform   func(map[string]string) map[string]string
}

// NewMigrator creates a migrator with built-in steps.
func NewMigrator() *Migrator {
	m := &Migrator{version: CurrentVersion}
	m.steps = []MigrationStep{
		{
			FromVersion: 0,
			ToVersion:   1,
			Transform:   migrateV0ToV1,
		},
	}
	return m
}

// Migrate reads raw config key-value pairs and upgrades them to the current version.
func (m *Migrator) Migrate(raw map[string]string) (map[string]string, int, error) {
	version := 0
	if vStr, ok := raw["config_version"]; ok {
		if v, err := strconv.Atoi(vStr); err == nil {
			version = v
		}
	}
	startVersion := version
	for _, step := range m.steps {
		if version == step.FromVersion {
			raw = step.Transform(raw)
			version = step.ToVersion
		}
	}
	raw["config_version"] = strconv.Itoa(m.version)
	return raw, startVersion, nil
}

// MigrateFile reads a config file, applies migrations, and returns the migrated content.
func (m *Migrator) MigrateFile(path string) (string, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("read config: %w", err)
	}
	raw := parseRaw(string(data))
	migrated, startVersion, err := m.Migrate(raw)
	if err != nil {
		return "", 0, err
	}
	if startVersion < m.version {
		// Write back migrated config.
		newData := serializeRaw(migrated)
		if err := os.WriteFile(path, []byte(newData), 0o644); err != nil {
			return "", 0, fmt.Errorf("write migrated config: %w", err)
		}
	}
	return serializeRaw(migrated), startVersion, nil
}

// parseRaw extracts key=value pairs from config text.
func parseRaw(text string) map[string]string {
	raw := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
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
		raw[key] = val
	}
	return raw
}

// serializeRaw converts key=value map back to config text.
func serializeRaw(raw map[string]string) string {
	var lines []string
	// Write version header first.
	if v, ok := raw["config_version"]; ok {
		lines = append(lines, fmt.Sprintf("config_version = %s", v))
		delete(raw, "config_version")
	}
	for k, v := range raw {
		lines = append(lines, fmt.Sprintf("%s = \"%s\"", k, v))
	}
	return strings.Join(lines, "\n") + "\n"
}

// migrateV0ToV1 adds default playback settings if missing.
func migrateV0ToV1(raw map[string]string) map[string]string {
	defaults := map[string]string{
		"crossfade_enabled":   "true",
		"crossfade_duration":  "5.0",
		"gapless_enabled":     "true",
		"dsp_pipeline_enabled": "true",
		"multi_room_enabled":  "true",
		"casting_enabled":     "true",
	}
	for k, v := range defaults {
		if _, ok := raw[k]; !ok {
			raw[k] = v
		}
	}
	return raw
}
