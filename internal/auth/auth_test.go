package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

func TestBearerAuth(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		authHeader string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid token",
			token:      "secret123",
			authHeader: "Bearer secret123",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "invalid token",
			token:      "secret123",
			authHeader: "Bearer wrongtoken",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing authorization header",
			token:      "secret123",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty token disables auth",
			token:      "",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "malformed header no space",
			token:      "secret123",
			authHeader: "Bearersecret123",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			token:      "secret123",
			authHeader: "Basic secret123",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bearer case insensitive",
			token:      "secret123",
			authHeader: "bearer secret123",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "extra spaces in token rejected",
			token:      "secret123",
			authHeader: "Bearer  secret123",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := BearerAuth(tt.token, okHandler)

			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				ct := rec.Header().Get("Content-Type")
				if ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				wa := rec.Header().Get("WWW-Authenticate")
				if wa != "Bearer" {
					t.Errorf("WWW-Authenticate = %q, want Bearer", wa)
				}
			}
		})
	}
}

func writeTokenFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "tokens.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTokenStore_Load(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		validTokens   []string
		invalidTokens []string
	}{
		{
			name:          "multiple tokens",
			fileContent:   "token-a\ntoken-b\ntoken-c\n",
			validTokens:   []string{"token-a", "token-b", "token-c"},
			invalidTokens: []string{"token-d", ""},
		},
		{
			name:          "comments and blank lines ignored",
			fileContent:   "# this is a comment\ntoken-a\n\n# another comment\ntoken-b\n",
			validTokens:   []string{"token-a", "token-b"},
			invalidTokens: []string{"# this is a comment"},
		},
		{
			name:          "whitespace trimmed",
			fileContent:   "  token-a  \n\ttoken-b\t\n",
			validTokens:   []string{"token-a", "token-b"},
			invalidTokens: []string{"  token-a  "},
		},
		{
			name:          "single token",
			fileContent:   "only-one\n",
			validTokens:   []string{"only-one"},
			invalidTokens: []string{"other"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTokenFile(t, dir, tt.fileContent)
			store, err := NewTokenStore(path)
			if err != nil {
				t.Fatalf("NewTokenStore() error: %v", err)
			}

			handler := store.Middleware(okHandler)

			for _, token := range tt.validTokens {
				req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Errorf("token %q: status = %d, want %d", token, rec.Code, http.StatusOK)
				}
			}

			for _, token := range tt.invalidTokens {
				req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("token %q: status = %d, want %d", token, rec.Code, http.StatusUnauthorized)
				}
			}
		})
	}
}

func TestTokenStore_FileNotFound(t *testing.T) {
	_, err := NewTokenStore("/nonexistent/path/tokens.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestTokenStore_HotReload(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "initial-token\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}
	if err := store.Watch(); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer store.Stop()

	handler := store.Middleware(okHandler)

	// Initial token should work
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer initial-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("initial token: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Overwrite file with new token
	if err := os.WriteFile(path, []byte("new-token\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Poll until the reload is picked up instead of a fixed sleep
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("hot-reload did not complete within 2s")
		case <-ticker.C:
			if store.valid("new-token") {
				goto reloaded
			}
		}
	}
reloaded:

	// New token should work
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer new-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("new token after reload: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Old token should be rejected
	req = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer initial-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("old token after reload: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTokenStore_StopIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}
	if err := store.Watch(); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	// Calling Stop twice should not panic
	store.Stop()
	store.Stop()
}

func TestTokenStore_StopWithoutWatch(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}

	// Stop without Watch should not panic
	store.Stop()
}

func TestTokenStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "# only comments\n\n# no real tokens\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}

	// All requests should be rejected
	handler := store.Middleware(okHandler)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTokenStore_WatchAlreadyWatching(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}
	if err := store.Watch(); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer store.Stop()

	// Second Watch should return nil (already watching)
	if err := store.Watch(); err != nil {
		t.Errorf("second Watch() = %v, want nil", err)
	}
}

func TestTokenStore_WatchAfterStop(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}

	store.Stop()

	// Watch after Stop should return nil (done channel closed)
	if err := store.Watch(); err != nil {
		t.Errorf("Watch() after Stop() = %v, want nil", err)
	}
}

func TestTokenStore_HotReloadIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}
	if err := store.Watch(); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer store.Stop()

	// Write a different file in the same directory — should not affect tokens
	otherPath := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(otherPath, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}

	// Give fsnotify time to process the event
	time.Sleep(100 * time.Millisecond)

	// Original token should still work
	if !store.valid("token-a") {
		t.Error("token-a should still be valid after unrelated file change")
	}
}

func TestTokenStore_HotReloadAtomicRename(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "original-token\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}
	if err := store.Watch(); err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	defer store.Stop()

	// Simulate atomic rename: write to temp file, rename over original
	tmpPath := filepath.Join(dir, "tokens.tmp")
	if err := os.WriteFile(tmpPath, []byte("renamed-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatal(err)
	}

	// Poll until reload
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("hot-reload via rename did not complete within 2s")
		case <-ticker.C:
			if store.valid("renamed-token") {
				goto done
			}
		}
	}
done:

	if store.valid("original-token") {
		t.Error("original-token should be rejected after rename reload")
	}
}

func TestTokenStore_Middleware_MissingHeader(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-a\n")

	store, err := NewTokenStore(path)
	if err != nil {
		t.Fatalf("NewTokenStore() error: %v", err)
	}

	handler := store.Middleware(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	wa := rec.Header().Get("WWW-Authenticate")
	if wa != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", wa)
	}
}
