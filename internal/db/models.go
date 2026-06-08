package db

import "time"

// Artist represents a music artist.
type Artist struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Album represents an album.
type Album struct {
	ID       int64   `json:"id"`
	ArtistID int64   `json:"artistId"`
	Title    string  `json:"title"`
	Year     int     `json:"year"`
	CoverArt *string `json:"coverArt,omitempty"`
}

// Track represents a single track.
type Track struct {
	ID               int64   `json:"id"`
	AlbumID          *int64  `json:"albumId,omitempty"`
	ArtistID         *int64  `json:"artistId,omitempty"`
	Title            string  `json:"title"`
	ArtistName       string  `json:"artistName"`
	AlbumTitle       string  `json:"albumTitle"`
	DiscNumber       int     `json:"discNumber"`
	TrackNumber      int     `json:"trackNumber"`
	Duration         float64 `json:"duration"`
	Bitrate          int     `json:"bitrate"`
	Format           string  `json:"format"`
	Path             string  `json:"path"`
	Mtime            int64   `json:"mtime"`
	Size             int64   `json:"size"`
	Lyrics           string  `json:"lyrics"`
	MediaType        string  `json:"mediaType"`
	PlaybackPosition float64 `json:"playbackPosition"`
	Genre            string  `json:"genre"`
}

// Playlist represents a user or smart playlist.
type Playlist struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	OwnerID    *int64    `json:"ownerId,omitempty"`
	Visibility string    `json:"visibility"`
	SmartQuery *string   `json:"smartQuery,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Session struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type Play struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"userId"`
	TrackID          int64     `json:"trackId"`
	PlayedAt         time.Time `json:"playedAt"`
	DurationListened float64   `json:"durationListened"`
	CompletionPct    float64   `json:"completionPct"`
}

type Stats struct {
	TotalPlays    int           `json:"totalPlays"`
	TotalDuration float64       `json:"totalDuration"`
	UniqueTracks  int           `json:"uniqueTracks"`
	UniqueArtists int           `json:"uniqueArtists"`
	TopTracks     []Track       `json:"topTracks"`
	TopArtists    []ArtistCount `json:"topArtists"`
}

type ArtistCount struct {
	Artist Artist `json:"artist"`
	Count  int    `json:"count"`
}
