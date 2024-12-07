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

// В будущем можно перевести на генерацию GUID.
func generateID(storage *Storage) (string, error) {
	const idLength = 6
	for {
		bytes := make([]byte, idLength)
		_, err := rand.Read(bytes)
		if err != nil {
			return "", fmt.Errorf("failed to generate random ID: %w", err)
		}
		id := base64.URLEncoding.EncodeToString(bytes)

		// Проверяем коллизию.
		if _, exists := storage.get(id); !exists {
			return id, nil
		}
	}
}

func newStorage() *Storage {
	return &Storage{
		mu:       &sync.Mutex{},
		urlStore: make(map[string]string),
		reverse:  make(map[string]string),
	}
}

func NewURLShortener(baseURL string) *URLShortener {
	return &URLShortener{
		baseURL: baseURL,
		storage: newStorage(),
	}
}

func (s *Storage) save(id, originalURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Если URL уже существует, ничего не делаем.
	if _, exists := s.reverse[originalURL]; exists {
		return
	}

	s.urlStore[id] = originalURL
	s.reverse[originalURL] = id
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
	if existingID, found := u.storage.getIDByURL(originalURL); found {
		shortURL := fmt.Sprintf("%s/%s", u.baseURL, existingID)
		w.WriteHeader(http.StatusOK) // Идемпотентное поведение: возвращаем 200 OK.
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(shortURL))
		return
	}

	id, err := generateID(u.storage)
	if err != nil {
		log.Printf("Error generating ID: %v\n", err)
		http.Error(w, "Failed to generate ID", http.StatusInternalServerError)
		return
	}

	u.storage.save(id, originalURL)

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
