package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/BrownBear56/contractor/cmd/shortener/config"
	"github.com/BrownBear56/contractor/cmd/shortener/handlers"
)

func main() {
	cfg := config.NewConfig()

	handlers.InitHandlers(cfg.BaseURL)

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/", handlers.PostHandler)
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		r.URL.Path = "/" + id
		handlers.GetHandler(w, r)
	})

	fmt.Printf("Server is running on http://%s\n", cfg.Address)
	if err := http.ListenAndServe(cfg.Address, r); err != nil {
		panic(err)
	}
}
