package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// setupTestServer installs a fresh temp DB in the global db handle (which the
// handlers use) and returns an authenticated user's API key.
func setupTestServer(t *testing.T) string {
	t.Helper()
	db = newTestDB(t)
	user, err := db.CreateUser("smoketester", "smoke-test-key")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user.APIKey
}

func TestHealthEndpoint(t *testing.T) {
	setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected health body: %v", body)
	}
}

func TestAuthRejection(t *testing.T) {
	setupTestServer(t)

	protected := []string{"/api/stats", "/api/playlists", "/api/tracks", "/api/artists"}
	for _, path := range protected {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		apiKeyAuth(handleStats)(rec, req) // handler body irrelevant; auth must reject first
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without API key: expected 401, got %d", path, rec.Code)
		}
	}

	// Invalid key must also be rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	apiKeyAuth(handleStats)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid API key: expected 401, got %d", rec.Code)
	}

	// Health is public: apiKeyAuth must pass it through.
	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec = httptest.NewRecorder()
	apiKeyAuth(handleHealth)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public health endpoint: expected 200, got %d", rec.Code)
	}
}

func TestStatsEndpoint(t *testing.T) {
	apiKey := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	apiKeyAuth(handleStats)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var stats map[string]int64
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := stats["tracks"]; !ok {
		t.Fatalf("stats missing tracks key: %v", stats)
	}
}

func TestPlaylistsRoundTrip(t *testing.T) {
	apiKey := setupTestServer(t)
	stubLyrics(t)

	// Seed a track so we can add it to the playlist.
	track := seedTrack(t, db, "API Artist", "API Album", "API Song", "/music/api/1.mp3")

	withAuth := func(req *http.Request) *http.Request {
		req.Header.Set("X-API-Key", apiKey)
		return req
	}

	// Create playlist.
	req := withAuth(httptest.NewRequest(http.MethodPost, "/api/playlists",
		strings.NewReader(`{"name":"Smoke Mix"}`)))
	rec := httptest.NewRecorder()
	apiKeyAuth(handlePlaylists)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create playlist: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var created Playlist
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created playlist: %v", err)
	}
	if created.ID == 0 || created.Name != "Smoke Mix" {
		t.Fatalf("unexpected created playlist: %+v", created)
	}

	// List playlists.
	req = withAuth(httptest.NewRequest(http.MethodGet, "/api/playlists", nil))
	rec = httptest.NewRecorder()
	apiKeyAuth(handlePlaylists)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list playlists: expected 200, got %d", rec.Code)
	}
	var list struct {
		Playlists []*Playlist `json:"playlists"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Playlists) != 1 || list.Playlists[0].ID != created.ID {
		t.Fatalf("list mismatch: %+v", list.Playlists)
	}

	// Add a track.
	req = withAuth(httptest.NewRequest(http.MethodPost,
		"/api/playlists/"+itoa(created.ID)+"/tracks",
		strings.NewReader(`{"track_id":`+itoa(track.ID)+`}`)))
	rec = httptest.NewRecorder()
	apiKeyAuth(handlePlaylistDetail)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add track: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Get playlist with tracks.
	req = withAuth(httptest.NewRequest(http.MethodGet, "/api/playlists/"+itoa(created.ID), nil))
	rec = httptest.NewRecorder()
	apiKeyAuth(handlePlaylistDetail)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get playlist: expected 200, got %d", rec.Code)
	}
	var detail struct {
		Playlist *Playlist        `json:"playlist"`
		Tracks   []*PlaylistTrack `json:"tracks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Tracks) != 1 || detail.Tracks[0].TrackID != track.ID {
		t.Fatalf("detail tracks mismatch: %+v", detail.Tracks)
	}

	// Delete the playlist.
	req = withAuth(httptest.NewRequest(http.MethodDelete, "/api/playlists/"+itoa(created.ID), nil))
	rec = httptest.NewRecorder()
	apiKeyAuth(handlePlaylistDetail)(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete playlist: expected 204, got %d", rec.Code)
	}

	// Confirm it's gone.
	req = withAuth(httptest.NewRequest(http.MethodGet, "/api/playlists/"+itoa(created.ID), nil))
	rec = httptest.NewRecorder()
	apiKeyAuth(handlePlaylistDetail)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted playlist: expected 404, got %d", rec.Code)
	}
}

func TestSmartPlaylistsEndpointRoundTrip(t *testing.T) {
	apiKey := setupTestServer(t)

	// Create.
	req := httptest.NewRequest(http.MethodPost, "/api/smart-playlists",
		strings.NewReader(`{"name":"Auto Mix","rule_type":"random","limit_num":5}`))
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()
	apiKeyAuth(handleSmartPlaylists)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create smart playlist: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var created SmartPlaylist
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Get detail (tracks will be empty; that's fine).
	req = httptest.NewRequest(http.MethodGet, "/api/smart-playlists/"+itoa(created.ID), nil)
	req.Header.Set("X-API-Key", apiKey)
	rec = httptest.NewRecorder()
	apiKeyAuth(handleSmartPlaylistDetail)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get smart playlist: expected 200, got %d", rec.Code)
	}

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/smart-playlists/"+itoa(created.ID), nil)
	req.Header.Set("X-API-Key", apiKey)
	rec = httptest.NewRecorder()
	apiKeyAuth(handleSmartPlaylistDetail)(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete smart playlist: expected 204, got %d", rec.Code)
	}
}

// itoa formats an int64 for URL paths.
func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
