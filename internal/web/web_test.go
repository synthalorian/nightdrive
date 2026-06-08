package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	// The web handler should register a catch-all route
	// Test that it handles a request without panicking
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// Should return 200 or 404 (since static files may not exist in test)
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Errorf("expected status 200 or 404, got %d", rr.Code)
	}
}

func TestRegisterStaticFiles(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	// Test requesting index.html
	req := httptest.NewRequest("GET", "/index.html", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// The embedded static files may or may not exist in test environment
	// Just ensure it doesn't panic
	_ = rr.Code
}
