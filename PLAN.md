# NightDrive Roadmap

## v0.1.0 ✅ — Initial scaffold
- Single binary Go backend
- SQLite metadata with WAL
- File scanner + fsnotify watcher
- Smart playlists via query language
- Gapless playback
- Subsonic API compatibility
- Vanilla JS retro player UI

## v0.2.0 — Tag parsing
- [ ] ID3 tag parsing for MP3
- [ ] Vorbis comments for FLAC/OGG
- [ ] MP4 atom parsing for M4A
- [ ] Cover art extraction from tags
- [ ] Multi-value tag support (genres, artists)

## v0.3.0 — Transcoding & accounts
- [ ] FFmpeg transcoding pipeline
- [ ] User accounts + secure Subsonic auth
- [ ] Last.fm scrobbling
- [ ] ReplayGain normalization

## v0.4.0 — Phase 4: Mobile & visualizer
- [ ] Mobile PWA + offline caching
- [ ] Real-time audio visualizer via Web Audio API
- [ ] Lyrics support (LRU cached, embedded + API fallback)
- [ ] Podcast / audiobook support (bookmark position)
- [ ] Dark/light theme toggle

## v0.5.0 — Social & discovery
- [ ] Multi-user support with permissions
- [ ] Shared playlists
- [ ] Listening history and stats
- [ ] Similar artist / album recommendations
- [ ] Radio mode (shuffle by genre/mood)

## v0.6.0 — Advanced playback
- [ ] Crossfade between tracks
- [ ] Gapless album playback verification
- [ ] DSP pipeline (EQ, compressor, loudness)
- [ ] Multi-room sync playback
- [ ] Casting (Chromecast, AirPlay)

## v0.7.0 — Pre-release polish
- [ ] Comprehensive test suite
- [ ] CI/CD with GitHub Actions
- [ ] Docker image
- [ ] Reverse proxy config examples
- [ ] Performance benchmark

## v0.8.0 — Stability
- [ ] Large library stress test (100K+ tracks)
- [ ] Concurrent user load testing
- [ ] Database migration system
- [ ] Log rotation and monitoring
- [ ] Beta testing feedback integration

## v0.9.0 — Release candidate
- [ ] Final API freeze
- [ ] Documentation complete
- [ ] Demo instance
- [ ] Release notes draft

## v1.0.0 — Ship it
- [ ] Tag v1.0.0
- [ ] GitHub release with binaries
- [ ] Announcement post
- [ ] Screencast demo
- [ ] Community feedback channel
