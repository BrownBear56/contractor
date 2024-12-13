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

func generateID() (string, error) {
	const idLength = 6
	bytes := make([]byte, idLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *Storage) saveID(id, originalURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, существует ли уже идентификатор.
	if _, ok := s.urlStore[id]; ok {
		return fmt.Errorf("ID %s already exists", id)
	}

	// Сохраняем идентификатор и URL.
	s.urlStore[id] = originalURL
	s.reverse[originalURL] = id
	return nil
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

	const maxRetries = 10
	var id string
	for range maxRetries {
		id, err = generateID()
		if err != nil {
			log.Printf("Error generating ID: %v\n", err)
			continue
		}

		// Попытка сохранить ID.
		if err := u.storage.saveID(id, originalURL); err == nil {
			break
		}
	}

	if id == "" {
		http.Error(w, "Failed to generate unique ID", http.StatusInternalServerError)
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
