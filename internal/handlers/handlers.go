package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/BrownBear56/contractor/internal/models"
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

func (u *URLShortener) validateAndGetURL(body []byte) (string, error) {
	originalURL := strings.TrimSpace(string(body))
	if originalURL == "" {
		return "", errors.New("empty URL")
	}
	if _, err := url.ParseRequestURI(originalURL); err != nil {
		return "", errors.New("invalid URL format")
	}
	return originalURL, nil
}

func (u *URLShortener) getOrCreateShortURL(originalURL string) (string, bool, error) {
	// Проверяем, существует ли уже такой URL.
	if existingID, ok := u.storage.getIDByURL(originalURL); ok {
		return fmt.Sprintf("%s/%s", u.baseURL, existingID), true, nil
	}

	const maxRetries = 10
	var id string
	for range maxRetries {
		generatedID, err := generateID()
		if err != nil {
			log.Printf("Error generating ID: %v\n", err)
			continue
		}

		if err := u.storage.saveID(generatedID, originalURL); err == nil {
			id = generatedID
			break
		}
	}

	if id == "" {
		return "", false, errors.New("failed to generate unique ID")
	}

	return fmt.Sprintf("%s/%s", u.baseURL, id), false, nil
}

func NewURLShortener(baseURL string) *URLShortener {
	return &URLShortener{
		baseURL: baseURL,
		storage: newStorage(),
	}
}

func (u *URLShortener) PostJSONHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var request models.Request
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "cannot decode request JSON body", http.StatusBadRequest)
		return
	}

	originalURL, err := u.validateAndGetURL([]byte(request.URL))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	shortURL, ok, err := u.getOrCreateShortURL(originalURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := models.Response{Result: shortURL}
	w.Header().Set("Content-Type", "application/json")
	if ok {
		w.WriteHeader(http.StatusOK) // URL уже существует.
	} else {
		w.WriteHeader(http.StatusCreated) // Новый URL.
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "error encoding response", http.StatusInternalServerError)
	}
}

func (u *URLShortener) PostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	originalURL, err := u.validateAndGetURL(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	shortURL, ok, err := u.getOrCreateShortURL(originalURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	if ok {
		w.WriteHeader(http.StatusOK) // URL уже существует.
	} else {
		w.WriteHeader(http.StatusCreated) // Новый URL.
	}
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
