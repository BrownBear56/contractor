package config

import (
	"flag"
	"fmt"
)

type Config struct {
	Address string
	BaseURL string
}

func NewConfig() *Config {
	address := flag.String("a", "localhost:8080", "HTTP server address (e.g., localhost:8888)")
	baseURL := flag.String("b", "http://localhost:8080", "Base URL for shortened links (e.g., http://localhost:8000/qsd54gFg)")

	flag.Parse()

	if *baseURL == "" {
		fmt.Println("Base URL cannot be empty. Using default value.")
		*baseURL = "http://localhost:8080"
	}

	return &Config{
		Address: *address,
		BaseURL: *baseURL,
	}
}
