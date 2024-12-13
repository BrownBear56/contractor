package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/BrownBear56/contractor/internal/config"
	"github.com/BrownBear56/contractor/internal/handlers"
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

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	urlShortener := handlers.NewURLShortener(s.cfg.BaseURL)

	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

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

	return nil
}
