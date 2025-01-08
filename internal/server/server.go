package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
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
	urlShortener := handlers.NewURLShortener(s.cfg.BaseURL, s.cfg.FileStoragePath, useFile, parentLogger)

	// Подключаем middleware.
	s.router.Use(func(next http.Handler) http.Handler {
		return logger.LoggingMiddleware(next, s.logger)
	}) // Наше кастомное middleware-логирование.
	s.router.Use(func(next http.Handler) http.Handler {
		return gzip.GzipMiddleware(next, s.logger)
	}) // Наше кастомное middleware-сжатие.

	s.router.Post("/api/shorten", urlShortener.PostJSONHandler)
	s.router.Post("/", urlShortener.PostHandler)
	s.router.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		r.URL.Path = "/" + id
		urlShortener.GetHandler(w, r)
	})
}

func (s *Server) Start() error {
	err := http.ListenAndServe(s.cfg.Address, s.router)

	if err != nil {
		return fmt.Errorf("failed to start server on %s: %w", s.cfg.Address, err)
	}

	s.logger.Info("Server is running", zap.String("address", s.cfg.Address))

	return nil
}
