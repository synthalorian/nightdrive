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
