# NightDrive

A self-hosted music streaming server for large local libraries. Fast, single-binary Go backend with SQLite metadata, a vanilla-JS retro player UI, file scanning/watching, smart playlists, gapless HTTP streaming, multi-room sync, DSP presets, casting, and Subsonic API compatibility.

- [Quick start](#quick-start)
- [Features](#features)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [REST API](#rest-api)
- [Subsonic API](#subsonic-api)
- [Usage examples](#usage-examples)
- [Demo instance](#demo-instance)
- [Deploying](#deploying)
- [Roadmap](#roadmap)
- [Changelog](./CHANGELOG.md)
- [License](#license)

---

## Quick start

```bash
cd nightdrive
go build -o nightdrive .
./nightdrive -config nightdrive.toml
```

Open http://localhost:4040 in your browser. The first run creates a default admin user:

- Username: `admin`
- Password: `admin`

Change the admin password before exposing NightDrive to any network.

---

## Features

- **Single binary** Go backend using only the stdlib HTTP server
- **SQLite** metadata with WAL mode for fast reads/writes
- **File scanner + watcher** with periodic rescan and `fsnotify` filesystem events
- **Smart playlists** via simple query language (`artist:`, `album:`, `title:`, `year:`, `minDuration:`, `maxDuration:`)
- **Gapless playback** via HTTP streaming, consistent `audio/*` content types, and album gapless analysis
- **Crossfade & DSP pipeline** with per-session EQ, compressor, and loudness controls
- **Multi-room sync playback** to keep multiple clients in lockstep
- **Casting** device discovery and control (Chromecast/AirPlay-style targets)
- **Subsonic API** compatibility for existing desktop and mobile clients
- **Listening history & stats** with top tracks, top artists, and play counts
- **Radio mode** by genre, artist similarity, or random shuffle
- **Lyrics** with inline editing and playback position bookmarks
- **Single-page vanilla JS frontend** with a retro synthwave player UI

---

## Architecture

```
nightdrive/
├── main.go                  # entry point, wires config / db / scanner / api / web
├── internal/
│   ├── config/              # TOML-ish config loader + migration
│   ├── db/                  # SQLite schema, models, queries
│   ├── scanner/             # fs walker + fsnotify watcher
│   ├── api/                 # REST + Subsonic handlers
│   ├── playback/            # playback sessions, queue, volume, crossfade
│   ├── gapless/             # gapless album analysis
│   ├── dsp/                 # DSP pipeline (EQ, compressor, loudness)
│   ├── sync/                # multi-room sync rooms
│   ├── cast/                # casting device management
│   ├── crashreport/         # crash dump reporter
│   ├── logging/             # structured logs
│   ├── recovery/            # panic recovery middleware
│   └── web/                 # embedded static SPA
├── internal/web/static/
│   └── index.html           # retro player UI
├── deploy/                  # reverse proxy configs + demo config
├── Dockerfile
└── nightdrive.toml          # local config template
```

### Backend

- `net/http` stdlib server with Go 1.22 pattern mux
- SQLite via `mattn/go-sqlite3` with WAL and single writer
- Bcrypt password hashing, session tokens, and optional Basic Auth
- Metadata extracted from filenames; tag parsing is a planned upgrade

### Frontend

- Zero build step: single `index.html` with embedded CSS and JS
- Fetches `/api/*` endpoints and streams via HTML5 `<audio>`
- Queue, play/pause, prev/next, seek, search, albums, artists, playlists, radio, sync rooms

---

## Configuration

Create `nightdrive.toml`:

```toml
listen = "0.0.0.0:4040"
database = "data/nightdrive.db"
music_dir = ["/home/synth/Music"]
scan_on_start = true

[transcode]
enabled = false
format = "mp3"
bitrate = 320

[playback]
crossfade_enabled = true
crossfade_duration = 5.0
gapless_enabled = true
dsp_pipeline_enabled = true
multi_room_enabled = true
casting_enabled = true
```

### Config fields

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `listen` | string | `0.0.0.0:4040` | HTTP listen address |
| `database` | string | `data/nightdrive.db` | SQLite database path |
| `music_dir` | []string | `[]` | Library root directories |
| `scan_on_start` | bool | `true` | Scan library on startup |
| `transcode.enabled` | bool | `false` | Enable FFmpeg transcoding (pipeline ready) |
| `transcode.format` | string | `mp3` | Target format |
| `transcode.bitrate` | int | `320` | Target bitrate |
| `playback.crossfade_enabled` | bool | `true` | Crossfade between tracks |
| `playback.crossfade_duration` | float | `5.0` | Crossfade duration in seconds |
| `playback.gapless_enabled` | bool | `true` | Gapless playback hints |
| `playback.dsp_pipeline_enabled` | bool | `true` | DSP pipeline available |
| `playback.multi_room_enabled` | bool | `true` | Sync rooms available |
| `playback.casting_enabled` | bool | `true` | Cast device management |

---

## REST API

All authenticated endpoints accept either:

- Header: `X-Auth-Token: <token>` obtained from `POST /api/auth/login`, or
- Basic Auth with username and password.

Unless noted, `/api/*` endpoints return JSON. Fields use camelCase.

### Common models

**Track**

```json
{
  "id": 1,
  "albumId": 2,
  "artistId": 3,
  "title": "Neon Highway",
  "artistName": "Synth Rider",
  "albumTitle": "Midnight Run",
  "discNumber": 1,
  "trackNumber": 4,
  "duration": 243.5,
  "bitrate": 320,
  "format": "mp3",
  "path": "/music/Synth Rider/Midnight Run/04 Neon Highway.mp3",
  "mtime": 1700000000,
  "size": 9876543,
  "lyrics": "",
  "mediaType": "music",
  "playbackPosition": 0,
  "genre": "Synthwave"
}
```

**Album**

```json
{
  "id": 2,
  "artistId": 3,
  "title": "Midnight Run",
  "year": 2023,
  "coverArt": "cover.jpg"
}
```

**Artist**

```json
{
  "id": 3,
  "name": "Synth Rider",
  "createdAt": "2023-01-01T00:00:00Z"
}
```

**Playlist**

```json
{
  "id": 10,
  "name": "Night Synth",
  "ownerId": 1,
  "visibility": "private",
  "smartQuery": "artist:synth year:2020",
  "createdAt": "2023-06-01T00:00:00Z",
  "updatedAt": "2023-06-01T00:00:00Z"
}
```

**User**

```json
{
  "id": 1,
  "username": "admin",
  "role": "admin",
  "createdAt": "2023-01-01T00:00:00Z"
}
```

### Health & feedback

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/health` | no | Health check |
| POST | `/api/feedback` | no | Submit feedback `{type, message, rating, metadata}` |

### Library

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/tracks?offset=&limit=&mediaType=` | no | List tracks |
| GET | `/api/tracks/{id}` | no | Track details |
| GET | `/api/tracks/{id}/stream` | no | Stream audio (supports range requests) |
| GET | `/api/tracks/{id}/cover` | no | Folder cover art |
| GET | `/api/tracks/{id}/lyrics` | no | Track lyrics |
| POST | `/api/tracks/{id}/lyrics` | yes | Save lyrics `{lyrics}` |
| POST | `/api/tracks/{id}/position` | yes | Bookmark position `{position}` |
| GET | `/api/albums` | no | List albums |
| GET | `/api/albums/{id}/tracks` | no | Album tracks |
| GET | `/api/artists` | no | List artists |
| GET | `/api/search?q=...&offset=&limit=` | no | Search tracks |
| GET | `/api/genres` | no | List genres |
| POST | `/api/scan` | admin | Trigger library scan |

### Playlists

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/playlists` | optional | List playlists for current user |
| POST | `/api/playlists` | yes | Create playlist `?name=&smartQuery=` or JSON body `{name, smartQuery}` |
| GET | `/api/playlists/{id}/tracks` | no | Playlist tracks |
| POST | `/api/playlists/{id}/tracks` | yes | Add track `?trackId=` or body `{trackId}` |
| DELETE | `/api/playlists/{id}/tracks/{trackId}` | yes | Remove track |
| DELETE | `/api/playlists/{id}` | yes | Delete playlist |
| POST | `/api/playlists/{id}/share` | yes | Share with user `{userId}` |
| DELETE | `/api/playlists/{id}/share/{userId}` | yes | Unshare with user |

### Auth & users

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/auth/login` | no | Login `{username, password}` -> `{token, user}` |
| POST | `/api/auth/logout` | yes | Logout (delete token) |
| GET | `/api/auth/me` | optional | Current auth status |
| GET | `/api/users` | admin | List users |
| POST | `/api/users` | admin | Create user `{username, password, role}` |
| DELETE | `/api/users/{id}` | admin | Delete user |

### History & stats

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/plays` | yes | Record a play `{trackId, durationListened, completionPct}` |
| GET | `/api/history?offset=&limit=` | yes | Personal listening history |
| GET | `/api/stats` | yes | Personal stats (plays, top tracks, top artists) |

### Recommendations & radio

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/recommendations/similar-albums?artistId=&excludeAlbumId=&limit=` | no | Similar albums |
| GET | `/api/recommendations/similar-artists?artistId=&limit=` | no | Similar artists |
| GET | `/api/recommendations/tracks?limit=` | yes | Recommended tracks for user |
| GET | `/api/radio?type=genre&genre=&limit=` | no | Radio by genre, artist, or random |

### Playback sessions

Playback sessions represent a remote-controllable player. Create a session on a device, then use it to play/pause/queue tracks or join a sync room.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/sessions` | yes | Create session `{deviceName}` |
| GET | `/api/sessions` | yes | List sessions |
| GET | `/api/sessions/{id}` | yes | Session details |
| DELETE | `/api/sessions/{id}` | yes | Delete session |
| POST | `/api/sessions/{id}/play` | yes | Play `{trackId, position}` |
| POST | `/api/sessions/{id}/pause` | yes | Pause `{position}` |
| POST | `/api/sessions/{id}/next` | yes | Next track |
| POST | `/api/sessions/{id}/prev` | yes | Previous track |
| POST | `/api/sessions/{id}/queue` | yes | Add to queue `{trackId}` |
| GET | `/api/sessions/{id}/queue` | yes | Get queue |
| DELETE | `/api/sessions/{id}/queue` | yes | Clear queue |
| POST | `/api/sessions/{id}/volume` | yes | Set volume `{volume}` |
| POST | `/api/sessions/{id}/crossfade` | yes | Set crossfade `{enabled, durationSec}` |

### Gapless analysis

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/gapless/analyze/{albumId}` | yes | Analyze an album for gapless playback |
| GET | `/api/gapless/report/{albumId}` | no | Get gapless verification report |
| POST | `/api/gapless/batch` | admin | Batch-analyze all albums |

### DSP presets

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/dsp/presets` | yes | List presets |
| POST | `/api/dsp/presets` | yes | Create preset `{name, eqLow, eqMid, eqHigh, compressorThreshold, compressorRatio, loudnessTarget, isDefault}` |
| GET | `/api/dsp/presets/{id}` | yes | Get preset |
| DELETE | `/api/dsp/presets/{id}` | yes | Delete preset |
| POST | `/api/dsp/process` | yes | Process samples `{samples, sampleRate, presetId}` |

### Sync rooms

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/sync/rooms` | optional | List rooms |
| POST | `/api/sync/rooms` | yes | Create room `{name}` |
| GET | `/api/sync/rooms/{id}` | optional | Room details |
| DELETE | `/api/sync/rooms/{id}` | yes | Delete room |
| POST | `/api/sync/rooms/{id}/join` | yes | Join session `{sessionId}` |
| POST | `/api/sync/rooms/{id}/leave` | yes | Leave session `{sessionId}` |
| POST | `/api/sync/rooms/{id}/play` | yes | Room play `{trackId}` |
| POST | `/api/sync/rooms/{id}/pause` | yes | Room pause |
| POST | `/api/sync/rooms/{id}/seek` | yes | Room seek `{position}` |

### Cast devices

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/cast/devices` | optional | List registered cast devices |
| POST | `/api/cast/discover` | yes | Trigger device discovery |
| POST | `/api/cast/devices` | yes | Register device `{name, type, host, port, protocol, capabilities}` |
| POST | `/api/cast/devices/{id}/play` | yes | Cast play |
| POST | `/api/cast/devices/{id}/pause` | yes | Cast pause |
| POST | `/api/cast/devices/{id}/stop` | yes | Cast stop |
| DELETE | `/api/cast/devices/{id}` | yes | Unregister device |

### Admin

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/admin/crashes?limit=` | admin | List recent crash reports |

---

## Subsonic API

NightDrive implements the Subsonic REST API at `/rest`. Use any Subsonic-compatible client with:

- Server URL: `http://localhost:4040/rest`
- Username / password: any valid NightDrive user
- No API token required; password auth uses the stored bcrypt hash

### Implemented endpoints

- `ping`
- `getLicense`
- `getMusicFolders`
- `getIndexes`
- `getArtists`
- `getArtist`
- `getAlbum`
- `getSong`
- `getRandomSongs`
- `search3`
- `stream`
- `getCoverArt`
- `getPlaylists`
- `getPlaylist`

Subsonic IDs use prefixes:

- Artists: `ar-{id}`
- Albums: `al-{id}`
- Tracks: `tr-{id}`
- Playlists: `pl-{id}`

---

## Usage examples

### Start the server and scan a library

```bash
go build -o nightdrive .
mkdir -p data
./nightdrive -config nightdrive.toml
```

### Log in and get a token

```bash
TOKEN=$(curl -s -X POST http://localhost:4040/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')
```

### Search tracks

```bash
curl -s "http://localhost:4040/api/search?q=neon" | jq '.'
```

### Create a smart playlist

```bash
curl -s -X POST "http://localhost:4040/api/playlists" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $TOKEN" \
  -d '{"name":"Night Synth","smartQuery":"artist:synth year:2020"}' | jq '.'
```

### Stream a track

```bash
curl -s "http://localhost:4040/api/tracks/1/stream" -o track.mp3
```

### Create a playback session and start a track

```bash
SESSION=$(curl -s -X POST http://localhost:4040/api/sessions \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $TOKEN" \
  -d '{"deviceName":"Living Room"}' | jq -r '.id')

curl -s -X POST "http://localhost:4040/api/sessions/${SESSION}/play" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $TOKEN" \
  -d '{"trackId":1}' | jq '.'
```

### Record a play for history/stats

```bash
curl -s -X POST http://localhost:4040/api/plays \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: $TOKEN" \
  -d '{"trackId":1,"durationListened":240,"completionPct":0.95}' | jq '.'
```

### Trigger a library scan

```bash
curl -s -X POST http://localhost:4040/api/scan \
  -H "X-Auth-Token: $TOKEN" | jq '.'
```

### Submit feedback

```bash
curl -s -X POST http://localhost:4040/api/feedback \
  -H "Content-Type: application/json" \
  -d '{"type":"feature","message":"Love the UI!","rating":5}' | jq '.'
```

### Use a Subsonic client

```bash
curl -s "http://localhost:4040/rest/ping?u=admin&p=admin" | jq '.'
curl -s "http://localhost:4040/rest/getArtists?u=admin&p=admin" | jq '.'
curl -s "http://localhost:4040/rest/stream?id=tr-1&u=admin&p=admin" -o track.mp3
```

---

## Demo instance

A sample demo configuration is included in `deploy/nightdrive.demo.toml`. It enables all playback features and points to a read-only music library:

```toml
listen = "0.0.0.0:4040"
database = "data/nightdrive.db"
music_dir = ["/srv/music/demo"]
scan_on_start = true

[transcode]
enabled = false
format = "mp3"
bitrate = 320

[playback]
crossfade_enabled = true
crossfade_duration = 5.0
gapless_enabled = true
dsp_pipeline_enabled = true
multi_room_enabled = true
casting_enabled = true
```

Run it with:

```bash
./nightdrive -config deploy/nightdrive.demo.toml
```

For a public demo, place NightDrive behind a reverse proxy with TLS. See `deploy/Caddyfile` and `deploy/nginx.conf` for examples.

---

## Deploying

### Docker

```bash
docker build -t nightdrive .
docker run -p 4040:4040 -v $(pwd)/data:/app/data -v /path/to/music:/music nightdrive
```

### Reverse proxy

Example `deploy/Caddyfile`:

```caddy
nightdrive.example.com {
    reverse_proxy localhost:4040
}
```

Example `deploy/nginx.conf`:

```nginx
server {
    listen 443 ssl http2;
    server_name nightdrive.example.com;

    location / {
        proxy_pass http://127.0.0.1:4040;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### Security checklist before exposing to the internet

1. Change the default `admin` password.
2. Create non-admin users for daily listening.
3. Run NightDrive behind a TLS-terminating reverse proxy.
4. Do not expose the SQLite database file via your web server.
5. Enable your OS firewall and limit access to trusted IPs if possible.

---

## Roadmap

See [PLAN.md](./PLAN.md) for the full roadmap.

Highlights:

- [ ] ID3 / Vorbis / MP4 tag parsing (planned in v0.2.0)
- [ ] FFmpeg transcoding pipeline
- [ ] Last.fm scrobbling
- [ ] ReplayGain normalization
- [ ] Mobile PWA + offline caching
- [ ] Real-time audio visualizer via Web Audio API
- [ ] Podcast / audiobook support
- [ ] CI/CD, Docker publishing, and benchmark suite

---

## Changelog

See [CHANGELOG.md](./CHANGELOG.md) for release notes.

---

## License

MIT
