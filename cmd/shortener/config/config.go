package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Address string
	BaseURL string
}

func NewConfig() *Config {
	// Флаги командной строки
	addressFlag := flag.String("a", "localhost:8080", "HTTP server address (e.g., localhost:8888)")
	baseURLFlag := flag.String("b", "http://localhost:8080", "Base URL for shortened links (e.g., http://localhost:8000/qsd54gFg)")

	flag.Parse()

	// Переменные окружения
	addressEnv := os.Getenv("SERVER_ADDRESS")
	baseURLEnv := os.Getenv("BASE_URL")

	// Приоритет: переменные окружения → флаги → значения по умолчанию
	address := addressEnv
	if address == "" {
		address = *addressFlag
	}

	baseURL := baseURLEnv
	if baseURL == "" {
		baseURL = *baseURLFlag
	}

	// Валидация базового URL
	if baseURL == "" {
		fmt.Println("Base URL cannot be empty. Using default value.")
		baseURL = "http://localhost:8080"
	}

	return &Config{
		Address: address,
		BaseURL: baseURL,
	}
}
