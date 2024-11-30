package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

var (
	urlStore = make(map[string]string)
	mu       sync.RWMutex
)

func generateID() string {
	bytes := make([]byte, 6)
	_, err := rand.Read(bytes)
	if err != nil {
		panic("failed to generate random ID")
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

func PostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	originalURL := strings.TrimSpace(string(body))

	id := generateID()

	mu.Lock()
	urlStore[id] = originalURL
	mu.Unlock()

	shortURL := fmt.Sprintf("http://localhost:8080/%s", id)
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(shortURL))
}

func GetHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	mu.RLock()
	originalURL, ok := urlStore[id]
	mu.RUnlock()

	if !ok {
		http.Error(w, "ID not found", http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}
