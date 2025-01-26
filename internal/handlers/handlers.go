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
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// URLShortener хранит базовый URL и объект Storage.
type URLShortener struct {
	storage    storage.Storage
	logger     logger.Logger
	dbConnPool *pgxpool.Pool
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

func (u *URLShortener) getShortURL(originalURL string) (string, bool, error) {
	// Проверяем, существует ли уже такой URL.
	if existingID, ok := u.storage.GetIDByURL(originalURL); ok {
		return existingID, true, nil
	}

	const maxRetries = 10
	var id string
	for range maxRetries {
		generatedID, err := generateID()
		if err != nil {
			u.logger.Error("Error generating ID: %v\n", zap.Error(err))
			continue
		}
		id = generatedID
		break
	}

	if id == "" {
		return "", false, errors.New("all attempts to generate a unique ID failed")
	}

	return id, false, nil
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
		return "", false, errors.New("all attempts to generate a unique ID failed")
	}

	return fmt.Sprintf("%s/%s", u.baseURL, id), false, nil
}

func NewURLShortener(baseURL string, fileStoragePath string,
	dbDSN string, useFile bool, parentLogger logger.Logger,
) *URLShortener {
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

	var dbPool *pgxpool.Pool
	if dbDSN != "" {
		config, err := pgxpool.ParseConfig(dbDSN)
		if err != nil {
			handlerLogger.Fatal("Failed to parse database DSN", zap.Error(err))
		}

		dbPool, err = pgxpool.New(context.Background(), config.ConnString())
		if err != nil {
			handlerLogger.Fatal("Failed to create connection pool", zap.Error(err))
		}
	}

	return &URLShortener{
		baseURL:    baseURL,
		storage:    storage.NewStorage(fileStoragePath, useFile, dbDSN, parentLogger),
		logger:     handlerLogger,
		dbConnPool: dbPool,
	}
}

func (u *URLShortener) PingHandler(w http.ResponseWriter, r *http.Request) {
	const dbPingTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), dbPingTimeout)
	defer cancel()

	if err := u.dbConnPool.Ping(ctx); err != nil {
		u.logger.Error("Database connection error", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (u *URLShortener) PostBatchHandler(w http.ResponseWriter, r *http.Request) {
	var requests []models.BatchRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &requests); err != nil || len(requests) == 0 {
		http.Error(w, "Invalid or empty batch", http.StatusBadRequest)
		return
	}

	// Подготовка данных для сохранения
	pairs := make(map[string]string)
	batchResults := make([]models.BatchResponse, 0, len(requests))

	for _, req := range requests {
		originalURL, err := u.validateAndGetURL([]byte(req.OriginalURL))
		if err != nil {
			http.Error(
				w, fmt.Sprintf("Invalid URL in batch: %s. Correlation ID: %s. Error: %v", req.OriginalURL, req.CorrelationID, err),
				http.StatusBadRequest)
			return
		}

		id, _, err := u.getShortURL(originalURL)
		if err != nil {
			u.logger.Error("failed to process URL", zap.String("originalURL", originalURL), zap.Error(err))
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		pairs[id] = originalURL

		// Формируем результат
		batchResults = append(batchResults, models.BatchResponse{
			CorrelationID: req.CorrelationID,                   // Оригинальный correlationID
			ShortURL:      fmt.Sprintf("%s/%s", u.baseURL, id), // Сформированный короткий URL
		})
	}

	if err := u.storage.SaveBatch(pairs); err != nil {
		u.logger.Error("Save batch error", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(batchResults)
}

func (u *URLShortener) PostJSONHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
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
		u.logger.Error("failed to process URL", zap.String("originalURL", originalURL), zap.Error(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	response := models.Response{Result: shortURL}
	w.Header().Set("Content-Type", "application/json")
	if ok {
		w.WriteHeader(http.StatusConflict) // URL уже существует.
	} else {
		w.WriteHeader(http.StatusCreated) // Новый URL.
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		u.logger.Error("error encoding response", zap.String("shortURL", shortURL), zap.Error(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
}

func (u *URLShortener) PostHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	originalURL, err := u.validateAndGetURL(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	shortURL, ok, err := u.getOrCreateShortURL(originalURL)
	if err != nil {
		u.logger.Error("failed to process URL", zap.String("originalURL", originalURL), zap.Error(err))
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	if ok {
		w.WriteHeader(http.StatusConflict) // URL уже существует.
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
