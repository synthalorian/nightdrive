# NightDrive

A self-hosted music streaming server for large local libraries. Fast, single-binary Go backend with SQLite metadata, a vanilla-JS retro player UI, file scanning/watching, smart playlists, gapless HTTP streaming, and Subsonic API compatibility.

## Quick start

```bash
cd nightdrive
go build -o nightdrive .
./nightdrive -config nightdrive.toml
```

Open http://localhost:4040 in your browser.

## Features

- **Single binary** Go backend using only the stdlib HTTP server
- **SQLite** metadata with WAL mode for fast reads/writes
- **File scanner + watcher** with periodic rescan and `fsnotify` filesystem events
- **Smart playlists** via simple query language (`artist:`, `album:`, `title:`, `year:`, `minDuration:`, `maxDuration:`)
- **Gapless playback** via HTTP streaming and consistent `audio/*` content types
- **Subsonic API** compatibility for existing clients (ping, getArtists, getAlbum, stream, search3, playlists, etc.)
- **Single-page vanilla JS frontend** with a retro synthwave player UI

## Architecture

```
nightdrive/
├── main.go                  # entry point, wires config / db / scanner / api / web
├── internal/
│   ├── config/              # TOML-ish config loader
│   ├── db/                  # SQLite schema, models, queries
│   ├── scanner/             # fs walker + fsnotify watcher
│   ├── api/                 # REST + Subsonic handlers
│   └── web/                 # embedded static SPA
└── internal/web/static/
    └── index.html           # retro player UI
```

### Backend

- `net/http` stdlib server with Go 1.22 pattern mux
- SQLite via `mattn/go-sqlite3` with WAL and single writer
- Metadata extracted from filenames; tag parsing is a planned upgrade
- Scanner runs on startup, then watches library roots

### Frontend

- Zero build step: single `index.html` with embedded CSS and JS
- Fetches `/api/*` endpoints and streams via HTML5 `<audio>`
- Queue, play/pause, prev/next, seek, search, albums, artists, playlists

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
```

## REST API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/tracks` | List tracks |
| GET | `/api/tracks/{id}` | Track details |
| GET | `/api/tracks/{id}/stream` | Stream audio |
| GET | `/api/tracks/{id}/cover` | Folder cover art |
| GET | `/api/albums` | List albums |
| GET | `/api/albums/{id}/tracks` | Album tracks |
| GET | `/api/artists` | List artists |
| GET | `/api/search?q=...` | Search |
| GET | `/api/playlists` | List playlists |
| POST | `/api/playlists` | Create playlist |
| GET | `/api/playlists/{id}/tracks` | Playlist tracks |
| POST | `/api/playlists/{id}/tracks` | Add track |
| DELETE | `/api/playlists/{id}/tracks/{trackId}` | Remove track |
| DELETE | `/api/playlists/{id}` | Delete playlist |
| POST | `/api/scan` | Trigger scan |

## Subsonic API

Implemented endpoints: `ping`, `getLicense`, `getMusicFolders`, `getIndexes`, `getArtists`, `getArtist`, `getAlbum`, `getSong`, `getRandomSongs`, `search3`, `stream`, `getCoverArt`, `getPlaylists`, `getPlaylist`.

Use any Subsonic client with server URL `http://localhost:4040/rest` and any username/password (authentication is currently a stub; add your own token/md5 logic before exposing to the internet).

## Smart playlists

Create a smart playlist by posting a query string:

```bash
curl -X POST 'http://localhost:4040/api/playlists?name=Night%20Synth&smartQuery=artist:synth year:2020'
```

Supported tokens (space-separated):
- `artist:foo`
- `album:foo`
- `title:foo`
- `year:YYYY`
- `minDuration:SS`
- `maxDuration:SS`

## Roadmap

- [ ] ID3 / Vorbis / MP4 tag parsing
- [ ] Transcoding pipeline (FFmpeg)
- [ ] User accounts + secure Subsonic auth
- [ ] Last.fm scrobbling
- [ ] ReplayGain normalization
- [ ] Lyrics support
- [ ] Mobile PWA + offline caching
- [ ] Real-time audio visualizer via Web Audio API
- [ ] Podcast / audiobook support

## License

MIT
