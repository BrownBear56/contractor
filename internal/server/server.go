package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/gzip"
	"github.com/BrownBear56/contractor/internal/handlers"
	"github.com/BrownBear56/contractor/internal/logger"
)

type Server struct {
	router *chi.Mux
	cfg    *config.Config
	logger logger.Logger
}

func New(cfg *config.Config, parentLogger logger.Logger) *Server {
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

	serverLogger, err := parentLogger.(*logger.ZapLogger).ReconfigureAndNamed(
		"Server",
		"info",             // Уровень логирования
		"json",             // Формат логов
		[]string{"stdout"}, // Вывод логов
		&customEncoderConfig,
	)
	if err != nil {
		log.Fatalf("Failed to reconfigure logger: %v", err)
	}

	s := &Server{
		router: chi.NewRouter(),
		cfg:    cfg,
		logger: serverLogger,
	}

	s.logger.Info("Setup routers", zap.String("status", "processing"))
	s.setupRoutes(parentLogger)
	s.logger.Info("Setup routers", zap.String("status", "success"))

	return s
}

func (s *Server) setupRoutes(parentLogger logger.Logger) {
	const useFile = true
	urlShortener := handlers.NewURLShortener(
		s.cfg.BaseURL, s.cfg.FileStoragePath, s.cfg.DatabaseDSN, useFile, parentLogger)

	// Подключаем middleware.
	s.router.Use(func(next http.Handler) http.Handler {
		return logger.LoggingMiddleware(next, s.logger)
	}) // Наше кастомное middleware-логирование.
	s.router.Use(func(next http.Handler) http.Handler {
		return gzip.GzipMiddleware(next, s.logger)
	}) // Наше кастомное middleware-сжатие.
	s.router.Use(AuthMiddleware([]byte(s.cfg.SecretKey)))

	s.router.Post("/api/shorten/batch", urlShortener.PostBatchHandler)
	s.router.Post("/api/shorten", urlShortener.PostJSONHandler)
	s.router.Post("/", urlShortener.PostHandler)
	s.router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		r.URL.Path = "/" + id
		urlShortener.GetHandler(w, r)
	})
	s.router.Get("/ping", urlShortener.PingHandler)
	s.router.Get("/api/user/urls", urlShortener.GetUserURLsHandler)
}

func (s *Server) Start() error {
	err := http.ListenAndServe(s.cfg.Address, s.router)

	if err != nil {
		return fmt.Errorf("failed to start server on %s: %w", s.cfg.Address, err)
	}

	s.logger.Info("Server is running", zap.String("address", s.cfg.Address))

	return nil
}

func AuthMiddleware(secretKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const cookieName = "user_id"
			cookie, err := r.Cookie(cookieName)

			var userID string
			if err == nil {
				// Проверяем подпись куки.
				userID, err = VerifyCookie(cookie.Value, secretKey)
				if err != nil {
					userID = ""
				}
			}

			if userID == "" {
				// Генерируем новый userID и подписываем его.
				userID = generateNewUserID()
				signedCookie := SignCookie(userID, secretKey)
				http.SetCookie(w, &http.Cookie{
					Name:  cookieName,
					Value: signedCookie,
					Path:  "/",
				})
			}

			// Передаём userID в контексте запроса.
			ctx := context.WithValue(r.Context(), handlers.UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func SignCookie(userID string, secretKey []byte) string {
	h := hmac.New(sha256.New, secretKey)
	h.Write([]byte(userID))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s.%s", userID, signature)
}

func VerifyCookie(cookieValue string, secretKey []byte) (string, error) {
	const cookiePartsCount = 2
	parts := strings.Split(cookieValue, ".")
	if len(parts) != cookiePartsCount {
		return "", errors.New("invalid cookie format")
	}

	userID := parts[0]
	signature, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 signature: %w", err)
	}

	h := hmac.New(sha256.New, secretKey)
	h.Write([]byte(userID))
	expectedSignature := h.Sum(nil)

	if !hmac.Equal(signature, expectedSignature) {
		return "", errors.New("invalid cookie signature")
	}

	return userID, nil
}

func generateNewUserID() string {
	return uuid.New().String()
}
