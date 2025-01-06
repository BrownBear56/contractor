package handlers

import (
	"context"
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
	"time"

	"github.com/BrownBear56/contractor/internal/logger"
	"github.com/BrownBear56/contractor/internal/models"
	"github.com/BrownBear56/contractor/internal/storage"
	"github.com/jackc/pgx/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// URLShortener хранит базовый URL и объект Storage.
type URLShortener struct {
	storage    storage.Storage
	logger     logger.Logger
	dbConnPool *pgx.Conn
	baseURL    string
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
	if existingID, ok := u.storage.GetIDByURL(originalURL); ok {
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

		if err := u.storage.SaveID(generatedID, originalURL); err == nil {
			id = generatedID
			break
		}
	}

	if id == "" {
		return "", false, errors.New("failed to generate unique ID")
	}

	return fmt.Sprintf("%s/%s", u.baseURL, id), false, nil
}

func NewURLShortener(
	baseURL string, fileStoragePath string, useFile bool, parentLogger logger.Logger) *URLShortener {
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
		&customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	return &URLShortener{
		baseURL: baseURL,
		storage: storage.NewStorage(fileStoragePath, useFile, parentLogger),
		logger:  handlerLogger,
	}
}

func (u *URLShortener) PingHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := u.dbConnPool.Ping(ctx); err != nil {
		u.logger.Error("Database connection error", zap.Error(err))
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
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

	originalURL, ok := u.storage.Get(id)
	if !ok {
		http.Error(w, "ID not found", http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", originalURL)
	w.WriteHeader(http.StatusTemporaryRedirect)
}
