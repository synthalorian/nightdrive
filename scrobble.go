package main

import (
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Scrobbler handles scrobbling to external services
type Scrobbler struct {
	db *DB
}

// NewScrobbler creates a new scrobbler
func NewScrobbler(db *DB) *Scrobbler {
	return &Scrobbler{db: db}
}

// Scrobble scrobbles a track play to all configured services for a user
func (s *Scrobbler) Scrobble(userID, trackID int64) {
	user, err := s.db.GetUserByID(userID)
	if err != nil {
		return
	}

	track, err := s.db.GetTrackByID(trackID)
	if err != nil {
		return
	}

	artist, err := s.db.GetAllArtists()
	if err != nil {
		return
	}

	var artistName string
	for _, a := range artist {
		if a.ID == track.ArtistID {
			artistName = a.Name
			break
		}
	}

	album, err := s.db.GetAlbumsByArtist(track.ArtistID)
	if err == nil {
		for _, al := range album {
			if al.ID == track.AlbumID {
				go s.scrobbleLastFM(user, artistName, al.Title, track.Title, track.Duration)
				go s.scrobbleListenBrainz(user, artistName, track.Title, al.Title)
				break
			}
		}
	}
}

// scrobbleLastFM scrobbles to Last.fm
func (s *Scrobbler) scrobbleLastFM(user *User, artist, album, track string, duration int) {
	if user.LastFMSessionKey == "" || user.LastFMAPIKey == "" {
		return
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sig := md5Hash(
		"api_key" + user.LastFMAPIKey +
			"artist" + artist +
			"methodtrack.scrobble" +
			"sk" + user.LastFMSessionKey +
			"timestamp" + timestamp +
			"track" + track +
			user.LastFMAPIKey,
	)

	params := url.Values{}
	params.Set("method", "track.scrobble")
	params.Set("api_key", user.LastFMAPIKey)
	params.Set("sk", user.LastFMSessionKey)
	params.Set("artist", artist)
	params.Set("track", track)
	if album != "" {
		params.Set("album", album)
	}
	if duration > 0 {
		params.Set("duration", strconv.Itoa(duration))
	}
	params.Set("timestamp", timestamp)
	params.Set("api_sig", sig)
	params.Set("format", "json")

	resp, err := http.PostForm("https://ws.audioscrobbler.com/2.0/", params)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// scrobbleListenBrainz scrobbles to ListenBrainz
func (s *Scrobbler) scrobbleListenBrainz(user *User, artist, track, album string) {
	// ListenBrainz uses token auth - we can use the user's API key as the token
	token := user.APIKey
	if token == "" {
		return
	}

	payload := map[string]interface{}{
		"listen_type": "single",
		"payload": []map[string]interface{}{
			{
				"listened_at": time.Now().Unix(),
				"track_metadata": map[string]interface{}{
					"artist_name":  artist,
					"track_name":   track,
					"release_name": album,
				},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.listenbrainz.org/1/submit-listens", strings.NewReader(string(jsonPayload)))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// md5Hash returns the MD5 hash of a string
func md5Hash(s string) string {
	h := md5.New()
	io.WriteString(h, s)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// LastFMTokenResponse represents the auth.getToken response
type LastFMTokenResponse struct {
	XMLName xml.Name `xml:"lfm"`
	Token   string   `xml:"token"`
}

// LastFMSessionResponse represents the auth.getSession response
type LastFMSessionResponse struct {
	XMLName xml.Name `xml:"lfm"`
	Session struct {
		Name       string `xml:"name"`
		Key        string `xml:"key"`
		Subscriber int    `xml:"subscriber"`
	} `xml:"session"`
}

// GetLastFMAuthURL returns the URL for Last.fm auth
func GetLastFMAuthURL(apiKey, token string) string {
	sig := md5Hash("api_key" + apiKey + "methodauth.getToken" + apiKey)
	return fmt.Sprintf("https://www.last.fm/api/auth/?api_key=%s&token=%s&api_sig=%s", apiKey, token, sig)
}

// GetLastFMToken gets a token from Last.fm
func GetLastFMToken(apiKey string) (string, error) {
	sig := md5Hash("api_key" + apiKey + "methodauth.getToken" + apiKey)
	params := url.Values{}
	params.Set("method", "auth.getToken")
	params.Set("api_key", apiKey)
	params.Set("api_sig", sig)
	params.Set("format", "xml")

	resp, err := http.PostForm("https://ws.audioscrobbler.com/2.0/", params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result LastFMTokenResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Token, nil
}

// GetLastFMSession gets a session key from Last.fm
func GetLastFMSession(apiKey, token string) (*LastFMSessionResponse, error) {
	sig := md5Hash("api_key" + apiKey + "methodauth.getSession" + "token" + token + apiKey)
	params := url.Values{}
	params.Set("method", "auth.getSession")
	params.Set("api_key", apiKey)
	params.Set("token", token)
	params.Set("api_sig", sig)
	params.Set("format", "xml")

	resp, err := http.PostForm("https://ws.audioscrobbler.com/2.0/", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result LastFMSessionResponse
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
