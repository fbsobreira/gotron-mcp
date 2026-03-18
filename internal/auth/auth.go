package auth

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// TokenStore holds a set of valid tokens with thread-safe access and
// optional file watching for hot-reload.
type TokenStore struct {
	mu       sync.RWMutex
	hashes   map[[sha256.Size]byte]struct{}
	path     string
	watcher  *fsnotify.Watcher
	done     chan struct{}
	stopOnce sync.Once
}

// NewTokenStore creates a store from a token file. Each line is a token;
// blank lines and lines starting with # are ignored.
func NewTokenStore(path string) (*TokenStore, error) {
	ts := &TokenStore{
		path: path,
		done: make(chan struct{}),
	}
	if err := ts.load(); err != nil {
		return nil, err
	}
	ts.mu.RLock()
	count := len(ts.hashes)
	ts.mu.RUnlock()
	if count == 0 {
		log.Printf("auth: warning: token file %q contains no valid tokens, all requests will be rejected", path)
	}
	return ts, nil
}

// Watch starts watching the token file's directory for changes and reloads
// automatically. Watching the directory ensures that atomic rename-based
// updates (used by editors and deployment tools) are detected.
func (ts *TokenStore) Watch() error {
	ts.mu.Lock()
	if ts.watcher != nil {
		ts.mu.Unlock()
		return nil
	}
	select {
	case <-ts.done:
		ts.mu.Unlock()
		return nil
	default:
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		ts.mu.Unlock()
		return err
	}
	dir := filepath.Dir(ts.path)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		ts.mu.Unlock()
		return err
	}
	ts.watcher = watcher
	ts.mu.Unlock()

	base := filepath.Base(ts.path)

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != base {
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) ||
				event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					if err := ts.load(); err != nil {
						log.Printf("auth: failed to reload token file: %v", err)
					} else {
						ts.mu.RLock()
						count := len(ts.hashes)
						ts.mu.RUnlock()
						log.Printf("auth: reloaded token file, %d tokens loaded", count)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("auth: file watcher error: %v", err)
			case <-ts.done:
				return
			}
		}
	}()

	return nil
}

// Stop closes the file watcher. It is safe to call multiple times.
func (ts *TokenStore) Stop() {
	ts.stopOnce.Do(func() {
		close(ts.done)
		ts.mu.Lock()
		w := ts.watcher
		ts.watcher = nil
		ts.mu.Unlock()
		if w != nil {
			_ = w.Close()
		}
	})
}

func (ts *TokenStore) load() error {
	f, err := os.Open(ts.path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	hashes := make(map[[sha256.Size]byte]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		h := sha256.Sum256([]byte(line))
		hashes[h] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	ts.mu.Lock()
	ts.hashes = hashes
	ts.mu.Unlock()
	return nil
}

func (ts *TokenStore) valid(token string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	h := sha256.Sum256([]byte(token))
	_, ok := ts.hashes[h]
	return ok
}

// Middleware returns HTTP middleware that validates bearer tokens against
// the store.
func (ts *TokenStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := extractBearer(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		if !ts.valid(token) {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BearerAuth returns middleware that validates an Authorization: Bearer <token>
// header using constant-time comparison. If token is empty, authentication is
// disabled and requests pass through.
func BearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	tokenHash := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided, ok := extractBearer(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		providedHash := sha256.Sum256([]byte(provided))
		if subtle.ConstantTimeCompare(providedHash[:], tokenHash[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearer(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
