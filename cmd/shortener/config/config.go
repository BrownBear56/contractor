package config

import (
	"flag"
	"log"
	"os"
)

type Config struct {
	Address string
	BaseURL string
}

func NewConfig() *Config {
	// Флаги командной строки.
	addressFlag := flag.String("a", "localhost:8080", "HTTP server address.")
	baseURLFlag := flag.String("b", "http://localhost:8080", "Base URL for shortened links.")

	flag.Parse()

	// Переменные окружения.
	addressEnv, addressExists := os.LookupEnv("SERVER_ADDRESS")
	baseURLEnv, baseURLExists := os.LookupEnv("BASE_URL")

	// Приоритет: переменные окружения → флаги → значения по умолчанию.
	address := *addressFlag
	if addressExists {
		address = addressEnv
	}

	baseURL := *baseURLFlag
	if baseURLExists {
		baseURL = baseURLEnv
	}

	// Валидация базового URL.
	if baseURL == "" {
		log.Println("Base URL cannot be empty. Using default value.")
		baseURL = "http://localhost:8080"
	}

	return &Config{
		Address: address,
		BaseURL: baseURL,
	}
}
