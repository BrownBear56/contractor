package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Storage инкапсулирует мьютекс и хранилище URL-ов.
type Storage struct {
	mu       *sync.Mutex
	urlStore map[string]string
	reverse  map[string]string
}

// URLShortener хранит базовый URL и объект Storage.
type URLShortener struct {
	storage *Storage
	baseURL string
}

func newStorage() *Storage {
	return &Storage{
		mu:       &sync.Mutex{},
		urlStore: make(map[string]string),
		reverse:  make(map[string]string),
	}
}

// В будущем можно перевести на генерацию GUID.
func (s *Storage) generateAndSaveID(originalURL string) (string, error) {
	const idLength = 6
	const maxRetries = 10

	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, существует ли уже такой URL.
	if existingID, exists := s.reverse[originalURL]; exists {
		return existingID, nil // Возвращаем существующий ID.
	}

	for range maxRetries {
		bytes := make([]byte, idLength)
		_, err := rand.Read(bytes)
		if err != nil {
			return "", fmt.Errorf("failed to generate random ID: %w", err)
		}
		id := base64.URLEncoding.EncodeToString(bytes)

		if _, exists := s.urlStore[id]; !exists {
			// Сохраняем идентификатор и URL.
			s.urlStore[id] = originalURL
			s.reverse[originalURL] = id
			return id, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique ID after %d attempts", maxRetries)
}

func (s *Storage) get(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	originalURL, ok := s.urlStore[id]
	return originalURL, ok
}

func (s *Storage) getIDByURL(originalURL string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.reverse[originalURL]
	return id, ok
}

func NewURLShortener(baseURL string) *URLShortener {
	return &URLShortener{
		baseURL: baseURL,
		storage: newStorage(),
	}
}

func (u *URLShortener) PostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	originalURL := strings.TrimSpace(string(body))

	// Проверяем валидность URL.
	if _, err := url.ParseRequestURI(originalURL); err != nil {
		http.Error(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	// Проверяем, существует ли уже такой URL.
	if existingID, ok := u.storage.getIDByURL(originalURL); ok {
		shortURL := fmt.Sprintf("%s/%s", u.baseURL, existingID)
		w.WriteHeader(http.StatusOK) // Идемпотентное поведение: возвращаем 200 OK.
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(shortURL))
		return
	}

	id, err := u.storage.generateAndSaveID(originalURL)
	if err != nil {
		log.Printf("Error generating ID: %v\n", err)
		http.Error(w, "Failed to generate ID", http.StatusInternalServerError)
		return
	}

	shortURL := fmt.Sprintf("%s/%s", u.baseURL, id)
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(shortURL))
}

func (u *URLShortener) GetHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	originalURL, ok := u.storage.get(id)
	if !ok {
		http.Error(w, "ID not found", http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}
