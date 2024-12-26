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
	"os"
	"strings"
	"sync"

	"github.com/BrownBear56/contractor/internal/models"
)

// Storage инкапсулирует мьютекс и хранилище URL-ов.
type Storage struct {
	mu          *sync.Mutex
	URLs        map[string]string
	reverseURLs map[string]string
	filePath    string
}

// URLShortener хранит базовый URL и объект Storage.
type URLShortener struct {
	storage *Storage
	baseURL string
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
	if _, ok := s.URLs[id]; ok {
		return fmt.Errorf("ID %s already exists", id)
	}

	// Сохраняем идентификатор и URL.
	s.URLs[id] = originalURL
	s.reverseURLs[originalURL] = id
	if err := s.saveToFile(); err != nil {
		return fmt.Errorf("failed to save data: %w", err)
	}

	return nil
}

func (s *Storage) get(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	originalURL, ok := s.URLs[id]
	return originalURL, ok
}

func (s *Storage) getIDByURL(originalURL string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.reverseURLs[originalURL]
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

// LoadFromFile загружает данные из файла.
func (s *Storage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil // Файл ещё не существует.
	} else if err != nil {
		return fmt.Errorf("error load from file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v\n", err)
		}
	}()

	decoder := json.NewDecoder(file)
	for {
		var data map[string]string
		if err := decoder.Decode(&data); err != nil {
			return fmt.Errorf("error decode data from file: %w", err)
		}
		if short, ok := data["short_url"]; ok {
			original := data["original_url"]
			s.URLs[original] = short
			s.reverseURLs[short] = original
		}
	}
}

func newStorage(filePath string) *Storage {
	s := &Storage{
		mu:          &sync.Mutex{},
		URLs:        make(map[string]string),
		reverseURLs: make(map[string]string),
		filePath:    filePath,
	}
	if err := s.loadFromFile(); err != nil { // Загружаем данные при инициализации.
		log.Printf("error load from file: %v", err)
	}
	return s
}

// SaveToFile сохраняет данные в файл.
func (s *Storage) saveToFile() error {
	file, err := os.Create(s.filePath)
	if err != nil {
		log.Printf("Error creating file: %v\n", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v\n", err)
		}
	}()

	encoder := json.NewEncoder(file)
	for original, short := range s.URLs {
		data := map[string]string{
			"short_url":    short,
			"original_url": original,
		}
		if err := encoder.Encode(data); err != nil {
			return fmt.Errorf("error encoding JSON: %w", err)
		}
	}
	return nil
}

func NewURLShortener(baseURL string, fileStoragePath string) *URLShortener {
	return &URLShortener{
		baseURL: baseURL,
		storage: newStorage(fileStoragePath),
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
