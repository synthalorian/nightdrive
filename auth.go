package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// contextKey is a type for context keys
type contextKey string

const userContextKey contextKey = "user"

// generateAPIKey generates a random 32-byte hex API key
func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// initDefaultUser creates an admin user if no users exist
func initDefaultUser() (*User, error) {
	count, err := db.GetUsersCount()
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	apiKey := generateAPIKey()
	user, err := db.CreateUser("admin", apiKey)
	if err != nil {
		return nil, err
	}

	log.Printf("Created default admin user. API Key: %s", apiKey)
	return user, nil
}

// getUserFromRequest extracts the user from request context
func getUserFromRequest(r *http.Request) *User {
	if u, ok := r.Context().Value(userContextKey).(*User); ok {
		return u
	}
	return nil
}

// apiKeyAuth middleware checks for API key in header or query param
// Allows unauthenticated access to streaming, art, health, index, and subsonic ping
func apiKeyAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Paths that don't require auth
		path := r.URL.Path
		if path == "/" ||
			path == "/api/health" ||
			strings.HasPrefix(path, "/api/stream/") ||
			strings.HasPrefix(path, "/api/art/") ||
			strings.HasPrefix(path, "/subsonic/rest/ping") ||
			strings.HasPrefix(path, "/subsonic/rest/getLicense") {
			next(w, r)
			return
		}

		// Check API key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		// For Subsonic endpoints, password is treated as API key
		if apiKey == "" && strings.HasPrefix(path, "/subsonic/") {
			apiKey = r.URL.Query().Get("p")
		}

		if apiKey == "" {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		user, err := db.GetUserByAPIKey(apiKey)
		if err != nil {
			http.Error(w, `{"error":"Invalid API key"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// handleRegister creates a new user
func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "Username required", http.StatusBadRequest)
		return
	}

	apiKey := generateAPIKey()
	user, err := db.CreateUser(req.Username, apiKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create user: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"id":       user.ID,
		"username": user.Username,
		"api_key":  user.APIKey,
	})
}

// handleMe returns current user info
func handleMe(w http.ResponseWriter, r *http.Request) {
	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	jsonResponse(w, map[string]interface{}{
		"id":       user.ID,
		"username": user.Username,
	})
}

// handleStarTrack stars or unstars a track
func handleStarTrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/tracks/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "star" {
		http.Error(w, "Invalid endpoint", http.StatusBadRequest)
		return
	}

	trackID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid track ID", http.StatusBadRequest)
		return
	}

	starred, err := db.ToggleStar(user.ID, trackID, "track")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to star track: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"starred": starred,
	})
}

// handleGetStars returns starred items for current user
func handleGetStars(w http.ResponseWriter, r *http.Request) {
	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	stars, err := db.GetAllStars(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get stars: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, stars)
}

// Import needed for fmt, strconv
// These will be resolved by the compiler since main.go already imports them
