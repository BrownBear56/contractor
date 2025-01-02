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

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type MemoryStore struct {
	mu          *sync.Mutex
	URLs        map[string]string
	reverseURLs map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		mu:          &sync.Mutex{},
		URLs:        make(map[string]string),
		reverseURLs: make(map[string]string),
	}
}

type FileStore struct {
	*MemoryStore
	filePath string
	logger   logger.Logger
}

type Storage struct {
	mu          *sync.Mutex
	URLs        map[string]string
	reverseURLs map[string]string
	filePath    string
	logger      logger.Logger
}

// URLShortener хранит базовый URL и объект Storage.
type URLShortener struct {
	storage *Storage
	baseURL string
	logger  logger.Logger
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
			u.logger.Error("Error generating ID: %v\n", zap.Error(err))
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
			s.logger.Error("Error closing file: %v\n", zap.Error(err))
		}
	}()

	decoder := json.NewDecoder(file)
	for {
		var data map[string]string
		if err := decoder.Decode(&data); err != nil {
			if errors.Is(err, io.EOF) {
				break // Достигнут конец файла, декодирование завершено успешно.
			}
			return fmt.Errorf("error decode data from file: %w", err)
		}
		if short, ok := data["short_url"]; ok {
			original := data["original_url"]
			s.URLs[original] = short
			s.reverseURLs[short] = original
		}
	}
	return nil
}

func newStorage(filePath string, parentLogger logger.Logger) *Storage {
	// Настройки для нового логгера.
	customEncoderConfig := zapcore.EncoderConfig{
		TimeKey:       "timestamp",
		LevelKey:      "severity",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "message",
		StacktraceKey: "stacktrace",
		EncodeTime:    zapcore.ISO8601TimeEncoder,
		EncodeLevel:   zapcore.CapitalLevelEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}

	storageLogger, err := parentLogger.(*logger.ZapLogger).ReconfigureAndNamed(
		"Storage",
		"info",             // Уровень логирования
		"json",             // Формат логов
		[]string{"stdout"}, // Вывод логов
		customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	s := &Storage{
		mu:          &sync.Mutex{},
		URLs:        make(map[string]string),
		reverseURLs: make(map[string]string),
		filePath:    filePath,
		logger:      storageLogger,
	}
	if err := s.loadFromFile(); err != nil { // Загружаем данные при инициализации.
		s.logger.Error("Failed to load data from file", zap.Error(err))
	}
	return s
}

// SaveToFile сохраняет данные в файл.
func (s *Storage) saveToFile() error {
	file, err := os.Create(s.filePath)
	if err != nil {
		s.logger.Error("Error creating file: %v\n", zap.Error(err))
	}
	defer func() {
		if err := file.Close(); err != nil {
			s.logger.Error("Error closing file: %v\n", zap.Error(err))
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

func NewURLShortener(
	baseURL string, fileStoragePath string, parentLogger logger.Logger) *URLShortener {
	// Настройки для нового логгера.
	customEncoderConfig := zapcore.EncoderConfig{
		TimeKey:       "timestamp",
		LevelKey:      "severity",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "message",
		StacktraceKey: "stacktrace",
		EncodeTime:    zapcore.ISO8601TimeEncoder,
		EncodeLevel:   zapcore.CapitalLevelEncoder,
		EncodeCaller:  zapcore.ShortCallerEncoder,
	}

	handlerLogger, err := parentLogger.(*logger.ZapLogger).ReconfigureAndNamed(
		"Handler",
		"info",             // Уровень логирования
		"json",             // Формат логов
		[]string{"stdout"}, // Вывод логов
		customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	return &URLShortener{
		baseURL: baseURL,
		storage: newStorage(fileStoragePath, parentLogger),
		logger:  handlerLogger,
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
		u.logger.Error("Failed to get or create short URL", zap.String("originalURL", originalURL), zap.Error(err))
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
		u.logger.Error("error encoding response", zap.String("shortURL", shortURL), zap.Error(err))
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
		u.logger.Error("Failed to get or create short URL", zap.String("originalURL", originalURL), zap.Error(err))
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
