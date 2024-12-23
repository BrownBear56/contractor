package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/handlers"
	"github.com/BrownBear56/contractor/internal/logger"
)

type Server struct {
	router *chi.Mux
	cfg    *config.Config
}

func New(cfg *config.Config) *Server {
	s := &Server{
		router: chi.NewRouter(),
		cfg:    cfg,
	}

	if err := logger.Initialize("info"); err != nil {
		log.Println("Failed logger initialize")
	}

	logger.Log.Info("Setup routers", zap.String("status", "processing"))
	s.setupRoutes()
	logger.Log.Info("Setup routers", zap.String("status", "success"))

	return s
}

func (s *Server) setupRoutes() {
	urlShortener := handlers.NewURLShortener(s.cfg.BaseURL)

	// Подключаем middleware.
	s.router.Use(logger.LoggingMiddleware) // Наше кастомное middleware-логирование.

	s.router.Post("/shorten", urlShortener.PostJSONHandler)
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

	logger.Log.Info("Server is running", zap.String("address", s.cfg.Address))

	return nil
}
