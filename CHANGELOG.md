# Changelog

All notable changes to NightDrive are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.0] - 2026-06-08

Release candidate: final API freeze, complete documentation, and a ready-to-run demo instance.

### Added

- Comprehensive README with full REST API documentation, request/response models, and usage examples.
- Demo instance configuration at `deploy/nightdrive.demo.toml` with all playback features enabled.
- This `CHANGELOG.md` documenting the v0.9.0 release.

### Documentation

- Documented every REST endpoint including auth, library, playlists, playback sessions, gapless analysis, DSP presets, sync rooms, cast devices, and admin routes.
- Documented Subsonic API compatibility and ID prefix conventions (`ar-`, `al-`, `tr-`, `pl-`).
- Added curl examples for login, search, smart playlists, streaming, playback sessions, history, scanning, and Subsonic clients.
- Added deployment guidance for Docker, Caddy, and nginx with a security checklist.

### Changed

- README refreshed to reflect current architecture, feature set, and roadmap.

### Fixed

- N/A — release candidate focused on documentation and packaging.

## [0.1.0] - 2023-01-01

Initial scaffold.

### Added

- Single binary Go backend.
- SQLite metadata with WAL mode.
- File scanner and fsnotify watcher.
- Smart playlists via query language.
- Gapless HTTP streaming.
- Subsonic API compatibility.
- Vanilla JS retro player UI.

[Unreleased]: https://github.com/synth/nightdrive/compare/v0.9.0...HEAD
[0.9.0]: https://github.com/synth/nightdrive/compare/v0.1.0...v0.9.0
[0.1.0]: https://github.com/synth/nightdrive/releases/tag/v0.1.0
