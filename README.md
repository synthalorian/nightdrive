# 🌙 NightDrive

A self-hosted music streaming server designed for large local libraries. No metadata scraping from online services — pure file organization.

## Features

- 🎵 **Music Streaming** — HTTP audio streaming with range request support
- 📂 **File-based** — no metadata wars, your organization is law
- 🗄️ **SQLite Database** — fast, reliable artist/album/track storage
- 🔍 **Library Scanner** — recursively scans your music directory
- 📋 **Playlist CRUD** — create, read, update, delete playlists
- 🎛️ **Smart playlists** — rules-based, auto-updating (recently added, most played, genre, year, random)
- 👁️ **File watcher** — recursive library watch with debounced auto-scan on changes
- 📡 **Subsonic API** — compatible with existing clients (ping, getAlbumList, getStarred, getPlaylists, scrobble)
- 🎨 **Retro player UI** — single-page web app with synthwave aesthetic
- 🚀 **Single binary** — Go, minimal dependencies
- 🔐 **Multi-user support** — API key auth with per-user playlists and stars
- 🎨 **Album art extraction** — extracts cover art from audio file metadata
- 🔧 **On-the-fly transcoding** — MP3/Opus transcoding via ffmpeg with caching

## Install

```bash
go build -o nightdrive .
```

## Usage

```bash
./nightdrive                          # Start server (default config)
NIGHTDRIVE_MUSIC=/path/to/music ./nightdrive  # Custom music path
PORT=8081 ./nightdrive               # Custom port
NIGHTDRIVE_WATCH=true ./nightdrive   # Auto-scan library on file changes
```

Then open `http://localhost:8080` in your browser.

The server creates a default admin user on first startup. The admin API key is printed in the logs.

## API Endpoints

### Core
- `GET /api/health` — Server health check
- `POST /api/scan` — Scan music library
- `GET /api/stats` — Library statistics
- `GET /api/artists` — List all artists
- `GET /api/artists/{id}` — Get artist's albums
- `GET /api/albums/{id}` — Get album's tracks
- `GET /api/tracks` — List all tracks
- `GET /api/tracks/{id}` — Get track details
- `POST /api/tracks/{id}/star` — Star/unstar a track
- `GET /api/search?q=query` — Search tracks
- `GET /api/stream/{id}` — Stream audio file (supports range requests, optional `?format=mp3|opus&bitrate=128` for transcoding)
- `GET /api/art/{albumId}` — Get album cover art
- `GET /api/transcode/{id}?format=mp3|opus&bitrate=128` — Transcode audio on-the-fly

### Playlists
- `GET /api/playlists` — List playlists (user-scoped)
- `POST /api/playlists` — Create playlist
- `GET /api/playlists/{id}` — Get playlist with tracks
- `PUT /api/playlists/{id}` — Update playlist
- `DELETE /api/playlists/{id}` — Delete playlist
- `POST /api/playlists/{id}/tracks` — Add track to playlist

### Smart Playlists
- `GET /api/smart-playlists` — List smart playlists
- `POST /api/smart-playlists` — Create smart playlist (`rule_type`: `recently_added`, `most_played`, `genre`, `year`, `random`)
- `GET /api/smart-playlists/{id}` — Get smart playlist with resolved tracks
- `PUT /api/smart-playlists/{id}` — Update smart playlist
- `DELETE /api/smart-playlists/{id}` — Delete smart playlist

### Auth
- `POST /api/auth/register` — Create user with username, returns api_key
- `GET /api/auth/me` — Return current user info
- `GET /api/stars` — Get starred items for current user

### Subsonic API
- `GET /subsonic/rest/ping.view` — Subsonic ping
- `GET /subsonic/rest/getLicense.view` — Get license info
- `GET /subsonic/rest/getAlbumList.view?type=newest|alphabeticalByArtist|alphabeticalByName|byYear&size=10&offset=0` — Get album list
- `GET /subsonic/rest/getStarred.view` — Get starred items
- `GET /subsonic/rest/getPlaylists.view` — Get playlists
- `GET /subsonic/rest/scrobble.view?id={trackId}` — Record a scrobble

Subsonic clients authenticate with `u=username` and `p=api_key` query parameters.

## Authentication

Most endpoints require authentication via:
- `X-API-Key` header, or
- `api_key` query parameter

For Subsonic endpoints, the `p` parameter is treated as the API key.

**Public endpoints** (no auth required):
- `/`, `/api/health`, `/api/stream/*`, `/api/art/*`, `/subsonic/rest/ping*`, `/subsonic/rest/getLicense*`

## Supported Formats

- MP3, FLAC, OGG, Opus, M4A, WAV, WMA, AAC

## File Organization

NightDrive expects your music organized as:

```
music/
  Artist Name/
    Album Title/
      01 - Track Name.mp3
      02 - Another Track.mp3
```

Or with `Artist - Album` format:

```
music/
  Artist Name - Album Title/
    01 - Track Name.mp3
```

## Config

Environment variables:
- `NIGHTDRIVE_MUSIC` — Path to music library (default: `~/music`)
- `PORT` — Server port (default: `8080`)
- `NIGHTDRIVE_WATCH` — Set to `true` or `1` to watch the library recursively and
  auto-scan on changes (default: off). Events are debounced (~3s of quiet) and
  bursts coalesce into a single scan. Newly created directories are picked up
  automatically.

## Roadmap

- [x] Full file scanner
- [x] Subsonic API completeness
- [x] Transcode on-the-fly (MP3, Opus)
- [x] Album art extraction
- [x] Multi-user support
- [x] File watcher for auto-scan
- [x] Smart playlists (rules engine)
- [ ] Mobile app (or PWA)

## License

MIT
